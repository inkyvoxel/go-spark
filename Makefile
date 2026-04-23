.PHONY: init start start-web start-worker check test fmt tidy sqlc vulncheck setup migrate-up migrate-down migrate-status tools

DB_PATH ?= ./data/app.db

init:
	go run ./cmd/app init

start:
	go run ./cmd/app all

start-web:
	go run ./cmd/app serve

start-worker:
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
	DATABASE_PATH=$(DB_PATH) go run ./cmd/app migrate up

migrate-down:
	DATABASE_PATH=$(DB_PATH) go run ./cmd/app migrate down

migrate-status:
	DATABASE_PATH=$(DB_PATH) go run ./cmd/app migrate status

tools:
	go tool sqlc version
	go tool goose --version
	go tool govulncheck -h >/dev/null 2>&1
