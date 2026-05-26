---
name: Bug report
about: Report a bug in PCMI
title: ''
labels: bug
assignees: ''
---

## Summary

<!-- One-line description of the bug -->

## PCMI Version

Run and paste the output:

```bash
curl -s http://localhost:8000/v1/health | jq .version
# or for gRPC: grpcurl -plaintext -H 'X-API-Key: testkey123' localhost:50051 pcmi.v1.HealthService/Health
```

**Version:** `vX.Y.Z` (from `GET /v1/health`)

## Environment

- [ ] Docker Compose (local)
- [ ] Kubernetes / Helm
- [ ] Bare metal / custom deploy
- Go version: `go version`
- PostgreSQL version:
- Redis version:

## Steps to Reproduce

1. 
2. 
3. 

## Expected Behavior

<!-- What you expected to happen -->

## Actual Behavior

<!-- What actually happened (include error messages, stack traces, logs) -->

## Relevant Logs / Output

```text
(paste here)
```

## Additional Context

- Link to related discussion or issue:
- Any recent changes (new migration, config, upgrade, etc.):

## Checklist

- [ ] I have searched existing issues and discussions
- [ ] I have included the PCMI version and environment details
- [ ] I can reproduce the issue with the latest `main` (or a tagged release)
- [ ] I am willing to provide more information if needed
