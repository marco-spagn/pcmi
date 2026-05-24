.PHONY: test test-race test-cover cover-check cover-report lint test-integration test-integration-bufconn test-integration-live test-integration-handler test-integration-all test-streams-integration test-circuit-breaker test-ratelimit-integration test-idempotency test-dedup test-distillation-policy test-key-lifecycle test-retrieval-scoring test-sessions-integration test-pagination bench-retrieval sdk-smoke sdk-go-test sdk-go-smoke sdk-all distillation-e2e distillation-policy-e2e synth-generate synth-list install-lint ci-like-github test-all test-full-real \
        build-mcp install-mcp test-mcp-unit test-mcp-smoke mcp-e2e smoke-sessions smoke-dedup \
        act-list free-dev-ports act-preflight act-all act-job act-lint act-test act-vuln act-trivy act-integration-smoke \
        env infra-deps-up infra-up infra-down infra-down-v infra-restart infra-ps infra-logs infra-wait-db \
        infra-wait infra-smoke smoke-importance up down test-all-local test-all-local-quick test-all-local-host deploy-structural-test \
        changelog-unreleased changelog-tag examples-smoke-structural examples-smoke \
        helm-lint helm-template helm-package admin-list-keys

GOLANGCI_LINT_VERSION ?= v2.1.6
GRPC_HOST ?= localhost:50051
GRPC_TEST_API_KEY ?= testkey123
DATABASE_URL ?= postgres://pcmi:pcmi@localhost:5432/pcmi?sslmode=disable
DOCKER_COMPOSE ?= docker compose
API_URL ?= http://localhost:8000

# Coverage thresholds. Keep these in sync with .github/workflows/ci.yml and
# scripts/ci_coverage_check.sh. Tighten as new tests land.
COVERAGE_MIN_TOTAL  ?= 22
COVERAGE_PKG_FLOORS ?= config:70,event:70,eventschema:85,metrics:70

# Set of packages that the coverage gate considers. Integration-heavy packages
# (cmd/* binaries) are intentionally excluded because they cannot run without a
# database/Redis/OpenAI key — they have their own e2e jobs.
#
# Pure-logic packages (crypto, embedding, model, telemetry) already ship unit
# tests but were previously omitted from the gate; they are included here to
# give the coverage signal a more honest denominator.
COVERAGE_PKGS = \
	./internal/config/... \
	./internal/crypto/... \
	./internal/deploy/... \
	./internal/embedding/... \
	./internal/event/... \
	./internal/eventschema/... \
	./internal/handler/... \
	./internal/metrics/... \
	./internal/middleware/... \
	./internal/model/... \
	./internal/repository/... \
	./internal/service/... \
	./internal/telemetry/... \
	./internal/ratelimit/... \
	./internal/version/... \
	./internal/webhook/... \
	./internal/worker/...

# ───────────────────────────────────────────────────────────────────────────────
# Local infrastructure (Docker Compose)
#
# Quick start (API on :8000, gRPC on :50051, worker health on :8081):
#   make infra-up      # postgres + redis + api + worker (build if needed)
#   make infra-smoke   # curl /v1/ready and /health
#   make infra-down    # stop containers
#
# Only DB + Redis (run API/worker with `go run` on the host):
#   make infra-deps-up
#   export DATABASE_URL REDIS_ADDR  # see .env.example
#   go run ./cmd/api
# ───────────────────────────────────────────────────────────────────────────────

# Create .env from .env.example when missing.
env:
	@test -f .env || (cp .env.example .env && echo "[infra] created .env from .env.example — set OPENAI_API_KEY if you need LLM features")

# Postgres + Redis only.
infra-deps-up: env free-dev-ports
	@echo "[infra] starting postgres + redis…"
	$(DOCKER_COMPOSE) up -d postgres redis
	@bash scripts/infra_wait.sh

# Full stack: postgres, redis, API (:8000, :50051), worker (:8081).
infra-up: env free-dev-ports
	@echo "[infra] starting postgres, redis, api, worker (build if needed)…"
	$(DOCKER_COMPOSE) up -d --build --remove-orphans postgres redis api worker
	@bash scripts/infra_wait.sh $(API_URL)/v1/ready

infra-down:
	@echo "[infra] stopping compose stack…"
	$(DOCKER_COMPOSE) down --remove-orphans
	@docker rm -f pcmi-api pcmi-worker 2>/dev/null || true

