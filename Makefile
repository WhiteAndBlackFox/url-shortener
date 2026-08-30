.PHONY: build proto test lint fmt vet up prod-up down migrate-up migrate-down

# Loads POSTGRES_*/DATABASE_URL/TEST_DATABASE_URL from .env (see .env.example)
# and exports them to every recipe below. Run `cp .env.example .env` first.
-include .env
export

build:
	go build -o bin/coreservice ./cmd/coreservice
	go build -o bin/gateway ./cmd/gateway

## Regenerates api/proto/linkpb from api/proto/link.proto. Requires protoc
## (system package) plus protoc-gen-go/protoc-gen-go-grpc (go install, see
## README). Generated code is committed, so this only needs to be run when
## link.proto changes - CI and Docker builds never need protoc.
proto:
	protoc \
		--go_out=. --go_opt=module=URLShortener \
		--go-grpc_out=. --go-grpc_opt=module=URLShortener \
		api/proto/link.proto

test:
	go test ./... -v

lint:
	docker run --rm -v $(CURDIR):/app -w /app golangci/golangci-lint:latest golangci-lint run

fmt:
	go fmt ./...

vet:
	go vet ./...

up: ## dev stack with hot reload (docker-compose.override.yml applied automatically)
	docker compose up --build -d

prod-up: ## lean, production-equivalent stack (no hot reload)
	docker compose -f docker-compose.yml up --build

down:
	docker compose down

migrate-up: ## runs the same migrate service defined in docker-compose.yml
	docker compose run --rm migrate -path=/migrations -database=$(DATABASE_URL) up

migrate-down:
	docker compose run --rm migrate -path=/migrations -database=$(DATABASE_URL) down