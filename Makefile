SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := build
MAKEFLAGS += --no-builtin-rules

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
GO_LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

SQLC := go tool -modfile=tools/sqlc/go.mod sqlc
GOLANGCI_LINT := go tool -modfile=tools/golangci-lint/go.mod golangci-lint
COMPOSE := docker compose -f deploy/compose/compose.yaml
SHOP_DIR := demo/shop
SHOP_LINT := go tool -modfile=../../tools/golangci-lint/go.mod golangci-lint
TYPE ?= errors
PCT ?= 30

.PHONY: build server web web-deps generate test lint ci demo demo-down demo-reset demo-fault demo-fault-reset e2e e2e-browser e2e-all tidy-check clean

build: web
	$(MAKE) server

server:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(GO_LDFLAGS)' -o bin/slashobserve-server ./cmd/slashobserve-server

web-deps:
	cd web && bun install --frozen-lockfile

web: web-deps
	cd web && bun run build

generate: web-deps
	$(SQLC) generate
	cd web && bun run spec:generate
	cd web && bun run api:generate

test: web-deps
	go test -race ./...
	cd $(SHOP_DIR) && go test -race ./...
	cd web && bun run test

lint: web-deps
	$(GOLANGCI_LINT) config verify
	$(GOLANGCI_LINT) fmt --diff
	$(GOLANGCI_LINT) run ./...
	cd $(SHOP_DIR) && $(SHOP_LINT) fmt --config ../../.golangci.yaml --diff
	cd $(SHOP_DIR) && $(SHOP_LINT) run --config ../../.golangci.yaml ./...
	$(SQLC) diff
	cd web && bun run typecheck
	cd web && bun run spec:check
	cd web && bun run api:check
	cd web && bun run lint
	cd web && bun run format:check

tidy-check:
	go mod tidy -diff
	cd $(SHOP_DIR) && go mod tidy -diff

ci:
	$(MAKE) lint test tidy-check
	$(MAKE) build

demo:
	SLASHOBSERVE_IMAGE=slashobserve-server:local SLASHOBSERVE_SHOP_IMAGE=slashobserve-demo-shop:local $(COMPOSE) up -d --build --wait --wait-timeout 300
	@echo "SlashObserve is running at http://localhost:$${SLASHOBSERVE_HOST_PORT:-8080}"
	@echo "Prometheus is running at http://localhost:$${SLASHOBSERVE_PROMETHEUS_HOST_PORT:-9090}"
	@echo "Shop fault API is at http://localhost:$${SLASHOBSERVE_SHOP_HOST_PORT:-8081}/fault"
	@echo "Sign in as admin@example.com / password (Dex, development only)"

demo-down:
	$(COMPOSE) down

demo-reset:
	$(COMPOSE) down -v

demo-fault:
	curl -fsS -X POST "http://localhost:$${SLASHOBSERVE_SHOP_HOST_PORT:-8081}/fault?type=$(TYPE)&pct=$(PCT)"

demo-fault-reset:
	curl -fsS -X DELETE "http://localhost:$${SLASHOBSERVE_SHOP_HOST_PORT:-8081}/fault"

e2e:
	scripts/e2e.sh

e2e-browser: web-deps
	scripts/e2e.sh browser

e2e-all:
	scripts/e2e.sh
	$(MAKE) e2e-browser

clean:
	rm -rf bin web/dist web/playwright-report web/test-results
	find internal/server/webdist -mindepth 1 ! -name .gitkeep -delete
