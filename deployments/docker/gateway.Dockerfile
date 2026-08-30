## base: shared by dev and builder - toolchain + downloaded deps only.
FROM golang:1.26-alpine AS base

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

## dev: base + air, for local hot-reload only. Source is bind-mounted at
## runtime (see docker-compose.override.yml), not baked into the image.
FROM base AS dev

RUN go install github.com/air-verse/air@latest

EXPOSE 8080
CMD ["air", "-c", ".air.gateway.toml"]

## builder: base + full source, compiles a static binary for the runtime stage.
FROM base AS builder

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/gateway ./cmd/gateway

## runtime: no Go toolchain, just the compiled binary. Default build target.
FROM alpine:3.20 AS runtime

RUN apk add --no-cache ca-certificates && \
    adduser -D -u 10001 appuser

COPY --from=builder /out/gateway /usr/local/bin/gateway

USER appuser
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/gateway"]
