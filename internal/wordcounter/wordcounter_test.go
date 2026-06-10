package wordcounter

import "testing"

// step is a tiny DSL row: a keycode + modifier flags + whether this event
// should be reported as a completed word by Observe.
type step struct {
	keycode int
	cmd     bool
	ctrl    bool
	opt     bool
	shift   bool
}

// run replays the steps and returns the total number of words completed.
func run(t *testing.T, steps []step) int {
	t.Helper()
	c := New()
	got := 0
	for i, s := range steps {
		if c.Observe(Event{
			Keycode:   s.keycode,
			CmdHeld:   s.cmd,
			CtrlHeld:  s.ctrl,
			OptHeld:   s.opt,
			ShiftHeld: s.shift,
		}) {
			got++
			t.Logf("step %d (keycode %d) completed a word", i, s.keycode)
		}
	}
	return got
}

func keys(codes ...int) []step {
	out := make([]step, len(codes))
	for i, k := range codes {
		out[i] = step{keycode: k}
	}
	return out
}

func TestSingleWordSpace(t *testing.T) {
	// h e l l o SPACE -> 1
	if got := run(t, keys(kcH, kcE, kcL, kcL, kcO, kcSpace)); got != 1 {
		t.Fatalf("want 1, got %d", got)
	}
}

func TestTwoWordsEndingInReturn(t *testing.T) {
	// h e l l o SPACE w o r l d RETURN -> 2
	if got := run(t, keys(kcH, kcE, kcL, kcL, kcO, kcSpace, kcW, kcO, kcR, kcL, kcD, kcReturn)); got != 2 {
		t.Fatalf("want 2, got %d", got)
	}
}

func TestConsecutiveSpacesDoNotDoubleCount(t *testing.T) {
	// SPACE SPACE SPACE alone -> 0
	if got := run(t, keys(kcSpace, kcSpace, kcSpace)); got != 0 {
		t.Fatalf("want 0, got %d", got)
	}
}

func TestSpotlightCmdSpace(t *testing.T) {
	// Cmd+SPACE invokes Spotlight; must not count.
	steps := []step{{keycode: kcSpace, cmd: true}}
	if got := run(t, steps); got != 0 {
		t.Fatalf("want 0, got %d", got)
	}
}

func TestCmdSDoesNotPolluteContent(t *testing.T) {
	// Cmd+S then h i SPACE -> 1 (the "S" while Cmd was held must NOT count as content)
	steps := []step{
		{keycode: kcS, cmd: true},
		{keycode: kcH},
		{keycode: kcI},
		{keycode: kcSpace},
	}
	if got := run(t, steps); got != 1 {
		t.Fatalf("want 1, got %d", got)
	}
}

func TestBackspaceDoesNotUncountWord(t *testing.T) {
	// h i BACKSPACE BACKSPACE SPACE -> 1
	// WordCounter never rolls back: once a word is armed, the trailing space
	// commits it regardless of how much was backspaced. (Old behaviour: 0.)
	if got := run(t, keys(kcH, kcI, kcDelete, kcDelete, kcSpace)); got != 1 {
		t.Fatalf("want 1, got %d", got)
	}
}

func TestBackspacePartialStillCounts(t *testing.T) {
	// h e l l o BACKSPACE SPACE -> still 1
	if got := run(t, keys(kcH, kcE, kcL, kcL, kcO, kcDelete, kcSpace)); got != 1 {
		t.Fatalf("want 1, got %d", got)
	}
}

func TestLoneBackspaceThenSpaceDoesNotCount(t *testing.T) {
	// BACKSPACE SPACE with no content typed -> 0
	// Backspace is a no-op, so it must not arm a phantom word.
	if got := run(t, keys(kcDelete, kcSpace)); got != 0 {
		t.Fatalf("want 0, got %d", got)
	}
}

func TestLoneReturnIgnored(t *testing.T) {
	// RETURN with no preceding content (e.g. dialog confirm) -> 0
	if got := run(t, keys(kcReturn)); got != 0 {
		t.Fatalf("want 0, got %d", got)
	}
}

func TestLoneTabIgnored(t *testing.T) {
	// TAB with no preceding content (e.g. dialog focus shift) -> 0
	if got := run(t, keys(kcTab)); got != 0 {
		t.Fatalf("want 0, got %d", got)
	}
}

func TestCmdTabAppSwitcher(t *testing.T) {
	// Cmd+TAB cycles apps; must not count.
	steps := []step{{keycode: kcTab, cmd: true}}
	if got := run(t, steps); got != 0 {
		t.Fatalf("want 0, got %d", got)
	}
}

func TestDoubleEnterParagraph(t *testing.T) {
	// h i RETURN RETURN -> 1 (only the first newline closes a word)
	if got := run(t, keys(kcH, kcI, kcReturn, kcReturn)); got != 1 {
		t.Fatalf("want 1, got %d", got)
	}
}

func TestPunctuationCountsAsContent(t *testing.T) {
	// "..." SPACE  -> 1   (three periods is still content)
	if got := run(t, keys(kcPeriod, kcPeriod, kcPeriod, kcSpace)); got != 1 {
		t.Fatalf("want 1, got %d", got)
	}
}

func TestDigitsCountAsContent(t *testing.T) {
	// 1 2 3 SPACE -> 1
	if got := run(t, keys(kc1, kc2, kc3, kcSpace)); got != 1 {
		t.Fatalf("want 1, got %d", got)
	}
}

func TestShiftAndOptionAreNotShortcuts(t *testing.T) {
	// Shift held while typing letters: still content, still completes a word.
	steps := []step{
		{keycode: kcH, shift: true},
		{keycode: kcI},
		{keycode: kcSpace},
	}
	if got := run(t, steps); got != 1 {
		t.Fatalf("want 1, got %d", got)
	}
}

func TestCtrlASelectAll(t *testing.T) {
	// Ctrl+A then SPACE -> 0 (Ctrl+A doesn't grow content; lone space doesn't fire)
	steps := []step{
		{keycode: kcA, ctrl: true},
		{keycode: kcSpace},
	}
	if got := run(t, steps); got != 0 {
		t.Fatalf("want 0, got %d", got)
	}
}

func TestArrowKeysIgnored(t *testing.T) {
	// Typing then arrow keys then space — arrows are not content but shouldn't reset.
	const arrowLeft = 123
	const arrowRight = 124
	steps := []step{
		{keycode: kcH},
		{keycode: kcI},
		{keycode: arrowLeft},
		{keycode: arrowRight},
		{keycode: kcSpace},
	}
	if got := run(t, steps); got != 1 {
		t.Fatalf("want 1, got %d", got)
	}
}