# Stop stack and delete Postgres volume (destructive — fresh DB on next up).
infra-down-v:
	@echo "[infra] stopping compose and removing volumes…"
	$(DOCKER_COMPOSE) down -v --remove-orphans

infra-restart: infra-down infra-up

infra-ps:
	$(DOCKER_COMPOSE) ps

infra-logs:
	$(DOCKER_COMPOSE) logs -f --tail=100

infra-wait-db:
	@bash scripts/infra_wait.sh

infra-wait:
	@bash scripts/infra_wait.sh $(API_URL)/v1/ready

# Manual smoke checks (requires API listening on :8000).
infra-smoke:
	@echo "=== GET $(API_URL)/v1/ready ==="
	@curl -sS "$(API_URL)/v1/ready" | jq .
	@echo "=== GET $(API_URL)/health ==="
	@curl -sS "$(API_URL)/health" | jq .

# PCMI-009: importance ranking + PATCH importance (see scripts/smoke_importance_retrieve.sh).
smoke-importance:
	@chmod +x scripts/smoke_importance_retrieve.sh
	@PCMI_BASE_URL=$(API_URL) ./scripts/smoke_importance_retrieve.sh

# PCMI-010: agent sessions curl E2E (see scripts/smoke_sessions.sh).
smoke-sessions:
	@chmod +x scripts/smoke_sessions.sh
	@PCMI_BASE_URL=$(API_URL) ./scripts/smoke_sessions.sh

# PCMI-011: content-hash dedup curl E2E (see scripts/smoke_dedup.sh).
smoke-dedup:
	@chmod +x scripts/smoke_dedup.sh
	@PCMI_BASE_URL=$(API_URL) ./scripts/smoke_dedup.sh

# List tenants and API keys from Postgres (dev/ops; no raw secrets in output).
admin-list-keys:
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/pcmi-admin list

# Shortcuts
up: infra-up
down: infra-down

# Complete local test suite (see scripts/test_all_local.sh --help).
test-all-local:
	@chmod +x scripts/test_all_local.sh scripts/infra_wait.sh
	@./scripts/test_all_local.sh

test-all-local-quick:
	@chmod +x scripts/test_all_local.sh
	@./scripts/test_all_local.sh --quick

test-all-local-host:
	@chmod +x scripts/test_all_local.sh scripts/infra_wait.sh
	@./scripts/test_all_local.sh --with-host

# Aliases (wrappers → test_all_local.sh)
verify-branch: test-all-local-quick
verify-branch-full: test-all-local
test-branch-manual: test-all-local

# Workflows, compose, openapi, Helm chart YAML/JSON, scripts bash -n, proto markers (no cluster).
deploy-structural-test:
	go test -race -count=1 ./internal/deploy/...

# AI framework examples (PCMI-016): structural smoke only (no pip / no API).
EXAMPLES_AI_DIRS = langchain llamaindex autogen crewai

examples-smoke-structural:
	@for dir in $(EXAMPLES_AI_DIRS); do \
		echo "[examples] $$dir structural"; \
		cd examples/$$dir && python3 smoke_test.py && cd ../..; \
	done

# Live smoke: requires infra-up, pip install per example, PCMI_API_KEY.
examples-smoke: infra-up
	@for dir in $(EXAMPLES_AI_DIRS); do \
		echo "[examples] $$dir"; \
		cd examples/$$dir && python3 -m pip install -q -r requirements.txt && \
			PCMI_BASE_URL=$(API_URL) PCMI_API_KEY=$(GRPC_TEST_API_KEY) PCMI_SMOKE_LIVE=1 python3 smoke_test.py && \
			cd ../..; \
	done
	$(MAKE) infra-down

# Release notes via git-cliff (see cliff.toml, docs/API-VERSIONING.md). Requires git-cliff on PATH.
changelog-unreleased:
	@command -v git-cliff >/dev/null 2>&1 || (echo "install git-cliff: https://git-cliff.org/docs/installation/" && exit 1)
	git cliff --config cliff.toml --unreleased

changelog-tag:
	@test -n "$(TAG)" || (echo "usage: make changelog-tag TAG=v1.48.0" && exit 1)
	@command -v git-cliff >/dev/null 2>&1 || (echo "install git-cliff: https://git-cliff.org/docs/installation/" && exit 1)
	git cliff --config cliff.toml --strip header --tag "$(TAG)"

# Unit tests (default; integration tests use build tag "integration").
test:
	go test ./...

# Race detector + coverage across the packages CI also runs.
test-race:
	go test -race -count=1 $(COVERAGE_PKGS)

