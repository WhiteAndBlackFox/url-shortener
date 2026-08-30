## base: shared by dev and builder - toolchain + downloaded deps only.
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
## runtime (see docker-compose.override.yml), not baked into the image.
FROM base AS dev

RUN go install github.com/air-verse/air@latest

EXPOSE 9091
CMD ["air", "-c", ".air.statservice.toml"]

## builder: base + full source, compiles a static binary for the runtime stage.
FROM base AS builder

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/statservice ./cmd/statservice

## runtime: no Go toolchain, just the compiled binary. Default build target.
FROM alpine:3.20 AS runtime

RUN apk add --no-cache ca-certificates && \
    adduser -D -u 10001 appuser

COPY --from=builder /out/statservice /usr/local/bin/statservice
COPY --from=builder /root/go/bin/grpc-health-probe /usr/local/bin/grpc-health-probe

USER appuser
EXPOSE 9091

ENTRYPOINT ["/usr/local/bin/statservice"]
