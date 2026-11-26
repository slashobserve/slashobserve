SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := build
MAKEFLAGS += --no-builtin-rules

VERSION ?= $(shell (git describe --tags --always --dirty 2>/dev/null || echo dev) | sed 's/^v//')
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
GO_LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

SQLC := go tool -modfile=tools/sqlc/go.mod sqlc
GOLANGCI_LINT := go tool -modfile=tools/golangci-lint/go.mod golangci-lint
GO_TEST_FLAGS := -race -timeout 20m $(if $(COVER),-coverprofile=coverage.out -covermode=atomic)
GO_TEST_PKGS ?= ./...
WEB_TEST_FLAGS := $(if $(COVER),--coverage)
# zod 3 under the shadcn CLI, a dev-only tool that pins zod ^3 and ships in no build
WEB_AUDIT_IGNORE := GHSA-mrhx-c7xc-m3w5
# hono and @hono/node-server under the MCP SDK's server transports, which observe-agent never loads; drop once hono 4.13.16 and @hono/node-server 2.1.6 are 14 days old (2026-11-16)
AGENT_AUDIT_IGNORE := GHSA-qv5m-9pm6-hqp3 GHSA-jpvv-r467-5p4w GHSA-4m76-xfj5-7975 GHSA-h7mg-cq63-vhpq GHSA-3gg2-335m-rxc6 GHSA-46wx-v44x-9hh2 GHSA-4h7h-rv7c-7m2v

.PHONY: build server cli agent agent-deps web web-deps generate test test-go test-web test-agent test-scripts test-benchmarks lint lint-go lint-web lint-web-types lint-web-eslint lint-web-knip lint-web-format lint-agent lint-deploy ci audit demo demo-build demo-search demo-code demo-down demo-reset demo-check demo-fault demo-fault-reset helm-kind helm-kind-down replicas-up replicas-down replicas-check loadtest e2e e2e-warm e2e-up e2e-browser e2e-all golden tidy-check clean

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

test: test-go test-web test-agent test-scripts test-benchmarks

test-go:
	go test $(GO_TEST_FLAGS) $(GO_TEST_PKGS)
	$(if $(COVER),go tool cover -func=coverage.out | tail -1)

test-web: web-deps
	cd web && bun run test $(WEB_TEST_FLAGS)

test-agent: agent-deps
	cd observe-agent && bun test

test-scripts:
	scripts/release-scripts-test.sh
	scripts/install-test.sh
	scripts/docs-lint-test.sh
	scripts/ci-changes-test.sh
	scripts/astronomy-test.sh
	scripts/replicas-test.sh

test-benchmarks:
	uv run --locked --project benchmarks pytest benchmarks/tests
	uv run --locked --project benchmarks/orca pytest benchmarks/orca/tests

lint: lint-go lint-web lint-agent lint-deploy

lint-go:
	$(GOLANGCI_LINT) config verify
	$(GOLANGCI_LINT) fmt --diff
	$(GOLANGCI_LINT) run ./...
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

ci:
	$(MAKE) lint test tidy-check
	$(MAKE) build

audit:
	go tool -modfile=tools/govulncheck/go.mod govulncheck ./...
	cd web && bun audit --prod
	cd web && bun audit --audit-level=high $(addprefix --ignore=,$(WEB_AUDIT_IGNORE))
	cd observe-agent && bun audit --prod $(addprefix --ignore=,$(AGENT_AUDIT_IGNORE))
	cd observe-agent && bun audit --audit-level=high

demo:
	scripts/astronomy.sh up --published

demo-build:
	VERSION=$(VERSION) COMMIT=$(COMMIT) scripts/astronomy.sh up

demo-search:
	VERSION=$(VERSION) COMMIT=$(COMMIT) scripts/astronomy.sh up --search

demo-code:
	VERSION=$(VERSION) COMMIT=$(COMMIT) scripts/astronomy.sh up --code

demo-down:
	scripts/astronomy.sh down

demo-reset:
	scripts/astronomy.sh down --volumes

demo-check:
	scripts/astronomy.sh check

demo-fault:
	scripts/astronomy.sh flag $(FLAG) $(VARIANT)

demo-fault-reset:
	scripts/astronomy.sh flag-reset

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

clean:
	rm -rf bin web/dist web/dist-panel web/playwright-report web/test-results web/coverage coverage.out
	find internal/server/webdist -mindepth 1 ! -name .gitkeep -delete
	find internal/server/mcp/panelapp -mindepth 1 ! -name .gitkeep -delete
