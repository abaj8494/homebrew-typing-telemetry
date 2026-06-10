package storage

import (
	"testing"
	"time"
)

func TestAddActiveTimeAccumulates(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	const date = "2026-06-01"
	if err := store.AddActiveTime(date, 1500); err != nil {
		t.Fatalf("AddActiveTime: %v", err)
	}
	if err := store.AddActiveTime(date, 500); err != nil {
		t.Fatalf("AddActiveTime: %v", err)
	}
	// Non-positive deltas are no-ops.
	if err := store.AddActiveTime(date, 0); err != nil {
		t.Fatalf("AddActiveTime(0): %v", err)
	}

	day, err := store.GetDayStats(date)
	if err != nil {
		t.Fatalf("GetDayStats: %v", err)
	}
	if day.ActiveMs != 2000 {
		t.Fatalf("active_ms: want 2000, got %d", day.ActiveMs)
	}
}

func TestUpdateFastestKeepsMax(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	const date = "2026-06-01"
	if err := store.UpdateFastest(date, 50, 60, 40); err != nil {
		t.Fatalf("UpdateFastest: %v", err)
	}
	// Lower values must not overwrite the stored maxima.
	if err := store.UpdateFastest(date, 30, 70, 10); err != nil {
		t.Fatalf("UpdateFastest: %v", err)
	}

	day, err := store.GetDayStats(date)
	if err != nil {
		t.Fatalf("GetDayStats: %v", err)
	}
	if day.FastestBurstWPM != 50 {
		t.Fatalf("burst: want 50, got %v", day.FastestBurstWPM)
	}
	if day.FastestWindowWPM != 70 {
		t.Fatalf("window: want 70, got %v", day.FastestWindowWPM)
	}
	if day.FastestMinuteWPM != 40 {
		t.Fatalf("minute: want 40, got %v", day.FastestMinuteWPM)
	}
}

func TestGetSpeedAggregate(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	seed := func(date string, words int64, activeMs int64, burst float64) {
		for i := int64(0); i < words; i++ {
			if err := store.IncrementWordCount(date); err != nil {
				t.Fatalf("IncrementWordCount: %v", err)
			}
		}
		if err := store.AddActiveTime(date, activeMs); err != nil {
			t.Fatalf("AddActiveTime: %v", err)
		}
		if err := store.UpdateFastest(date, burst, 0, 0); err != nil {
			t.Fatalf("UpdateFastest: %v", err)
		}
	}

	seed("2026-06-01", 100, 60000, 70) // older day
	seed("2026-06-05", 50, 30000, 90)  // newer day, higher burst

	// All-time: both days.
	all, err := store.GetSpeedAggregate("")
	if err != nil {
		t.Fatalf("GetSpeedAggregate(all): %v", err)
	}
	if all.Words != 150 {
		t.Fatalf("all words: want 150, got %d", all.Words)
	}
	if all.ActiveMs != 90000 {
		t.Fatalf("all active_ms: want 90000, got %d", all.ActiveMs)
	}
	if all.FastestBurstWPM != 90 {
		t.Fatalf("all fastest burst: want 90, got %v", all.FastestBurstWPM)
	}

	// Range: only the newer day.
	recent, err := store.GetSpeedAggregate("2026-06-03")
	if err != nil {
		t.Fatalf("GetSpeedAggregate(range): %v", err)
	}
	if recent.Words != 50 {
		t.Fatalf("range words: want 50, got %d", recent.Words)
	}
	if recent.ActiveMs != 30000 {
		t.Fatalf("range active_ms: want 30000, got %d", recent.ActiveMs)
	}
	if recent.FastestBurstWPM != 90 {
		t.Fatalf("range fastest burst: want 90, got %v", recent.FastestBurstWPM)
	}
}

func TestGetSpeedAggregateEmpty(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	agg, err := store.GetSpeedAggregate("")
	if err != nil {
		t.Fatalf("GetSpeedAggregate on empty db: %v", err)
	}
	if agg.Words != 0 || agg.ActiveMs != 0 || agg.FastestBurstWPM != 0 {
		t.Fatalf("empty db should be all zero, got %+v", agg)
	}
}

func TestBackfillActiveTime(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Seed raw keystrokes directly with controlled timestamps.
	const date = "2026-06-01"
	insert := func(secOffset int) {
		ts := time.Date(2026, 6, 1, 12, 0, secOffset, 0, time.UTC).Format("2006-01-02 15:04:05")
		if _, err := store.db.Exec(
			"INSERT INTO keystrokes (keycode, date, hour, timestamp) VALUES (?, ?, ?, ?)",
			1, date, 12, ts,
		); err != nil {
			t.Fatalf("seed keystroke: %v", err)
		}
	}
	// Three keystrokes 1s apart => two 1000ms gaps => 2000ms active.
	insert(0)
	insert(1)
	insert(2)
	// A long idle gap (>2s cap) contributes nothing.
	insert(10)

	if err := store.BackfillActiveTime(); err != nil {
		t.Fatalf("BackfillActiveTime: %v", err)
	}

	day, err := store.GetDayStats(date)
	if err != nil {
		t.Fatalf("GetDayStats: %v", err)
	}
	if day.ActiveMs != 2000 {
		t.Fatalf("backfilled active_ms: want 2000, got %d", day.ActiveMs)
	}

	// Second run must be a no-op (guarded by the settings flag) — it must not
	// double-count.
	if err := store.BackfillActiveTime(); err != nil {
		t.Fatalf("BackfillActiveTime (2nd): %v", err)
	}
	day, err = store.GetDayStats(date)
	if err != nil {
		t.Fatalf("GetDayStats: %v", err)
	}
	if day.ActiveMs != 2000 {
		t.Fatalf("after 2nd backfill: want 2000 (no double-count), got %d", day.ActiveMs)
	}
}
