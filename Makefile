.PHONY: test test-race test-cover cover-check cover-report lint test-integration sdk-smoke distillation-e2e install-lint \
        act-list act-preflight act-all act-job act-lint act-test act-vuln act-trivy act-integration-smoke \
        env infra-deps-up infra-up infra-down infra-down-v infra-restart infra-ps infra-logs infra-wait-db \
        infra-wait infra-smoke up down test-all-local test-all-local-quick test-all-local-host deploy-structural-test \
        helm-lint helm-template helm-package

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
infra-deps-up: env
	@echo "[infra] starting postgres + redis…"
	$(DOCKER_COMPOSE) up -d postgres redis
	@bash scripts/infra_wait.sh

# Full stack: postgres, redis, API (:8000, :50051), worker (:8081).
infra-up: env
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

# Live gRPC + DB tests. Start API/worker and Postgres/Redis first (see scripts/ci_integration_smoke.sh).
test-integration:
	GRPC_HOST=$(GRPC_HOST) GRPC_TEST_API_KEY=$(GRPC_TEST_API_KEY) DATABASE_URL=$(DATABASE_URL) \
		go test -tags=integration -count=1 ./internal/grpc/...

# HTTP SDK smoke (Python + TypeScript). Requires API on :8000 (see scripts/ci_integration_smoke.sh).
sdk-smoke:
	PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=$(GRPC_TEST_API_KEY) \
		bash scripts/ci_sdk_smoke.sh

# Full distillation pipeline e2e (Docker + OpenAI). Local only; artifacts in .pcmi_test_out/.
distillation-e2e:
	bash scripts/run_pcmi_distillation_test.sh

# GitHub Actions: add CI_start to the commit message to run the remote pipeline
# (e.g. git commit -m "fix: foo CI_start"). Without it, only the ci-gate job runs.
# Manual run: gh workflow run CI

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

# Free host ports 5432 / 6379 before `act push`. GitHub Actions defines
# integration-smoke service containers on those ports; nektos/act still
# provisions them before running steps — even when ACT=true skips the real
# work. A prior `make act-integration-smoke` (or `docker compose up`) leaves
# pcmi-postgres/redis bound and act fails with "port is already allocated".
# Opt out: SKIP_ACT_PORT_CLEANUP=1 make act-all
act-preflight:
	@if [ "$${SKIP_ACT_PORT_CLEANUP:-}" = "1" ]; then \
	  echo "[act-preflight] SKIP_ACT_PORT_CLEANUP=1 — leaving compose as-is"; \
	else \
	  echo "[act-preflight] docker compose down — freeing :5432 / :6379 for act service containers"; \
	  docker compose down --remove-orphans 2>/dev/null || true; \
	fi

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
act-job:
	@if [ -z "$(JOB)" ]; then \
	  echo "Usage: make act-job JOB=<job-name>"; \
	  echo "Available jobs:"; \
	  act --list; \
	  exit 1; \
	fi
	act -j $(JOB)

# Shortcuts for the most useful individual jobs.
act-lint:
	act -j golangci-lint

act-test:
	act -j go

act-vuln:
	act -j security

# `aquasecurity/trivy-action` is hard to run under act because the composite
# action installs the trivy binary via cache restore that act doesn't fully
# emulate. Locally we get an identical scan by running the upstream image
# directly — same severity, same exit code, same flags as the workflow.
act-trivy:
	@echo "[act-trivy] building images…"
	docker build -f Dockerfile.api    -t pcmi-api:ci    .
	docker build -f Dockerfile.worker -t pcmi-worker:ci .
	@for img in pcmi-api:ci pcmi-worker:ci; do \
	  echo "[act-trivy] scanning $$img"; \
	  docker run --rm \
	    -v /var/run/docker.sock:/var/run/docker.sock \
	    aquasec/trivy:latest image \
	    --severity HIGH,CRITICAL \
	    --exit-code 1 \
	    --ignore-unfixed \
	    --pkg-types os,library \
	    --scanners vuln \
	    "$$img" || exit 1; \
	done

# Full integration smoke outside act — same scripts as CI, but Postgres/Redis via
# docker compose on your machine (see scripts/act_integration_smoke_host.sh).
act-integration-smoke:
	bash scripts/act_integration_smoke_host.sh

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
