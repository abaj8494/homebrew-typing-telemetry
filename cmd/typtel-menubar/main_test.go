//go:build darwin
// +build darwin

package main

import (
	"strings"
	"testing"
)

func TestFormatAbsolute(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{"zero", 0, "0"},
		{"single digit", 5, "5"},
		{"double digit", 42, "42"},
		{"triple digit", 100, "100"},
		{"999", 999, "999"},
		{"1000", 1000, "1,000"},
		{"1234", 1234, "1,234"},
		{"12345", 12345, "12,345"},
		{"123456", 123456, "123,456"},
		{"1234567", 1234567, "1,234,567"},
		{"one million", 1000000, "1,000,000"},
		{"one billion", 1000000000, "1,000,000,000"},
		{"negative", -100, "-100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatAbsolute(tt.input)
			if result != tt.expected {
				t.Errorf("formatAbsolute(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatAbsoluteEdgeCases(t *testing.T) {
	// Test very large numbers
	result := formatAbsolute(123456789012)
	if !strings.Contains(result, ",") {
		t.Error("Expected commas in large number")
	}

	// Count commas: 123,456,789,012 has 3 commas
	commaCount := strings.Count(result, ",")
	if commaCount != 3 {
		t.Errorf("Expected 3 commas in %q, got %d", result, commaCount)
	}

	// Verify exact format
	if result != "123,456,789,012" {
		t.Errorf("formatAbsolute(123456789012) = %q, want '123,456,789,012'", result)
	}
}

func TestShowPermissionAlert(t *testing.T) {
	// This just prints to stdout - verify it doesn't panic
	showPermissionAlert()
}

func TestGetLogDir(t *testing.T) {
	dir, err := getLogDir()
	if err != nil {
		t.Errorf("getLogDir() error = %v", err)
	}

	// Should return a path containing typtel
	if !strings.Contains(dir, "typtel") {
		t.Errorf("getLogDir() = %q, expected to contain 'typtel'", dir)
	}

	// Should return a path containing logs
	if !strings.Contains(dir, "logs") {
		t.Errorf("getLogDir() = %q, expected to contain 'logs'", dir)
	}
}

func TestVersionVariable(t *testing.T) {
	// Version should be set (either "dev" or a real version)
	if Version == "" {
		t.Error("Version should not be empty")
	}
}
