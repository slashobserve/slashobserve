SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := build
MAKEFLAGS += --no-builtin-rules

VERSION ?= $(shell (git describe --tags --always --dirty 2>/dev/null || echo dev) | sed 's/^v//')
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
GO_LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

SQLC := go tool -modfile=tools/sqlc/go.mod sqlc
GOLANGCI_LINT := go tool -modfile=tools/golangci-lint/go.mod golangci-lint
COMPOSE := docker compose -f deploy/compose/compose.yaml
COMPOSE_BUILD := $(COMPOSE) -f deploy/compose/compose.build.yaml
SHOP_DIR := demo/shop
SHOP_LINT := go tool -modfile=../../tools/golangci-lint/go.mod golangci-lint
TYPE ?= errors
PCT ?= 30
# zod 3 under the shadcn CLI, a dev-only tool that pins zod ^3 and ships in no build
WEB_AUDIT_IGNORE := GHSA-mrhx-c7xc-m3w5

define DEMO_BANNER
	@echo "SlashObserve is running at http://localhost:$${SLASHOBSERVE_HOST_PORT:-8080}"
	@echo "Prometheus is running at http://localhost:$${SLASHOBSERVE_PROMETHEUS_HOST_PORT:-9090}"
	@echo "Shop fault API is at http://localhost:$${SLASHOBSERVE_SHOP_HOST_PORT:-8081}/fault"
	@echo "Sign in as admin@example.com / password (Dex, development only)"
endef

.PHONY: build server cli web web-deps generate test lint ci audit demo demo-build demo-down demo-reset demo-fault demo-fault-reset e2e e2e-browser e2e-all evals golden tidy-check clean

build: web
	$(MAKE) server cli

server:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(GO_LDFLAGS)' -o bin/slashobserve-server ./cmd/slashobserve-server

cli:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(GO_LDFLAGS)' -o bin/observe ./cmd/observe

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
	scripts/release-scripts-test.sh
	scripts/install-test.sh

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
	cd web && bun run knip
	cd web && bun run format:check

golden:
	go test ./pkg/grafana -run 'TestCorpusGoldens|TestOptionMapIsCurrent' -update

tidy-check:
	go mod tidy -diff
	cd $(SHOP_DIR) && go mod tidy -diff

ci:
	$(MAKE) lint test tidy-check
	$(MAKE) build

audit:
	go tool -modfile=tools/govulncheck/go.mod govulncheck ./...
	cd $(SHOP_DIR) && go tool -modfile=../../tools/govulncheck/go.mod govulncheck ./...
	cd web && bun audit --prod
	cd web && bun audit --audit-level=high $(addprefix --ignore=,$(WEB_AUDIT_IGNORE))

demo:
	$(COMPOSE) up -d --build --wait --wait-timeout 300
	$(DEMO_BANNER)

demo-build:
	SLASHOBSERVE_IMAGE=slashobserve-server:local $(COMPOSE_BUILD) up -d --build --wait --wait-timeout 300
	$(DEMO_BANNER)

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

evals:
	go run ./evals/runner $(EVALS_ARGS)

clean:
	rm -rf bin web/dist web/playwright-report web/test-results
	find internal/server/webdist -mindepth 1 ! -name .gitkeep -delete
