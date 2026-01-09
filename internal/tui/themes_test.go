package tui

import (
	"testing"
)

func TestThemesExist(t *testing.T) {
	expectedThemes := []string{"default", "gruvbox", "tokyonight", "catppuccin"}

	for _, name := range expectedThemes {
		if _, ok := Themes[name]; !ok {
			t.Errorf("Expected theme %q to exist", name)
		}
	}
}

func TestThemeNamesMatchThemes(t *testing.T) {
	// ThemeNames should contain exactly the keys in Themes
	if len(ThemeNames) != len(Themes) {
		t.Errorf("ThemeNames length %d doesn't match Themes length %d",
			len(ThemeNames), len(Themes))
	}

	for _, name := range ThemeNames {
		if _, ok := Themes[name]; !ok {
			t.Errorf("ThemeNames contains %q but Themes doesn't", name)
		}
	}
}

func TestThemeStructHasAllFields(t *testing.T) {
	for name, theme := range Themes {
		if theme.Name == "" {
			t.Errorf("Theme %q has empty Name", name)
		}
		if theme.PrimaryAccent == "" {
			t.Errorf("Theme %q has empty PrimaryAccent", name)
		}
		if theme.SecondaryAccent == "" {
			t.Errorf("Theme %q has empty SecondaryAccent", name)
		}
		if theme.CorrectText == "" {
			t.Errorf("Theme %q has empty CorrectText", name)
		}
		if theme.ErrorText == "" {
			t.Errorf("Theme %q has empty ErrorText", name)
		}
		if theme.LabelText == "" {
			t.Errorf("Theme %q has empty LabelText", name)
		}
		if theme.RemainingText == "" {
			t.Errorf("Theme %q has empty RemainingText", name)
		}
		if theme.Border == "" {
			t.Errorf("Theme %q has empty Border", name)
		}
		if theme.SelectedBg == "" {
			t.Errorf("Theme %q has empty SelectedBg", name)
		}
	}
}

func TestDefaultTheme(t *testing.T) {
	theme := Themes["default"]

	if theme.Name != "Default" {
		t.Errorf("Expected default theme name 'Default', got %q", theme.Name)
	}
}

func TestGruvboxTheme(t *testing.T) {
	theme := Themes["gruvbox"]

	if theme.Name != "Gruvbox" {
		t.Errorf("Expected gruvbox theme name 'Gruvbox', got %q", theme.Name)
	}
}

func TestTokyoNightTheme(t *testing.T) {
	theme := Themes["tokyonight"]

	if theme.Name != "Tokyo Night" {
		t.Errorf("Expected tokyonight theme name 'Tokyo Night', got %q", theme.Name)
	}
}

func TestCatppuccinTheme(t *testing.T) {
	theme := Themes["catppuccin"]

	if theme.Name != "Catppuccin" {
		t.Errorf("Expected catppuccin theme name 'Catppuccin', got %q", theme.Name)
	}
}

func TestSetThemeValid(t *testing.T) {
	// Save original theme
	original := CurrentTheme

	// Set to gruvbox
	SetTheme("gruvbox")
	if CurrentTheme.Name != "Gruvbox" {
		t.Errorf("Expected CurrentTheme to be Gruvbox, got %q", CurrentTheme.Name)
	}

	// Restore
	CurrentTheme = original
	regenerateStyles()
}

func TestSetThemeInvalid(t *testing.T) {
	// Save original theme
	originalName := CurrentTheme.Name

	// Try to set invalid theme
	SetTheme("nonexistent")

	// Should remain unchanged
	if CurrentTheme.Name != originalName {
		t.Errorf("Invalid theme should not change CurrentTheme, was %q now %q",
			originalName, CurrentTheme.Name)
	}
}

func TestThemeColorsAreHexFormat(t *testing.T) {
	for name, theme := range Themes {
		colors := []struct {
			field string
			value string
		}{
			{"PrimaryAccent", theme.PrimaryAccent},
			{"SecondaryAccent", theme.SecondaryAccent},
			{"CorrectText", theme.CorrectText},
			{"ErrorText", theme.ErrorText},
			{"LabelText", theme.LabelText},
			{"RemainingText", theme.RemainingText},
			{"Border", theme.Border},
			{"SelectedBg", theme.SelectedBg},
		}

		for _, c := range colors {
			if len(c.value) != 7 || c.value[0] != '#' {
				t.Errorf("Theme %q field %s has invalid hex color %q", name, c.field, c.value)
			}
		}
	}
}

func TestCurrentThemeInitialized(t *testing.T) {
	// CurrentTheme should be initialized at package init
	if CurrentTheme.Name == "" {
		t.Error("CurrentTheme should be initialized")
	}
}

func TestRegenerateStylesDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("regenerateStyles panicked: %v", r)
		}
	}()

	regenerateStyles()
}

func TestSetThemeCycleAllThemes(t *testing.T) {
	original := CurrentTheme

	for _, name := range ThemeNames {
		SetTheme(name)
		if CurrentTheme.Name != Themes[name].Name {
			t.Errorf("SetTheme(%q) failed, expected %q got %q",
				name, Themes[name].Name, CurrentTheme.Name)
		}
	}

	// Restore
	CurrentTheme = original
	regenerateStyles()
}
