package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// newTestStore creates a test store with a temporary database
func newTestStore(t *testing.T) (*Store, func()) {
	t.Helper()

	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "typtel-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to open database: %v", err)
	}

	if err := initSchema(db); err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to init schema: %v", err)
	}

	store := &Store{db: db}
	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

func TestRecordKeystroke(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Record a keystroke
	err := store.RecordKeystroke(42)
	if err != nil {
		t.Fatalf("RecordKeystroke failed: %v", err)
	}

	// Verify it was recorded
	stats, err := store.GetTodayStats()
	if err != nil {
		t.Fatalf("GetTodayStats failed: %v", err)
	}

	if stats.Keystrokes != 1 {
		t.Errorf("Expected 1 keystroke, got %d", stats.Keystrokes)
	}

	// Record more keystrokes
	for i := 0; i < 99; i++ {
		if err := store.RecordKeystroke(i % 50); err != nil {
			t.Fatalf("RecordKeystroke failed: %v", err)
		}
	}

	stats, err = store.GetTodayStats()
	if err != nil {
		t.Fatalf("GetTodayStats failed: %v", err)
	}

	if stats.Keystrokes != 100 {
		t.Errorf("Expected 100 keystrokes, got %d", stats.Keystrokes)
	}
}

func TestIncrementWordCount(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	date := time.Now().Format("2006-01-02")

	// Increment word count
	err := store.IncrementWordCount(date)
	if err != nil {
		t.Fatalf("IncrementWordCount failed: %v", err)
	}

	stats, err := store.GetTodayStats()
	if err != nil {
		t.Fatalf("GetTodayStats failed: %v", err)
	}

	if stats.Words != 1 {
		t.Errorf("Expected 1 word, got %d", stats.Words)
	}

	// Increment more
	for i := 0; i < 49; i++ {
		store.IncrementWordCount(date)
	}

	stats, _ = store.GetTodayStats()
	if stats.Words != 50 {
		t.Errorf("Expected 50 words, got %d", stats.Words)
	}
}

func TestGetDayStats(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Get stats for non-existent day
	stats, err := store.GetDayStats("2020-01-01")
	if err != nil {
		t.Fatalf("GetDayStats failed: %v", err)
	}

	if stats.Keystrokes != 0 || stats.Words != 0 {
		t.Errorf("Expected 0 keystrokes and 0 words for empty day, got %d and %d", stats.Keystrokes, stats.Words)
	}

	if stats.Date != "2020-01-01" {
		t.Errorf("Expected date 2020-01-01, got %s", stats.Date)
	}
}

func TestGetWeekStats(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Record some keystrokes
	for i := 0; i < 10; i++ {
		store.RecordKeystroke(i)
	}

	stats, err := store.GetWeekStats()
	if err != nil {
		t.Fatalf("GetWeekStats failed: %v", err)
	}

	if len(stats) != 7 {
		t.Errorf("Expected 7 days of stats, got %d", len(stats))
	}

	// Today should have 10 keystrokes
	todayStats := stats[6]
	if todayStats.Keystrokes != 10 {
		t.Errorf("Expected 10 keystrokes for today, got %d", todayStats.Keystrokes)
	}
}

func TestGetHourlyStats(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Record keystrokes
	for i := 0; i < 5; i++ {
		store.RecordKeystroke(42)
	}

	date := time.Now().Format("2006-01-02")
	stats, err := store.GetHourlyStats(date)
	if err != nil {
		t.Fatalf("GetHourlyStats failed: %v", err)
	}

	if len(stats) != 24 {
		t.Errorf("Expected 24 hours of stats, got %d", len(stats))
	}

	// Current hour should have 5 keystrokes
	currentHour := time.Now().Hour()
	if stats[currentHour].Keystrokes != 5 {
		t.Errorf("Expected 5 keystrokes for current hour, got %d", stats[currentHour].Keystrokes)
	}

	// Verify hour is set correctly
	for i, stat := range stats {
		if stat.Hour != i {
			t.Errorf("Expected hour %d, got %d", i, stat.Hour)
		}
	}
}

