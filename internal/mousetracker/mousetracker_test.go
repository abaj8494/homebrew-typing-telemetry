//go:build darwin
// +build darwin

package mousetracker

import (
	"math"
	"testing"
)

func TestDefaultPPI(t *testing.T) {
	// Verify the default PPI constant
	if DefaultPPI != 100.0 {
		t.Errorf("Expected DefaultPPI to be 100.0, got %f", DefaultPPI)
	}
}

func TestGetAveragePPI(t *testing.T) {
	ppi := GetAveragePPI()

	// Should return a positive value
	if ppi <= 0 {
		t.Errorf("Expected positive PPI, got %f", ppi)
	}

	// Should be reasonable for typical displays (50-600 PPI range)
	if ppi < 50 || ppi > 600 {
		t.Logf("Warning: PPI %f is outside typical range (50-600)", ppi)
	}
}

func TestPixelsToInches(t *testing.T) {
	ppi := GetAveragePPI()

	tests := []struct {
		name   string
		pixels float64
	}{
		{"zero pixels", 0},
		{"one pixel", 1},
		{"ten pixels", 10},
		{"hundred pixels", 100},
		{"thousand pixels", 1000},
		{"large value", 10000},
		{"negative value", -100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PixelsToInches(tt.pixels)
			expected := tt.pixels / ppi

			if math.Abs(result-expected) > 0.0001 {
				t.Errorf("PixelsToInches(%f) = %f, want %f", tt.pixels, result, expected)
			}
		})
	}
}

func TestPixelsToFeet(t *testing.T) {
	ppi := GetAveragePPI()

	tests := []struct {
		name   string
		pixels float64
	}{
		{"zero pixels", 0},
		{"one pixel", 1},
		{"hundred pixels", 100},
		{"thousand pixels", 1000},
		{"twelve inches worth", ppi * 12}, // Should equal exactly 1 foot
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PixelsToFeet(tt.pixels)
			expectedInches := tt.pixels / ppi
			expectedFeet := expectedInches / 12.0

			if math.Abs(result-expectedFeet) > 0.0001 {
				t.Errorf("PixelsToFeet(%f) = %f, want %f", tt.pixels, result, expectedFeet)
			}
		})
	}
}

func TestPixelsToInchesAndFeetRelationship(t *testing.T) {
	// PixelsToFeet should equal PixelsToInches / 12
	testPixels := []float64{0, 100, 500, 1000, 5000}

	for _, pixels := range testPixels {
		inches := PixelsToInches(pixels)
		feet := PixelsToFeet(pixels)

		expectedFeet := inches / 12.0
		if math.Abs(feet-expectedFeet) > 0.0001 {
			t.Errorf("For %f pixels: feet=%f but inches/12=%f", pixels, feet, expectedFeet)
		}
	}
}

func TestGetDisplayCount(t *testing.T) {
	count := GetDisplayCount()

	// Should have at least one display in a test environment
	if count < 0 {
		t.Errorf("Display count should not be negative, got %d", count)
	}

	// Log the count for debugging
	t.Logf("Detected %d display(s)", count)
}

func TestMousePositionStruct(t *testing.T) {
	// Test MousePosition struct
	pos := MousePosition{X: 100.5, Y: 200.5}

	if pos.X != 100.5 {
		t.Errorf("Expected X=100.5, got %f", pos.X)
	}
	if pos.Y != 200.5 {
		t.Errorf("Expected Y=200.5, got %f", pos.Y)
	}
}

func TestMouseMovementStruct(t *testing.T) {
	// Test MouseMovement struct
	mov := MouseMovement{X: 150.0, Y: 250.0, Distance: 50.5}

	if mov.X != 150.0 {
		t.Errorf("Expected X=150.0, got %f", mov.X)
	}
	if mov.Y != 250.0 {
		t.Errorf("Expected Y=250.0, got %f", mov.Y)
	}
	if mov.Distance != 50.5 {
		t.Errorf("Expected Distance=50.5, got %f", mov.Distance)
	}
}

func TestMouseClickStruct(t *testing.T) {
	// Test MouseClick struct (empty struct)
	click := MouseClick{}
	_ = click // Just ensure it compiles and can be instantiated
}

func TestPixelConversionZeroPixels(t *testing.T) {
	// Zero pixels should convert to zero inches and feet
	inches := PixelsToInches(0)
	feet := PixelsToFeet(0)

	if inches != 0 {
		t.Errorf("Expected 0 inches for 0 pixels, got %f", inches)
	}
	if feet != 0 {
		t.Errorf("Expected 0 feet for 0 pixels, got %f", feet)
	}
}

func TestPixelConversionLargeValues(t *testing.T) {
	// Test with very large pixel values (e.g., full day of mouse movement)
	// Assuming 10000 pixels is a reasonable amount of movement
	largePixels := 100000.0

	inches := PixelsToInches(largePixels)
	feet := PixelsToFeet(largePixels)

	// Results should be positive and finite
	if inches <= 0 || math.IsInf(inches, 0) || math.IsNaN(inches) {
		t.Errorf("PixelsToInches(%f) returned invalid value: %f", largePixels, inches)
	}
	if feet <= 0 || math.IsInf(feet, 0) || math.IsNaN(feet) {
		t.Errorf("PixelsToFeet(%f) returned invalid value: %f", largePixels, feet)
	}

	// Log for visibility
	t.Logf("%.0f pixels = %.2f inches = %.2f feet", largePixels, inches, feet)
}

