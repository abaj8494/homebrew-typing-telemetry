//go:build linux

package keylogger

// Linux keystroke capture. It mirrors the public surface of the darwin
// CGEventTap implementation (Start/Stop, KeystrokeEvent + modifier helpers,
// CheckAccessibilityPermissions, GetCurrentModifiers, PlaySound) so the rest of
// the codebase — wordcounter, speedtracker, storage — is reused unchanged.
//
// Capture is done by polling the X server's global key-state bitmap
// (internal/x11) and reporting each key's down-transition. Two details make the
// downstream pipeline work without modification:
//
//   - Keycodes are translated from X/evdev space into the macOS virtual
//     keycodes that wordcounter and storage.ClassifyKeycode are written
//     against, via evdevToMac below.
//   - Held keys and inertia's synthetic key-down repeats never produce a new
//     down-transition (the state bit is already set), so each physical press
//     counts exactly once — matching the darwin tap's "ignore auto-repeat and
//     synthetic events" behaviour for free.

import (
	"errors"
	"sync"

	"github.com/aayushbajaj/typing-telemetry/internal/x11"
)

// Modifier flag bits. The values mirror the darwin build's CGEventFlags bits so
// KeystrokeEvent.CmdHeld()/CtrlHeld()/etc. and wordcounter behave identically on
// both platforms. On Linux, Super (Mod4) takes the "Cmd" slot and Alt the
// "Opt" slot.
const (
	flagShift = 1 << 17
	flagCtrl  = 1 << 18
	flagOpt   = 1 << 19
	flagCmd   = 1 << 20
)

// KeystrokeEvent carries a (translated, macOS-space) keycode plus the modifier
// flag state at the moment the key went down.
type KeystrokeEvent struct {
	Keycode int
	Flags   uint64
}

// CmdHeld reports whether Super/Command was held when this event fired.
func (e KeystrokeEvent) CmdHeld() bool { return e.Flags&flagCmd != 0 }

// CtrlHeld reports whether Control was held when this event fired.
func (e KeystrokeEvent) CtrlHeld() bool { return e.Flags&flagCtrl != 0 }

// OptHeld reports whether Alt/Option was held when this event fired.
func (e KeystrokeEvent) OptHeld() bool { return e.Flags&flagOpt != 0 }

// ShiftHeld reports whether Shift was held when this event fired.
func (e KeystrokeEvent) ShiftHeld() bool { return e.Flags&flagShift != 0 }

// ModifierFlags mirrors the darwin type for API parity.
type ModifierFlags struct {
	Cmd   bool
	Ctrl  bool
	Opt   bool
	Shift bool
}

// System sound IDs are darwin-only; the constants exist for API parity and
// PlaySound is a no-op on Linux.
const (
	SoundTink     = 1103
	SoundPop      = 1104
	SoundBoop     = 1105
	SoundGlass    = 1107
	SoundMorse    = 1108
	SoundPurr     = 1110
	SoundHero     = 1114
	SoundSubmerge = 1117
)

// PlaySound is a no-op on Linux.
func PlaySound(int) {}

var (
	mu            sync.Mutex
	keystrokeChan chan KeystrokeEvent
	poller        *x11.Poller
	running       bool
)

// CheckAccessibilityPermissions reports whether keystroke capture is possible —
// i.e. whether an X server is reachable. Unlike macOS there is no TCC prompt.
func CheckAccessibilityPermissions() bool {
	return x11.Available()
}

// GetCurrentModifiers queries the live modifier state from the X server.
func GetCurrentModifiers() ModifierFlags {
	m, ok := x11.QueryState()
	if !ok {
		return ModifierFlags{}
	}
	f := flagsFromKeymap(m)
	return ModifierFlags{
		Cmd:   f&flagCmd != 0,
		Ctrl:  f&flagCtrl != 0,
		Opt:   f&flagOpt != 0,
		Shift: f&flagShift != 0,
	}
}

// Start begins capturing keystrokes and returns a channel of KeystrokeEvents.
func Start() (<-chan KeystrokeEvent, error) {
	mu.Lock()
	defer mu.Unlock()

	if running {
		return nil, errors.New("keylogger already running")
	}
	if !x11.Available() {
		return nil, errors.New("cannot connect to X display (is DISPLAY set?)")
	}

	keystrokeChan = make(chan KeystrokeEvent, 1000)
	p, err := x11.StartPoller(x11.DefaultPollInterval, onTransition)
	if err != nil {
		close(keystrokeChan)
		keystrokeChan = nil
		return nil, err
	}
	poller = p
	running = true
	return keystrokeChan, nil
}

// onTransition is invoked by the poller for every key-state change. Only
// down-transitions are counted; the channel send happens under mu so Stop can
// close the channel safely.
func onTransition(code uint8, down bool, state x11.Keymap) {
	if !down {
		return
	}
	ev := KeystrokeEvent{Keycode: translate(code), Flags: flagsFromKeymap(state)}

	mu.Lock()
	ch := keystrokeChan
	if ch != nil {
		select {
		case ch <- ev:
		default: // channel full, drop keystroke
		}
	}
	mu.Unlock()
}

// Stop stops the keylogger.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if !running {
		return
	}
	if poller != nil {
		poller.Stop()
		poller = nil
	}
	if keystrokeChan != nil {
		close(keystrokeChan)
		keystrokeChan = nil
	}
	running = false
}
