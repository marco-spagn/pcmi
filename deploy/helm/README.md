# PCMI Helm chart

Single packaged deployment for the PCMI API + Worker. Replaces the dual
`k8s/` and `deploy/k8s/` trees (PR #4 removes the duplicate `k8s/` directory
in the repo root; `deploy/k8s/` remains as the static reference manifests
for users who don't want Helm).

## Quick install

```bash
helm install pcmi ./deploy/helm/pcmi \
  --namespace pcmi --create-namespace \
  --set-string secrets.DATABASE_URL="postgres://pcmi:pcmi@pcmi-postgres:5432/pcmi" \
  --set-string secrets.ADMIN_API_KEY="$(openssl rand -hex 16)" \
  --set-string secrets.PCMI_ENCRYPTION_KEY="$(openssl rand -base64 32)"
```

## Production override

```bash
cp deploy/helm/pcmi/values-prod.yaml.example deploy/helm/pcmi/values-prod.yaml
# fill in image refs + secrets via your secrets operator
helm upgrade --install pcmi ./deploy/helm/pcmi \
  -f deploy/helm/pcmi/values-prod.yaml \
  --namespace pcmi --create-namespace
```

## Local validation

Both targets are runnable without a cluster:

```bash
make helm-lint        # `helm lint` over the chart
make helm-template    # render templates to stdout
```

The Helm chart structure is also verified by
`internal/deploy/deploy_test.go:TestHelmChartStructure`, which runs in CI
alongside the existing Grafana / Prometheus alert tests.

## Layout

| File | Purpose |
|---|---|
| `Chart.yaml` | metadata; `appVersion` mirrors `internal/version/version.go` |
| `values.yaml` | defaults for every tunable (image, replicas, resources, HPA, PDB, config, secrets, TLS, OTel) |
| `values-prod.yaml.example` | starting point for production overrides |
| `templates/_helpers.tpl` | standard Helm name/label helpers |
| `templates/configmap.yaml` | non-secret PCMI runtime config |
| `templates/secret.yaml` | secrets (DB URL, OPENAI key, encryption key, ...) |
| `templates/api-deployment.yaml` | API Deployment + checksum-driven rolling restart on config/secret change |
| `templates/worker-deployment.yaml` | Worker Deployment |
| `templates/service.yaml` | Service exposing both HTTP (:8000) and gRPC (:50051) |
| `templates/hpa.yaml` | optional API HPA |
| `templates/pdb.yaml` | API PodDisruptionBudget |

## TLS (PR #3 wiring)

When `tls.enabled=true`, the chart mounts `tls.secretName` (a standard
`kubernetes.io/tls` Secret) at `/etc/pcmi/tls` and the rendered Secret sets
`PCMI_TLS_CERT` / `PCMI_TLS_KEY` accordingly. `config.Load()` then turns on
TLS for both the Fiber HTTP server and the gRPC server.

## Migrating from `deploy/k8s/`

Static manifests under `deploy/k8s/` map 1:1 onto chart templates. The only
behavioural delta is the automatic rolling-restart annotation
(`checksum/config`, `checksum/secret`) — when you edit values.yaml,
`helm upgrade` recreates the pods with the new env without needing
`kubectl rollout restart`.

If you were previously using `k8s/` (without the `deploy/` prefix), please
switch to either the Helm chart or `deploy/k8s/`. PR #4 removes the
ambiguous duplicate.
