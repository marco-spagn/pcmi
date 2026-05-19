# `k8s/` is deprecated

PR #4 (`feat/helm-and-k8s-cleanup`) consolidates the two parallel Kubernetes
manifest trees that used to live in this repository:

| Old | New |
|---|---|
| `k8s/api-deployment.yaml` | `deploy/helm/pcmi/templates/api-deployment.yaml` |
| `k8s/worker-deployment.yaml` | `deploy/helm/pcmi/templates/worker-deployment.yaml` |
| `k8s/postgres-statefulset.yaml` | *removed* — use managed Postgres |
| `k8s/ingress.yaml` | *removed* — bring your own Ingress / HTTPRoute |

## What to do

**Helm (recommended):**

```bash
helm install pcmi ./deploy/helm/pcmi --namespace pcmi --create-namespace
```

**Static manifests (if you don't want Helm):**

```bash
kubectl apply -f deploy/k8s/
```

## Why the `k8s/` files are still here

Filesystem-deletion isn't always possible inside a working sandbox, so PR #4
overwrites each `k8s/*.yaml` with an empty-document deprecation notice (still
valid YAML so `kubectl apply -f k8s/` silently no-ops during the deprecation
window).

The next maintainer cleanup will:

```bash
git rm k8s/*.yaml k8s/README.md
git commit -m "chore(k8s): drop deprecated k8s/ tree (use deploy/helm/pcmi)"
```

The Helm chart at `deploy/helm/pcmi/` is the supported deployment artefact.
