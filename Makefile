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
GO_TEST_PKGS ?= ./...
WEB_TEST_FLAGS := $(if $(COVER),--coverage)
# zod 3 under the shadcn CLI, a dev-only tool that pins zod ^3 and ships in no build
WEB_AUDIT_IGNORE := GHSA-mrhx-c7xc-m3w5
# hono and @hono/node-server under the MCP SDK's server transports, which observe-agent never loads; drop once hono 4.13.16 and @hono/node-server 2.1.6 are 14 days old (2026-11-16)
AGENT_AUDIT_IGNORE := GHSA-qv5m-9pm6-hqp3 GHSA-jpvv-r467-5p4w GHSA-4m76-xfj5-7975 GHSA-h7mg-cq63-vhpq GHSA-3gg2-335m-rxc6 GHSA-46wx-v44x-9hh2 GHSA-4h7h-rv7c-7m2v

define DEMO_BANNER
	@echo "SlashObserve is running at http://localhost:$${SLASHOBSERVE_HOST_PORT:-8080}"
	@echo "Prometheus is running at http://localhost:$${SLASHOBSERVE_PROMETHEUS_HOST_PORT:-9090}"
	@echo "Shop fault API is at http://localhost:$${SLASHOBSERVE_SHOP_HOST_PORT:-8081}/fault"
	@echo "Sign in as admin@example.com / password (Dex, development only)"
endef

.PHONY: build server cli agent agent-deps web web-deps generate test test-go test-shop test-web test-agent test-scripts test-benchmarks lint lint-go lint-web lint-web-types lint-web-eslint lint-web-knip lint-web-format lint-agent lint-deploy ci audit demo demo-build demo-search demo-search-down demo-down demo-reset demo-fault demo-fault-reset demo-k8s demo-k8s-down astronomy astronomy-build astronomy-search astronomy-down astronomy-reset astronomy-check astronomy-flag helm-kind helm-kind-down replicas-up replicas-down replicas-check loadtest e2e e2e-warm e2e-up e2e-browser e2e-all evals evals-stack-up evals-stack-down evals-replay evals-live evals-compare golden tidy-check clean

build: web
	$(MAKE) server cli agent

server:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(GO_LDFLAGS)' -o bin/slashobserve-server ./cmd/slashobserve-server

cli:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(GO_LDFLAGS)' -o bin/observe ./cmd/observe

web-deps:
	cd web && bun install --frozen-lockfile

agent-deps:
	cd observe-agent && bun install --frozen-lockfile

agent: agent-deps
	cd observe-agent && bun build --compile --minify --define OBSERVE_AGENT_VERSION='"$(VERSION)"' src/main.ts --outfile ../bin/observe-agent

web: web-deps
	cd web && bun run build

generate: web-deps
	$(SQLC) generate
	go test ./internal/surface -run TestStatesReferenceIsCurrent -update
	cd web && bun run spec:generate
	cd web && bun run api:generate

test: test-go test-shop test-web test-agent test-scripts test-benchmarks

test-go:
	go test $(GO_TEST_FLAGS) $(GO_TEST_PKGS)
	$(if $(COVER),go tool cover -func=coverage.out | tail -1)

test-shop:
	cd $(SHOP_DIR) && go test -race ./...

test-web: web-deps
	cd web && bun run test $(WEB_TEST_FLAGS)

test-agent: agent-deps
	cd observe-agent && bun test

test-scripts:
	scripts/release-scripts-test.sh
	scripts/install-test.sh
	scripts/docs-lint-test.sh
	scripts/ci-changes-test.sh

test-benchmarks:
	uv run --locked --project benchmarks pytest benchmarks/tests

lint: lint-go lint-web lint-agent lint-deploy

lint-go:
	$(GOLANGCI_LINT) config verify
	$(GOLANGCI_LINT) fmt --diff
	$(GOLANGCI_LINT) run ./...
	cd $(SHOP_DIR) && $(SHOP_LINT) fmt --config ../.golangci.yaml --diff
	cd $(SHOP_DIR) && $(SHOP_LINT) run --config ../.golangci.yaml ./...
	$(SQLC) diff

lint-web: lint-web-types lint-web-eslint lint-web-knip lint-web-format

lint-web-types: web-deps
	cd web && bun run typecheck
	cd web && bun run spec:check
	cd web && bun run api:check

