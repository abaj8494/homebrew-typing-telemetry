// Package speedtracker turns a stream of keystroke and word-completion
// timestamps into typing-speed signals: active typing time (with idle
// auto-pause) and "fastest pace" records modelled after a Garmin run.
//
// It is deliberately pure — no storage, no keylogger, no clock of its own.
// The caller passes an explicit time.Time into every method, which keeps the
// timing maths trivially unit-testable. The Tracker is not goroutine-safe; it
// is owned by the single keystroke goroutine.
package speedtracker

import "time"

const (
	// WindowWords is the segment length for the fastest-burst metric: the
	// fastest WindowWords consecutive words, measuring time over a fixed word
	// "distance" (the typing analogue of Garmin's fastest-km split).
	WindowWords = 10

	// IdleCapMs caps the gap credited between two consecutive keystrokes. A
	// gap longer than this is treated as the user being idle and contributes
	// no active time (auto-pause).
	IdleCapMs = 2000

	// RollWindowSec is the sliding window for the fastest-60s metric.
	RollWindowSec = 60

	// MaxWPM is a sanity ceiling. Candidate paces above it (key-repeat bursts,
	// pastes, clock glitches) are discarded rather than recorded.
	MaxWPM = 400.0
)

// Sample holds the candidate fastest-pace WPM values produced by a single
// completed word. A field is 0 when no candidate is available for that metric
// (not enough history yet) or when the value exceeded MaxWPM and was dropped.
type Sample struct {
	Burst  float64 // fastest WindowWords-word segment ending at this word
	Window float64 // words completed in the trailing RollWindowSec, as WPM
	Minute float64 // words completed in the current clock-minute, as WPM
}

// Tracker accumulates the state needed to compute the speed signals.
type Tracker struct {
	hasLast bool
	lastKey time.Time

	// wordTimes holds recent word-completion times, oldest first. It is
	// pruned to the minimum needed to serve both the burst (last
	// WindowWords+1 entries) and the 60s window (entries within RollWindowSec).
	wordTimes []time.Time

	hasMinute   bool
	minuteKey   int64 // unix minute index of the current clock-minute bucket
	minuteCount int   // words completed in that clock-minute so far
}

// New returns a fresh Tracker.
func New() *Tracker { return &Tracker{} }

// OnKeystroke records a keystroke at time now and returns the active typing
// time to credit for it, in milliseconds. The first keystroke (and the first
// after an idle gap longer than IdleCapMs) credits nothing — there is no prior
// interval to count, or the interval was idle. Otherwise it credits the gap
// since the previous keystroke.
func (t *Tracker) OnKeystroke(now time.Time) int64 {
	if !t.hasLast {
		t.hasLast = true
		t.lastKey = now
		return 0
	}
	delta := now.Sub(t.lastKey).Milliseconds()
	t.lastKey = now
	if delta <= 0 || delta > IdleCapMs {
		return 0
	}
	return delta
}

// OnWord records a completed word at time now and returns the candidate
// fastest-pace WPM values for it. The caller keeps the running maximum of each
// field. Values at or below 0, or above MaxWPM, are reported as 0.
func (t *Tracker) OnWord(now time.Time) Sample {
	t.wordTimes = append(t.wordTimes, now)

	var s Sample

	// Burst: WindowWords words span from the (WindowWords+1)-th-last completion
	// to now. We need WindowWords+1 timestamps to bound that span.
	if n := len(t.wordTimes); n >= WindowWords+1 {
		start := t.wordTimes[n-1-WindowWords]
		if mins := now.Sub(start).Minutes(); mins > 0 {
			s.Burst = capWPM(float64(WindowWords) / mins)
		}
	}

	// Window: count completions within the trailing RollWindowSec. Because the
	// window is exactly one minute long, the count *is* the WPM.
	cutoff := now.Add(-RollWindowSec * time.Second)
	count := 0
	for _, wt := range t.wordTimes {
		if !wt.Before(cutoff) {
			count++
		}
	}
	s.Window = capWPM(float64(count))

	// Minute: words within the current wall-clock minute, as WPM.
	m := now.Unix() / 60
	if !t.hasMinute || m != t.minuteKey {
		t.hasMinute = true
		t.minuteKey = m
		t.minuteCount = 0
	}
	t.minuteCount++
	s.Minute = capWPM(float64(t.minuteCount))

	t.prune(cutoff)
	return s
}

// prune drops word timestamps that are older than the 60s window and no longer
// needed for the burst calculation, keeping the slice bounded.
func (t *Tracker) prune(cutoff time.Time) {
	for len(t.wordTimes) > WindowWords+1 && t.wordTimes[0].Before(cutoff) {
		t.wordTimes = t.wordTimes[1:]
	}
}

// capWPM returns v unless it is non-positive or exceeds the sanity ceiling, in
// which case it returns 0.
func capWPM(v float64) float64 {
	if v <= 0 || v > MaxWPM {
		return 0
	}
	return v
}