# Race + coverage profile (writes coverage.out). Used by the coverage gate.
test-cover:
	go test -race -count=1 \
	  -coverprofile=coverage.out \
	  -covermode=atomic \
	  $(COVERAGE_PKGS)

# Run the coverage threshold script against the existing coverage.out.
# Override thresholds via env: COVERAGE_MIN_TOTAL=30 make cover-check
cover-check:
	COVERAGE_MIN_TOTAL=$(COVERAGE_MIN_TOTAL) \
	COVERAGE_PKG_FLOORS=$(COVERAGE_PKG_FLOORS) \
	  bash scripts/ci_coverage_check.sh

# Render a human-readable per-function report on top of coverage.out.
cover-report:
	@test -f coverage.out || (echo "coverage.out missing — run 'make test-cover' first" && exit 1)
	go tool cover -func=coverage.out | tail -n 30

# Requires golangci-lint v2 (see install-lint). Config: .golangci.yml with version: "2".
lint: install-lint
	@command -v golangci-lint >/dev/null 2>&1 || $(MAKE) install-lint
	golangci-lint run ./...

install-lint:
	@command -v golangci-lint >/dev/null 2>&1 || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

# gRPC integration (-tags=integration), split like CI:
#
#   bufconn — in-process server + miniredis; needs Postgres with migrations only (no TCP gRPC).
#   live    — dials GRPC_HOST; needs pcmi-api running (docker compose or ./bin/pcmi-api).
#
# Full suite: make test-integration (bufconn first, then live + resolve-tenant).
test-integration-bufconn:
	DATABASE_URL=$(DATABASE_URL) go test -tags=integration -count=1 ./internal/grpc -run '^TestIntegrationBufconn_'

test-integration-live:
	DATABASE_URL=$(DATABASE_URL) GRPC_HOST=$(GRPC_HOST) GRPC_TEST_API_KEY=$(GRPC_TEST_API_KEY) \
		go test -tags=integration -count=1 ./internal/grpc -run '^TestGRPC|^TestResolveTenantIntegration$$'

# HTTP handler integration tests (Postgres + miniredis). Skips flaky SSE httptest by default — see docs/integration-testing.md.
test-integration-handler:
	PCMI_SKIP_SSE_HTTPTEST=1 DATABASE_URL=$(DATABASE_URL) \
		go test -tags=integration -count=1 ./internal/handler/...

test-integration:
	@$(MAKE) test-integration-bufconn
	@$(MAKE) test-integration-live

# Redis Streams durable bus (//go:build integration).
test-streams-integration:
	go test -tags=integration -run TestStream ./internal/event/...

test-circuit-breaker:
	go test -race -count=1 -run 'TestCircuitBreaker|TestOpenAIProvider_Wrapped|TestEmbeddingWorker_' ./internal/embedding/... ./internal/worker/...

test-idempotency:
	go test -race -count=1 -run TestIdempotency ./internal/middleware/...
	go test -race -count=1 -run TestIdempotency ./internal/repository/...

test-key-lifecycle:
	PCMI_SKIP_SSE_HTTPTEST=1 go test -tags=integration -race -count=1 -run 'TestKey' ./internal/handler/...

# Cursor-based pagination on list endpoints (PCMI-014).
test-pagination:
	go test -race -count=1 -run TestPagination ./internal/repository/... ./internal/handler/... ./internal/model/...

# Hybrid retrieval scoring: importance fusion + temporal decay (PCMI-009).
test-retrieval-scoring:
	go test -race -count=1 -run 'TestImportance|TestTemporalDecay|TestAccessCount|TestDecayDisabled|TestImportanceEndpoint|TestHybridScore|TestRecency|TestDefaultScoring' ./internal/repository/...

bench-retrieval:
	go test -bench=BenchmarkHybridScore -benchmem -benchtime=5s ./internal/repository/...

# Agent sessions and working memory (PCMI-010).
test-sessions-integration:
	PCMI_SKIP_SSE_HTTPTEST=1 go test -tags=integration -race -count=1 -run 'TestSession' ./internal/handler/...

# Content-hash deduplication at ingest (PCMI-011).
test-dedup:
	go test -race -count=1 -run 'TestDedup|TestContentHash|TestParseDedupMode|TestNormalizeContentForHash|TestStoreMemoryHandler_Dedup' ./internal/model/... ./internal/service/... ./internal/handler/...

