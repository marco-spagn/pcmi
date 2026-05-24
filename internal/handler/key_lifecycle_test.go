//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

func createAdminAPIKey(t *testing.T, app *fiber.App, tenantID, name, role string) (id, raw string) {
	t.Helper()
	body := fmt.Sprintf(`{"tenant_id":%q,"name":%q,"role":%q}`, tenantID, name, role)
	resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/admin/api-keys", body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create key %d: %s", resp.StatusCode, b)
	}
	var out struct {
		ID     string `json:"id"`
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.ID, out.APIKey
}

func authedGET(t *testing.T, app *fiber.App, apiKey, path string) *http.Response {
	t.Helper()
	req := reqAuthed(t, http.MethodGet, path, "")
	req.Header.Set("X-API-Key", apiKey)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func setKeyGraceEnds(t *testing.T, pool *pgxpool.Pool, keyID string, ends time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`UPDATE api_keys SET rotation_grace_ends_at = $2 WHERE id = $1::uuid`, keyID, ends)
	if err != nil {
		t.Fatal(err)
	}
}

func setKeyExpiresAt(t *testing.T, pool *pgxpool.Pool, keyID string, expires time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`UPDATE api_keys SET expires_at = $2 WHERE id = $1::uuid`, keyID, expires)
	if err != nil {
		t.Fatal(err)
	}
}

func TestKeyRotation_CreatesNewKey(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	tid := "00000000-0000-0000-0000-000000000000"
	oldID, oldRaw := createAdminAPIKey(t, app, tid, "rotate-new-"+time.Now().Format("150405"), "user")

	rotURL := "/v1/admin/api-keys/" + url.PathEscape(oldID) + "/rotate"
	resp, err := app.Test(reqAuthed(t, http.MethodPost, rotURL, `{"name":"rotated-name"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("rotate %d: %s", resp.StatusCode, b)
	}
	var rotated struct {
		ID              string `json:"id"`
		APIKey          string `json:"api_key"`
		PreviousKeyID   string `json:"previous_key_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rotated); err != nil {
		t.Fatal(err)
	}
	if rotated.ID == "" || rotated.ID == oldID || rotated.APIKey == "" || rotated.APIKey == oldRaw {
		t.Fatalf("rotate response: %+v", rotated)
	}
	if rotated.PreviousKeyID != oldID {
		t.Fatalf("previous_key_id=%q want %q", rotated.PreviousKeyID, oldID)
	}
}

func TestKeyRotation_OldKeyWorksInGracePeriod(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	tid := "00000000-0000-0000-0000-000000000000"
	oldID, oldRaw := createAdminAPIKey(t, app, tid, "grace-active-"+time.Now().Format("150405"), "user")
	rotURL := "/v1/admin/api-keys/" + url.PathEscape(oldID) + "/rotate"
	resp, err := app.Test(reqAuthed(t, http.MethodPost, rotURL, `{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("rotate status %d", resp.StatusCode)
	}

	check := authedGET(t, app, oldRaw, "/v1/stats")
	defer check.Body.Close()
	if check.StatusCode == 401 {
		t.Fatal("old key should work during grace period")
	}
}

func TestKeyRotation_OldKeyExpiredAfterGrace(t *testing.T) {
	app, pool, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	tid := "00000000-0000-0000-0000-000000000000"
	oldID, oldRaw := createAdminAPIKey(t, app, tid, "grace-expired-"+time.Now().Format("150405"), "user")
	rotURL := "/v1/admin/api-keys/" + url.PathEscape(oldID) + "/rotate"
	resp, err := app.Test(reqAuthed(t, http.MethodPost, rotURL, `{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	setKeyGraceEnds(t, pool, oldID, time.Now().Add(-time.Minute))

	check := authedGET(t, app, oldRaw, "/v1/stats")
	defer check.Body.Close()
	if check.StatusCode != 401 {
		t.Fatalf("old key after grace: want 401, got %d", check.StatusCode)
	}
}

func TestKeyRevocation_ImmediateBlock(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	tid := "00000000-0000-0000-0000-000000000000"
	keyID, raw := createAdminAPIKey(t, app, tid, "revoke-"+time.Now().Format("150405"), "user")

	delURL := "/v1/admin/api-keys/" + url.PathEscape(keyID)
	resp, err := app.Test(reqAuthed(t, http.MethodDelete, delURL, ""))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("revoke status %d", resp.StatusCode)
	}

	check := authedGET(t, app, raw, "/v1/stats")
	defer check.Body.Close()
	if check.StatusCode != 401 {
		t.Fatalf("revoked key: want 401, got %d", check.StatusCode)
	}
}

func TestKeyExpiry_AutoBlockAfterExpiresAt(t *testing.T) {
	app, pool, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	tid := "00000000-0000-0000-0000-000000000000"
	keyID, raw := createAdminAPIKey(t, app, tid, "expired-"+time.Now().Format("150405"), "user")
	setKeyExpiresAt(t, pool, keyID, time.Now().Add(-time.Hour))

	check := authedGET(t, app, raw, "/v1/stats")
	defer check.Body.Close()
	if check.StatusCode != 401 {
		t.Fatalf("expired key: want 401, got %d", check.StatusCode)
	}
}

func TestKeyLifecycle_LastUsedAtUpdated(t *testing.T) {
	app, pool, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	tid := "00000000-0000-0000-0000-000000000000"
	keyID, raw := createAdminAPIKey(t, app, tid, "last-used-"+time.Now().Format("150405"), "user")

	check := authedGET(t, app, raw, "/v1/stats")
	check.Body.Close()
	if check.StatusCode == 401 {
		t.Fatal("unexpected 401 for fresh key")
	}
	if check.StatusCode != 200 {
		t.Fatalf("stats with new key: want 200, got %d", check.StatusCode)
	}

	deadline := time.Now().Add(5 * time.Second)
	var lastUsed *time.Time
	var lastIP *string
	for {
		err := pool.QueryRow(context.Background(),
			`SELECT last_used_at, last_used_ip FROM api_keys WHERE id = $1::uuid`, keyID,
		).Scan(&lastUsed, &lastIP)
		if err != nil {
			t.Fatal(err)
		}
		if lastUsed != nil && lastIP != nil && *lastIP != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("last_used_at/ip not updated within timeout: at=%v ip=%v", lastUsed, lastIP)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastUsed.Before(time.Now().Add(-2 * time.Minute)) {
		t.Fatalf("last_used_at not updated: %v", lastUsed)
	}
}

func TestKeyLifecycle_AuditLogContainsRotation(t *testing.T) {
	app, pool, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	tid := "00000000-0000-0000-0000-000000000000"
	oldID, _ := createAdminAPIKey(t, app, tid, "audit-rot-"+time.Now().Format("150405"), "user")
	rotURL := "/v1/admin/api-keys/" + url.PathEscape(oldID) + "/rotate"
	resp, err := app.Test(reqAuthed(t, http.MethodPost, rotURL, `{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var rotated struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rotated); err != nil {
		t.Fatal(err)
	}

	var count int
	err = pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM audit_log
		WHERE tenant_id = $1::uuid
		  AND event_type = 'api_key_rotation'
		  AND api_key_id = $2::uuid
		  AND request_body->>'previous_key_id' = $3`,
		tid, rotated.ID, oldID,
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count < 1 {
		t.Fatal("expected api_key_rotation audit row")
	}
}