func TestSettings(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Test SetSetting and GetSetting
	err := store.SetSetting("test_key", "test_value")
	if err != nil {
		t.Fatalf("SetSetting failed: %v", err)
	}

	value, err := store.GetSetting("test_key")
	if err != nil {
		t.Fatalf("GetSetting failed: %v", err)
	}

	if value != "test_value" {
		t.Errorf("Expected 'test_value', got %q", value)
	}

	// Test updating setting
	err = store.SetSetting("test_key", "new_value")
	if err != nil {
		t.Fatalf("SetSetting (update) failed: %v", err)
	}

	value, _ = store.GetSetting("test_key")
	if value != "new_value" {
		t.Errorf("Expected 'new_value', got %q", value)
	}

	// Test non-existent key
	value, err = store.GetSetting("nonexistent")
	if err != nil {
		t.Fatalf("GetSetting for nonexistent key failed: %v", err)
	}
	if value != "" {
		t.Errorf("Expected empty string for nonexistent key, got %q", value)
	}
}

func TestMenubarSettings(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Test default settings
	settings := store.GetMenubarSettings()
	if !settings.ShowKeystrokes {
		t.Error("Expected ShowKeystrokes to default to true")
	}
	if !settings.ShowWords {
		t.Error("Expected ShowWords to default to true")
	}
	if settings.ShowClicks {
		t.Error("Expected ShowClicks to default to false")
	}
	if settings.ShowDistance {
		t.Error("Expected ShowDistance to default to false")
	}

	// Modify and save settings
	settings.ShowKeystrokes = false
	settings.ShowClicks = true
	err := store.SaveMenubarSettings(settings)
	if err != nil {
		t.Fatalf("SaveMenubarSettings failed: %v", err)
	}

	// Retrieve and verify
	settings = store.GetMenubarSettings()
	if settings.ShowKeystrokes {
		t.Error("Expected ShowKeystrokes to be false after save")
	}
	if settings.ShowClicks != true {
		t.Error("Expected ShowClicks to be true after save")
	}
}

func TestInertiaSettings(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Test default settings
	settings := store.GetInertiaSettings()
	if settings.Enabled {
		t.Error("Expected Enabled to default to false")
	}
	if settings.MaxSpeed != InertiaSpeedFast {
		t.Errorf("Expected MaxSpeed to default to %q, got %q", InertiaSpeedFast, settings.MaxSpeed)
	}
	if settings.Threshold != 200 {
		t.Errorf("Expected Threshold to default to 200, got %d", settings.Threshold)
	}
	if settings.AccelRate != 1.0 {
		t.Errorf("Expected AccelRate to default to 1.0, got %f", settings.AccelRate)
	}

	// Modify settings
	store.SetInertiaEnabled(true)
	store.SetInertiaMaxSpeed(InertiaSpeedVeryFast)
	store.SetInertiaThreshold(150)
	store.SetInertiaAccelRate(1.5)

	// Verify
	settings = store.GetInertiaSettings()
	if !settings.Enabled {
		t.Error("Expected Enabled to be true")
	}
	if settings.MaxSpeed != InertiaSpeedVeryFast {
		t.Errorf("Expected MaxSpeed to be %q, got %q", InertiaSpeedVeryFast, settings.MaxSpeed)
	}
	if settings.Threshold != 150 {
		t.Errorf("Expected Threshold to be 150, got %d", settings.Threshold)
	}
	if settings.AccelRate != 1.5 {
		t.Errorf("Expected AccelRate to be 1.5, got %f", settings.AccelRate)
	}
}

func TestMouseTracking(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Test default
	if !store.IsMouseTrackingEnabled() {
		t.Error("Expected mouse tracking to be enabled by default")
	}

	// Disable
	store.SetMouseTrackingEnabled(false)
	if store.IsMouseTrackingEnabled() {
		t.Error("Expected mouse tracking to be disabled")
	}

	// Re-enable
	store.SetMouseTrackingEnabled(true)
	if !store.IsMouseTrackingEnabled() {
		t.Error("Expected mouse tracking to be enabled")
	}
}

func TestDistanceUnit(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Test default
	unit := store.GetDistanceUnit()
	if unit != DistanceUnitFeet {
		t.Errorf("Expected default unit to be %q, got %q", DistanceUnitFeet, unit)
	}

	// Change unit
	store.SetDistanceUnit(DistanceUnitCars)
	unit = store.GetDistanceUnit()
	if unit != DistanceUnitCars {
		t.Errorf("Expected unit to be %q, got %q", DistanceUnitCars, unit)
	}

	store.SetDistanceUnit(DistanceUnitFrisbee)
	unit = store.GetDistanceUnit()
	if unit != DistanceUnitFrisbee {
		t.Errorf("Expected unit to be %q, got %q", DistanceUnitFrisbee, unit)
	}
}

