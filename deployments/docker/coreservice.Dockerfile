## base: shared by dev and builder - toolchain + downloaded deps only.
## Copying go.mod/go.sum before any other source means this layer (and the
## go mod download layer) is cached as long as dependencies don't change.
## grpc-health-probe is installed here (not just in "builder") so both the
## dev (hot-reload) and runtime images can serve docker-compose's
## healthcheck, which execs this binary against the gRPC health service
## registered in transport/grpc/server.go.
FROM golang:1.26-alpine AS base

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
RUN go install github.com/grpc-ecosystem/grpc-health-probe@v0.4.24

## dev: base + air, for local hot-reload only. Source is bind-mounted at
## runtime (see docker-compose.override.yml), not baked into the image, so
## air rebuilds/restarts the binary as files change on the host. Never used
## to produce what ships to production.
FROM base AS dev

RUN go install github.com/air-verse/air@latest

EXPOSE 9090
CMD ["air", "-c", ".air.coreservice.toml"]

## builder: base + full source, compiles a static binary for the runtime stage.
FROM base AS builder

COPY . .

# CGO_ENABLED=0: our dependencies (pgx, gorm) are pure Go, so we can produce
# a fully static binary that doesn't need glibc/musl at runtime.
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/coreservice ./cmd/coreservice

## runtime: no Go toolchain, just the compiled binary. This is the default
## build target (last stage in the file) and what actually ships.
FROM alpine:3.20 AS runtime

RUN apk add --no-cache ca-certificates && \
    adduser -D -u 10001 appuser

COPY --from=builder /out/coreservice /usr/local/bin/coreservice
COPY --from=builder /root/go/bin/grpc-health-probe /usr/local/bin/grpc-health-probe

USER appuser
EXPOSE 9090

ENTRYPOINT ["/usr/local/bin/coreservice"]
