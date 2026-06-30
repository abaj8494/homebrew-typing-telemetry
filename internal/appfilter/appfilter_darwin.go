//go:build darwin
// +build darwin

// Package appfilter exposes the frontmost macOS application's bundle ID and
// a thread-safe allowlist used by typing-telemetry's "strict word counting"
// mode to drop keystrokes that occur in non-allowlisted apps.
package appfilter

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#include <Cocoa/Cocoa.h>
#include <stdlib.h>
#include <string.h>

// Returns a malloc'd C string with the frontmost app's bundle identifier,
// or NULL if it cannot be determined. The Go caller must free() the result.
static char *frontmostBundleID(void) {
    @autoreleasepool {
        NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
        if (app == nil) {
            return NULL;
        }
        NSString *bundleID = [app bundleIdentifier];
        if (bundleID == nil) {
            return NULL;
        }
        const char *cstr = [bundleID UTF8String];
        if (cstr == NULL) {
            return NULL;
        }
        return strdup(cstr);
    }
}
*/
import "C"

import (
	"sync"
	"unsafe"
)

// Frontmost returns the bundle identifier of the currently frontmost
// application (e.g. "com.apple.Safari"), or "" if it cannot be determined.
// Safe to call from any goroutine; the underlying NSWorkspace property is
// thread-safe per Apple's documentation.
func Frontmost() string {
	p := C.frontmostBundleID()
	if p == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p)
}

// Filter holds a thread-safe allowlist of bundle IDs.
type Filter struct {
	mu      sync.RWMutex
	allow   map[string]struct{}
	enabled bool
}

// New returns an empty, disabled filter. When the filter is disabled,
// IsAllowed always returns true (i.e. nothing is filtered out).
func New() *Filter {
	return &Filter{allow: map[string]struct{}{}}
}

// SetEnabled toggles whether the allowlist actually filters.
func (f *Filter) SetEnabled(enabled bool) {
	f.mu.Lock()
	f.enabled = enabled
	f.mu.Unlock()
}

// Enabled reports whether the filter is currently enforcing the allowlist.
func (f *Filter) Enabled() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.enabled
}

// SetAllowlist replaces the allowlist atomically.
func (f *Filter) SetAllowlist(bundleIDs []string) {
	set := make(map[string]struct{}, len(bundleIDs))
	for _, id := range bundleIDs {
		if id == "" {
			continue
		}
		set[id] = struct{}{}
	}
	f.mu.Lock()
	f.allow = set
	f.mu.Unlock()
}

// IsAllowed reports whether keystrokes from the given bundle ID should count.
// When the filter is disabled, every bundle is allowed.
func (f *Filter) IsAllowed(bundleID string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if !f.enabled {
		return true
	}
	if bundleID == "" {
		// Unknown frontmost — be conservative and drop, matching Feather's
		// behavior of only counting when context is recognized.
		return false
	}
	_, ok := f.allow[bundleID]
	return ok
}
