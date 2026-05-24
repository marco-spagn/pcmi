# ─────────────────────────────────────────────────────────────────────────────
# PCMI — combined image (LEGACY)
#
# This Dockerfile builds an image containing BOTH pcmi-api and pcmi-worker
# binaries. It exists for backward compatibility with older deploy scripts.
#
# For new deployments prefer the dedicated images:
#   - Dockerfile.api      → minimal API image
#   - Dockerfile.worker   → minimal Worker image
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
