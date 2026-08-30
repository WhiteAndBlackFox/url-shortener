# URL Shortener

A microservice URL shortener written in Go, built as a portfolio project to demonstrate a realistic service-oriented stack: three independently deployable Go services, PostgreSQL, Redis, RabbitMQ, gRPC for internal communication, a public REST API, and the DevOps scaffolding (Docker, CI, health checks, structured logging, request tracing) that goes with running them together.

The project was built in eight incremental phases — each one shippable and demoable on its own. The full roadmap, the decisions made at each step and *why*, live in [`plans/plans.md`](plans/plans.md); this README covers what the system looks like now that it's finished, and how to run it.

## Architecture

```mermaid
flowchart LR
    Client(["Client"]) -->|HTTP :8080| Gateway["API Gateway"]

    Gateway -->|gRPC| Core["Core Service"]
    Gateway -->|gRPC| Stat["Stat Service"]
    Gateway -.->|"publish click<br/>(async, fire-and-forget)"| MQ[["RabbitMQ"]]

    MQ -.->|consume, worker pool| Stat

    Core --> Redis[("Redis<br/>cache")]
    Core --> PG[("PostgreSQL")]
    Stat --> PG
```

**API Gateway** is the only public entrypoint. It terminates HTTP, calls Core/Stat Service over gRPC, and publishes click events to RabbitMQ without blocking the redirect response.

**Core Service** owns link creation and resolution. It's gRPC-only — never reachable directly by clients — and caches lookups in Redis (cache-aside, Postgres stays the source of truth).

**Stat Service** owns click analytics. It's also gRPC-only for reads; on the write side, a fixed-size worker pool consumes click events from RabbitMQ, batches them, and writes them to Postgres with one multi-row `INSERT` per batch — a small, self-contained demonstration of goroutines/channels doing real work (fan-out from a shared channel, size-or-timer-triggered flush, at-least-once ack semantics).

## Why it's shaped this way

A few decisions that most shaped the design (full rationale for every phase is in [`plans/plans.md`](plans/plans.md)):

- **Monolith first, then split.** Core Service started as a self-contained REST+Postgres service (phase 1) before the Gateway/gRPC split (phase 4) — the business logic and API contract were proven stable before paying the cost of a network boundary between them.
- **Gateway is the *only* public surface.** Once it existed, Core and Stat Service's REST/HTTP paths were retired entirely rather than kept around "just in case" — one contract per capability, not two overlapping ones.
- **Domain code doesn't know about transport or storage.** `internal/link` and `internal/stats` depend on repository *interfaces*, not on Postgres or GORM; the interfaces are declared by the consumer (the domain package), not the implementation. This is why the Redis cache in phase 3 is a decorator that implements `link.Repository` — `link.Service` was never touched to add caching, only `main.go`'s wiring changed.
- **Redis cache lives in Core Service, not the Gateway.** The Gateway calls Core on every request, including redirects; caching is centralized in one place instead of duplicated (and needing invalidation) in two.
- **A message queue exists because writes shouldn't block reads.** RabbitMQ decouples "tell the user where to go" (must be fast) from "record that it happened" (can be async, batched, retried).

## Quick start

```sh
cp .env.example .env
make up
```

This builds and starts everything — Postgres, Redis, RabbitMQ, migrations, and all three Go services — with hot reload (edits to `.go` files rebuild and restart automatically, no `docker compose up --build` needed per change).

For the lean, production-equivalent build instead (no hot reload, the image that would actually ship):

```sh
make prod-up
```

Run `make help` for every available command (build, test, lint, migrations, codegen, …).

### Services

