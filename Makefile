.PHONY: run test fmt tidy sqlc migrate-up migrate-down

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
	sqlc generate

migrate-up:
	goose -dir $(GOOSE_MIGRATION_DIR) $(GOOSE_DRIVER) "$(GOOSE_DBSTRING)" up

migrate-down:
	goose -dir $(GOOSE_MIGRATION_DIR) $(GOOSE_DRIVER) "$(GOOSE_DBSTRING)" down