func TestRecordMouseMovement(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Record movement
	err := store.RecordMouseMovement(100, 100, 50.5)
	if err != nil {
		t.Fatalf("RecordMouseMovement failed: %v", err)
	}

	stats, err := store.GetTodayMouseStats()
	if err != nil {
		t.Fatalf("GetTodayMouseStats failed: %v", err)
	}

	if stats.TotalDistance != 50.5 {
		t.Errorf("Expected total distance 50.5, got %f", stats.TotalDistance)
	}
	if stats.MovementCount != 1 {
		t.Errorf("Expected movement count 1, got %d", stats.MovementCount)
	}

	// Record more movements
	store.RecordMouseMovement(150, 150, 70.7)
	store.RecordMouseMovement(200, 200, 70.7)

	stats, _ = store.GetTodayMouseStats()
	expectedDistance := 50.5 + 70.7 + 70.7
	if stats.TotalDistance != expectedDistance {
		t.Errorf("Expected total distance %f, got %f", expectedDistance, stats.TotalDistance)
	}
	if stats.MovementCount != 3 {
		t.Errorf("Expected movement count 3, got %d", stats.MovementCount)
	}
}

func TestRecordMouseClick(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Record clicks
	for i := 0; i < 10; i++ {
		err := store.RecordMouseClick()
		if err != nil {
			t.Fatalf("RecordMouseClick failed: %v", err)
		}
	}

	stats, err := store.GetTodayMouseStats()
	if err != nil {
		t.Fatalf("GetTodayMouseStats failed: %v", err)
	}

	if stats.ClickCount != 10 {
		t.Errorf("Expected click count 10, got %d", stats.ClickCount)
	}
}

