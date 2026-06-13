// Package ingest hosts the opt-in, Tailscale-bound, token-gated HTTP API that
// accepts absolute daily keystroke aggregates from external devices (e.g. a
// reMarkable tablet) into the dedicated device_* tables. It is pure Go (no
// CGO) so it is portable and httptest-able, and it never touches the macOS
// daily_summary capture path.
//
// The trust boundary is the tailnet plus the bearer token: bind to the Mac's
// Tailscale IP (not 0.0.0.0) and the port is unreachable off-tailnet. The
// optional source-IP allowlist pins ingest to specific tailnet peers.
package ingest

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"regexp"
	"time"

	"github.com/aayushbajaj/typing-telemetry/internal/storage"
)

// maxBodyBytes caps an ingest PUT body. The payload is a handful of integers,
// so 8 KiB is generous slack for whitespace/formatting.
const maxBodyBytes = 8 << 10

var (
	deviceIDRe = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
	dateRe     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// Server is the device-ingest HTTP listener. Construct it with New and run it
// with Start.
type Server struct {
	store *storage.Store
	token string
	addr  string          // host:port to bind
	peers map[string]bool // optional source-IP allowlist; empty = allow any tailnet peer
	ver   string
}

// New builds a Server. peers is an optional source-IP allowlist (empty allows
// any tailnet peer that can reach the bound address).
func New(store *storage.Store, token, addr string, peers []string, version string) *Server {
	peerSet := make(map[string]bool, len(peers))
	for _, p := range peers {
		if p != "" {
			peerSet[p] = true
		}
	}
	return &Server{store: store, token: token, addr: addr, peers: peerSet, ver: version}
}

// Handler returns the routed http.Handler. Exposed so tests can wrap it in an
// httptest.Server without binding a real port.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Liveness probe — intentionally unauthenticated so a device can confirm
	// reachability before it holds a token.
	mux.HandleFunc("GET /v1/health", s.handleHealth)

	mux.HandleFunc("PUT /v1/devices/{id}/days/{date}", s.guard(s.handlePutDay))
	mux.HandleFunc("GET /v1/devices/{id}/days/{date}", s.guard(s.handleGetDay))
	mux.HandleFunc("GET /v1/devices/{id}/days", s.guard(s.handleGetDays))
	mux.HandleFunc("DELETE /v1/devices/{id}/days/{date}", s.guard(s.handleDeleteDay))
	mux.HandleFunc("DELETE /v1/devices/{id}", s.guard(s.handleDeleteDevice))
	mux.HandleFunc("GET /v1/devices", s.guard(s.handleListDevices))

	// Read-only: THIS Mac's own daily aggregates (the local capture path), so a
	// device can pull the Mac's stats back and show it alongside its own feeds.
	mux.HandleFunc("GET /v1/self/days", s.guard(s.handleGetSelfDays))

	return mux
}

// Start binds the listener and serves until ctx is cancelled, at which point it
// gracefully shuts down.
func (s *Server) Start(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errc <- err
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errc:
		return err
	}
}

// guard wraps a handler with auth, the optional source-IP allowlist, and {id}/
// {date} path validation. It runs on every route except /v1/health.
func (s *Server) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Bearer token (constant-time).
		const prefix = "Bearer "
		auth := r.Header.Get("Authorization")
		if len(auth) <= len(prefix) || auth[:len(prefix)] != prefix ||
			subtle.ConstantTimeCompare([]byte(auth[len(prefix):]), []byte(s.token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// 2. Optional source-IP allowlist.
		if len(s.peers) > 0 {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			if !s.peers[host] {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}

		// 3. Path-param validation.
		if id := r.PathValue("id"); id != "" && !deviceIDRe.MatchString(id) {
			http.Error(w, "bad device id", http.StatusBadRequest)
			return
		}
		if date := r.PathValue("date"); date != "" && !dateRe.MatchString(date) {
			http.Error(w, "bad date", http.StatusBadRequest)
			return
		}

		next(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": s.ver})
}

func (s *Server) handlePutDay(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var c storage.DeviceDayCounts
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	// 4. Absolute counts can never be negative.
	if c.Keystrokes < 0 || c.Letters < 0 || c.Modifiers < 0 || c.Special < 0 ||
		c.Words < 0 || c.ActiveMs < 0 {
		http.Error(w, "negative counts", http.StatusBadRequest)
		return
	}
	if err := s.store.UpsertDeviceDay(r.PathValue("id"), r.PathValue("date"), c); err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetDay(w http.ResponseWriter, r *http.Request) {
	c, err := s.store.GetDeviceDay(r.PathValue("id"), r.PathValue("date"))
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if c == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleGetDays(w http.ResponseWriter, r *http.Request) {
	since := r.URL.Query().Get("since")
	if since != "" && !dateRe.MatchString(since) {
		http.Error(w, "bad since", http.StatusBadRequest)
		return
	}
	days, err := s.store.GetDeviceDays(r.PathValue("id"), since)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if days == nil {
		days = []storage.DeviceDay{}
	}
	writeJSON(w, http.StatusOK, days)
}

func (s *Server) handleGetSelfDays(w http.ResponseWriter, r *http.Request) {
	since := r.URL.Query().Get("since")
	if since != "" && !dateRe.MatchString(since) {
		http.Error(w, "bad since", http.StatusBadRequest)
		return
	}
	days, err := s.store.GetSelfDays(since)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if days == nil {
		days = []storage.DeviceDay{}
	}
	writeJSON(w, http.StatusOK, days)
}

func (s *Server) handleDeleteDay(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteDeviceDay(r.PathValue("id"), r.PathValue("date")); err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteDevice(r.PathValue("id")); err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.store.ListDevices()
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	if devices == nil {
		devices = []storage.DeviceInfo{}
	}
	writeJSON(w, http.StatusOK, devices)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
