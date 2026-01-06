// +build darwin

package inertia

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation -framework ApplicationServices

#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>
#include <stdbool.h>

// Callback declarations
extern CGEventRef goInertiaEventCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon);

// Event tap for inertia - uses kCGEventTapOptionDefault to allow modification
static CFMachPortRef inertiaEventTap = NULL;
static CFRunLoopSourceRef inertiaRunLoopSource = NULL;

static CGEventRef inertiaEventCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon) {
    return goInertiaEventCallback(proxy, type, event, refcon);
}

static int createInertiaEventTap() {
    if (inertiaEventTap != NULL) {
        return 1; // Already created
    }

    // Listen for keyDown, keyUp, and flagsChanged (for shift)
    CGEventMask eventMask = CGEventMaskBit(kCGEventKeyDown) |
                            CGEventMaskBit(kCGEventKeyUp) |
                            CGEventMaskBit(kCGEventFlagsChanged);

    // Use kCGEventTapOptionDefault to allow event modification/suppression
    inertiaEventTap = CGEventTapCreate(
        kCGSessionEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionDefault,
        eventMask,
        inertiaEventCallback,
        NULL
    );

    if (inertiaEventTap == NULL) {
        return 0;
    }

    return 1;
}

static void runInertiaEventLoop() {
    if (inertiaEventTap == NULL) {
        return;
    }

    inertiaRunLoopSource = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, inertiaEventTap, 0);
    CFRunLoopAddSource(CFRunLoopGetCurrent(), inertiaRunLoopSource, kCFRunLoopCommonModes);
    CGEventTapEnable(inertiaEventTap, true);
    CFRunLoopRun();
}

static void stopInertiaEventLoop() {
    if (inertiaEventTap != NULL) {
        CGEventTapEnable(inertiaEventTap, false);
        if (inertiaRunLoopSource != NULL) {
            CFRunLoopRemoveSource(CFRunLoopGetCurrent(), inertiaRunLoopSource, kCFRunLoopCommonModes);
            CFRelease(inertiaRunLoopSource);
            inertiaRunLoopSource = NULL;
        }
        CFRelease(inertiaEventTap);
        inertiaEventTap = NULL;
    }
    CFRunLoopStop(CFRunLoopGetCurrent());
}

// Synthesize a key event
static void postKeyEvent(CGKeyCode keycode, bool keyDown) {
    CGEventRef event = CGEventCreateKeyboardEvent(NULL, keycode, keyDown);
    if (event != NULL) {
        CGEventPost(kCGHIDEventTap, event);
        CFRelease(event);
    }
}

// Check if event is an autorepeat
static bool isAutorepeatEvent(CGEventRef event) {
    return CGEventGetIntegerValueField(event, kCGKeyboardEventAutorepeat) != 0;
}

// Get keycode from event
static int getKeycode(CGEventRef event) {
    return (int)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
}

// Get event flags (for modifier keys)
static uint64_t getEventFlags(CGEventRef event) {
    return CGEventGetFlags(event);
}

// Check accessibility
static int checkInertiaAccessibility() {
    return AXIsProcessTrusted();
}

// Return null event ref (to suppress events)
static CGEventRef nullEventRef() {
    return NULL;
}
*/
import "C"

import (
	"sync"
	"time"
	"unsafe"
)

// Config holds inertia configuration
type Config struct {
	Enabled       bool
	MaxSpeed      string  // "infinite", "fast", "medium"
	Threshold     int     // ms before acceleration starts (default 150)
	AccelRate     float64 // acceleration multiplier (default 1.0)
}

// DefaultConfig returns the default inertia configuration
func DefaultConfig() Config {
	return Config{
		Enabled:   false,
		MaxSpeed:  "fast",
		Threshold: 150,
		AccelRate: 1.0,
	}
}

// Acceleration table: key_count thresholds for each speed step
// Based on reference: { 7, 12, 17, 21, 24, 26, 28, 30 }
var accelerationTable = []int{7, 12, 17, 21, 24, 26, 28, 30}

