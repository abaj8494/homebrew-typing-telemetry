//go:build darwin
// +build darwin

package keylogger

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation -framework ApplicationServices -framework AudioToolbox

#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>

extern void goKeystrokeCallback(int keycode, int isRepeat, int isInertia);
extern void goModifierCallback(int keycode);

// Magic value to identify inertia-generated synthetic events (must match inertia package)
#define INERTIA_EVENT_MARKER 0x494E4552

// Track previous modifier flags to detect key down vs key up
static CGEventFlags previousFlags = 0;

static CGEventRef eventCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon) {
    if (type == kCGEventKeyDown) {
        CGKeyCode keycode = (CGKeyCode)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
        // Check if this is a key repeat event (holding key down)
        int isRepeat = (int)CGEventGetIntegerValueField(event, kCGKeyboardEventAutorepeat);
        // Check if this is a synthetic event from inertia
        int isInertia = (CGEventGetIntegerValueField(event, kCGEventSourceUserData) == INERTIA_EVENT_MARKER) ? 1 : 0;
        goKeystrokeCallback((int)keycode, isRepeat, isInertia);
    } else if (type == kCGEventFlagsChanged) {
        // Handle modifier key presses (Shift, Ctrl, Command, Option, etc.)
        CGEventFlags currentFlags = CGEventGetFlags(event);
        CGKeyCode keycode = (CGKeyCode)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);

        // Check if this is a key down (flag added) by comparing with previous state
        // We detect key down when a modifier flag is newly set
        CGEventFlags diff = currentFlags ^ previousFlags;
        int isKeyDown = (currentFlags & diff) != 0;

        if (isKeyDown) {
            goModifierCallback((int)keycode);
        }

        previousFlags = currentFlags;
    }
    return event;
}

static CFMachPortRef createEventTap() {
    CGEventMask eventMask = CGEventMaskBit(kCGEventKeyDown) | CGEventMaskBit(kCGEventFlagsChanged);
    CFMachPortRef eventTap = CGEventTapCreate(
        kCGSessionEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionListenOnly,
        eventMask,
        eventCallback,
        NULL
    );
    return eventTap;
}

static int isEventTapValid(CFMachPortRef eventTap) {
    return eventTap != NULL;
}

static int checkAccessibilityPermissions() {
    return AXIsProcessTrusted();
}

// Get current modifier flags from the system (not tracked state)
static uint64_t getCurrentModifierFlags() {
    return (uint64_t)CGEventSourceFlagsState(kCGEventSourceStateCombinedSessionState);
}

// Play a system sound (uses AudioServices)
#include <AudioToolbox/AudioToolbox.h>
static void playSystemSound(int soundID) {
    AudioServicesPlaySystemSound((SystemSoundID)soundID);
}

static void runEventLoop(CFMachPortRef eventTap) {
    CFRunLoopSourceRef runLoopSource = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, eventTap, 0);
    CFRunLoopAddSource(CFRunLoopGetCurrent(), runLoopSource, kCFRunLoopCommonModes);
    CGEventTapEnable(eventTap, true);
    CFRunLoopRun();
}
*/
import "C"
import (
	"errors"
	"sync"
)

var (
	keystrokeChan chan int
	mu            sync.Mutex
	running       bool
)

//export goKeystrokeCallback
func goKeystrokeCallback(keycode C.int, isRepeat C.int, isInertia C.int) {
	// Ignore key repeat events - holding a key counts as 1 keypress
	if isRepeat != 0 {
		return
	}

	// Ignore synthetic events from inertia - holding with inertia counts as 1 keypress
	if isInertia != 0 {
		return
	}

	mu.Lock()
	defer mu.Unlock()
	if keystrokeChan != nil {
		select {
		case keystrokeChan <- int(keycode):
		default:
			// Channel full, drop keystroke
		}
	}
}

//export goModifierCallback
func goModifierCallback(keycode C.int) {
	// Handle modifier key press (solo press of Shift, Ctrl, Command, etc.)
	mu.Lock()
	defer mu.Unlock()
	if keystrokeChan != nil {
		select {
		case keystrokeChan <- int(keycode):
		default:
			// Channel full, drop keystroke
		}
	}
}

// CheckAccessibilityPermissions returns true if the app has accessibility permissions
func CheckAccessibilityPermissions() bool {
	return C.checkAccessibilityPermissions() != 0
}

// ModifierFlags represents the current state of modifier keys
type ModifierFlags struct {
	Cmd   bool
	Ctrl  bool
	Opt   bool
	Shift bool
}

// GetCurrentModifiers returns the current modifier key state from the system
// This queries the actual hardware state, not tracked state
func GetCurrentModifiers() ModifierFlags {
	flags := uint64(C.getCurrentModifierFlags())
	return ModifierFlags{
		Cmd:   flags&(1<<20) != 0, // kCGEventFlagMaskCommand
		Ctrl:  flags&(1<<18) != 0, // kCGEventFlagMaskControl
		Opt:   flags&(1<<19) != 0, // kCGEventFlagMaskAlternate
		Shift: flags&(1<<17) != 0, // kCGEventFlagMaskShift
	}
}

// System sound IDs for macOS
const (
	SoundTink     = 1103 // Short tink sound
	SoundPop      = 1104 // Pop sound
	SoundBoop     = 1105 // Boop sound
	SoundGlass    = 1107 // Glass sound (good for activation)
	SoundMorse    = 1108 // Morse code sound
	SoundPurr     = 1110 // Purr sound (good for deactivation)
	SoundHero     = 1114 // Hero sound
	SoundSubmerge = 1117 // Submerge sound
)

// PlaySound plays a system sound by ID
func PlaySound(soundID int) {
	C.playSystemSound(C.int(soundID))
}

// Start begins capturing keystrokes and returns a channel that receives keycodes
func Start() (<-chan int, error) {
	mu.Lock()
	defer mu.Unlock()

	if running {
		return nil, errors.New("keylogger already running")
	}

	if !CheckAccessibilityPermissions() {
		return nil, errors.New("accessibility permissions not granted - please enable in System Preferences > Privacy & Security > Accessibility")
	}

	keystrokeChan = make(chan int, 1000)

	go func() {
		eventTap := C.createEventTap()
		if C.isEventTapValid(eventTap) == 0 {
			return
		}
		mu.Lock()
		running = true
		mu.Unlock()
		C.runEventLoop(eventTap)
	}()

	return keystrokeChan, nil
}

// Stop stops the keylogger
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if keystrokeChan != nil {
		close(keystrokeChan)
		keystrokeChan = nil
	}
	running = false
}
