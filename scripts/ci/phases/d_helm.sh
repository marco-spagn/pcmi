#!/usr/bin/env bash
# Phase D — helm lint + kubeconform (CI job: helm-lint). Skipped when SKIP_HELM=1.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT"

if [[ "${SKIP_HELM:-}" == "1" ]]; then
  echo "[ci] SKIP Helm/kubeconform (SKIP_HELM=1)"
  exit 0
fi

HELM_IMAGE="${HELM_IMAGE:-alpine/helm:3.14.4}"
KUBECONFORM_IMAGE="${KUBECONFORM_IMAGE:-ghcr.io/yannh/kubeconform:v0.6.7}"
RENDERED="/tmp/pcmi-ci-rendered-$$-$RANDOM.yaml"

run_helm() {
  if command -v helm >/dev/null 2>&1; then
    helm "$@"
  else
    docker run --rm -v "$ROOT:/work" -w /work "$HELM_IMAGE" "$@"
  fi
}

run_helm lint deploy/helm/pcmi --strict
run_helm template pcmi deploy/helm/pcmi >"$RENDERED"
if [[ ! -s "$RENDERED" ]]; then
  echo "[ci] helm template produced empty file" >&2
  exit 1
fi

if command -v kubeconform >/dev/null 2>&1; then
  kubeconform -summary -strict -schema-location default "$RENDERED"
elif docker info >/dev/null 2>&1; then
  docker run --rm -v /tmp:/tmp:ro "$KUBECONFORM_IMAGE" -summary -strict -schema-location default "$RENDERED"
else
  echo "[ci] SKIP kubeconform — install kubeconform or start Docker" >&2
  exit 1
fi
rm -f "$RENDERED"