// Base repeat interval in milliseconds (macOS default is ~35ms)
const baseRepeatInterval = 35

// Max speed caps (repeat interval in ms)
var maxSpeedCaps = map[string]int{
	"infinite": 5,   // ~200 keys/sec
	"fast":     10,  // ~100 keys/sec (8x faster than base)
	"medium":   20,  // ~50 keys/sec (4x faster than base)
}

// State tracking
type keyState struct {
	isHeld        bool
	keyCount      int
	lastEventTime time.Time
	repeatTimer   *time.Timer
	stopChan      chan struct{}
}

var (
	mu             sync.RWMutex
	config         Config
	running        bool
	keyStates      = make(map[int]*keyState)  // keycode -> state
	lastShiftTap   time.Time
	shiftTapCount  int
)

// Global reference to prevent GC
var inertiaInstance *Inertia

// Inertia manages the key acceleration system
type Inertia struct {
	config Config
	stopCh chan struct{}
}

// New creates a new Inertia instance
func New(cfg Config) *Inertia {
	return &Inertia{
		config: cfg,
		stopCh: make(chan struct{}),
	}
}

// Start begins the inertia system
func Start(cfg Config) error {
	mu.Lock()
	defer mu.Unlock()

	if running {
		return nil
	}

	config = cfg

	if !cfg.Enabled {
		return nil
	}

	if C.checkInertiaAccessibility() == 0 {
		return nil // Silently fail if no accessibility
	}

	if C.createInertiaEventTap() == 0 {
		return nil
	}

	running = true
	inertiaInstance = New(cfg)

	go func() {
		C.runInertiaEventLoop()
	}()

	return nil
}

// Stop stops the inertia system
func Stop() {
	mu.Lock()
	defer mu.Unlock()

	if !running {
		return
	}

	// Stop all active key repeats
	for _, state := range keyStates {
		if state.stopChan != nil {
			close(state.stopChan)
		}
		if state.repeatTimer != nil {
			state.repeatTimer.Stop()
		}
	}
	keyStates = make(map[int]*keyState)

	C.stopInertiaEventLoop()
	running = false
	inertiaInstance = nil
}

// UpdateConfig updates the inertia configuration
func UpdateConfig(cfg Config) {
	mu.Lock()
	defer mu.Unlock()
	config = cfg

	// If disabled, stop any active repeats
	if !cfg.Enabled {
		for _, state := range keyStates {
			if state.stopChan != nil {
				close(state.stopChan)
			}
			if state.repeatTimer != nil {
				state.repeatTimer.Stop()
			}
		}
		keyStates = make(map[int]*keyState)
	}
}

// IsRunning returns whether inertia is active
func IsRunning() bool {
	mu.RLock()
	defer mu.RUnlock()
	return running && config.Enabled
}

// getAccelerationStep calculates the current speed step based on key_count
func getAccelerationStep(keyCount int) int {
	for idx, threshold := range accelerationTable {
		if threshold > keyCount {
			return idx + 1
		}
	}
	return len(accelerationTable) + 1
}

// getRepeatInterval calculates the repeat interval based on acceleration
func getRepeatInterval(keyCount int, cfg Config) time.Duration {
	step := getAccelerationStep(keyCount)

	// Base interval decreases with each step
	// Apply acceleration rate multiplier
	interval := float64(baseRepeatInterval) / (float64(step) * cfg.AccelRate)

	// Apply max speed cap
	minInterval := maxSpeedCaps[cfg.MaxSpeed]
	if minInterval == 0 {
		minInterval = maxSpeedCaps["fast"]
	}

	if interval < float64(minInterval) {
		interval = float64(minInterval)
	}

	return time.Duration(interval) * time.Millisecond
}