func TestMouseLeaderboard(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Record some mouse movements to create leaderboard data
	store.RecordMouseMovement(100, 100, 500)

	entries, err := store.GetMouseLeaderboard(10)
	if err != nil {
		t.Fatalf("GetMouseLeaderboard failed: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(entries))
	}

	if len(entries) > 0 && entries[0].Rank != 1 {
		t.Errorf("Expected rank 1, got %d", entries[0].Rank)
	}
}

func TestTypingTestStats(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Test default stats
	stats := store.GetTypingTestStats()
	if stats.PersonalBest != 0 {
		t.Errorf("Expected default PB to be 0, got %f", stats.PersonalBest)
	}
	if stats.TestCount != 0 {
		t.Errorf("Expected default test count to be 0, got %d", stats.TestCount)
	}

	// Save a result
	err := store.SaveTypingTestResult(75.5)
	if err != nil {
		t.Fatalf("SaveTypingTestResult failed: %v", err)
	}

	stats = store.GetTypingTestStats()
	if stats.PersonalBest != 75.5 {
		t.Errorf("Expected PB to be 75.5, got %f", stats.PersonalBest)
	}
	if stats.TestCount != 1 {
		t.Errorf("Expected test count to be 1, got %d", stats.TestCount)
	}

	// Save a lower result (should not update PB)
	store.SaveTypingTestResult(60.0)
	stats = store.GetTypingTestStats()
	if stats.PersonalBest != 75.5 {
		t.Errorf("Expected PB to remain 75.5, got %f", stats.PersonalBest)
	}
	if stats.TestCount != 2 {
		t.Errorf("Expected test count to be 2, got %d", stats.TestCount)
	}

	// Save a higher result (should update PB)
	store.SaveTypingTestResult(100.0)
	stats = store.GetTypingTestStats()
	if stats.PersonalBest != 100.0 {
		t.Errorf("Expected PB to be 100.0, got %f", stats.PersonalBest)
	}
}

func TestTypingTestModeStats(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	mode := TypingTestMode{WordCount: 25, Punctuation: true}

	// Save result for mode
	err := store.SaveTypingTestResultForMode(80.0, mode)
	if err != nil {
		t.Fatalf("SaveTypingTestResultForMode failed: %v", err)
	}

	stats := store.GetTypingTestStatsForMode(mode)
	if stats.PersonalBest != 80.0 {
		t.Errorf("Expected mode PB to be 80.0, got %f", stats.PersonalBest)
	}

	// Different mode should have separate stats
	mode2 := TypingTestMode{WordCount: 50, Punctuation: false}
	stats2 := store.GetTypingTestStatsForMode(mode2)
	if stats2.PersonalBest != 0 {
		t.Errorf("Expected different mode PB to be 0, got %f", stats2.PersonalBest)
	}
}

func TestTypingTestModeKey(t *testing.T) {
	tests := []struct {
		mode     TypingTestMode
		expected string
	}{
		{TypingTestMode{WordCount: 10, Punctuation: true}, "mode_10_punct"},
		{TypingTestMode{WordCount: 25, Punctuation: false}, "mode_25_no_punct"},
		{TypingTestMode{WordCount: 100, Punctuation: true}, "mode_100_punct"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.mode.ModeKey()
			if result != tt.expected {
				t.Errorf("ModeKey() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTypingTestTheme(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Test default
	theme := store.GetTypingTestTheme()
	if theme != "default" {
		t.Errorf("Expected default theme, got %q", theme)
	}

	// Set theme
	store.SetTypingTestTheme("dracula")
	theme = store.GetTypingTestTheme()
	if theme != "dracula" {
		t.Errorf("Expected 'dracula' theme, got %q", theme)
	}
}

func TestTypingTestCustomTexts(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Test default (empty)
	texts := store.GetTypingTestCustomTexts()
	if texts != "" {
		t.Errorf("Expected empty custom texts, got %q", texts)
	}

	// Set custom texts
	customTexts := "First custom text\n---\nSecond custom text"
	store.SetTypingTestCustomTexts(customTexts)

	texts = store.GetTypingTestCustomTexts()
	if texts != customTexts {
		t.Errorf("Expected %q, got %q", customTexts, texts)
	}
}

func TestHelperFunctions(t *testing.T) {
	// Test abs
	if abs(-5.0) != 5.0 {
		t.Error("abs(-5.0) should be 5.0")
	}
	if abs(5.0) != 5.0 {
		t.Error("abs(5.0) should be 5.0")
	}
	if abs(0) != 0 {
		t.Error("abs(0) should be 0")
	}

	// Test boolToString
	if boolToString(true) != "true" {
		t.Error("boolToString(true) should be 'true'")
	}
	if boolToString(false) != "false" {
		t.Error("boolToString(false) should be 'false'")
	}

	// Test parseInt
	v, err := parseInt("123")
	if err != nil || v != 123 {
		t.Errorf("parseInt('123') = %d, %v; want 123, nil", v, err)
	}

	// Test parseFloat
	f, err := parseFloat("3.14")
	if err != nil || f != 3.14 {
		t.Errorf("parseFloat('3.14') = %f, %v; want 3.14, nil", f, err)
	}

	// Test intToString
	if intToString(42) != "42" {
		t.Error("intToString(42) should be '42'")
	}

	// Test floatToString
	if floatToString(3.14159) != "3.14" {
		t.Errorf("floatToString(3.14159) = %q, want '3.14'", floatToString(3.14159))
	}
}

func TestParseIntEdgeCases(t *testing.T) {
	// Test invalid input
	_, err := parseInt("not_a_number")
	if err == nil {
		t.Error("parseInt('not_a_number') should return an error")
	}

	// Test empty string
	_, err = parseInt("")
	if err == nil {
		t.Error("parseInt('') should return an error")
	}

	// Test negative number
	v, err := parseInt("-42")
	if err != nil || v != -42 {
		t.Errorf("parseInt('-42') = %d, %v; want -42, nil", v, err)
	}

	// Test zero
	v, err = parseInt("0")
	if err != nil || v != 0 {
		t.Errorf("parseInt('0') = %d, %v; want 0, nil", v, err)
	}
}

func TestParseFloatEdgeCases(t *testing.T) {
	// Test invalid input
	_, err := parseFloat("not_a_number")
	if err == nil {
		t.Error("parseFloat('not_a_number') should return an error")
	}

	// Test empty string
	_, err = parseFloat("")
	if err == nil {
		t.Error("parseFloat('') should return an error")
	}

	// Test negative number
	f, err := parseFloat("-3.14")
	if err != nil || f != -3.14 {
		t.Errorf("parseFloat('-3.14') = %f, %v; want -3.14, nil", f, err)
	}

	// Test zero
	f, err = parseFloat("0.0")
	if err != nil || f != 0.0 {
		t.Errorf("parseFloat('0.0') = %f, %v; want 0.0, nil", f, err)
	}

	// Test integer string
	f, err = parseFloat("42")
	if err != nil || f != 42.0 {
		t.Errorf("parseFloat('42') = %f, %v; want 42.0, nil", f, err)
	}
}

func TestGetHistoricalStats(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Get stats with no data
	stats, err := store.GetHistoricalStats(7)
	if err != nil {
		t.Fatalf("GetHistoricalStats failed: %v", err)
	}

	if len(stats) != 7 {
		t.Errorf("Expected 7 days of stats, got %d", len(stats))
	}

	// All should have 0 keystrokes initially
	for _, s := range stats {
		if s.Keystrokes != 0 {
			t.Errorf("Expected 0 keystrokes for %s, got %d", s.Date, s.Keystrokes)
		}
	}
}

func TestGetAllHourlyStatsForDays(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Record some keystrokes
	for i := 0; i < 10; i++ {
		if err := store.RecordKeystroke(42); err != nil {
			t.Fatalf("RecordKeystroke failed: %v", err)
		}
	}

	// Get hourly stats
	stats, err := store.GetAllHourlyStatsForDays(1)
	if err != nil {
		t.Fatalf("GetAllHourlyStatsForDays failed: %v", err)
	}

	// Should have today's stats
	today := time.Now().Format("2006-01-02")
	if hourlyStats, ok := stats[today]; ok {
		// Find current hour
		currentHour := time.Now().Hour()
		found := false
		for _, hs := range hourlyStats {
			if hs.Hour == currentHour && hs.Keystrokes == 10 {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected to find 10 keystrokes in current hour")
		}
	}
}

func TestGetMouseHistoricalStats(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Get historical stats with no data
	stats, err := store.GetMouseHistoricalStats(7)
	if err != nil {
		t.Fatalf("GetMouseHistoricalStats failed: %v", err)
	}

	if len(stats) != 7 {
		t.Errorf("Expected 7 days of stats, got %d", len(stats))
	}

	// All should have 0 distance initially
	for _, s := range stats {
		if s.TotalDistance != 0 {
			t.Errorf("Expected 0 distance for %s, got %f", s.Date, s.TotalDistance)
		}
	}
}

func TestGetWeekMouseStats(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Get week stats with no data
	stats, err := store.GetWeekMouseStats()
	if err != nil {
		t.Fatalf("GetWeekMouseStats failed: %v", err)
	}

	if len(stats) != 7 {
		t.Errorf("Expected 7 days of stats, got %d", len(stats))
	}
}

func TestSettingsEdgeCases(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Test empty key
	err := store.SetSetting("", "value")
	if err != nil {
		t.Logf("SetSetting with empty key: %v", err)
	}

	// Test empty value
	err = store.SetSetting("test_key", "")
	if err != nil {
		t.Fatalf("SetSetting with empty value failed: %v", err)
	}
	val, _ := store.GetSetting("test_key")
	if val != "" {
		t.Errorf("Expected empty value, got %q", val)
	}

	// Test overwriting
	store.SetSetting("test_key", "first")
	store.SetSetting("test_key", "second")
	val, _ = store.GetSetting("test_key")
	if val != "second" {
		t.Errorf("Expected 'second', got %q", val)
	}
}

func TestMouseLeaderboardEmpty(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Get leaderboard with no data
	leaderboard, err := store.GetMouseLeaderboard(10)
	if err != nil {
		t.Fatalf("GetMouseLeaderboard failed: %v", err)
	}

	// Should be empty
	if len(leaderboard) != 0 {
		t.Errorf("Expected empty leaderboard, got %d entries", len(leaderboard))
	}
}

func TestRecordMouseClickMultiple(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Record multiple clicks
	for i := 0; i < 5; i++ {
		if err := store.RecordMouseClick(); err != nil {
			t.Fatalf("RecordMouseClick failed: %v", err)
		}
	}

	// Check stats
	stats, err := store.GetTodayMouseStats()
	if err != nil {
		t.Fatalf("GetTodayMouseStats failed: %v", err)
	}

	if stats.ClickCount != 5 {
		t.Errorf("Expected 5 clicks, got %d", stats.ClickCount)
	}
}

func TestAbsEdgeCases(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{-0.0, 0.0},
		{1.5, 1.5},
		{-1.5, 1.5},
		{-1000000.5, 1000000.5},
		{0.00001, 0.00001},
		{-0.00001, 0.00001},
	}

	for _, tt := range tests {
		result := abs(tt.input)
		if result != tt.expected {
			t.Errorf("abs(%f) = %f, want %f", tt.input, result, tt.expected)
		}
	}
}

func TestIntToStringEdgeCases(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{-1, "-1"},
		{-1000000, "-1000000"},
		{2147483647, "2147483647"},
	}

	for _, tt := range tests {
		result := intToString(tt.input)
		if result != tt.expected {
			t.Errorf("intToString(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestFloatToStringEdgeCases(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{0.0, "0.00"},
		{-1.0, "-1.00"},
		{1.999, "2.00"},
		{1.001, "1.00"},
		{100.456, "100.46"},
	}

	for _, tt := range tests {
		result := floatToString(tt.input)
		if result != tt.expected {
			t.Errorf("floatToString(%f) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// TestClassifyKeycode tests the key type classification function
func TestClassifyKeycode(t *testing.T) {
	tests := []struct {
		name     string
		keycode  int
		expected string
	}{
		// Letters (A-Z physical keys on ANSI layout)
		{"A key (keycode 0)", 0, "letter"},
		{"S key (keycode 1)", 1, "letter"},
		{"D key (keycode 2)", 2, "letter"},
		{"F key (keycode 3)", 3, "letter"},
		{"Q key (keycode 12)", 12, "letter"},
		{"W key (keycode 13)", 13, "letter"},
		{"E key (keycode 14)", 14, "letter"},
		{"Z key (keycode 6)", 6, "letter"},
		{"M key (keycode 46)", 46, "letter"},

		// Modifier keys
		{"Left Shift (keycode 56)", 56, "modifier"},
		{"Right Shift (keycode 60)", 60, "modifier"},
		{"Left Control (keycode 59)", 59, "modifier"},
		{"Right Control (keycode 62)", 62, "modifier"},
		{"Left Option (keycode 58)", 58, "modifier"},
		{"Right Option (keycode 61)", 61, "modifier"},
		{"Left Command (keycode 55)", 55, "modifier"},
		{"Right Command (keycode 54)", 54, "modifier"},
		{"Fn key (keycode 63)", 63, "modifier"},
		{"Caps Lock (keycode 57)", 57, "modifier"},

		// Special keys (numbers, punctuation, function keys, etc.)
		{"Space (keycode 49)", 49, "special"},
		{"Return (keycode 36)", 36, "special"},
		{"Tab (keycode 48)", 48, "special"},
		{"Backspace (keycode 51)", 51, "special"},
		{"Escape (keycode 53)", 53, "special"},
		{"1 key (keycode 18)", 18, "special"},
		{"0 key (keycode 29)", 29, "special"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyKeycode(tt.keycode)
			if result != tt.expected {
				t.Errorf("ClassifyKeycode(%d) = %q, want %q", tt.keycode, result, tt.expected)
			}
		})
	}
}

// TestKeystrokeCountingIsNotDoubled verifies that each keystroke is counted exactly once
func TestKeystrokeCountingIsNotDoubled(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Record 100 keystrokes (using letter keycodes)
	for i := 0; i < 100; i++ {
		err := store.RecordKeystroke(0) // 'A' key
		if err != nil {
			t.Fatalf("RecordKeystroke failed: %v", err)
		}
	}

	stats, err := store.GetTodayStats()
	if err != nil {
		t.Fatalf("GetTodayStats failed: %v", err)
	}

	// Verify exactly 100 keystrokes were recorded (not doubled)
	if stats.Keystrokes != 100 {
		t.Errorf("Expected 100 keystrokes, got %d (possible doubling bug!)", stats.Keystrokes)
	}

	// Verify key types are also correct
	if stats.Letters != 100 {
		t.Errorf("Expected 100 letters, got %d", stats.Letters)
	}
	if stats.Modifiers != 0 {
		t.Errorf("Expected 0 modifiers, got %d", stats.Modifiers)
	}
	if stats.Special != 0 {
		t.Errorf("Expected 0 special keys, got %d", stats.Special)
	}
}

// TestWordCountingIsNotDoubled verifies that each word boundary increments word count exactly once
func TestWordCountingIsNotDoubled(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	date := time.Now().Format("2006-01-02")

	// Increment word count 50 times
	for i := 0; i < 50; i++ {
		err := store.IncrementWordCount(date)
		if err != nil {
			t.Fatalf("IncrementWordCount failed: %v", err)
		}
	}

	stats, err := store.GetTodayStats()
	if err != nil {
		t.Fatalf("GetTodayStats failed: %v", err)
	}

	// Verify exactly 50 words were recorded (not doubled)
	if stats.Words != 50 {
		t.Errorf("Expected 50 words, got %d (possible doubling bug!)", stats.Words)
	}
}

// TestMixedKeyTypeCounting verifies counting works correctly for mixed key types
func TestMixedKeyTypeCounting(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Simulate typing "Hello World" with shift:
	// Shift, H, e, l, l, o, Space, Shift, W, o, r, l, d
	keycodes := []int{
		56,  // Left Shift (modifier)
		4,   // H (letter)
		14,  // e (letter)
		37,  // l (letter)
		37,  // l (letter)
		31,  // o (letter)
		49,  // Space (special)
		56,  // Left Shift (modifier)
		13,  // W (letter)
		31,  // o (letter)
		15,  // r (letter)
		37,  // l (letter)
		2,   // d (letter)
	}

	for _, keycode := range keycodes {
		err := store.RecordKeystroke(keycode)
		if err != nil {
			t.Fatalf("RecordKeystroke failed: %v", err)
		}
	}

	stats, err := store.GetTodayStats()
	if err != nil {
		t.Fatalf("GetTodayStats failed: %v", err)
	}

	// Total keystrokes should be exactly 13
	if stats.Keystrokes != 13 {
		t.Errorf("Expected 13 keystrokes, got %d", stats.Keystrokes)
	}

	// Letters: H, e, l, l, o, W, o, r, l, d = 10
	if stats.Letters != 10 {
		t.Errorf("Expected 10 letters, got %d", stats.Letters)
	}

	// Modifiers: Shift, Shift = 2
	if stats.Modifiers != 2 {
		t.Errorf("Expected 2 modifiers, got %d", stats.Modifiers)
	}

	// Special: Space = 1
	if stats.Special != 1 {
		t.Errorf("Expected 1 special key, got %d", stats.Special)
	}
}

// TestKeyTypeTotalsMatchKeystrokeTotal verifies that letters + modifiers + special = keystrokes
func TestKeyTypeTotalsMatchKeystrokeTotal(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Record a variety of keystrokes
	keycodesAndTypes := []struct {
		keycode  int
		keyType  string
	}{
		{0, "letter"},    // A
		{1, "letter"},    // S
		{56, "modifier"}, // Shift
		{49, "special"},  // Space
		{55, "modifier"}, // Command
		{36, "special"},  // Return
		{12, "letter"},   // Q
		{59, "modifier"}, // Control
		{51, "special"},  // Backspace
	}

	for _, kt := range keycodesAndTypes {
		err := store.RecordKeystroke(kt.keycode)
		if err != nil {
			t.Fatalf("RecordKeystroke failed: %v", err)
		}
	}

	stats, err := store.GetTodayStats()
	if err != nil {
		t.Fatalf("GetTodayStats failed: %v", err)
	}

	// Verify total matches
	total := stats.Letters + stats.Modifiers + stats.Special
	if total != stats.Keystrokes {
		t.Errorf("Key type totals (%d + %d + %d = %d) don't match keystrokes (%d)",
			stats.Letters, stats.Modifiers, stats.Special, total, stats.Keystrokes)
	}

	// Verify expected counts
	if stats.Keystrokes != 9 {
		t.Errorf("Expected 9 keystrokes, got %d", stats.Keystrokes)
	}
	if stats.Letters != 3 {
		t.Errorf("Expected 3 letters, got %d", stats.Letters)
	}
	if stats.Modifiers != 3 {
		t.Errorf("Expected 3 modifiers, got %d", stats.Modifiers)
	}
	if stats.Special != 3 {
		t.Errorf("Expected 3 special keys, got %d", stats.Special)
	}
}
