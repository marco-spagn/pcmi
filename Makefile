.PHONY: test lint test-integration sdk-smoke distillation-e2e install-lint

GOLANGCI_LINT_VERSION ?= v2.1.6
GRPC_HOST ?= localhost:50051
GRPC_TEST_API_KEY ?= testkey123
DATABASE_URL ?= postgres://pcmi:pcmi@localhost:5432/pcmi?sslmode=disable

# Unit tests (default; integration tests use build tag "integration").
test:
	go test ./...

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
