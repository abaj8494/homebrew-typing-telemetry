//go:build linux

// Package x11 is the Linux counterpart to the macOS CoreGraphics event plumbing
// used by the keylogger and inertia packages. It provides, in pure Go
// (github.com/jezek/xgb, no C libraries), a Poller: a global keyboard-state
// monitor built on xproto.QueryKeymap. It samples the X server's 256-bit
// physical key vector at a fixed rate and reports press/release transitions —
// the analogue of a listen-only CGEventTap. QueryKeymap reports physical key
// state regardless of which window is focused, needs no elevated privilege (no
// /dev/input access, no `input` group), and is immune to XKB auto-repeat (a
// held key keeps its bit set, so it never re-fires).
//
// Why polling rather than XRecord or XInput2: this build of jezek/xgb ships
// neither a usable XRecord reply stream (its cookie reader consumes the
// EnableContext cookie after the first reply, so the stream deadlocks) nor an
// xinput package. QueryKeymap is plain core protocol and sidesteps both.
package x11

import (
	"sync"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// DefaultPollInterval samples at ~125 Hz: fast enough to catch any human
// keystroke (an 8 ms tap is far shorter than anyone types) while costing only a
// few hundred tiny round-trips per second on the loopback X socket.
const DefaultPollInterval = 8 * time.Millisecond

// Keymap is the X server's physical key-state bitmap: bit k is set when X
// keycode k is currently down. A keycode is the evdev code plus 8.
type Keymap [32]byte

// Down reports whether the given raw X keycode is currently pressed.
func (m Keymap) Down(keycode uint8) bool {
	return m[keycode>>3]&(1<<(keycode&7)) != 0
}

// Available reports whether an X server can be reached. It opens and closes a
// throwaway connection, so it honours $DISPLAY exactly like the real clients.
func Available() bool {
	c, err := xgb.NewConn()
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// QueryState returns the current keyboard state via a one-shot connection. The
// boolean is false if the server could not be reached or queried.
func QueryState() (Keymap, bool) {
	var m Keymap
	c, err := xgb.NewConn()
	if err != nil {
		return m, false
	}
	defer c.Close()
	reply, err := xproto.QueryKeymap(c).Reply()
	if err != nil {
		return m, false
	}
	copy(m[:], reply.Keys)
	return m, true
}

// Poller samples the keyboard state on its own connection and reports
// transitions to a handler on a background goroutine.
type Poller struct {
	conn *xgb.Conn
	stop chan struct{}

	mu     sync.Mutex
	closed bool
}

// StartPoller begins sampling the keyboard every interval (use
// DefaultPollInterval for the sensible default). For every key whose state
// changes between samples, handler is called with the raw X keycode, whether it
// is now down, and the full keymap at that sample. handler must not block.
func StartPoller(interval time.Duration, handler func(keycode uint8, down bool, state Keymap)) (*Poller, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}
	p := &Poller{conn: conn, stop: make(chan struct{})}
	go p.loop(interval, handler)
	return p, nil
}

func (p *Poller) loop(interval time.Duration, handler func(uint8, bool, Keymap)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var prev Keymap
	first := true
	for {
		select {
		case <-p.stop:
			return
		case <-ticker.C:
		}

		reply, err := xproto.QueryKeymap(p.conn).Reply()
		if err != nil {
			return // connection closed by Stop, or a fatal protocol error
		}
		var cur Keymap
		copy(cur[:], reply.Keys)

		if !first {
			for i := 0; i < len(cur); i++ {
				diff := cur[i] ^ prev[i]
				if diff == 0 {
					continue
				}
				for bit := 0; bit < 8; bit++ {
					if diff&(1<<bit) == 0 {
						continue
					}
					code := uint8(i*8 + bit)
					handler(code, cur.Down(code), cur)
				}
			}
		}
		prev = cur
		first = false
	}
}

// Stop ends sampling and releases the connection. Safe to call once.
func (p *Poller) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	close(p.stop)
	p.conn.Close()
}
