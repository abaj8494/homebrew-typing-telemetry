//go:build darwin
// +build darwin

package main

import (
	"fmt"
	"log"
	"sync"

	"github.com/aayushbajaj/typing-telemetry/internal/speedtracker"
	"github.com/aayushbajaj/typing-telemetry/internal/storage"
)

// speedState batches typing-speed measurements produced by the keystroke
// goroutine so the database is written on the stats ticker rather than once per
// keystroke. Active time is accumulated as additive deltas; fastest paces are
// kept as running maxima (UpdateFastest is MAX-based, so re-flushing a max is
// harmless). It is safe for concurrent use.
type speedState struct {
	mu        sync.Mutex
	pendingMs map[string]int64               // date -> un-flushed active ms
	dailyMax  map[string]speedtracker.Sample // date -> running fastest maxima
}

func newSpeedState() *speedState {
	return &speedState{
		pendingMs: make(map[string]int64),
		dailyMax:  make(map[string]speedtracker.Sample),
	}
}

// speedAcc is shared between the keystroke goroutine (writer) and the stats
// ticker (flusher). speedTracker, by contrast, is owned solely by the
// keystroke goroutine and needs no locking.
var (
	speedAcc     = newSpeedState()
	speedTracker = speedtracker.New()
)

// addActive credits active typing time for a day.
func (s *speedState) addActive(date string, ms int64) {
	if ms <= 0 {
		return
	}
	s.mu.Lock()
	s.pendingMs[date] += ms
	s.mu.Unlock()
}

// recordSample folds a fastest-pace sample into the day's running maxima.
func (s *speedState) recordSample(date string, sample speedtracker.Sample) {
	s.mu.Lock()
	cur := s.dailyMax[date]
	if sample.Burst > cur.Burst {
		cur.Burst = sample.Burst
	}
	if sample.Window > cur.Window {
		cur.Window = sample.Window
	}
	if sample.Minute > cur.Minute {
		cur.Minute = sample.Minute
	}
	s.dailyMax[date] = cur
	s.mu.Unlock()
}

// flush writes the accumulated active time and fastest paces to storage and
// clears the in-memory buffers. Active-time entries must be cleared (they are
// additive deltas); fastest entries are safe to clear because the stored value
// is a maximum that AddFastest will never lower.
func (s *speedState) flush(store *storage.Store) {
	s.mu.Lock()
	pendingMs := s.pendingMs
	dailyMax := s.dailyMax
	s.pendingMs = make(map[string]int64)
	s.dailyMax = make(map[string]speedtracker.Sample)
	s.mu.Unlock()

	for date, ms := range pendingMs {
		if err := store.AddActiveTime(date, ms); err != nil {
			log.Printf("Failed to flush active time: %v", err)
		}
	}
	for date, m := range dailyMax {
		if err := store.UpdateFastest(date, m.Burst, m.Window, m.Minute); err != nil {
			log.Printf("Failed to flush fastest pace: %v", err)
		}
	}
}

// formatWPM renders a words-per-minute value for the menu, showing a placeholder
// when no pace has been recorded yet.
func formatWPM(wpm float64) string {
	if wpm <= 0 {
		return "-- WPM"
	}
	return fmt.Sprintf("%.0f WPM", wpm)
}
