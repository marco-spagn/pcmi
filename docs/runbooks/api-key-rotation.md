## Runbook: API Key Rotation

### Symptoms
- Suspected key compromise
- Scheduled rotation (e.g. every 90 days for production keys)
- Team member or service account is being decommissioned
- You want to follow least-privilege / key hygiene best practices

### Prerequisites
- An admin API key (`role: admin`)
- Access to the PCMI Admin UI (`GET /v1/admin/ui`) or the Admin gRPC/HTTP endpoints
- `make admin-list-keys` (or direct SQL) to see existing keys (only hash prefixes are shown)

### Safe Rotation Procedure

1. **Create the replacement key**
   - Use the Admin API or UI to create a new key with the **same role** as the one being rotated.
   - Give it a clear name (e.g. `prod-agent-rotation-2026-05`).
   - Record the **full secret immediately** — it is only shown once.

2. **Distribute the new key**
   - Update all clients, agents, CI jobs, scripts, and SDK initializations.
   - Prefer configuration management / secrets manager over hard-coded values.
   - Perform a rolling update where possible (new instances use new key first).

3. **Verify the new key works**
   ```bash
   curl -s -H "X-API-Key: $NEW_KEY" "$PCMI_BASE_URL/v1/health"
   curl -s -H "X-API-Key: $NEW_KEY" -X POST "$PCMI_BASE_URL/v1/memories" \
     -d '{"path":"rotation.test","content":"key rotation verification"}'
   ```

4. **Revoke or expire the old key**
   - Preferred: Use the **revoke** endpoint (immediate invalidation).
   - Alternative (lower risk): Set a short `expires_at` on the old key and let natural expiry happen after a grace period.
   - Monitor for a few minutes that no more traffic is using the old key (via audit logs or metrics).

5. **Clean up**
   - Remove the old key from any remaining local `.env` files, password managers, or old scripts.
   - Update any runbooks or onboarding docs that referenced the old key name.

### Rollback
- If something breaks after revocation, you can create yet another new key and repeat the process.
- Old keys that were only expired (not revoked) can be re-enabled via the admin interface if needed.

### Related
- [Key lifecycle documentation](../USAGE.md#api-key-lifecycle)
- `make test-key-lifecycle`
- `make admin-list-keys`
- Admin UI at `/v1/admin/ui`

### Prevention
- Use short-lived keys + automated rotation where possible.
- Never share admin keys.
- Monitor the `audit_log` for unusual key usage patterns.