| Service | Reachable at | Role |
|---|---|---|
| API Gateway | `http://localhost:8080` | Public HTTP API — the only one |
| Core Service | `localhost:9090` (gRPC) | Internal only; published for local `grpcurl` debugging |
| Stat Service | `localhost:9091` (gRPC) | Internal only; published for local `grpcurl` debugging |
| PostgreSQL | `localhost:5432` | |
| Redis | `localhost:6379` | |
| RabbitMQ | `localhost:5672` (AMQP), `localhost:15672` (management UI) | UI login is `RABBITMQ_USER`/`RABBITMQ_PASSWORD` from `.env` |

RabbitMQ's built-in `guest` account only accepts loopback connections, which container-to-container traffic never is — so a dedicated application user is created instead (see `docker-compose.yml`).

## API

`POST /links`, `GET /{code}` (redirect), `GET /links/{code}` (info), `GET /links/{code}/stats`, `GET /health`.

Full interactive documentation (OpenAPI/Swagger, generated from code annotations — see `internal/gateway/transport/http/*.go`):

```
http://localhost:8080/swagger/index.html
```

`grpcurl` also works directly against Core/Stat Service without a local `.proto` copy (both register gRPC reflection):

```sh
grpcurl -plaintext localhost:9090 list
grpcurl -plaintext -d '{"url":"https://example.com"}' localhost:9090 link.v1.LinkService/CreateLink
```

## Observability

- **Health checks**: `GET /health` on the Gateway (liveness only — deliberately doesn't check downstream services, to avoid cascading restarts); Core/Stat Service speak the standard gRPC health protocol (`grpc.health.v1.Health`), checked via `grpc-health-probe` inside the container. All three are wired into `docker-compose`'s `healthcheck`/`depends_on: condition: service_healthy`.
- **Request tracing**: every HTTP request gets an `X-Request-Id` (reused if the caller sent one, generated otherwise), which then rides gRPC metadata into Core/Stat Service and an AMQP message header into RabbitMQ. Every `request`/`rpc` log line carries `request_id` — one ID greps across all three services' logs for a single user request.
- **Structured logging** (zap, JSON) in every service.

## Testing

```sh
make test                                    # unit tests only
TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/urlshortener?sslmode=disable make test   # + integration tests
```

Integration tests are gated behind `TEST_DATABASE_URL` and skip cleanly without it, so `make test` alone never requires a running Postgres. Notable patterns used across the suite:

- Repository tests run against a real Postgres (gated), not mocks.
- The Redis cache decorator is tested against `miniredis` (in-memory, no real broker needed).
- Cross-service HTTP↔gRPC paths (Gateway → Core, Gateway → Stat Service) are tested with a real gRPC server and client talking over `bufconn` (in-memory, no TCP port) instead of mocking the generated client.
- The RabbitMQ worker pool is tested against hand-constructed `amqp.Delivery` values with a fake `Acknowledger` — no broker needed to prove batching, ack/nack, and panic-survival behavior.

## Development

- `make proto` — regenerate `api/proto/{linkpb,statspb}` after editing a `.proto` file (requires `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc` locally).
- `make swagger` — regenerate `api/openapi` after editing Swagger annotations (requires the `swag` CLI locally).

Generated code (`api/proto/linkpb`, `api/proto/statspb`, `api/openapi`) is committed to the repo, so none of the above tools are needed to build, test, lint, or run the project — only to regenerate after a contract change.

## Project layout

```
cmd/{coreservice,gateway,statservice}/   entrypoints
internal/{link,stats}/                   domain logic — no transport/storage imports
internal/{coreservice,gateway,statservice}/   per-service transport + repository adapters
internal/platform/                       shared infra: config, logging, db/cache/mq clients, grpc middleware, request-id
internal/cache/                          Redis cache-aside decorator (implements link.Repository)
api/proto/, api/openapi/                 contracts (source + generated code)
migrations/                              SQL migrations (golang-migrate)
deployments/docker/                      one Dockerfile per service
```

## Out of scope

Auth, rate limiting, and TLS between internal services were explicitly left out — not oversights, just outside what this project set out to demonstrate. See the "stretch" phase note in `plans/plans.md` if you're curious what a next increment would look like.