# Automatic distillation policy engine (PCMI-012).
test-distillation-policy:
	go test -race -count=1 -run 'TestDistillationPolicy_|TestDistillationRun_' ./internal/worker/...

# Policy unit tests + quick distillation smoke (PCMI-012 gate companion).
distillation-policy-e2e: test-distillation-policy
	$(MAKE) distill-smoke PRESET=$(PRESET) SYNTH_SEED=$(SYNTH_SEED)

# Distributed Redis rate limiter (miniredis) + middleware probes/roles.
test-ratelimit-integration:
	go test -race -count=1 -run 'TestRedisRateLimiter|TestRateLimitMiddleware_' ./internal/ratelimit/... ./internal/middleware/...

# Historical alias: single go test line (same as bufconn + live + pcmiv1 empty).
test-integration-all:
	DATABASE_URL=$(DATABASE_URL) GRPC_HOST=$(GRPC_HOST) GRPC_TEST_API_KEY=$(GRPC_TEST_API_KEY) \
		go test -tags=integration -count=1 ./internal/grpc/...

# HTTP SDK smoke (Python + TypeScript). Requires API on :8000 (see scripts/ci_integration_smoke.sh).
sdk-smoke:
	PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=$(GRPC_TEST_API_KEY) \
		bash scripts/ci_sdk_smoke.sh

# Go SDK unit tests (httptest; no live API required).
sdk-go-test:
	cd sdk/go && go test -race -count=1 ./...

# Go SDK smoke: docker infra + unit tests + examples/basic against live API.
sdk-go-smoke: infra-up sdk-go-test
	cd sdk/go && PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=$(GRPC_TEST_API_KEY) go run ./examples/basic
	$(MAKE) infra-down

# All HTTP SDK checks (Python/TS smoke + Go unit tests).
sdk-all: sdk-smoke sdk-go-test

# Synthetic data presets: soc | finance | advertising | healthcare | custom
PRESET ?= soc
SYNTH_NUM ?= 1000
SYNTH_SEED ?= 42
PYTHON ?= python3

# Generate JSONL only (no Docker). Example: make synth-generate PRESET=finance SYNTH_NUM=200
synth-list:
	PYTHONPATH=scripts $(PYTHON) -m pcmi_synth list

synth-generate:
	PYTHONPATH=scripts $(PYTHON) -m pcmi_synth generate \
		--preset $(PRESET) --num $(SYNTH_NUM) --seed $(SYNTH_SEED) \
		--dry-run --output .pcmi_test_out/$(PRESET)_seed$(SYNTH_SEED)_n$(SYNTH_NUM).jsonl

# Full distillation pipeline e2e (Docker + OpenAI). Local only; artifacts in .pcmi_test_out/.
distillation-e2e:
	PRESET=$(PRESET) bash scripts/run_pcmi_distillation_test.sh \
		--preset $(PRESET) --num $(SYNTH_NUM) --seed $(SYNTH_SEED)

# Quick smoke: 100 records → 10 distilled (preset soc by default)
distill-smoke:
	PRESET=$(PRESET) bash scripts/run_pcmi_distillation_test.sh \
		--preset $(PRESET) --num 100 --seed $(SYNTH_SEED) --no-build

# GitHub Actions CI runs on push/PR; manual run: gh workflow run CI

# ───────────────────────────────────────────────────────────────────────────────
# Local GitHub Actions runner (act) — replaces GitHub-hosted runs.
# Use when you've burned your CI minutes or want fast iteration.
#
# Requires: brew install act + running Docker daemon.
# Defaults are in .actrc (image, arch, --reuse).
# See docs/local-ci.md for the full guide.
# ───────────────────────────────────────────────────────────────────────────────

# List every job the workflow defines.
act-list:
	act --list

# Free host ports 5432 / 6379 (project compose + leftover act service containers).
# Opt out: SKIP_ACT_PORT_CLEANUP=1
free-dev-ports:
	@chmod +x scripts/free_dev_ports.sh
	@FREE_DEV_PORTS_LABEL="[free-dev-ports]" bash scripts/free_dev_ports.sh

act-preflight: free-dev-ports

# Run the full pipeline locally. Inside act: `trivy-images` is a stub + `make act-trivy`;
# `integration-smoke` is a stub + `make act-integration-smoke` (host-side compose + bins).
# WARNING: ~15-25 minutes; downloads ~2 GB on first run.
act-all: act-preflight
	act push
	@echo ""
	@echo "[act-all] running Trivy image scan on host (act cannot run aquasecurity/trivy-action)…"
	@$(MAKE) act-trivy
	@echo ""
	@echo "[act-all] running integration smoke on host (act service networking ≠ GitHub runner)…"
	@$(MAKE) act-integration-smoke