func TestPixelConversionNegativeValues(t *testing.T) {
	// Negative pixel values should produce negative results
	// (though in practice distances are always positive)
	negativePixels := -100.0

	inches := PixelsToInches(negativePixels)
	feet := PixelsToFeet(negativePixels)

	if inches >= 0 {
		t.Errorf("Expected negative inches for negative pixels, got %f", inches)
	}
	if feet >= 0 {
		t.Errorf("Expected negative feet for negative pixels, got %f", feet)
	}
}

func TestResetForNewDay(t *testing.T) {
	// Reset state
	mu.Lock()
	initialized = true
	lastX = 100.0
	lastY = 200.0
	mu.Unlock()

	// Call ResetForNewDay
	ResetForNewDay()

	// Check that initialized is false
	mu.Lock()
	init := initialized
	mu.Unlock()

	if init {
		t.Error("Expected initialized to be false after ResetForNewDay")
	}
}

func TestCheckAccessibilityPermissions(t *testing.T) {
	// This test just verifies the function doesn't crash
	// The actual return value depends on system permissions
	result := CheckAccessibilityPermissions()
	t.Logf("CheckAccessibilityPermissions() = %v", result)
}

func TestGetCurrentPosition(t *testing.T) {
	// This test just verifies the function returns valid coordinates
	pos := GetCurrentPosition()

	// Coordinates should be finite numbers
	if math.IsNaN(pos.X) || math.IsInf(pos.X, 0) {
		t.Errorf("X coordinate is not a valid number: %f", pos.X)
	}
	if math.IsNaN(pos.Y) || math.IsInf(pos.Y, 0) {
		t.Errorf("Y coordinate is not a valid number: %f", pos.Y)
	}

	t.Logf("Current mouse position: (%.2f, %.2f)", pos.X, pos.Y)
}

func TestPixelConversionConsistency(t *testing.T) {
	// Test that conversions are mathematically consistent
	testPixels := []float64{0, 100, 1000, 10000, 100000}

	for _, px := range testPixels {
		inches := PixelsToInches(px)
		feet := PixelsToFeet(px)

		// feet should equal inches / 12
		expectedFeet := inches / 12.0
		if math.Abs(feet-expectedFeet) > 0.0001 {
			t.Errorf("Inconsistent conversion for %f pixels: inches=%f, feet=%f (expected %f)",
				px, inches, feet, expectedFeet)
		}
	}
}

func TestPixelConversionWithKnownPPI(t *testing.T) {
	ppi := GetAveragePPI()
	t.Logf("System PPI: %f", ppi)

	// At PPI=100 (common default):
	// 100 pixels = 1 inch
	// 1200 pixels = 12 inches = 1 foot
	if ppi == 100.0 {
		inches := PixelsToInches(100)
		if inches != 1.0 {
			t.Errorf("At PPI=100, 100 pixels should be 1 inch, got %f", inches)
		}

		feet := PixelsToFeet(1200)
		if feet != 1.0 {
			t.Errorf("At PPI=100, 1200 pixels should be 1 foot, got %f", feet)
		}
	}
}

func TestMouseMovementStructFields(t *testing.T) {
	mov := MouseMovement{
		X:        123.456,
		Y:        789.012,
		Distance: 100.5,
	}

	if mov.X != 123.456 {
		t.Errorf("X field incorrect: %f", mov.X)
	}
	if mov.Y != 789.012 {
		t.Errorf("Y field incorrect: %f", mov.Y)
	}
	if mov.Distance != 100.5 {
		t.Errorf("Distance field incorrect: %f", mov.Distance)
	}
}

func TestGetAveragePPIReturnsPositive(t *testing.T) {
	ppi := GetAveragePPI()

	if ppi <= 0 {
		t.Errorf("PPI should be positive, got %f", ppi)
	}

	// PPI should be reasonable (between 50 and 500 for most displays)
	if ppi < 50 || ppi > 500 {
		t.Logf("Warning: PPI %f is outside typical range", ppi)
	}
}

func TestDefaultPPIValue(t *testing.T) {
	// DefaultPPI is the fallback when CGO fails
	if DefaultPPI != 100.0 {
		t.Errorf("DefaultPPI should be 100.0, got %f", DefaultPPI)
	}
}

func TestPixelsToInchesReturnsZeroForZero(t *testing.T) {
	result := PixelsToInches(0)
	if result != 0 {
		t.Errorf("PixelsToInches(0) should return 0, got %f", result)
	}
}

func TestPixelsToFeetReturnsZeroForZero(t *testing.T) {
	result := PixelsToFeet(0)
	if result != 0 {
		t.Errorf("PixelsToFeet(0) should return 0, got %f", result)
	}
}

func TestConversionMathematicalProperties(t *testing.T) {
	// Test linearity: f(2x) = 2*f(x)
	x := 1000.0
	fx := PixelsToInches(x)
	f2x := PixelsToInches(2 * x)

	if math.Abs(f2x-2*fx) > 0.0001 {
		t.Errorf("Linearity failed: f(2x)=%f, 2*f(x)=%f", f2x, 2*fx)
	}

	// Test additivity: f(x+y) = f(x) + f(y)
	y := 500.0
	fy := PixelsToInches(y)
	fxy := PixelsToInches(x + y)

	if math.Abs(fxy-(fx+fy)) > 0.0001 {
		t.Errorf("Additivity failed: f(x+y)=%f, f(x)+f(y)=%f", fxy, fx+fy)
	}
}
