SHELL := /usr/bin/env bash
GO ?= go
SERVICES := api-service runtime-worker tool-gateway llm-gateway

.PHONY: fmt lint test build clean run-api run-worker run-tool run-llm docker-up docker-down docker-ps migrate-up migrate-down migrate-status health verify-services

fmt:
	gofmt -w $(shell find . -name '*.go' -not -path './.git/*')

lint:
	golangci-lint run ./...

test:
	$(GO) test ./...

build:
	mkdir -p bin
	for service in $(SERVICES); do \
		$(GO) build -trimpath -o bin/$$service ./cmd/$$service; \
	done

clean:
	rm -rf bin coverage.out

run-api:
	$(GO) run ./cmd/api-service

run-worker:
	$(GO) run ./cmd/runtime-worker

run-tool:
	$(GO) run ./cmd/tool-gateway

run-llm:
	$(GO) run ./cmd/llm-gateway

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

docker-ps:
	docker compose ps

migrate-up:
	./scripts/migrate.sh up

migrate-down:
	./scripts/migrate.sh down

migrate-status:
	./scripts/migrate.sh status

health:
	./scripts/healthcheck.sh

verify-services:
	./scripts/verify-services.sh
