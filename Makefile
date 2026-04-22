.PHONY: run run-all run-web run-worker check test fmt tidy sqlc vulncheck setup migrate-up migrate-down migrate-status tools

DB_PATH ?= ./data/app.db
GOOSE_DRIVER ?= sqlite3
GOOSE_DBSTRING ?= $(DB_PATH)
GOOSE_MIGRATION_DIR ?= ./migrations

run:
	go run ./cmd/app

run-all:
	go run ./cmd/app all

run-web:
	go run ./cmd/app web

run-worker:
	go run ./cmd/app worker

check:
	$(MAKE) fmt
	$(MAKE) tidy
	$(MAKE) sqlc
	$(MAKE) vulncheck
	$(MAKE) test

test:
	go test ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy

sqlc:
	go tool sqlc generate

vulncheck:
	go tool govulncheck ./...

setup: migrate-up

migrate-up:
	mkdir -p $(dir $(DB_PATH))
	go tool goose -dir $(GOOSE_MIGRATION_DIR) $(GOOSE_DRIVER) "$(GOOSE_DBSTRING)" up

migrate-down:
	mkdir -p $(dir $(DB_PATH))
	go tool goose -dir $(GOOSE_MIGRATION_DIR) $(GOOSE_DRIVER) "$(GOOSE_DBSTRING)" down

migrate-status:
	mkdir -p $(dir $(DB_PATH))
	go tool goose -dir $(GOOSE_MIGRATION_DIR) $(GOOSE_DRIVER) "$(GOOSE_DBSTRING)" status

tools:
	go tool sqlc version
	go tool goose --version
	go tool govulncheck -h >/dev/null 2>&1
