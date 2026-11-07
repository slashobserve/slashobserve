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
SHOP_DIR := demo
SHOP_LINT := go tool -modfile=../tools/golangci-lint/go.mod golangci-lint
TYPE ?= errors
EVALS_STACK_MODE ?= replay
EVALS_MATRIX ?= evals/matrix.yaml
EVALS_REPORT_DIR ?= evals/reports/live
PCT ?= 30
GO_TEST_FLAGS := -race -timeout 20m $(if $(COVER),-coverprofile=coverage.out -covermode=atomic)
WEB_TEST_FLAGS := $(if $(COVER),--coverage)
# zod 3 under the shadcn CLI, a dev-only tool that pins zod ^3 and ships in no build
WEB_AUDIT_IGNORE := GHSA-mrhx-c7xc-m3w5

define DEMO_BANNER
	@echo "SlashObserve is running at http://localhost:$${SLASHOBSERVE_HOST_PORT:-8080}"
	@echo "Prometheus is running at http://localhost:$${SLASHOBSERVE_PROMETHEUS_HOST_PORT:-9090}"
	@echo "Shop fault API is at http://localhost:$${SLASHOBSERVE_SHOP_HOST_PORT:-8081}/fault"
	@echo "Sign in as admin@example.com / password (Dex, development only)"
endef

.PHONY: build server cli web web-deps generate test lint ci audit demo demo-build demo-search demo-search-down demo-down demo-reset demo-fault demo-fault-reset demo-k8s demo-k8s-down helm-kind helm-kind-down e2e e2e-browser e2e-all evals evals-stack-up evals-stack-down evals-replay evals-live evals-compare golden tidy-check clean

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
	go test $(GO_TEST_FLAGS) ./...
	$(if $(COVER),go tool cover -func=coverage.out | tail -1)
	cd $(SHOP_DIR) && go test -race ./...
	cd web && bun run test $(WEB_TEST_FLAGS)
	scripts/release-scripts-test.sh
	scripts/install-test.sh

lint: web-deps
	$(GOLANGCI_LINT) config verify
	$(GOLANGCI_LINT) fmt --diff
	$(GOLANGCI_LINT) run ./...
	cd $(SHOP_DIR) && $(SHOP_LINT) fmt --config ../.golangci.yaml --diff
	cd $(SHOP_DIR) && $(SHOP_LINT) run --config ../.golangci.yaml ./...
	$(SQLC) diff
	cd web && bun run typecheck
	cd web && bun run spec:check
	cd web && bun run api:check
	cd web && bun run lint
	cd web && bun run knip
	cd web && bun run format:check
	scripts/docs-lint.sh
	scripts/helm-check.sh

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
	cd $(SHOP_DIR) && go tool -modfile=../tools/govulncheck/go.mod govulncheck ./...
	cd web && bun audit --prod
	cd web && bun audit --audit-level=high $(addprefix --ignore=,$(WEB_AUDIT_IGNORE))

demo:
	$(COMPOSE) up -d --wait --wait-timeout 300
	$(DEMO_BANNER)

demo-build:
	SLASHOBSERVE_IMAGE=slashobserve-server:local SLASHOBSERVE_SHOP_IMAGE=slashobserve-demo-shop:local $(COMPOSE_BUILD) up -d --build --wait --wait-timeout 300
	$(DEMO_BANNER)

demo-search:
	SLASHOBSERVE_IMAGE=slashobserve-server:local SLASHOBSERVE_SHOP_IMAGE=slashobserve-demo-shop:local $(COMPOSE_BUILD) --profile search up -d --build --wait --wait-timeout 600
	$(DEMO_BANNER)
	@echo "Elasticsearch is running at http://localhost:$${SLASHOBSERVE_ELASTICSEARCH_HOST_PORT:-9200}"
	@echo "OpenSearch is running at http://localhost:$${SLASHOBSERVE_OPENSEARCH_HOST_PORT:-9201}"

demo-search-down:
	$(COMPOSE) --profile search stop elasticsearch opensearch otel-collector-search

demo-down:
	$(COMPOSE) --profile search down

demo-reset:
	$(COMPOSE) --profile search down -v

demo-fault:
	curl -fsS -X POST "http://localhost:$${SLASHOBSERVE_SHOP_HOST_PORT:-8081}/fault?type=$(TYPE)&pct=$(PCT)"

demo-fault-reset:
	curl -fsS -X DELETE "http://localhost:$${SLASHOBSERVE_SHOP_HOST_PORT:-8081}/fault"

demo-k8s:
	scripts/demo-k8s.sh up

demo-k8s-down:
	scripts/demo-k8s.sh down

helm-kind:
	scripts/helm-kind.sh up
	scripts/helm-kind.sh check

helm-kind-down:
	scripts/helm-kind.sh down

e2e:
	scripts/e2e.sh

e2e-browser: web-deps
	scripts/e2e.sh browser

e2e-all:
	scripts/e2e.sh
	$(MAKE) e2e-browser

evals:
	go run ./evals/runner $(EVALS_ARGS)

evals-stack-up:
	scripts/evals-stack.sh up $(EVALS_STACK_MODE)

evals-stack-down:
	scripts/evals-stack.sh down

evals-replay:
	scripts/evals-stack.sh up replay
	go run ./evals/runner --server "http://localhost:$${SLASHOBSERVE_EVALS_PORT:-28080}" --prometheus "http://localhost:$${SLASHOBSERVE_EVALS_PROMETHEUS_PORT:-29090}" --suite replay --bootstrap-dex --min-pass 1

evals-live:
	go run ./evals/runner --server "http://localhost:$${SLASHOBSERVE_EVALS_PORT:-28080}" --prometheus "http://localhost:$${SLASHOBSERVE_EVALS_PROMETHEUS_PORT:-29090}" --suite live --matrix $(EVALS_MATRIX) --bootstrap-dex --out $(EVALS_REPORT_DIR)

evals-compare:
	@test -n "$(EVALS_BASE)" -a -n "$(EVALS_NEW)" || { echo "usage: make evals-compare EVALS_BASE=<report.json> EVALS_NEW=<report.json>" >&2; exit 2; }
	go run ./evals/runner --compare "$(EVALS_BASE)" "$(EVALS_NEW)"

clean:
	rm -rf bin web/dist web/playwright-report web/test-results web/coverage coverage.out
	find internal/server/webdist -mindepth 1 ! -name .gitkeep -delete
