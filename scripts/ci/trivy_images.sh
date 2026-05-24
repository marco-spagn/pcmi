#!/usr/bin/env bash
# Trivy scan for API + worker images (same policy as CI job trivy-images / make act-trivy).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

docker build -f Dockerfile.api -t pcmi-api:ci .
docker build -f Dockerfile.worker -t pcmi-worker:ci .

for img in pcmi-api:ci pcmi-worker:ci; do
  echo "[trivy] scanning $img"
  docker run --rm \
    -v /var/run/docker.sock:/var/run/docker.sock \
    aquasec/trivy:latest image \
    --severity HIGH,CRITICAL \
    --exit-code 1 \
    --ignore-unfixed \
    --pkg-types os,library \
    --scanners vuln \
    --format table \
    "$img"
done
