.PHONY: build test unit integration cover lint up down

build:
	go build -o bin/ ./cmd/...

UNIT_PKGS := ./cmd/... ./internal/...

test:
	go test -race ./...

unit:
	go test -race -count=1 $(UNIT_PKGS)

integration:
	docker compose up -d --build server proxy
	SIP_COMPOSE=1 go test -race -count=1 ./test/... || (docker compose down; exit 1)
	docker compose down

cover:
	go test -count=1 -coverprofile=coverage.out $(UNIT_PKGS)
	go tool cover -func=coverage.out | tail -1

GOLANGCI_LINT_VERSION := v2.14.0

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

up:
	docker compose up --build

down:
	docker compose down