// startKeyRepeat starts the accelerating key repeat for a held key
func startKeyRepeat(keycode int) {
	mu.Lock()
	state, exists := keyStates[keycode]
	if !exists {
		state = &keyState{
			stopChan: make(chan struct{}),
		}
		keyStates[keycode] = state
	}

	// Reset if there was an old repeat
	if state.stopChan != nil {
		select {
		case <-state.stopChan:
			// Already closed
		default:
			close(state.stopChan)
		}
	}

	state.isHeld = true
	state.keyCount = 0
	state.lastEventTime = time.Now()
	state.stopChan = make(chan struct{})
	cfg := config
	mu.Unlock()

	// Start the repeat goroutine
	go func(kc int, stopCh chan struct{}) {
		for {
			mu.RLock()
			s, ok := keyStates[kc]
			if !ok || !s.isHeld || !config.Enabled {
				mu.RUnlock()
				return
			}
			s.keyCount++
			interval := getRepeatInterval(s.keyCount, cfg)
			mu.RUnlock()

			select {
			case <-stopCh:
				return
			case <-time.After(interval):
				// Post the key event
				C.postKeyEvent(C.CGKeyCode(kc), C.bool(true))
				C.postKeyEvent(C.CGKeyCode(kc), C.bool(false))
			}
		}
	}(keycode, state.stopChan)
}

// stopKeyRepeat stops the key repeat for a released key
func stopKeyRepeat(keycode int) {
	mu.Lock()
	defer mu.Unlock()

	state, exists := keyStates[keycode]
	if !exists {
		return
	}

	state.isHeld = false
	if state.stopChan != nil {
		select {
		case <-state.stopChan:
			// Already closed
		default:
			close(state.stopChan)
		}
	}
}

// resetAllAcceleration resets all key acceleration (for double-tap shift)
func resetAllAcceleration() {
	mu.Lock()
	defer mu.Unlock()

	for _, state := range keyStates {
		state.keyCount = 0
	}
}

// handleShiftTap checks for double-tap shift and resets acceleration
func handleShiftTap() {
	now := time.Now()

	mu.Lock()
	defer mu.Unlock()

	if now.Sub(lastShiftTap) < 300*time.Millisecond {
		shiftTapCount++
		if shiftTapCount >= 2 {
			// Double-tap detected - reset all acceleration
			for _, state := range keyStates {
				state.keyCount = 0
			}
			shiftTapCount = 0
		}
	} else {
		shiftTapCount = 1
	}
	lastShiftTap = now
}

// Shift keycodes (left=56, right=60)
const (
	leftShiftKeycode  = 56
	rightShiftKeycode = 60
)

//export goInertiaEventCallback
func goInertiaEventCallback(proxy C.CGEventTapProxy, eventType C.CGEventType, event C.CGEventRef, refcon unsafe.Pointer) C.CGEventRef {
	mu.RLock()
	enabled := config.Enabled
	mu.RUnlock()

	if !enabled {
		return event
	}

	keycode := int(C.getKeycode(event))

	switch eventType {
	case C.kCGEventKeyDown:
		isAutorepeat := C.isAutorepeatEvent(event) != false

		if isAutorepeat {
			// Suppress macOS autorepeat - we handle it ourselves
			mu.RLock()
			state, exists := keyStates[keycode]
			isOurKey := exists && state.isHeld
			mu.RUnlock()

			if isOurKey {
				return C.nullEventRef() // Suppress the event
			}
		} else {
			// Initial key press - start our accelerating repeat
			startKeyRepeat(keycode)
		}

	case C.kCGEventKeyUp:
		stopKeyRepeat(keycode)

	case C.kCGEventFlagsChanged:
		// Check for shift key taps
		flags := uint64(C.getEventFlags(event))
		isShiftDown := (flags & uint64(C.kCGEventFlagMaskShift)) != 0

		if !isShiftDown && (keycode == leftShiftKeycode || keycode == rightShiftKeycode) {
			// Shift was released - count as a tap
			handleShiftTap()
		}
	}

	return event
}