lint-web-eslint: web-deps
	cd web && bun run lint

lint-web-knip: web-deps
	cd web && bun run knip

lint-web-format: web-deps
	cd web && bun run format:check

lint-agent: agent-deps
	cd observe-agent && bun run typecheck
	cd observe-agent && bun run format:check

lint-deploy:
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
	cd observe-agent && bun audit --prod $(addprefix --ignore=,$(AGENT_AUDIT_IGNORE))
	cd observe-agent && bun audit --audit-level=high

demo:
	$(COMPOSE) up -d --wait --wait-timeout 300
	$(DEMO_BANNER)

demo-build:
	VERSION=$(VERSION) COMMIT=$(COMMIT) SLASHOBSERVE_IMAGE=slashobserve-server:local SLASHOBSERVE_SHOP_IMAGE=slashobserve-demo-shop:local $(COMPOSE_BUILD) up -d --build --wait --wait-timeout 300
	$(DEMO_BANNER)

demo-search:
	VERSION=$(VERSION) COMMIT=$(COMMIT) SLASHOBSERVE_IMAGE=slashobserve-server:local SLASHOBSERVE_SHOP_IMAGE=slashobserve-demo-shop:local $(COMPOSE_BUILD) --profile search up -d --build --wait --wait-timeout 600
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

astronomy:
	scripts/astronomy.sh up

astronomy-build:
	scripts/astronomy.sh up --build

astronomy-search:
	scripts/astronomy.sh up --build --search

astronomy-down:
	scripts/astronomy.sh down

astronomy-reset:
	scripts/astronomy.sh down --volumes

astronomy-check:
	scripts/astronomy.sh check

astronomy-flag:
	scripts/astronomy.sh flag $(FLAG) $(VARIANT)

helm-kind:
	scripts/helm-kind.sh up
	scripts/helm-kind.sh check

helm-kind-down:
	scripts/helm-kind.sh down

replicas-up:
	scripts/replicas.sh up

replicas-down:
	scripts/replicas.sh down

replicas-check:
	scripts/replicas.sh down
	scripts/replicas.sh up
	scripts/replicas.sh check

loadtest:
	scripts/replicas.sh down
	scripts/replicas.sh up
	scripts/replicas.sh stop 2
	@failed=0; set -e; \
	scripts/replicas.sh load servers1-workers8 || failed=1; \
	scripts/replicas.sh start 2; \
	scripts/replicas.sh load servers2-workers8 || failed=1; \
	scripts/replicas.sh workers 2; \
	scripts/replicas.sh load --only agent servers2-workers2 || failed=1; \
	scripts/replicas.sh workers 16; \
	scripts/replicas.sh load --only agent servers2-workers16 || failed=1; \
	exit $$failed

e2e:
	scripts/e2e.sh

e2e-warm:
	scripts/e2e.sh warm

e2e-up:
	scripts/e2e.sh up

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
	scripts/evals-stack.sh up --no-history replay
	go run ./evals/runner --server "http://localhost:$${SLASHOBSERVE_EVALS_PORT:-28080}" --prometheus "http://localhost:$${SLASHOBSERVE_EVALS_PROMETHEUS_PORT:-29090}" --suite replay --await-history --bootstrap-dex --min-pass 1 $(if $(EVALS_OUT),--out "$(EVALS_OUT)")

evals-live:
	go run ./evals/runner --server "http://localhost:$${SLASHOBSERVE_EVALS_PORT:-28080}" --prometheus "http://localhost:$${SLASHOBSERVE_EVALS_PROMETHEUS_PORT:-29090}" --suite live --matrix $(EVALS_MATRIX) --bootstrap-dex --out $(EVALS_REPORT_DIR)

evals-compare:
	@test -n "$(EVALS_BASE)" -a -n "$(EVALS_NEW)" || { echo "usage: make evals-compare EVALS_BASE=<report.json> EVALS_NEW=<report.json>" >&2; exit 2; }
	go run ./evals/runner --compare "$(EVALS_BASE)" "$(EVALS_NEW)"

clean:
	rm -rf bin web/dist web/playwright-report web/test-results web/coverage coverage.out
	find internal/server/webdist -mindepth 1 ! -name .gitkeep -delete
