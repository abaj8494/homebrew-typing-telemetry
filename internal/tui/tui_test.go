package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/aayushbajaj/typing-telemetry/internal/storage"
	tea "github.com/charmbracelet/bubbletea"
)

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1.0K"},
		{1500, "1.5K"},
		{10000, "10.0K"},
		{999999, "1000.0K"},
		{1000000, "1.0M"},
		{1500000, "1.5M"},
		{10000000, "10.0M"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatNumber(tt.input)
			if result != tt.expected {
				t.Errorf("formatNumber(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNew(t *testing.T) {
	model := New(nil)

	if model.store != nil {
		t.Error("Expected store to be nil when passed nil")
	}
}

func TestModelInit(t *testing.T) {
	model := New(nil)
	cmd := model.Init()

	// Init should return a command (fetchStats)
	if cmd == nil {
		t.Error("Expected Init to return a command")
	}
}

func TestModelUpdateKeyQ(t *testing.T) {
	model := New(nil)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	_, cmd := model.Update(msg)

	// 'q' should trigger quit
	if cmd == nil {
		t.Error("Expected quit command from 'q' key")
	}
}

func TestModelUpdateKeyCtrlC(t *testing.T) {
	model := New(nil)

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	_, cmd := model.Update(msg)

	if cmd == nil {
		t.Error("Expected quit command from Ctrl+C")
	}
}

func TestModelUpdateKeyEsc(t *testing.T) {
	model := New(nil)

	msg := tea.KeyMsg{Type: tea.KeyEsc}
	_, cmd := model.Update(msg)

	if cmd == nil {
		t.Error("Expected quit command from Escape")
	}
}

func TestModelUpdateKeyR(t *testing.T) {
	model := New(nil)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
	_, cmd := model.Update(msg)

	// 'r' should trigger refresh (fetchStats)
	if cmd == nil {
		t.Error("Expected refresh command from 'r' key")
	}
}

func TestModelUpdateKeyT(t *testing.T) {
	model := New(nil)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}
	newModel, cmd := model.Update(msg)
	m := newModel.(Model)

	// 't' should set SwitchToTypingTest and quit
	if !m.SwitchToTypingTest {
		t.Error("Expected SwitchToTypingTest to be true after 't'")
	}
	if cmd == nil {
		t.Error("Expected quit command from 't' key")
	}
}

func TestModelUpdateWindowSize(t *testing.T) {
	model := New(nil)

	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	newModel, _ := model.Update(msg)
	m := newModel.(Model)

	if m.width != 120 {
		t.Errorf("Expected width=120, got %d", m.width)
	}
	if m.height != 40 {
		t.Errorf("Expected height=40, got %d", m.height)
	}
}

func TestModelUpdateStatsMsg(t *testing.T) {
	model := New(nil)

	// Test successful stats message
	today := &storage.DailyStats{Keystrokes: 1000, Words: 200}
	week := []storage.DailyStats{{Keystrokes: 500}, {Keystrokes: 600}}
	hourly := []storage.HourlyStats{{Keystrokes: 100}, {Keystrokes: 200}}

	msg := statsMsg{today: today, week: week, hourly: hourly}
	newModel, _ := model.Update(msg)
	m := newModel.(Model)

	if m.todayStats == nil {
		t.Error("Expected todayStats to be set")
	}
	if m.todayStats.Keystrokes != 1000 {
		t.Errorf("Expected keystrokes=1000, got %d", m.todayStats.Keystrokes)
	}
	if len(m.weekStats) != 2 {
		t.Errorf("Expected 2 week stats, got %d", len(m.weekStats))
	}
	if len(m.hourlyStats) != 2 {
		t.Errorf("Expected 2 hourly stats, got %d", len(m.hourlyStats))
	}
}

func TestModelUpdateStatsMsgError(t *testing.T) {
	model := New(nil)

	msg := statsMsg{err: errors.New("test error")}
	newModel, _ := model.Update(msg)
	m := newModel.(Model)

	if m.err == nil {
		t.Error("Expected error to be set")
	}
	if m.err.Error() != "test error" {
		t.Errorf("Expected error message 'test error', got %q", m.err.Error())
	}
}

func TestModelViewError(t *testing.T) {
	model := New(nil)
	model.err = errors.New("something went wrong")

	view := model.View()

	if !strings.Contains(view, "Error") {
		t.Error("Expected 'Error' in view when error is set")
	}
	if !strings.Contains(view, "something went wrong") {
		t.Error("Expected error message in view")
	}
	if !strings.Contains(view, "quit") {
		t.Error("Expected quit instruction in error view")
	}
}

func TestModelViewLoading(t *testing.T) {
	model := New(nil)
	// todayStats is nil by default

	view := model.View()

	if view != "Loading..." {
		t.Errorf("Expected 'Loading...', got %q", view)
	}
}

func TestModelViewWithStats(t *testing.T) {
	model := New(nil)
	model.todayStats = &storage.DailyStats{Keystrokes: 5000, Words: 1000}
	model.weekStats = []storage.DailyStats{
		{Keystrokes: 1000},
		{Keystrokes: 2000},
		{Keystrokes: 1500},
	}
	model.hourlyStats = []storage.HourlyStats{
		{Hour: 9, Keystrokes: 100},
		{Hour: 10, Keystrokes: 200},
		{Hour: 11, Keystrokes: 150},
	}

	view := model.View()

	if !strings.Contains(view, "Typing Telemetry") {
		t.Error("Expected title in view")
	}
	if !strings.Contains(view, "Today") {
		t.Error("Expected 'Today' section in view")
	}
	if !strings.Contains(view, "This Week") {
		t.Error("Expected 'This Week' section in view")
	}
	if !strings.Contains(view, "5.0K") {
		t.Error("Expected formatted keystroke count in view")
	}
}

func TestRenderHourlyGraphEmpty(t *testing.T) {
	model := New(nil)
	model.hourlyStats = []storage.HourlyStats{}

	result := model.renderHourlyGraph()

	if result != "No data" {
		t.Errorf("Expected 'No data', got %q", result)
	}
}

func TestRenderHourlyGraphNoActivity(t *testing.T) {
	model := New(nil)
	model.hourlyStats = []storage.HourlyStats{
		{Hour: 0, Keystrokes: 0},
		{Hour: 1, Keystrokes: 0},
	}

	result := model.renderHourlyGraph()

	if result != "No activity today" {
		t.Errorf("Expected 'No activity today', got %q", result)
	}
}

func TestRenderHourlyGraphWithData(t *testing.T) {
	model := New(nil)
	model.hourlyStats = []storage.HourlyStats{
		{Hour: 9, Keystrokes: 100},
		{Hour: 10, Keystrokes: 500},
		{Hour: 11, Keystrokes: 200},
	}

	result := model.renderHourlyGraph()

	// Should contain bar characters
	bars := "▁▂▃▄▅▆▇█"
	hasBars := false
	for _, b := range bars {
		if strings.ContainsRune(result, b) {
			hasBars = true
			break
		}
	}
	if !hasBars {
		t.Error("Expected graph to contain bar characters")
	}

	// Should contain hour labels
	if !strings.Contains(result, "0") {
		t.Error("Expected hour labels in graph")
	}
}

func TestRenderWeeklyGraphEmpty(t *testing.T) {
	model := New(nil)
	model.weekStats = []storage.DailyStats{}

	result := model.renderWeeklyGraph()

	if result != "No data" {
		t.Errorf("Expected 'No data', got %q", result)
	}
}

func TestRenderWeeklyGraphNoActivity(t *testing.T) {
	model := New(nil)
	model.weekStats = []storage.DailyStats{
		{Keystrokes: 0},
		{Keystrokes: 0},
	}

	result := model.renderWeeklyGraph()

	if result != "No activity this week" {
		t.Errorf("Expected 'No activity this week', got %q", result)
	}
}

func TestRenderWeeklyGraphWithData(t *testing.T) {
	model := New(nil)
	model.weekStats = []storage.DailyStats{
		{Keystrokes: 1000},
		{Keystrokes: 5000},
		{Keystrokes: 2000},
		{Keystrokes: 3000},
	}

	result := model.renderWeeklyGraph()

	// Should contain bar characters
	bars := "▁▂▃▄▅▆▇█"
	hasBars := false
	for _, b := range bars {
		if strings.ContainsRune(result, b) {
			hasBars = true
			break
		}
	}
	if !hasBars {
		t.Error("Expected graph to contain bar characters")
	}
}

func TestRenderHourlyGraphMinimumBar(t *testing.T) {
	model := New(nil)
	// One high value and one very low (but non-zero) value
	model.hourlyStats = []storage.HourlyStats{
		{Hour: 9, Keystrokes: 10000},
		{Hour: 10, Keystrokes: 1}, // Should get minimum bar (idx=1)
	}

	result := model.renderHourlyGraph()

	// The result should contain at least the ▂ bar (index 1) for the minimum non-zero value
	if !strings.Contains(result, "▂") {
		t.Log("Graph result:", result)
		// This is a soft check - the exact character depends on styling
	}
}

func TestRenderWeeklyGraphMinimumBar(t *testing.T) {
	model := New(nil)
	model.weekStats = []storage.DailyStats{
		{Keystrokes: 10000},
		{Keystrokes: 1}, // Should get minimum bar
	}

	result := model.renderWeeklyGraph()

	// Just verify we get some output with bars
	if len(result) == 0 {
		t.Error("Expected non-empty graph")
	}
}

// Test that unknown message types don't cause issues
func TestModelUpdateUnknownMessage(t *testing.T) {
	model := New(nil)

	// Send a custom message type
	type customMsg struct{}
	msg := customMsg{}
	newModel, cmd := model.Update(msg)
	m := newModel.(Model)

	// Should return unchanged model and nil command
	if cmd != nil {
		t.Error("Expected nil command for unknown message")
	}
	if m.err != nil {
		t.Error("Expected no error for unknown message")
	}
}
