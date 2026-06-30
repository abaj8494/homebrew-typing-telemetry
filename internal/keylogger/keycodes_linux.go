//go:build linux

package keylogger

import "github.com/aayushbajaj/typing-telemetry/internal/x11"

// X11 keycodes are the Linux evdev key code plus 8. The downstream pipeline
// (wordcounter, storage.ClassifyKeycode) is written against macOS virtual
// keycodes, so we translate evdev -> macOS here. Only the keys those consumers
// care about need entries: letters, the digit row, common punctuation, the
// whitespace/edit keys (space, return, tab, backspace), and the modifiers.
//
// evdev codes come from <linux/input-event-codes.h>; macOS values match the
// constants in internal/wordcounter and the classification in storage.
const xKeycodeOffset = 8

var evdevToMac = map[int]int{
	// Letters
	30: 0, 31: 1, 32: 2, 33: 3, 34: 5, 35: 4, 36: 38, 37: 40, 38: 37, // A S D F G H J K L
	16: 12, 17: 13, 18: 14, 19: 15, 20: 17, 21: 16, 22: 32, 23: 34, 24: 31, 25: 35, // Q W E R T Y U I O P
	44: 6, 45: 7, 46: 8, 47: 9, 48: 11, 49: 45, 50: 46, // Z X C V B N M

	// Digit row
	2: 18, 3: 19, 4: 20, 5: 21, 6: 23, 7: 22, 8: 26, 9: 28, 10: 25, 11: 29, // 1..9 0

	// Punctuation
	12: 27, // minus
	13: 24, // equal
	26: 33, // [
	27: 30, // ]
	43: 42, // backslash
	39: 41, // ;
	40: 39, // '
	51: 43, // ,
	52: 47, // .
	53: 44, // /
	41: 50, // `

	// Whitespace / editing
	57:  49,  // space
	28:  36,  // enter -> Return
	15:  48,  // tab
	14:  51,  // backspace -> Delete
	111: 117, // delete (forward) -> ForwardDel

	// Modifiers (classified as "modifier" by storage.ClassifyKeycode)
	42: 56, 54: 60, // left/right shift
	29: 59, 97: 62, // left/right control
	56: 58, 100: 61, // left/right alt -> option
	125: 55, 126: 54, // left/right meta -> command
	58: 57, // caps lock
}

// translate converts a raw X keycode into a macOS-space keycode. Unmapped keys
// (function keys, arrows, keypad, media keys, …) get a high sentinel so they
// still register as a "special" keystroke without ever being mistaken for a
// content or whitespace key.
func translate(xcode uint8) int {
	ev := int(xcode) - xKeycodeOffset
	if mac, ok := evdevToMac[ev]; ok {
		return mac
	}
	return 0x20000 + ev
}

// Modifier keys, as raw X keycodes (evdev + 8), grouped by the flag they set.
var (
	xShift = []uint8{42 + xKeycodeOffset, 54 + xKeycodeOffset}
	xCtrl  = []uint8{29 + xKeycodeOffset, 97 + xKeycodeOffset}
	xAlt   = []uint8{56 + xKeycodeOffset, 100 + xKeycodeOffset}
	xSuper = []uint8{125 + xKeycodeOffset, 126 + xKeycodeOffset}
)

func anyDown(m x11.Keymap, codes []uint8) bool {
	for _, c := range codes {
		if m.Down(c) {
			return true
		}
	}
	return false
}

// flagsFromKeymap derives the CGEventFlags-compatible modifier mask from the
// physical key state.
func flagsFromKeymap(m x11.Keymap) uint64 {
	var f uint64
	if anyDown(m, xShift) {
		f |= flagShift
	}
	if anyDown(m, xCtrl) {
		f |= flagCtrl
	}
	if anyDown(m, xAlt) {
		f |= flagOpt
	}
	if anyDown(m, xSuper) {
		f |= flagCmd
	}
	return f
}
