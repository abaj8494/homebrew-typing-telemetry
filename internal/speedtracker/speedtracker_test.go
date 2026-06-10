package speedtracker

import (
	"testing"
	"time"
)

// base is an arbitrary fixed instant; tests build timestamps relative to it.
// It is aligned to a minute boundary so clock-minute bucketing is predictable.
var base = time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)

func at(ms int) time.Time { return base.Add(time.Duration(ms) * time.Millisecond) }

func TestOnKeystrokeFirstCreditsZero(t *testing.T) {
	tr := New()
	if got := tr.OnKeystroke(at(0)); got != 0 {
		t.Fatalf("first keystroke: want 0, got %d", got)
	}
}

func TestOnKeystrokeCreditsGap(t *testing.T) {
	tr := New()
	tr.OnKeystroke(at(0))
	if got := tr.OnKeystroke(at(120)); got != 120 {
		t.Fatalf("want 120, got %d", got)
	}
	if got := tr.OnKeystroke(at(200)); got != 80 {
		t.Fatalf("want 80, got %d", got)
	}
}

func TestOnKeystrokeIdleGapPaused(t *testing.T) {
	tr := New()
	tr.OnKeystroke(at(0))
	// Gap longer than IdleCapMs is treated as idle: credits nothing.
	if got := tr.OnKeystroke(at(IdleCapMs + 500)); got != 0 {
		t.Fatalf("idle gap: want 0, got %d", got)
	}
	// The clock still advances, so the next keystroke measures from the idle one.
	if got := tr.OnKeystroke(at(IdleCapMs + 600)); got != 100 {
		t.Fatalf("after idle: want 100, got %d", got)
	}
}

func TestOnKeystrokeExactlyAtCap(t *testing.T) {
	tr := New()
	tr.OnKeystroke(at(0))
	if got := tr.OnKeystroke(at(IdleCapMs)); got != IdleCapMs {
		t.Fatalf("gap at cap: want %d, got %d", IdleCapMs, got)
	}
}

func TestOnKeystrokeBackwardsClock(t *testing.T) {
	tr := New()
	tr.OnKeystroke(at(500))
	if got := tr.OnKeystroke(at(400)); got != 0 {
		t.Fatalf("backwards clock: want 0, got %d", got)
	}
}

func TestActiveTimeAccumulatesOverBurst(t *testing.T) {
	tr := New()
	var total int64
	// 6 keystrokes, 100ms apart: 5 intervals * 100ms = 500ms active.
	for i := 0; i < 6; i++ {
		total += tr.OnKeystroke(at(i * 100))
	}
	if total != 500 {
		t.Fatalf("want 500ms active, got %d", total)
	}
}

// word advances OnWord and returns the sample, for readability in tests.
func word(tr *Tracker, ms int) Sample { return tr.OnWord(at(ms)) }

func TestBurstNeedsFullWindow(t *testing.T) {
	tr := New()
	// The first WindowWords words cannot complete a WindowWords-word segment.
	for i := 0; i < WindowWords; i++ {
		if s := word(tr, i*1000); s.Burst != 0 {
			t.Fatalf("word %d: burst should be 0 until window fills, got %v", i, s.Burst)
		}
	}
	// The (WindowWords+1)-th word at 1s spacing => WindowWords words over
	// WindowWords seconds => 60 WPM.
	s := word(tr, WindowWords*1000)
	if s.Burst != 60 {
		t.Fatalf("burst: want 60, got %v", s.Burst)
	}
}

func TestBurstAboveCeilingDropped(t *testing.T) {
	tr := New()
	// WindowWords+1 words, 100ms apart => WindowWords words over 1s => 600 WPM,
	// which exceeds MaxWPM and must be discarded.
	var last Sample
	for i := 0; i <= WindowWords; i++ {
		last = word(tr, i*100)
	}
	if last.Burst != 0 {
		t.Fatalf("burst above ceiling: want 0, got %v", last.Burst)
	}
}

func TestWindowCountsTrailingMinute(t *testing.T) {
	tr := New()
	// 5 words inside the first 5 seconds.
	var s Sample
	for i := 0; i < 5; i++ {
		s = word(tr, i*1000)
	}
	if s.Window != 5 {
		t.Fatalf("window: want 5, got %v", s.Window)
	}
	// A word well past the 60s window leaves only itself inside the window.
	s = word(tr, 70*1000)
	if s.Window != 1 {
		t.Fatalf("window after gap: want 1, got %v", s.Window)
	}
}

func TestMinuteBucketResetsOnNewMinute(t *testing.T) {
	tr := New()
	// Three words in the first clock-minute.
	if s := word(tr, 0); s.Minute != 1 {
		t.Fatalf("minute: want 1, got %v", s.Minute)
	}
	if s := word(tr, 10*1000); s.Minute != 2 {
		t.Fatalf("minute: want 2, got %v", s.Minute)
	}
	if s := word(tr, 20*1000); s.Minute != 3 {
		t.Fatalf("minute: want 3, got %v", s.Minute)
	}
	// Cross into the next clock-minute: count resets.
	if s := word(tr, 65*1000); s.Minute != 1 {
		t.Fatalf("minute after rollover: want 1, got %v", s.Minute)
	}
}

func TestPruneKeepsTrackerBounded(t *testing.T) {
	tr := New()
	// Feed many words spread over minutes; the retained slice must stay small.
	for i := 0; i < 500; i++ {
		word(tr, i*1000)
	}
	if len(tr.wordTimes) > RollWindowSec+WindowWords+1 {
		t.Fatalf("wordTimes grew unbounded: %d entries", len(tr.wordTimes))
	}
}
