package ingest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aayushbajaj/typing-telemetry/internal/storage"
)

const testToken = "s3cr3t-token"

// newTestServer builds an httptest.Server backed by a temp-DB store. Pointing
// $HOME at a temp dir keeps storage.New() off the real database.
func newTestServer(t *testing.T, peers []string) (*httptest.Server, *storage.Store) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	store, err := storage.New()
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	srv := httptest.NewServer(New(store, testToken, "", peers, "1.4142").Handler())
	t.Cleanup(srv.Close)
	return srv, store
}

// do issues a request with an optional bearer token and returns the response.
func do(t *testing.T, method, url, token string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

func TestHealthNoAuth(t *testing.T) {
	srv, _ := newTestServer(t, nil)

	resp := do(t, http.MethodGet, srv.URL+"/v1/health", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d, want 200", resp.StatusCode)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["ok"] != true || got["version"] != "1.4142" {
		t.Fatalf("unexpected health body: %+v", got)
	}
}

func TestAuth(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	url := srv.URL + "/v1/devices"

	// Missing token.
	resp := do(t, http.MethodGet, url, "", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401", resp.StatusCode)
	}

	// Wrong token.
	resp = do(t, http.MethodGet, url, "wrong", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad-token status = %d, want 401", resp.StatusCode)
	}

	// Correct token.
	resp = do(t, http.MethodGet, url, testToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("good-token status = %d, want 200", resp.StatusCode)
	}
}

func TestBadDate(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	body := strings.NewReader(`{"keystrokes":1}`)
	resp := do(t, http.MethodPut, srv.URL+"/v1/devices/ferrari/days/2026-6-1", testToken, body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad-date status = %d, want 400", resp.StatusCode)
	}
}

func TestBadDeviceID(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	// Uppercase is rejected by ^[a-z0-9-]{1,32}$.
	resp := do(t, http.MethodGet, srv.URL+"/v1/devices/FERRARI/days/2026-06-13", testToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad-id status = %d, want 400", resp.StatusCode)
	}
}

func TestPeerAllowlist(t *testing.T) {
	// httptest connects from loopback, which is not in this allowlist.
	srv, _ := newTestServer(t, []string{"100.123.223.91"})
	resp := do(t, http.MethodGet, srv.URL+"/v1/devices", testToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("disallowed-peer status = %d, want 403", resp.StatusCode)
	}
}

func TestNegativeCountsRejected(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	body := strings.NewReader(`{"keystrokes":-5}`)
	resp := do(t, http.MethodPut, srv.URL+"/v1/devices/ferrari/days/2026-06-13", testToken, body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("negative-counts status = %d, want 400", resp.StatusCode)
	}
}

func TestOversizedBodyRejected(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	// Valid JSON padded past the 8 KiB cap with whitespace.
	big := `{"keystrokes":1` + strings.Repeat(" ", 9000) + `}`
	resp := do(t, http.MethodPut, srv.URL+"/v1/devices/ferrari/days/2026-06-13",
		testToken, strings.NewReader(big))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized-body status = %d, want 400", resp.StatusCode)
	}
}

