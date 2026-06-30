//go:build linux

// Package inertia (Linux) reproduces the macOS key-repeat acceleration on X11.
//
// macOS suppresses the OS auto-repeat with a CGEventTap and posts its own
// accelerating synthetic key-downs. That route does not port to X11: a passive
// monitor can't suppress events, and an XTEST KeyPress on an already-pressed key
// is de-duplicated by the server (so keydown-only repeats deliver nothing),
// while injecting press+release pairs corrupts the held-key state (the injected
// release clears the logical key and a physically-held key never re-asserts, so
// the real release becomes undetectable).
//
// The robust Linux-native equivalent is to let the X server's *own* auto-repeat
// be the repeat engine and accelerate its rate while a key is held:
//
//   - The server owns the key state, so repeats stop the instant the user
//     physically releases — no release detection, no injection, no corruption.
//   - On enable we set a short initial delay (= Threshold) and a slow base rate.
//   - A QueryKeymap poller (internal/x11) notices a key going down and ramps the
//     global repeat rate up along the same acceleration curve as the darwin
//     build, up to the MaxSpeed cap; on release it resets to the base rate.
//
// Rate/delay are applied with `xset r rate` (XKB controls); the core X protocol
// exposes only auto-repeat on/off, not the rate. If xset is unavailable inertia
// silently does nothing, matching the darwin "fail quietly" behaviour.
package inertia

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aayushbajaj/typing-telemetry/internal/x11"
)

// Config holds inertia configuration (identical shape to the darwin build).
type Config struct {
	Enabled   bool
	MaxSpeed  string  // "ultra_fast", "very_fast", "pretty_fast", "fast", "medium", "slow"
	Threshold int     // ms before acceleration starts
	AccelRate float64 // how quickly the curve is climbed (>1 = faster)
}

// DefaultConfig returns the default inertia configuration.
func DefaultConfig() Config {
	return Config{Enabled: false, MaxSpeed: "fast", Threshold: 200, AccelRate: 1.0}
}

// accelerationTable holds the key-count thresholds for each speed step (same
// curve as the macOS reference implementation).
var accelerationTable = []int{7, 12, 17, 21, 24, 26, 28, 30}

// baseRepeatInterval is the slowest synthetic repeat interval, in ms.
const baseRepeatInterval = 35

// maxSpeedCaps is the floor (fastest) repeat interval, in ms, per speed name.
var maxSpeedCaps = map[string]int{
	"ultra_fast":  7,
	"very_fast":   8,
	"pretty_fast": 10,
	"fast":        12,
	"medium":      20,
	"slow":        50,
}

func getAccelerationStep(keyCount int) int {
	for idx, threshold := range accelerationTable {
		if threshold > keyCount {
			return idx + 1
		}
	}
	return len(accelerationTable) + 1
}

// getRepeatInterval returns the synthetic repeat interval for a given key-count
// along the acceleration curve, clamped to the MaxSpeed cap.
func getRepeatInterval(keyCount int, cfg Config) time.Duration {
	scaled := int(float64(keyCount) * cfg.AccelRate)
	step := getAccelerationStep(scaled)
	interval := float64(baseRepeatInterval) / float64(step)

	minInterval := maxSpeedCaps[cfg.MaxSpeed]
	if minInterval == 0 {
		minInterval = maxSpeedCaps["fast"]
	}
	if interval < float64(minInterval) {
		interval = float64(minInterval)
	}
	return time.Duration(interval) * time.Millisecond
}

// capInterval is the fastest interval (ms) for the configured MaxSpeed.
func capInterval(cfg Config) int {
	c := maxSpeedCaps[cfg.MaxSpeed]
	if c == 0 {
		c = maxSpeedCaps["fast"]
	}
	return c
}

// rateForInterval converts a repeat interval in ms to an xset rate in Hz.
func rateForInterval(ms int) int {
	if ms < 1 {
		ms = 1
	}
	r := 1000 / ms
	if r < 1 {
		r = 1
	}
	return r
}

var (
	mu      sync.Mutex
	config  Config
	running bool
	poller  *x11.Poller
	ramps   = map[uint8]chan struct{}{} // raw X keycode -> active hold ramp

	// original auto-repeat delay/rate captured at Start, restored at Stop.
	origDelay = 660
	origRate  = 25
)

// modifier evdev codes (keys that should never trigger an acceleration ramp).
var modifierEvdev = map[int]bool{
	42: true, 54: true, // shift
	29: true, 97: true, // control
	56: true, 100: true, // alt
	125: true, 126: true, // meta/super
	58: true, // caps lock
}

func isModifier(xcode uint8) bool { return modifierEvdev[int(xcode)-8] }

// --- xset helpers -----------------------------------------------------------

func xsetAvailable() bool {
	_, err := exec.LookPath("xset")
	return err == nil
}

// setRepeat sets the global auto-repeat initial delay (ms) and rate (Hz).
func setRepeat(delayMs, rateHz int) {
	_ = exec.Command("xset", "r", "rate", strconv.Itoa(delayMs), strconv.Itoa(rateHz)).Run()
}

