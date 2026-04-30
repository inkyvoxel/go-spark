.PHONY: init start start-web start-worker build-prod check test fmt tidy sqlc vulncheck migrate-up migrate-down migrate-status tools

PROD_GOOS ?= linux
PROD_GOARCH ?= amd64
PROD_CGO_ENABLED ?= 0
PROD_BIN ?= ./bin/app

init:
	@bash scripts/init.sh

start:
	go run ./cmd/app all

start-web:
	go run ./cmd/app serve

start-worker:
	go run ./cmd/app worker

build-prod:
	mkdir -p $(dir $(PROD_BIN))
	CGO_ENABLED=$(PROD_CGO_ENABLED) GOOS=$(PROD_GOOS) GOARCH=$(PROD_GOARCH) go build -trimpath -ldflags="-s -w" -o $(PROD_BIN) ./cmd/app

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

migrate-up:
	go run ./cmd/app migrate up

migrate-down:
	go run ./cmd/app migrate down

migrate-status:
	go run ./cmd/app migrate status

tools:
	go tool sqlc version
	go tool goose --version
	go tool govulncheck -h >/dev/null 2>&1
