package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aayushbajaj/typing-telemetry/internal/storage"
	"github.com/aayushbajaj/typing-telemetry/pkg/stats"
)

// pixelsPerInch matches DefaultPPI used by the charts renderer. Kept as a
// constant here so the JSON surface stays stable even if the chart code
// changes its conversion approach later.
const pixelsPerInch = 100.0

// metersPerInch is the SI conversion factor.
const metersPerInch = 0.0254

// TodayJSON is the stable schema returned by `typtel today --json`.
// Field names are snake_case so consumers in shell pipelines and other
// languages can parse them with conventional tooling. Mouse distance is
// reported in both raw pixels (lossless) and metres (human-friendly).
type TodayJSON struct {
	Date            string  `json:"date"`
	Keystrokes      int64   `json:"keystrokes"`
	Words           int64   `json:"words"`
	Letters         int64   `json:"letters"`
	Modifiers       int64   `json:"modifiers"`
	Special         int64   `json:"special"`
	MouseClicks     int64   `json:"mouse_clicks"`
	MouseDistancePx float64 `json:"mouse_distance_px"`
	MouseDistanceM  float64 `json:"mouse_distance_m"`
	ActiveHours     int     `json:"active_hours"`
	AvgWPM          float64 `json:"avg_wpm"`
}

// DayJSON is a per-day breakdown used inside StatsJSON.
type DayJSON struct {
	Date       string `json:"date"`
	Keystrokes int64  `json:"keystrokes"`
	Words      int64  `json:"words"`
}

// StatsJSON is the stable schema returned by `typtel stats --json`. It
// covers today plus the trailing 7-day window so a consumer can render a
// summary without making multiple calls. The week slice is chronological
// (oldest first) to match what the underlying storage layer returns.
type StatsJSON struct {
	Today        TodayJSON `json:"today"`
	Week         []DayJSON `json:"week"`
	WeekTotals   DayJSON   `json:"week_totals"`
	WeekAverages struct {
		Keystrokes float64 `json:"keystrokes"`
		Words      float64 `json:"words"`
	} `json:"week_averages"`
	Speed SpeedJSON `json:"speed"`

	// Devices is an additive, optional map of external-device feeds keyed by
	// device_id. It is omitted entirely when no devices are registered, so the
	// macOS watchdog's existing contract is byte-unchanged.
	Devices map[string]DeviceJSON `json:"devices,omitempty"`
}

// DeviceJSON is one external device's entry in the optional "devices" block.
// Today reuses the storage counts shape (absolute totals for today's date).
type DeviceJSON struct {
	Name     string                  `json:"name"`
	Today    storage.DeviceDayCounts `json:"today"`
	LastSeen string                  `json:"last_seen"`
}

// SpeedJSON is the typing-speed section of `typtel stats --json`. AvgWPM is
// keyed by rolling window ("day", "week", "month", "year", "all"); Fastest
// holds the all-time best pace for each of the three tracked methods.
type SpeedJSON struct {
	AvgWPM  map[string]float64 `json:"avg_wpm"`
	Fastest struct {
		BurstWPM  float64 `json:"burst_wpm"`
		WindowWPM float64 `json:"window_wpm"`
		MinuteWPM float64 `json:"minute_wpm"`
	} `json:"fastest"`
}

func pixelsToMeters(px float64) float64 {
	return (px / pixelsPerInch) * metersPerInch
}

// buildTodayJSON assembles the today payload from storage. Errors from the
// mouse and hourly queries are swallowed (with zero defaults) so that an
// empty install — no mouse data, no hourly data — still produces a valid
// JSON document. Keystroke/word failure is fatal because that's the
// primary signal.
func buildTodayJSON(store *storage.Store) (TodayJSON, error) {
	// Ensure historical active time exists so avg_wpm is meaningful even if the
	// menubar hasn't run since upgrading. Guarded — runs at most once.
	if err := store.BackfillActiveTime(); err != nil {
		return TodayJSON{}, fmt.Errorf("backfill active time: %w", err)
	}

	date := time.Now().Format("2006-01-02")
	day, err := store.GetTodayStats()
	if err != nil {
		return TodayJSON{}, fmt.Errorf("get today stats: %w", err)
	}

	out := TodayJSON{
		Date:       date,
		Keystrokes: day.Keystrokes,
		Words:      day.Words,
		Letters:    day.Letters,
		Modifiers:  day.Modifiers,
		Special:    day.Special,
	}

	if mouse, err := store.GetTodayMouseStats(); err == nil && mouse != nil {
		out.MouseClicks = mouse.ClickCount
		out.MouseDistancePx = mouse.TotalDistance
		out.MouseDistanceM = pixelsToMeters(mouse.TotalDistance)
	}

	if hourly, err := store.GetHourlyStats(date); err == nil {
		for _, h := range hourly {
			if h.Keystrokes > 0 {
				out.ActiveHours++
			}
		}
	}

	if speed, err := store.GetSpeedAggregate(date); err == nil {
		out.AvgWPM = stats.AverageWPM(speed.Words, speed.ActiveMs)
	}

	return out, nil
}

