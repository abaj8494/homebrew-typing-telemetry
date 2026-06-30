// Package push is the outbound counterpart to internal/ingest: a small,
// opt-in HTTP client that PUTs this machine's daily typing aggregates to a host
// typtel's device-ingest API over Tailscale. It is pure Go (no CGO) so it is
// portable across the macOS menubar, the Linux tray, and the CLI.
//
// It is entirely inert unless explicitly configured: nothing here opens a
// socket, reads a token, or contacts the network until a caller has loaded an
// enabled Config (see LoadConfig) and constructed a Client. A single-device
// user who never runs `typtel push enable` is never touched by this package.
//
// Counts are ABSOLUTE day totals, never deltas: the host stores them
// INSERT-OR-REPLACE, so re-pushing the same day is idempotent and a missed
// push is corrected by the next one.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/aayushbajaj/typing-telemetry/internal/storage"
)

// deviceIDRe mirrors the host's accepted device id shape (internal/ingest).
var deviceIDRe = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// DefaultTimeout bounds a single push/health request.
const DefaultTimeout = 10 * time.Second

// Config describes where and as whom to push. BaseURL is the host typtel root
// (e.g. "http://100.93.238.15:8889"); the API paths are appended internally.
type Config struct {
	BaseURL  string
	Token    string
	DeviceID string
	Name     string // optional friendly name shown on the host; sent as ?name=
	Timeout  time.Duration
}

// Client posts daily aggregates to a host's ingest API.
type Client struct {
	cfg   Config
	base  string
	httpc *http.Client
}

// New validates cfg and returns a ready Client.
func New(cfg Config) (*Client, error) {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("push: base URL is required")
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("push: base URL must be a http(s) URL, got %q", cfg.BaseURL)
	}
	if !deviceIDRe.MatchString(cfg.DeviceID) {
		return nil, fmt.Errorf("push: device id must match [a-z0-9-]{1,32}, got %q", cfg.DeviceID)
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, fmt.Errorf("push: token is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &Client{
		cfg:   cfg,
		base:  cfg.BaseURL,
		httpc: &http.Client{Timeout: cfg.Timeout},
	}, nil
}

// Health calls the unauthenticated liveness probe; nil means reachable.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/v1/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("push: health check returned %s", resp.Status)
	}
	return nil
}

// PutDay uploads one day's absolute counts. A non-empty Config.Name is sent as
// a ?name= query so the host can show a friendly name instead of the bare id.
func (c *Client) PutDay(ctx context.Context, date string, counts storage.DeviceDayCounts) error {
	body, err := json.Marshal(counts)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/v1/devices/%s/days/%s", c.base, c.cfg.DeviceID, date)
	if c.cfg.Name != "" {
		endpoint += "?name=" + url.QueryEscape(c.cfg.Name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		// Deliberately omit the body/token from the error.
		return fmt.Errorf("push: PUT day returned %s", resp.Status)
	}
	return nil
}

// PushToday uploads today's local aggregates.
func (c *Client) PushToday(ctx context.Context, store *storage.Store) error {
	return c.PushDay(ctx, store, time.Now().Format("2006-01-02"))
}

// PushDay uploads the local aggregates for a specific YYYY-MM-DD date.
func (c *Client) PushDay(ctx context.Context, store *storage.Store, date string) error {
	stats, err := store.GetDayStats(date)
	if err != nil {
		return err
	}
	return c.PutDay(ctx, date, toCounts(stats))
}

// toCounts maps a local DailyStats to the wire shape (1:1 fields).
func toCounts(d *storage.DailyStats) storage.DeviceDayCounts {
	if d == nil {
		return storage.DeviceDayCounts{}
	}
	return storage.DeviceDayCounts{
		Keystrokes: d.Keystrokes,
		Letters:    d.Letters,
		Modifiers:  d.Modifiers,
		Special:    d.Special,
		Words:      d.Words,
		ActiveMs:   d.ActiveMs,
	}
}

// LoadConfig reads the push settings from the store. The returned enabled flag
// is the master opt-in switch; when false, callers must not push. LoadConfig
// lives here (not in storage) so storage need not import push.
func LoadConfig(store *storage.Store) (cfg Config, enabled bool, err error) {
	enabled = store.GetSettingBool(storage.SettingPushEnabled)
	cfg = Config{
		BaseURL:  store.GetSettingOr(storage.SettingPushBaseURL, ""),
		Token:    store.GetSettingOr(storage.SettingPushToken, ""),
		DeviceID: store.GetSettingOr(storage.SettingPushDeviceID, ""),
		Name:     store.GetSettingOr(storage.SettingPushDeviceName, ""),
	}
	return cfg, enabled, nil
}

// LoopConfig tunes RunLoop.
type LoopConfig struct {
	Interval time.Duration // push cadence; defaults to 45s when <= 0
	Logf     func(string, ...any)
}

// RunLoop periodically pushes today's counts until ctx is cancelled. It pushes
// once immediately, then on each tick. When the local date rolls over it pushes
// the previous day one last time before switching to the new day, so the final
// totals for a day are not stranded. Errors are logged (if Logf is set) and the
// loop continues — pushing is best-effort.
func RunLoop(ctx context.Context, store *storage.Store, c *Client, lc LoopConfig) {
	if lc.Interval <= 0 {
		lc.Interval = 45 * time.Second
	}
	logf := lc.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}

	lastDate := time.Now().Format("2006-01-02")
	push := func() {
		now := time.Now().Format("2006-01-02")
		if now != lastDate {
			// Flush the day that just ended before moving on.
			if err := c.PushDay(ctx, store, lastDate); err != nil {
				logf("[push] %s: %v", lastDate, err)
			}
			lastDate = now
		}
		if err := c.PushDay(ctx, store, now); err != nil {
			logf("[push] %s: %v", now, err)
		}
	}

	push()
	ticker := time.NewTicker(lc.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			push()
		}
	}
}
