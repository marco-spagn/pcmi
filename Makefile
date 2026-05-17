.PHONY: test test-race lint test-integration sdk-smoke distillation-e2e install-lint \
        act-list act-all act-job act-lint act-test act-vuln act-trivy

GOLANGCI_LINT_VERSION ?= v2.1.6
GRPC_HOST ?= localhost:50051
GRPC_TEST_API_KEY ?= testkey123
DATABASE_URL ?= postgres://pcmi:pcmi@localhost:5432/pcmi?sslmode=disable

# Unit tests (default; integration tests use build tag "integration").
test:
	go test ./...

# Race detector + coverage across the packages CI also runs.
test-race:
	go test -race -count=1 \
	  ./internal/config/... \
	  ./internal/deploy/... \
	  ./internal/event/... \
	  ./internal/eventschema/... \
	  ./internal/handler/... \
	  ./internal/metrics/... \
	  ./internal/middleware/... \
	  ./internal/repository/... \
	  ./internal/service/... \
	  ./internal/version/... \
	  ./internal/webhook/... \
	  ./internal/worker/...

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

# Run the full pipeline locally (all jobs, in dependency order).
# WARNING: ~15-20 minutes; downloads ~2 GB on first run.
act-all:
	act push

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

act-trivy:
	act -j trivy-images