# Run a single job by name. Example: make act-job JOB=integration-smoke
act-job: act-preflight
	@if [ -z "$(JOB)" ]; then \
	  echo "Usage: make act-job JOB=<job-name>"; \
	  echo "Available jobs:"; \
	  act --list; \
	  exit 1; \
	fi
	act -j $(JOB)

# Shortcuts for the most useful individual jobs.
act-lint: act-preflight
	act -j golangci-lint

act-test: act-preflight
	act -j go

act-vuln: act-preflight
	act -j security

# `aquasecurity/trivy-action` is hard to run under act because the composite
# action installs the trivy binary via cache restore that act doesn't fully
# emulate. Locally we get an identical scan by running the upstream image
# directly — same severity, same exit code, same flags as the workflow.
act-trivy:
	@chmod +x scripts/ci/trivy_images.sh
	bash scripts/ci/trivy_images.sh

# Full integration smoke outside act — same scripts as CI, but Postgres/Redis via
# docker compose on your machine (see scripts/act_integration_smoke_host.sh).
act-integration-smoke: act-preflight
	bash scripts/act_integration_smoke_host.sh

# Replica locale della CI GitHub (workflow CI): lint/vuln/helm opzionali,
# go test -race -tags=integration (+ gate coverage) salvo CI_LIKE_NO_RACE=1, poi integration-smoke.
# Tra un pacchetto e l'altro può non esserci output per molti minuti (-race è lento).
# Su laptop: PCMI_GO_TEST_P=1 CI_LIKE_HEARTBEAT_SECS=120 make ci-like-github
#             CI_LIKE_NO_RACE=1 — Phase F senza race (più veloce; CI GitHub usa ancora -race)
#                             oppure CI_LIKE_GO_VERBOSE=1 per log dei singoli test
# Solo smoke HTTP/gRPC/SDK: `make act-integration-smoke` oppure `./scripts/ci_like_github.sh --integration-smoke`
# Alias: one command for full host CI parity (auto-frees :5432 / :6379 first).
test-all: ci-like-github

ci-like-github: act-preflight
	@chmod +x scripts/ci_like_github.sh scripts/free_dev_ports.sh scripts/ci/*.sh scripts/ci/phases/*.sh
	bash scripts/ci_like_github.sh

# Simulazione production-like: CI host + E2E OpenAI (se chiave) + MCP + importance + sessions.
# Vedi docs/local-ci.md § Simulazione completa.
test-full-real:
	@chmod +x scripts/run_full_validation.sh
	bash scripts/run_full_validation.sh

# ───────────────────────────────────────────────────────────────────────────────
# Helm — single packaged Kubernetes deployment (PR #4).
# Requires: helm v3.13+ on PATH. CI installs it via azure/setup-helm.
# ───────────────────────────────────────────────────────────────────────────────

HELM_CHART_DIR ?= deploy/helm/pcmi

# `helm lint` exercises Chart.yaml validity, values.yaml schema (if present),
# and template rendering against the default values. Fails on `--strict`
# warnings (missing required keys, invalid label keys, etc.).
helm-lint:
	helm lint $(HELM_CHART_DIR) --strict

# Render every template to stdout — useful to eyeball before committing.
# Pipe to `| kubectl apply --dry-run=client -f -` for a server-side validation.
helm-template:
	helm template pcmi $(HELM_CHART_DIR)

# Build a redistributable tarball (deploy/helm/pcmi-<version>.tgz).
helm-package:
	helm package $(HELM_CHART_DIR) --destination deploy/helm

# ───────────────────────────────────────────────────────────────────────────────
# MCP server (stdio JSON-RPC) — PR PCMI-008
# ───────────────────────────────────────────────────────────────────────────────

build-mcp:
	go build -o bin/pcmi-mcp ./cmd/mcp

install-mcp:
	go install ./cmd/mcp

test-mcp-unit:
	go test -race -count=1 ./cmd/mcp/...

test-mcp-smoke: build-mcp
	@echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"1.0"}}}' \
	  | PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123 ./bin/pcmi-mcp 2>/dev/null | grep '"result"'

mcp-e2e: infra-up build-mcp test-mcp-smoke test-mcp-unit
	@$(MAKE) infra-down
