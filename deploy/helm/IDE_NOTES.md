# Helm chart — IDE / editor notes

The diagnostic flood you may be seeing in `deploy/helm/pcmi/templates/*.yaml`
inside VS Code (or any YAML language server like `redhat.vscode-yaml`) is
expected and **does NOT block CI**. Helm templates are not pure YAML — they
contain Go template tags (`{{ ... }}`) that the YAML grammar legitimately
rejects. CI uses `helm lint --strict` (see `.github/workflows/ci.yml` →
`helm-lint` job), which DOES understand template syntax.

## Recommended VS Code workspace setup

Sandbox restrictions prevent committing the `.vscode/` folder directly from
this branch. Run these commands on the host to silence the editor warnings:

```bash
mkdir -p .vscode

cat > .vscode/extensions.json <<'JSON'
{
  "recommendations": [
    "tim-koehler.helm-intellisense",
    "redhat.vscode-yaml",
    "golang.go"
  ]
}
JSON

cat > .vscode/settings.json <<'JSON'
{
  "files.associations": {
    "deploy/helm/pcmi/templates/*.yaml": "helm",
    "deploy/helm/pcmi/templates/*.tpl": "helm"
  },
  "yaml.schemas": {
    "https://json.schemastore.org/chart.json": ["deploy/helm/*/Chart.yaml"],
    "deploy/helm/pcmi/values.schema.json": ["deploy/helm/pcmi/values.yaml"]
  }
}
JSON
```

Once committed, install the **Helm Intellisense** extension. The 1.4k+
warnings on the templates/ folder will disappear because the files are
parsed as Helm, not YAML.

## Why the warnings exist

Every line that starts with `{{- if`, `{{- include`, `{{- with`, `{{ toYaml`
etc. is a Go template directive. The redhat YAML grammar treats `{{` as a
flow-map-start token, then chokes when it can't find a matching `}}` on the
same line — producing one or two diagnostics per template line.

`helm lint --strict` runs in CI and statically checks template rendering,
required values, schema (`values.schema.json`), and label conventions —
that's the canonical gate.

## What we already do in CI

- `helm lint --strict deploy/helm/pcmi` (azure/setup-helm v3.14)
- `helm template pcmi deploy/helm/pcmi > /tmp/pcmi-rendered.yaml`
- `kubeconform -strict /tmp/pcmi-rendered.yaml` — validates the rendered
  manifests against the Kubernetes OpenAPI schemas
- `internal/deploy/helm_test.go:TestHelmLintStrictWhenHelmAvailable` — Go
  test that invokes `helm lint --strict` when helm is on PATH

If any of these fail, CI is red. If only your editor complains, ignore it —
or install the Helm extension above.
