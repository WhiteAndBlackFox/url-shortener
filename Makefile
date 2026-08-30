.PHONY: help build proto swagger test lint fmt vet up prod-up down migrate-up migrate-down

# Loads POSTGRES_*/DATABASE_URL/TEST_DATABASE_URL from .env (see .env.example)
# and exports them to every recipe below. Run `cp .env.example .env` first.
-include .env
export

.DEFAULT_GOAL := help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

build: ## Build all three service binaries into ./bin
	go build -o bin/coreservice ./cmd/coreservice
	go build -o bin/gateway ./cmd/gateway
	go build -o bin/statservice ./cmd/statservice

proto: ## Regenerate api/proto/{linkpb,statspb} from *.proto (requires protoc, protoc-gen-go, protoc-gen-go-grpc - see README). Generated code is committed; CI/Docker never need protoc.
	protoc \
		--go_out=. --go_opt=module=URLShortener \
		--go-grpc_out=. --go-grpc_opt=module=URLShortener \
		api/proto/link.proto api/proto/stats.proto

swagger: ## Regenerate api/openapi from Gateway's swaggo annotations (requires the swag CLI - see README). Generated code is committed; CI/Docker never need swag.
	swag init -g cmd/gateway/main.go -o api/openapi --parseDependency

test: ## Run all unit tests; set TEST_DATABASE_URL to also run integration tests
	go test ./... -v

lint: ## Run golangci-lint (via Docker, no local install needed)
	docker run --rm -v $(CURDIR):/app -w /app golangci/golangci-lint:latest golangci-lint run

fmt: ## Format all Go source
	go fmt ./...

vet: ## Run go vet
	go vet ./...

up: ## Start the full dev stack with hot reload (docker-compose.override.yml applied automatically)
	docker compose up --build -d

prod-up: ## Start the lean, production-equivalent stack (no hot reload)
	docker compose -f docker-compose.yml up --build

down: ## Stop the stack
	docker compose down

migrate-up: ## Apply all pending migrations (runs the same migrate service defined in docker-compose.yml)
	docker compose run --rm migrate -path=/migrations -database=$(DATABASE_URL) up

migrate-down: ## Roll back all migrations
	docker compose run --rm migrate -path=/migrations -database=$(DATABASE_URL) down
