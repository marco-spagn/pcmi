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