func enableRepeat() { _ = exec.Command("xset", "r", "on").Run() }

// readRepeat parses the current delay/rate from `xset q` so Stop can restore
// the user's original settings. Falls back to typical defaults on any error.
func readRepeat() (delayMs, rateHz int) {
	delayMs, rateHz = 660, 25
	out, err := exec.Command("xset", "q").Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		// "    auto repeat delay:  660    repeat rate:  25"
		if !strings.Contains(line, "auto repeat delay:") {
			continue
		}
		f := strings.Fields(line)
		for i, w := range f {
			if w == "delay:" && i+1 < len(f) {
				if v, e := strconv.Atoi(f[i+1]); e == nil {
					delayMs = v
				}
			}
			if w == "rate:" && i+1 < len(f) {
				if v, e := strconv.Atoi(f[i+1]); e == nil {
					rateHz = v
				}
			}
		}
	}
	return
}

// --- lifecycle --------------------------------------------------------------

// Start begins the inertia system. It is a no-op (returning nil) when disabled,
// when X is unavailable, or when xset is missing — mirroring the darwin build's
// "fail silently" behaviour.
func Start(cfg Config) error {
	mu.Lock()
	defer mu.Unlock()

	if running {
		return nil
	}
	config = cfg
	if !cfg.Enabled || !x11.Available() || !xsetAvailable() {
		return nil
	}

	origDelay, origRate = readRepeat()
	enableRepeat()
	// Slow base rate + Threshold delay: a held key starts repeating only after
	// the threshold, then the ramp accelerates it.
	setRepeat(cfg.Threshold, rateForInterval(baseRepeatInterval))

	p, err := x11.StartPoller(x11.DefaultPollInterval, onTransition)
	if err != nil {
		setRepeat(origDelay, origRate)
		return nil
	}
	poller = p
	ramps = map[uint8]chan struct{}{}
	running = true
	return nil
}

// Stop stops the inertia system and restores the user's auto-repeat settings.
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
	for k, ch := range ramps {
		close(ch)
		delete(ramps, k)
	}
	setRepeat(origDelay, origRate)
	enableRepeat()
	running = false
}

// UpdateConfig changes the running configuration, starting or stopping the
// system as the Enabled flag dictates.
func UpdateConfig(cfg Config) {
	mu.Lock()
	wasRunning := running
	mu.Unlock()

	switch {
	case cfg.Enabled && !wasRunning:
		Start(cfg)
	case !cfg.Enabled && wasRunning:
		Stop()
	default:
		mu.Lock()
		config = cfg
		mu.Unlock()
	}
}

// IsRunning reports whether inertia is active.
func IsRunning() bool {
	mu.Lock()
	defer mu.Unlock()
	return running && config.Enabled
}

// --- ramp -------------------------------------------------------------------

// rampTick is how often the ramp bumps the global repeat rate while a key is
// held. Small enough to feel like a smooth acceleration, large enough to keep
// the xset invocations modest.
const rampTick = 60 * time.Millisecond

// onTransition is the poller callback. Because we never inject, every
// transition is a genuine physical press or release.
func onTransition(code uint8, down bool, _ x11.Keymap) {
	if isModifier(code) {
		return
	}
	if down {
		startRamp(code)
	} else {
		stopRamp(code)
	}
}

// startRamp begins accelerating the repeat rate for a held key, cancelling any
// other key's ramp first (matching the darwin stopOtherKeys behaviour).
func startRamp(code uint8) {
	mu.Lock()
	if !running {
		mu.Unlock()
		return
	}
	cfg := config
	for k, ch := range ramps {
		close(ch)
		delete(ramps, k)
	}
	stop := make(chan struct{})
	ramps[code] = stop
	mu.Unlock()

	go rampLoop(cfg, stop)
}

// stopRamp ends a key's ramp and resets the global rate to the slow base so the
// next held key starts slow again.
func stopRamp(code uint8) {
	mu.Lock()
	defer mu.Unlock()
	if ch, ok := ramps[code]; ok {
		close(ch)
		delete(ramps, code)
	}
	if running {
		setRepeat(config.Threshold, rateForInterval(baseRepeatInterval))
	}
}

// rampLoop waits the threshold, then walks the acceleration curve, raising the
// global repeat rate every rampTick until it reaches the MaxSpeed cap.
func rampLoop(cfg Config, stop chan struct{}) {
	select {
	case <-stop:
		return
	case <-time.After(time.Duration(cfg.Threshold) * time.Millisecond):
	}

	capMs := capInterval(cfg)
	count := 0
	for {
		count += 3
		ms := int(getRepeatInterval(count, cfg) / time.Millisecond)
		setRepeat(cfg.Threshold, rateForInterval(ms))
		if ms <= capMs {
			return // reached top speed; the server keeps repeating at this rate
		}
		select {
		case <-stop:
			return
		case <-time.After(rampTick):
		}
	}
}
