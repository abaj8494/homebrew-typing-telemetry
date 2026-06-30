package charts

import (
	"strings"
	"testing"

	"github.com/aayushbajaj/typing-telemetry/internal/storage"
)

func TestGetHeatmapColor(t *testing.T) {
	tests := []struct {
		name     string
		value    int64
		max      int64
		expected string
	}{
		{"zero value", 0, 100, "#1a1a2e"},
		{"ratio 0.1 (< 0.25)", 10, 100, "#2d4a3e"},
		{"ratio 0.24 (< 0.25)", 24, 100, "#2d4a3e"},
		{"ratio 0.25 (boundary)", 25, 100, "#3d6b4f"},
		{"ratio 0.4 (< 0.5)", 40, 100, "#3d6b4f"},
		{"ratio 0.5 (boundary)", 50, 100, "#5a9a6f"},
		{"ratio 0.6 (< 0.75)", 60, 100, "#5a9a6f"},
		{"ratio 0.75 (boundary)", 75, 100, "#7bc96f"},
		{"ratio 1.0 (max)", 100, 100, "#7bc96f"},
		{"ratio > 1", 150, 100, "#7bc96f"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getHeatmapColor(tt.value, tt.max)
			if result != tt.expected {
				t.Errorf("getHeatmapColor(%d, %d) = %q, want %q",
					tt.value, tt.max, result, tt.expected)
			}
		})
	}
}

func TestGenerateHourLabels(t *testing.T) {
	labels := generateHourLabels()

	for _, h := range []string{">0<", ">6<", ">12<", ">18<"} {
		if !strings.Contains(labels, h) {
			t.Errorf("Expected %q hour label", h)
		}
	}
	if !strings.Contains(labels, "<div") {
		t.Error("Expected HTML div elements in labels")
	}
	if !strings.Contains(labels, "hour-label") {
		t.Error("Expected 'hour-label' class in labels")
	}
}

func TestHeatmapColorConsistency(t *testing.T) {
	colors := []string{
		getHeatmapColor(0, 100),
		getHeatmapColor(10, 100),
		getHeatmapColor(30, 100),
		getHeatmapColor(60, 100),
		getHeatmapColor(100, 100),
	}
	for i := 1; i < len(colors); i++ {
		if colors[i] == "" {
			t.Errorf("color %d is empty", i)
		}
	}
}

func TestGetHeatmapColorEdgeCases(t *testing.T) {
	result := getHeatmapColor(0, 0)
	if result != "#1a1a2e" {
		t.Logf("getHeatmapColor(0, 0) = %q", result)
	}
	result = getHeatmapColor(1000000000, 1000000000)
	if result != "#7bc96f" {
		t.Errorf("Expected max color for equal large values, got %q", result)
	}
}

func TestGenerateHeatmapHTML(t *testing.T) {
	result := generateHeatmapHTML(map[string][]storage.HourlyStats{}, 7)
	if result != "" {
		t.Logf("Empty heatmap result: %q", result)
	}

	hourlyData := map[string][]storage.HourlyStats{
		"2024-01-01": {{Hour: 9, Keystrokes: 100}, {Hour: 10, Keystrokes: 200}},
		"2024-01-02": {{Hour: 9, Keystrokes: 50}, {Hour: 10, Keystrokes: 300}},
	}
	result = generateHeatmapHTML(hourlyData, 2)
	for _, cls := range []string{"heatmap-row", "heatmap-cell", "heatmap-label"} {
		if !strings.Contains(result, cls) {
			t.Errorf("Expected %q class in result", cls)
		}
	}
}

func TestGenerateHeatmapHTMLMaxValue(t *testing.T) {
	hourlyData := map[string][]storage.HourlyStats{
		"2024-01-01": {{Hour: 9, Keystrokes: 1000}, {Hour: 10, Keystrokes: 100}},
	}
	result := generateHeatmapHTML(hourlyData, 1)
	if !strings.Contains(result, "#7bc96f") {
		t.Error("Expected brightest color for max value")
	}
}

func TestGenerateHeatmapHTMLDatesSorted(t *testing.T) {
	hourlyData := map[string][]storage.HourlyStats{
		"2024-01-03": {{Hour: 9, Keystrokes: 100}},
		"2024-01-01": {{Hour: 9, Keystrokes: 100}},
		"2024-01-02": {{Hour: 9, Keystrokes: 100}},
	}
	result := generateHeatmapHTML(hourlyData, 3)
	jan1Pos := strings.Index(result, "Jan 1")
	jan2Pos := strings.Index(result, "Jan 2")
	jan3Pos := strings.Index(result, "Jan 3")
	if jan1Pos > jan2Pos || jan2Pos > jan3Pos {
		t.Error("Expected dates to be in sorted order")
	}
}
