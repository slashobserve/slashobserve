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

.PHONY: build server web web-deps generate test lint ci demo demo-down e2e e2e-browser e2e-all clean

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

test:
	go test -race ./...

lint: web-deps
	$(GOLANGCI_LINT) config verify
	$(GOLANGCI_LINT) fmt --diff
	$(GOLANGCI_LINT) run ./...
	$(SQLC) diff
	cd web && bun run typecheck
	cd web && bun run lint
	cd web && bun run format:check

ci:
	$(MAKE) lint test
	$(MAKE) build

demo:
	SLASHOBSERVE_IMAGE=slashobserve-server:local $(COMPOSE) up -d --build --wait --wait-timeout 180
	@echo "SlashObserve is running at http://localhost:$${SLASHOBSERVE_HOST_PORT:-8080}"

demo-down:
	$(COMPOSE) down

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
