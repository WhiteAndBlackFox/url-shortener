.PHONY: build run test lint fmt vet

build: ## Появится в Фазе 1, когда будет cmd/coreservice
	@echo "TODO: реализовать в Фазе 1"

run: ## Появится в Фазе 1
	@echo "TODO: реализовать в Фазе 1"

test:
	go test ./...

lint: ## golangci-lint появится в Фазе 2
	@echo "TODO: реализовать в Фазе 2 (golangci-lint)"

fmt:
	go fmt ./...

vet:
	go vet ./...