package pcmi

import (
	"context"
	"net/url"
	"strconv"
)

// ListTenants returns admin tenants (GET /v1/admin/tenants).
func (c *Client) ListTenants(ctx context.Context, limit int, cursor string) (map[string]any, error) {
	q := url.Values{"limit": {strconv.Itoa(limit)}}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	var out map[string]any
	if err := c.doJSON(ctx, "GET", "/v1/admin/tenants?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateTenant creates a tenant (POST /v1/admin/tenants).
func (c *Client) CreateTenant(ctx context.Context, slug, name string, settings map[string]any) (map[string]any, error) {
	if settings == nil {
		settings = map[string]any{}
	}
	body := map[string]any{"slug": slug, "name": name, "settings": settings}
	var out map[string]any
	if err := c.doJSON(ctx, "POST", "/v1/admin/tenants", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListAPIKeys lists API keys (GET /v1/admin/api-keys).
func (c *Client) ListAPIKeys(ctx context.Context, tenantID string, limit int, cursor string) (map[string]any, error) {
	q := url.Values{"limit": {strconv.Itoa(limit)}}
	if tenantID != "" {
		q.Set("tenant_id", tenantID)
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	var out map[string]any
	if err := c.doJSON(ctx, "GET", "/v1/admin/api-keys?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateAPIKey creates an API key (POST /v1/admin/api-keys).
func (c *Client) CreateAPIKey(ctx context.Context, name, tenantID, role, expiresAt string) (map[string]any, error) {
	if role == "" {
		role = "user"
	}
	body := map[string]any{"name": name, "role": role}
	if tenantID != "" {
		body["tenant_id"] = tenantID
	}
	if expiresAt != "" {
		body["expires_at"] = expiresAt
	}
	var out map[string]any
	if err := c.doJSON(ctx, "POST", "/v1/admin/api-keys", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RotateAPIKey rotates an API key (POST /v1/admin/api-keys/{id}/rotate).
func (c *Client) RotateAPIKey(ctx context.Context, keyID, name string) (map[string]any, error) {
	path := "/v1/admin/api-keys/" + url.PathEscape(keyID) + "/rotate"
	var out map[string]any
	if err := c.doJSON(ctx, "POST", path, map[string]any{"name": name}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListAudit lists audit log entries (GET /v1/audit).
func (c *Client) ListAudit(ctx context.Context, limit, offset int, since string) (map[string]any, error) {
	q := url.Values{
		"limit":  {strconv.Itoa(limit)},
		"offset": {strconv.Itoa(offset)},
	}
	if since != "" {
		q.Set("since", since)
	}
	var out map[string]any
	if err := c.doJSON(ctx, "GET", "/v1/audit?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// VerifyAudit recomputes the tenant's tamper-evident audit hash chain
// (GET /v1/audit/verify). The result has "valid", "checked", "head_seq",
// "head_hash" and, when the chain is broken, "first_break".
func (c *Client) VerifyAudit(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := c.doJSON(ctx, "GET", "/v1/audit/verify", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ExportAudit downloads a sealed JSONL export of the audit chain
// (GET /v1/audit/export, admin role). Zero values use the server defaults
// (from the first row, up to the head, 10000 rows). Verify the bytes offline
// with `pcmi audit verify-export`.
func (c *Client) ExportAudit(ctx context.Context, fromSeq, toSeq int64, limit int) ([]byte, error) {
	q := url.Values{}
	if fromSeq > 0 {
		q.Set("from_seq", strconv.FormatInt(fromSeq, 10))
	}
	if toSeq > 0 {
		q.Set("to_seq", strconv.FormatInt(toSeq, 10))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := "/v1/audit/export"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return c.doRaw(ctx, "GET", path, nil)
}

// RetentionPolicy is the body of PutRetentionPolicy. PathPrefix "" addresses
// the tenant-wide policy; nil rules are cleared.
type RetentionPolicy struct {
	PathPrefix              string `json:"path_prefix"`
	SupersededRetentionDays *int   `json:"superseded_retention_days"`
	MaxAgeDays              *int   `json:"max_age_days"`
	Description             string `json:"description"`
}

// ListRetentionPolicies lists namespace retention policies (GET /v1/retention-policies).
func (c *Client) ListRetentionPolicies(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := c.doJSON(ctx, "GET", "/v1/retention-policies", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// PutRetentionPolicy creates or replaces a retention policy (admin).
func (c *Client) PutRetentionPolicy(ctx context.Context, p RetentionPolicy) (map[string]any, error) {
	var out map[string]any
	if err := c.doJSON(ctx, "PUT", "/v1/retention-policies", p, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteRetentionPolicy deletes the policy for pathPrefix (admin).
func (c *Client) DeleteRetentionPolicy(ctx context.Context, pathPrefix string) error {
	q := url.Values{"path_prefix": {pathPrefix}}
	return c.doJSON(ctx, "DELETE", "/v1/retention-policies?"+q.Encode(), nil, nil)
}

// EraseMemories erases every version of the memories under pathPrefix
// (POST /v1/memories/erase, admin, GDPR). Call with dryRun first.
func (c *Client) EraseMemories(ctx context.Context, pathPrefix string, dryRun bool, reason string) (map[string]any, error) {
	var out map[string]any
	body := map[string]any{"path_prefix": pathPrefix, "dry_run": dryRun, "reason": reason}
	if err := c.doJSON(ctx, "POST", "/v1/memories/erase", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// UsageStats returns LLM / embedding token usage and estimated cost
// (GET /v1/stats/usage). from / to are YYYY-MM-DD (UTC, "" = default window);
// groupBy is a comma list of day, operation, model — or "none".
func (c *Client) UsageStats(ctx context.Context, from, to, groupBy string) (map[string]any, error) {
	q := url.Values{}
	if from != "" {
		q.Set("from", from)
	}
	if to != "" {
		q.Set("to", to)
	}
	if groupBy != "" {
		q.Set("group_by", groupBy)
	}
	path := "/v1/stats/usage"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out map[string]any
	if err := c.doJSON(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
