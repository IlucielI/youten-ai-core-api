.PHONY: help run build test docker-up docker-down migrate-create migrate-up migrate-down migrate-status migrate-force

GOCMD ?= $(shell which go 2>/dev/null || which $(HOME)/go-local/go/bin/go 2>/dev/null || echo go)

# Default target
help:
	@echo "Available commands:"
	@echo "  make run                   Run the API server"
	@echo "  make build                 Build the API binary into bin/api"
	@echo "  make test                  Run all unit tests"
	@echo "  make docker-up             Start PostgreSQL, Redis, RabbitMQ, MinIO, and Mailpit"
	@echo "  make docker-down           Stop all background Docker services"
	@echo "  make migrate-create name=X Create a new pair of migration files"
	@echo "  make migrate-up [step=N]   Apply pending migrations (or top N steps)"
	@echo "  make migrate-down [step=N] Rollback last migration (or N steps)"
	@echo "  make migrate-status        Check current migration version and dirty state"
	@echo "  make migrate-force version=N Force migration version state"

run:
	$(GOCMD) run cmd/api/main.go

build:
	mkdir -p bin
	$(GOCMD) build -o bin/api cmd/api/main.go

test:
	$(GOCMD) test -v ./...

docker-up:
	docker compose --env-file .env -f deployment/docker-compose.yaml up -d postgres redis rabbitmq mailpit meet-runner

docker-down:
	docker compose -f deployment/docker-compose.yaml down

migrate-create:
	@if [ -z "$(name)" ]; then echo "Error: 'name' is required. Example: make migrate-create name=create_users_table"; exit 1; fi
	$(GOCMD) run cmd/api/main.go migrate create $(name)

migrate-up:
	$(GOCMD) run cmd/api/main.go migrate up $(step)

migrate-down:
	$(GOCMD) run cmd/api/main.go migrate down $(step)

migrate-status:
	$(GOCMD) run cmd/api/main.go migrate status

migrate-force:
	@if [ -z "$(version)" ]; then echo "Error: 'version' is required. Example: make migrate-force version=20260928120000"; exit 1; fi
	$(GOCMD) run cmd/api/main.go migrate force $(version)
