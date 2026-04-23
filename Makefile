.PHONY: init start start-web start-worker check test fmt tidy sqlc vulncheck setup migrate-up migrate-down migrate-status tools

DB_PATH ?= ./data/app.db
GOOSE_DRIVER ?= sqlite3
GOOSE_DBSTRING ?= $(DB_PATH)
GOOSE_MIGRATION_DIR ?= ./migrations

init:
	go run ./cmd/app init

start:
	go run ./cmd/app start

start-web:
	go run ./cmd/app start web

start-worker:
	go run ./cmd/app start worker

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
