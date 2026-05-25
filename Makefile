# shadraw backend developer makefile
# Most targets run in the local shell. Use docker-compose for the full stack.

SHELL := /bin/bash
PKG    := ./...
BIN    := bin/server
MIG_DIR := migrations
MIG_DSN ?= $${DB_DSN:-postgres://shadraw:shadraw@localhost:5432/shadraw?sslmode=disable}

.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: run
run: ## Run the server locally (requires .env)
	go run ./cmd/server

.PHONY: build
build: ## Build server binary
	go build -o $(BIN) ./cmd/server

.PHONY: test
test: ## Run unit + integration tests with race detector and coverage
	go test -race -cover $(PKG)

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run

.PHONY: fmt
fmt: ## Format code
	gofumpt -l -w .

.PHONY: tidy
tidy: ## go mod tidy
	go mod tidy

.PHONY: migrate-up
migrate-up: ## Apply all up migrations
	migrate -path $(MIG_DIR) -database "$(MIG_DSN)" up

.PHONY: migrate-down
migrate-down: ## Roll back one migration
	migrate -path $(MIG_DIR) -database "$(MIG_DSN)" down 1

.PHONY: migrate-new
migrate-new: ## Create new migration pair (NAME=short_name)
	@if [ -z "$(NAME)" ]; then echo "usage: make migrate-new NAME=add_xxx" && exit 1; fi
	migrate create -ext sql -dir $(MIG_DIR) -seq -digits 3 $(NAME)
