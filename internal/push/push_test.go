package push

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aayushbajaj/typing-telemetry/internal/ingest"
	"github.com/aayushbajaj/typing-telemetry/internal/storage"
)

const testToken = "0123456789abcdef0123456789abcdef"

// newHost spins up an in-memory ingest host (its own temp store) behind an
// httptest server, returning the base URL and the host's store.
func newHost(t *testing.T) (string, *storage.Store) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	hostStore, err := storage.New()
	if err != nil {
		t.Fatalf("host store: %v", err)
	}
	t.Cleanup(func() { hostStore.Close() })

	srv := ingest.New(hostStore, testToken, "", nil, "test")
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts.URL, hostStore
}

func TestNewValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		ok   bool
	}{
		{"ok", Config{BaseURL: "http://h:8889", Token: "t", DeviceID: "kali"}, true},
		{"trailing slash", Config{BaseURL: "http://h:8889/", Token: "t", DeviceID: "kali"}, true},
		{"no url", Config{Token: "t", DeviceID: "kali"}, false},
		{"bad scheme", Config{BaseURL: "ftp://h", Token: "t", DeviceID: "kali"}, false},
		{"no token", Config{BaseURL: "http://h", DeviceID: "kali"}, false},
		{"bad id", Config{BaseURL: "http://h", Token: "t", DeviceID: "Kali Box"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.cfg)
			if tc.ok && err != nil {
				t.Fatalf("expected ok, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func TestHealthAndPushToday(t *testing.T) {
	base, hostStore := newHost(t)

	// Seed the pushing side's local store with some activity.
	t.Setenv("HOME", t.TempDir()) // separate "device" home/db
	devStore, err := storage.New()
	if err != nil {
		t.Fatalf("device store: %v", err)
	}
	defer devStore.Close()
	for i := 0; i < 5; i++ {
		if err := devStore.RecordKeystroke(0); err != nil { // keycode 0 = 'a' = letter
			t.Fatal(err)
		}
	}

	c, err := New(Config{BaseURL: base, Token: testToken, DeviceID: "kali", Name: "Kali Box"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if err := c.Health(ctx); err != nil {
		t.Fatalf("health: %v", err)
	}
	if err := c.PushToday(ctx, devStore); err != nil {
		t.Fatalf("push today: %v", err)
	}

	// The host should now have the device's day + the friendly name (via ?name=).
	today := todayStr()
	got, err := hostStore.GetDeviceDay("kali", today)
	if err != nil || got == nil {
		t.Fatalf("host GetDeviceDay: got=%v err=%v", got, err)
	}
	if got.Keystrokes != 5 {
		t.Fatalf("keystrokes = %d, want 5", got.Keystrokes)
	}
	devices, err := hostStore.ListDevices()
	if err != nil || len(devices) != 1 {
		t.Fatalf("ListDevices: %v %v", devices, err)
	}
	if devices[0].Name != "Kali Box" {
		t.Fatalf("device name = %q, want %q", devices[0].Name, "Kali Box")
	}
}

func TestWrongTokenRejected(t *testing.T) {
	base, _ := newHost(t)
	c, err := New(Config{BaseURL: base, Token: "wrong-token", DeviceID: "kali"})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.PutDay(context.Background(), todayStr(), storage.DeviceDayCounts{Keystrokes: 1}); err == nil {
		t.Fatal("expected auth error with wrong token, got nil")
	}
}

func todayStr() string {
	return time.Now().Format("2006-01-02")
}
