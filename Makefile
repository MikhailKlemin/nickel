-include .env
export

APP_NAME := nickel
DB_HOST ?= localhost
DB_PORT ?= 5432
DB_CONTAINER ?= nickel_db
MIGRATIONS_DIR := ./migrations
DATABASE_URL := postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(POSTGRES_DB)?sslmode=disable

.PHONY: help docker-up docker-down docker-logs docker-ps \
        db-shell db-tables db-version \
        migrate-up migrate-down migrate-reset migrate-version migrate-force migrate-create \
        test build run-import run-server

help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "\033[36m%-18s\033[0m %s\n", $$1, $$2}'

docker-up: ## Start PostgreSQL container
	docker compose up -d postgres

docker-down: ## Stop PostgreSQL container
	docker compose down

docker-logs: ## Follow PostgreSQL logs
	docker compose logs -f postgres

docker-ps: ## Show compose services
	docker compose ps

db-shell: ## Open psql inside the running Postgres container
	docker exec -it $(DB_CONTAINER) psql -U $(POSTGRES_USER) -d $(POSTGRES_DB)

db-tables: ## List tables in the database
	docker exec -it $(DB_CONTAINER) psql -U $(POSTGRES_USER) -d $(POSTGRES_DB) -c '\dt'

db-version: ## Show migration tracker table
	docker exec -it $(DB_CONTAINER) psql -U $(POSTGRES_USER) -d $(POSTGRES_DB) -c 'SELECT * FROM schema_migrations;'

migrate-up: ## Apply all pending migrations
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" up

migrate-down: ## Roll back all migrations
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" down -all

migrate-reset: ## Roll back all migrations and re-apply them
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" down -all
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" up

migrate-version: ## Show current migration version
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" version

migrate-force: ## Force migration version, usage: make migrate-force VERSION=2
	@if [ -z "$(VERSION)" ]; then echo "Usage: make migrate-force VERSION=2"; exit 1; fi
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" force $(VERSION)

migrate-create: ## Create a new sequential SQL migration, usage: make migrate-create NAME=add_balance_column
	@if [ -z "$(NAME)" ]; then echo "Usage: make migrate-create NAME=your_migration_name"; exit 1; fi
	migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $(NAME)

test: ## Run Go tests
	go test ./...

build: ## Build all main packages
	go build ./...

run-import: ## Run import command
	go run ./cmd/import

run-server: ## Run API server
	go run ./cmd/server
