# ─────────────────────────────────────────────────────────────────────────────
# PCMI — combined image (release / ghcr.io)
#
# Builds an image containing BOTH pcmi-api and pcmi-worker binaries. Used by
# the release workflow (.github/workflows/release.yml) to publish the single
# ghcr.io/marco-spagn/pcmi container on every git tag and main push.
#
# Multi-arch: linux/amd64, linux/arm64.
#
# For local development with docker compose, prefer the dedicated images:
#   - Dockerfile.api      → minimal API image  (docker-compose.yml)
#   - Dockerfile.worker   → minimal Worker image (docker-compose.yml)
#
# Benefits of the split:
#   - smaller attack surface per node
#   - more accurate vulnerability scans (trivy/govulncheck)
#   - cannot be misused to expose the wrong process
#
# This file will be removed in a future release. Update your deploy now.
# ─────────────────────────────────────────────────────────────────────────────

FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w" -o /pcmi-api     ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w" -o /pcmi-worker  ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w" -o /pcmi-migrate ./cmd/migrate

FROM alpine:3.21
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app

COPY --from=builder /pcmi-api     .
COPY --from=builder /pcmi-worker  .
COPY --from=builder /pcmi-migrate .
COPY migrations/ migrations/

EXPOSE 8000 50051
CMD ["./pcmi-api"]
