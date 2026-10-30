.PHONY: help up down migrate-up migrate-down migrate-create migrate-force migrate-version seed seed-core db-setup db-reset test-db dev build run

help:
	@echo "Available commands:"
	@echo ""
	@echo "Docker:"
	@echo "  make up                      - Build and start Postgres, API and dashboard in Docker"
	@echo "  make down                    - Stop the stack"
	@echo ""
	@echo "Development:"
	@echo "  make dev                     - Run with hot reload (Air)"
	@echo "  make build                   - Build application binary"
	@echo "  make run                     - Run application"
	@echo ""
	@echo "Database migrations:"
	@echo "  make migrate-up              - Run all pending migrations"
	@echo "  make migrate-down            - Rollback last migration"
	@echo "  make migrate-create NAME=xxx - Create new migration file"
	@echo "  make migrate-force V=xxx     - Force migration to specific version"
	@echo "  make migrate-version         - Show current migration version"
	@echo ""
	@echo "Seeding commands:"
	@echo "  make seed                    - Run all seeders"
	@echo "  make seed-core               - Run core module seeders only"
	@echo ""
	@echo "Setup commands:"
	@echo "  make db-setup                - Run migrations + seeders (fresh setup)"
	@echo "  make db-reset                - Drop, migrate, and seed database"
	@echo "  make test-db                 - Create, migrate, and seed the test database"

DB_URL=postgresql://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)
MIGRATIONS_PATH=internal/database/migrations
SEEDERS_PATH=internal/database/seeders
PSQL=docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U $(DB_USER)

include .env
export

up:
	docker compose up -d --build --wait

down:
	docker compose down

migrate-up:
	@echo "Running core migrations..."
	migrate -path $(MIGRATIONS_PATH)/core -database "$(DB_URL)&x-migrations-table=schema_migrations_core" up

migrate-down:
	@if [ -z "$(MODULE)" ]; then echo "Error: MODULE is required. Usage: make migrate-down MODULE=core"; exit 1; fi
	migrate -path $(MIGRATIONS_PATH)/$(MODULE) -database "$(DB_URL)&x-migrations-table=schema_migrations_$(MODULE)" down 1

migrate-create:
	@if [ -z "$(NAME)" ]; then echo "Error: NAME is required. Usage: make migrate-create NAME=your_migration_name MODULE=core"; exit 1; fi
	@if [ -z "$(MODULE)" ]; then echo "Error: MODULE is required. Available: core"; exit 1; fi
	migrate create -ext sql -dir $(MIGRATIONS_PATH)/$(MODULE) -format 20060102150405 $(NAME)

migrate-force:
	@if [ -z "$(V)" ]; then echo "Error: V is required. Usage: make migrate-force V=1 MODULE=core"; exit 1; fi
	@if [ -z "$(MODULE)" ]; then echo "Error: MODULE is required. Available: core"; exit 1; fi
	migrate -path $(MIGRATIONS_PATH)/$(MODULE) -database "$(DB_URL)&x-migrations-table=schema_migrations_$(MODULE)" force $(V)

migrate-version:
	@migrate -path $(MIGRATIONS_PATH)/core -database "$(DB_URL)&x-migrations-table=schema_migrations_core" version

seed-core:
	@for file in $(SEEDERS_PATH)/core/*.sql; do \
		echo "Running seeder: $$(basename $$file)"; \
		$(PSQL) -d $(DB_NAME) -v admin_email="$(SEED_ADMIN_EMAIL)" -v admin_password="$(SEED_ADMIN_PASSWORD)" < $$file || exit 1; \
	done

seed: seed-core

db-setup:
	@$(MAKE) migrate-up
	@$(MAKE) seed

db-reset:
	$(PSQL) -d postgres -c "DROP DATABASE IF EXISTS $(DB_NAME) WITH (FORCE);"
	$(PSQL) -d postgres -c "CREATE DATABASE $(DB_NAME);"
	@$(MAKE) db-setup

TEST_DB_NAME=claude_usage_test

test-db:
	@$(PSQL) -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = '$(TEST_DB_NAME)'" | grep -q 1 || $(PSQL) -d postgres -c "CREATE DATABASE $(TEST_DB_NAME);"
	migrate -path $(MIGRATIONS_PATH)/core -database "$(TEST_DATABASE_URL)&x-migrations-table=schema_migrations_core" up
	@$(MAKE) seed-core DB_NAME=$(TEST_DB_NAME)

build:
	go build -o bin/claude-usage-server cmd/api/main.go

run:
	go run cmd/api/main.go

dev:
	@~/go/bin/air