func TestCRUDHappyPath(t *testing.T) {
	srv, _ := newTestServer(t, nil)
	base := srv.URL + "/v1/devices/ferrari/days/2026-06-13"

	// PUT → 204.
	counts := storage.DeviceDayCounts{
		Keystrokes: 4210, Letters: 3100, Modifiers: 620,
		Special: 490, Words: 780, ActiveMs: 1380000,
	}
	buf, _ := json.Marshal(counts)
	resp := do(t, http.MethodPut, base, testToken, bytes.NewReader(buf))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT status = %d, want 204", resp.StatusCode)
	}

	// GET one day → 200 with the same counts.
	resp = do(t, http.MethodGet, base, testToken, nil)
	var gotDay storage.DeviceDayCounts
	if err := json.NewDecoder(resp.Body).Decode(&gotDay); err != nil {
		t.Fatalf("decode day: %v", err)
	}
	resp.Body.Close()
	if gotDay != counts {
		t.Fatalf("GET day = %+v, want %+v", gotDay, counts)
	}

	// GET range → list with one entry.
	resp = do(t, http.MethodGet, srv.URL+"/v1/devices/ferrari/days?since=2026-06-01", testToken, nil)
	var days []storage.DeviceDay
	if err := json.NewDecoder(resp.Body).Decode(&days); err != nil {
		t.Fatalf("decode days: %v", err)
	}
	resp.Body.Close()
	if len(days) != 1 || days[0].Date != "2026-06-13" || days[0].Keystrokes != 4210 {
		t.Fatalf("GET range = %+v", days)
	}

	// GET /v1/devices → device auto-registered.
	resp = do(t, http.MethodGet, srv.URL+"/v1/devices", testToken, nil)
	var infos []storage.DeviceInfo
	if err := json.NewDecoder(resp.Body).Decode(&infos); err != nil {
		t.Fatalf("decode devices: %v", err)
	}
	resp.Body.Close()
	if len(infos) != 1 || infos[0].DeviceID != "ferrari" {
		t.Fatalf("GET devices = %+v", infos)
	}

	// GET absent day → 404.
	resp = do(t, http.MethodGet, srv.URL+"/v1/devices/ferrari/days/2026-01-01", testToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET absent = %d, want 404", resp.StatusCode)
	}

	// DELETE the day → 204, then GET → 404.
	resp = do(t, http.MethodDelete, base, testToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE day = %d, want 204", resp.StatusCode)
	}
	resp = do(t, http.MethodGet, base, testToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET after delete = %d, want 404", resp.StatusCode)
	}

	// DELETE the device → 204, then the list is empty.
	resp = do(t, http.MethodDelete, srv.URL+"/v1/devices/ferrari", testToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE device = %d, want 204", resp.StatusCode)
	}
	resp = do(t, http.MethodGet, srv.URL+"/v1/devices", testToken, nil)
	infos = nil
	if err := json.NewDecoder(resp.Body).Decode(&infos); err != nil {
		t.Fatalf("decode devices after delete: %v", err)
	}
	resp.Body.Close()
	if len(infos) != 0 {
		t.Fatalf("expected no devices after delete, got %+v", infos)
	}
}

func TestGetSelfDays(t *testing.T) {
	srv, store := newTestServer(t, nil)
	url := srv.URL + "/v1/self/days"

	// Requires auth like every non-health route.
	resp := do(t, http.MethodGet, url, "", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401", resp.StatusCode)
	}

	// A malformed since is rejected.
	resp = do(t, http.MethodGet, url+"?since=2026-6-1", testToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad-since status = %d, want 400", resp.StatusCode)
	}

	// Empty before any local typing.
	resp = do(t, http.MethodGet, url, testToken, nil)
	var days []storage.DeviceDay
	if err := json.NewDecoder(resp.Body).Decode(&days); err != nil {
		t.Fatalf("decode empty: %v", err)
	}
	resp.Body.Close()
	if len(days) != 0 {
		t.Fatalf("expected no self days, got %+v", days)
	}

	// Seed the Mac's own daily_summary via the local capture path, then it
	// should surface as a self day with keystrokes > 0.
	for i := 0; i < 5; i++ {
		if err := store.RecordKeystroke(0); err != nil { // 'A'
			t.Fatalf("RecordKeystroke: %v", err)
		}
	}
	resp = do(t, http.MethodGet, url, testToken, nil)
	days = nil
	if err := json.NewDecoder(resp.Body).Decode(&days); err != nil {
		t.Fatalf("decode seeded: %v", err)
	}
	resp.Body.Close()
	if len(days) != 1 {
		t.Fatalf("expected 1 self day, got %d: %+v", len(days), days)
	}
	if days[0].Keystrokes != 5 {
		t.Fatalf("self day keystrokes = %d, want 5", days[0].Keystrokes)
	}
}
