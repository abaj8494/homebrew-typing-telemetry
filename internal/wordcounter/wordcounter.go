// Package wordcounter detects completed words from a keystroke stream.
//
// The counter is intentionally stricter than the previous space/return/tab
// heuristic: a word boundary only completes a word when at least one printable
// character has been typed since the previous boundary, modifier-held key
// presses (Cmd, Ctrl) are treated as shortcuts and ignored, and backspace
// rolls back the in-progress character count.
package wordcounter

// macOS virtual keycodes used by the counter.
const (
	kcA            = 0
	kcS            = 1
	kcD            = 2
	kcF            = 3
	kcH            = 4
	kcG            = 5
	kcZ            = 6
	kcX            = 7
	kcC            = 8
	kcV            = 9
	kcB            = 11
	kcQ            = 12
	kcW            = 13
	kcE            = 14
	kcR            = 15
	kcY            = 16
	kcT            = 17
	kc1            = 18
	kc2            = 19
	kc3            = 20
	kc4            = 21
	kc6            = 22
	kc5            = 23
	kcEqual        = 24
	kc9            = 25
	kc7            = 26
	kcMinus        = 27
	kc8            = 28
	kc0            = 29
	kcRightBracket = 30
	kcO            = 31
	kcU            = 32
	kcLeftBracket  = 33
	kcI            = 34
	kcP            = 35
	kcReturn       = 36
	kcL            = 37
	kcJ            = 38
	kcQuote        = 39
	kcK            = 40
	kcSemicolon    = 41
	kcBackslash    = 42
	kcComma        = 43
	kcSlash        = 44
	kcN            = 45
	kcM            = 46
	kcPeriod       = 47
	kcTab          = 48
	kcSpace        = 49
	kcBacktick     = 50
	kcDelete       = 51 // Backspace (left of "=")
	kcForwardDel   = 117
)

// Counter is a stateful word-boundary detector.
// Not goroutine-safe — owned by the single keystroke goroutine.
type Counter struct {
	contentChars int
}

// New returns a fresh Counter.
func New() *Counter { return &Counter{} }

// Event describes one keystroke as seen by the counter.
type Event struct {
	Keycode  int
	CmdHeld  bool
	CtrlHeld bool
	// OptHeld and ShiftHeld are accepted for symmetry but do not change behavior:
	// both modifiers regularly participate in text input.
	OptHeld   bool
	ShiftHeld bool
}

// Observe processes one keystroke and returns true iff a word just completed.
func (c *Counter) Observe(e Event) bool {
	// Shortcut combos (Cmd or Ctrl held) never affect word state. This filters
	// out Cmd+Space (Spotlight), Cmd+Tab (app switcher), Cmd+S (save), Ctrl+A
	// (start of line in terminals), etc.
	if e.CmdHeld || e.CtrlHeld {
		return false
	}

	switch e.Keycode {
	case kcDelete, kcForwardDel:
		if c.contentChars > 0 {
			c.contentChars--
		}
		return false
	case kcSpace, kcReturn, kcTab:
		if c.contentChars > 0 {
			c.contentChars = 0
			return true
		}
		return false
	}

	if isContentKey(e.Keycode) {
		c.contentChars++
	}
	return false
}

// Reset clears in-progress state. Intended for tests and for "boundary"
// events like app switches when strict mode is enabled.
func (c *Counter) Reset() { c.contentChars = 0 }

// isContentKey reports whether a keycode represents a printable character
// that should grow the in-progress word.
func isContentKey(keycode int) bool {
	switch keycode {
	// Letters
	case kcA, kcB, kcC, kcD, kcE, kcF, kcG, kcH, kcI, kcJ, kcK, kcL, kcM,
		kcN, kcO, kcP, kcQ, kcR, kcS, kcT, kcU, kcV, kcW, kcX, kcY, kcZ:
		return true
	// Digit row
	case kc0, kc1, kc2, kc3, kc4, kc5, kc6, kc7, kc8, kc9:
		return true
	// Common punctuation that appears mid-word or as part of typed content
	case kcMinus, kcEqual, kcLeftBracket, kcRightBracket, kcBackslash,
		kcSemicolon, kcQuote, kcComma, kcPeriod, kcSlash, kcBacktick:
		return true
	}
	return false
}
