.PHONY: run test fmt tidy sqlc migrate-up migrate-down migrate-status tools

DB_PATH ?= ./data/app.db
GOOSE_DRIVER ?= sqlite3
GOOSE_DBSTRING ?= $(DB_PATH)
GOOSE_MIGRATION_DIR ?= ./migrations

run:
	go run ./cmd/app

test:
	go test ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy

sqlc:
	go tool sqlc generate

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