func runTodayJSON() error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	payload, err := buildTodayJSON(store)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func runStatsJSON() error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	today, err := buildTodayJSON(store)
	if err != nil {
		return err
	}

	week, err := store.GetWeekStats()
	if err != nil {
		return fmt.Errorf("get week stats: %w", err)
	}

	stats := StatsJSON{Today: today}
	stats.Week = make([]DayJSON, 0, len(week))
	var totalK, totalW int64
	for _, d := range week {
		stats.Week = append(stats.Week, DayJSON{
			Date:       d.Date,
			Keystrokes: d.Keystrokes,
			Words:      d.Words,
		})
		totalK += d.Keystrokes
		totalW += d.Words
	}
	stats.WeekTotals = DayJSON{Keystrokes: totalK, Words: totalW}
	if n := len(week); n > 0 {
		stats.WeekAverages.Keystrokes = float64(totalK) / float64(n)
		stats.WeekAverages.Words = float64(totalW) / float64(n)
	}

	speed, err := buildSpeedJSON(store)
	if err != nil {
		return fmt.Errorf("get speed stats: %w", err)
	}
	stats.Speed = speed

	devices, err := buildDevicesJSON(store)
	if err != nil {
		return fmt.Errorf("get device stats: %w", err)
	}
	stats.Devices = devices

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(stats)
}

// buildDevicesJSON assembles the optional "devices" block. It returns nil when
// no devices are registered so the key is omitted entirely (omitempty),
// preserving the byte-for-byte JSON contract for installs that never use the
// ingest API. Each device's "today" is its absolute counts for today's date.
func buildDevicesJSON(store *storage.Store) (map[string]DeviceJSON, error) {
	infos, err := store.ListDevices()
	if err != nil {
		return nil, err
	}
	if len(infos) == 0 {
		return nil, nil
	}

	today := time.Now().Format("2006-01-02")
	out := make(map[string]DeviceJSON, len(infos))
	for _, info := range infos {
		entry := DeviceJSON{Name: info.Name, LastSeen: info.LastSeen}
		if c, err := store.GetDeviceDay(info.DeviceID, today); err != nil {
			return nil, err
		} else if c != nil {
			entry.Today = *c
		}
		out[info.DeviceID] = entry
	}
	return out, nil
}

// buildSpeedJSON assembles the typing-speed section: average WPM over rolling
// windows and the all-time fastest paces. Windows mirror the menubar (today,
// trailing 7/30/365 days, all-time).
func buildSpeedJSON(store *storage.Store) (SpeedJSON, error) {
	now := time.Now()
	windows := []struct {
		key   string
		since string
	}{
		{"day", now.Format("2006-01-02")},
		{"week", now.AddDate(0, 0, -6).Format("2006-01-02")},
		{"month", now.AddDate(0, 0, -29).Format("2006-01-02")},
		{"year", now.AddDate(0, 0, -364).Format("2006-01-02")},
		{"all", ""},
	}

	out := SpeedJSON{AvgWPM: make(map[string]float64, len(windows))}
	var all storage.SpeedAggregate
	for _, w := range windows {
		agg, err := store.GetSpeedAggregate(w.since)
		if err != nil {
			return SpeedJSON{}, err
		}
		out.AvgWPM[w.key] = stats.AverageWPM(agg.Words, agg.ActiveMs)
		if w.key == "all" {
			all = agg
		}
	}
	out.Fastest.BurstWPM = all.FastestBurstWPM
	out.Fastest.WindowWPM = all.FastestWindowWPM
	out.Fastest.MinuteWPM = all.FastestMinuteWPM
	return out, nil
}
