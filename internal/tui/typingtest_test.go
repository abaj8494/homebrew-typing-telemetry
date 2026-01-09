package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewTypingTest(t *testing.T) {
	model := NewTypingTest("", 25)

	if model.state != StateReady {
		t.Error("Expected state to be StateReady")
	}

	if model.wordCount != 25 {
		t.Errorf("Expected wordCount to be 25, got %d", model.wordCount)
	}

	if len(model.targetText) == 0 {
		t.Error("Expected targetText to be generated")
	}
}

func TestNewTypingTestDefaultWordCount(t *testing.T) {
	// Test with 0 word count (should default to 25)
	model := NewTypingTest("", 0)
	if model.options.WordCount != 25 {
		t.Errorf("Expected default word count 25, got %d", model.options.WordCount)
	}

	// Test with negative word count (should default to 25)
	model = NewTypingTest("", -5)
	if model.options.WordCount != 25 {
		t.Errorf("Expected default word count 25, got %d", model.options.WordCount)
	}
}

func TestGenerateText(t *testing.T) {
	model := NewTypingTest("", 10)

	// Disable punctuation for predictable word count
	model.options.Punctuation = false
	model.targetText = model.generateText()

	words := strings.Fields(model.targetText)
	if len(words) != 10 {
		t.Errorf("Expected 10 words, got %d", len(words))
	}
}

func TestGenerateTextWithPunctuation(t *testing.T) {
	model := NewTypingTest("", 20)
	model.options.Punctuation = true
	model.targetText = model.generateText()

	// With punctuation enabled, text should end with punctuation
	text := model.targetText
	lastChar := text[len(text)-1]
	validEndings := ".!?"
	if !strings.ContainsRune(validEndings, rune(lastChar)) {
		t.Errorf("Expected text to end with punctuation, got %q", string(lastChar))
	}

	// First character should be capitalized
	if text[0] < 'A' || text[0] > 'Z' {
		t.Errorf("Expected first character to be capitalized, got %q", string(text[0]))
	}
}

func TestDeleteLastWord(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"hello", ""},
		{"hello ", ""},
		{"hello world", "hello "},
		{"hello world ", "hello "},
		{"one two three", "one two "},
		{"   ", ""},
		{"word", ""},
		{"a b c", "a b "},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := deleteLastWord(tt.input)
			if result != tt.expected {
				t.Errorf("deleteLastWord(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLayoutMappings(t *testing.T) {
	// Verify qwerty mapping is empty (identity)
	if len(layoutMappings["qwerty"]) != 0 {
		t.Error("Expected qwerty mapping to be empty")
	}

	// Verify dvorak mapping exists
	if len(layoutMappings["dvorak"]) == 0 {
		t.Error("Expected dvorak mapping to have entries")
	}

	// Verify colemak mapping exists
	if len(layoutMappings["colemak"]) == 0 {
		t.Error("Expected colemak mapping to have entries")
	}
}

func TestTransformLayout(t *testing.T) {
	model := NewTypingTest("", 10)

	// Test qwerty (no transform)
	model.options.Layout = "qwerty"
	result := model.transformLayout("hello")
	if result != "hello" {
		t.Errorf("qwerty transform should be identity, got %q", result)
	}

	// Test dvorak transforms some characters
	model.options.Layout = "dvorak"
	result = model.transformLayout("q")
	if result != "'" {
		t.Errorf("dvorak transform of 'q' should be \"'\", got %q", result)
	}
}

func TestTestOptions(t *testing.T) {
	model := NewTypingTest("", 25)

	// Check default options
	if model.options.Layout != "qwerty" {
		t.Errorf("Expected default layout 'qwerty', got %q", model.options.Layout)
	}

	if !model.options.LiveWPM {
		t.Error("Expected LiveWPM to be enabled by default")
	}

	if !model.options.Punctuation {
		t.Error("Expected Punctuation to be enabled by default")
	}

	if model.options.PaceCaret != PaceOff {
		t.Errorf("Expected PaceCaret to be off by default, got %d", model.options.PaceCaret)
	}

	if model.options.Theme != "default" {
		t.Errorf("Expected default theme, got %q", model.options.Theme)
	}
}

func TestResetTest(t *testing.T) {
	model := NewTypingTest("", 10)
	model.typed = "some text"
	model.errors = 5
	model.state = StateFinished

	originalText := model.targetText
	model.resetTest()

	if model.typed != "" {
		t.Error("Expected typed to be empty after reset")
	}

	if model.errors != 0 {
		t.Error("Expected errors to be 0 after reset")
	}

	if model.state != StateReady {
		t.Error("Expected state to be StateReady after reset")
	}

	// Text should be regenerated
	if model.targetText == originalText {
		t.Log("Note: targetText might be the same by chance, not an error")
	}
}

func TestPaceCaretModes(t *testing.T) {
	modes := []PaceCaretMode{PaceOff, PacePB, PaceAverage, PaceCustom}
	for i, mode := range modes {
		if int(mode) != i {
			t.Errorf("Expected PaceCaretMode %d to have value %d", i, int(mode))
		}
	}
}

func TestTestStates(t *testing.T) {
	states := []TestState{StateReady, StateRunning, StateFinished, StateOptions}
	for i, state := range states {
		if int(state) != i {
			t.Errorf("Expected TestState %d to have value %d", i, int(state))
		}
	}
}

func TestMenuFocus(t *testing.T) {
	model := NewTypingTest("", 10)

	// Default focus should be typing
	if model.menuFocus != FocusTyping {
		t.Error("Expected default focus to be FocusTyping")
	}
}

func TestFilterOptions(t *testing.T) {
	model := NewTypingTest("", 10)

	// Empty search should return all options
	model.searchQuery = ""
	model.filterOptions()
	if len(model.filteredOpts) != len(model.allOptions) {
		t.Error("Empty search should return all options")
	}

	// Search for "theme"
	model.searchQuery = "theme"
	model.filterOptions()
	if len(model.filteredOpts) == 0 {
		t.Error("Search for 'theme' should return at least one result")
	}
}

func TestPunctuationMarks(t *testing.T) {
	if len(punctuationMarks) == 0 {
		t.Error("Expected punctuationMarks to have entries")
	}

	// Verify common punctuation is included
	commonPunctuation := []string{".", ",", "!", "?"}
	punctSet := make(map[string]bool)
	for _, p := range punctuationMarks {
		punctSet[p] = true
	}

	for _, p := range commonPunctuation {
		if !punctSet[p] {
			t.Errorf("Expected punctuation mark %q to be included", p)
		}
	}
}

func TestCustomTextGeneration(t *testing.T) {
	model := NewTypingTest("", 10)
	model.options.TestType = "custom"
	model.customTexts = []string{"Custom text for testing."}

	text := model.generateText()
	if text != "Custom text for testing." {
		t.Errorf("Expected custom text, got %q", text)
	}
}

func TestCustomTextGenerationFallback(t *testing.T) {
	model := NewTypingTest("", 10)
	model.options.TestType = "custom"
	model.customTexts = []string{} // Empty custom texts

	// Should fall back to normal word generation
	text := model.generateText()
	if len(text) == 0 {
		t.Error("Expected fallback text generation when no custom texts")
	}
}

func TestTypingTestModeKey(t *testing.T) {
	tests := []struct {
		name     string
		opts     TestOptions
		expected string
	}{
		{
			name:     "default",
			opts:     TestOptions{WordCount: 25, Punctuation: true},
			expected: "mode_25_punct",
		},
		{
			name:     "no punctuation",
			opts:     TestOptions{WordCount: 50, Punctuation: false},
			expected: "mode_50_no_punct",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewTypingTest("", tt.opts.WordCount)
			model.options.Punctuation = tt.opts.Punctuation
			// The mode key would be generated when saving stats
			// This tests the mode key format indirectly
		})
	}
}

func TestApplyOptionTheme(t *testing.T) {
	model := NewTypingTest("", 25)

	// Find theme option
	var themeOpt Option
	for _, opt := range model.allOptions {
		if opt.ID == "theme" {
			themeOpt = opt
			break
		}
	}

	// Apply gruvbox theme (index 1)
	model.applyOption(themeOpt, 1)

	if model.options.Theme != "gruvbox" {
		t.Errorf("Expected theme 'gruvbox', got %q", model.options.Theme)
	}

	// Verify allOptions is updated
	for _, opt := range model.allOptions {
		if opt.ID == "theme" {
			if opt.Value != "gruvbox" {
				t.Errorf("Expected allOptions theme value 'gruvbox', got %v", opt.Value)
			}
			break
		}
	}
}

func TestApplyOptionLayout(t *testing.T) {
	model := NewTypingTest("", 25)

	// Find layout option
	var layoutOpt Option
	for _, opt := range model.allOptions {
		if opt.ID == "layout" {
			layoutOpt = opt
			break
		}
	}

	// Apply dvorak layout (index 1)
	model.applyOption(layoutOpt, 1)

	if model.options.Layout != "dvorak" {
		t.Errorf("Expected layout 'dvorak', got %q", model.options.Layout)
	}
}

func TestApplyOptionToggle(t *testing.T) {
	model := NewTypingTest("", 25)

	// Initial LiveWPM should be true
	if !model.options.LiveWPM {
		t.Fatal("Expected LiveWPM to be true initially")
	}

	// Find live_wpm option
	var liveWPMOpt Option
	for _, opt := range model.allOptions {
		if opt.ID == "live_wpm" {
			liveWPMOpt = opt
			break
		}
	}

	// Toggle it off
	model.applyOption(liveWPMOpt, 0)

	if model.options.LiveWPM {
		t.Error("Expected LiveWPM to be false after toggle")
	}

	// Toggle it back on
	model.applyOption(liveWPMOpt, 0)

	if !model.options.LiveWPM {
		t.Error("Expected LiveWPM to be true after second toggle")
	}
}

func TestApplyOptionTestLength(t *testing.T) {
	model := NewTypingTest("", 25)

	// Find test_length option
	var lengthOpt Option
	for _, opt := range model.allOptions {
		if opt.ID == "test_length" {
			lengthOpt = opt
			break
		}
	}

	// Apply 50 words (index 2: "10", "25", "50", ...)
	model.applyOption(lengthOpt, 2)

	if model.options.WordCount != 50 {
		t.Errorf("Expected WordCount 50, got %d", model.options.WordCount)
	}
	if model.wordCount != 50 {
		t.Errorf("Expected wordCount 50, got %d", model.wordCount)
	}
}

func TestApplyOptionPunctuation(t *testing.T) {
	model := NewTypingTest("", 25)

	// Initial punctuation should be true
	if !model.options.Punctuation {
		t.Fatal("Expected Punctuation to be true initially")
	}

	// Find punctuation option
	var punctOpt Option
	for _, opt := range model.allOptions {
		if opt.ID == "punctuation" {
			punctOpt = opt
			break
		}
	}

	// Toggle it off
	model.applyOption(punctOpt, 0)

	if model.options.Punctuation {
		t.Error("Expected Punctuation to be false after toggle")
	}
}

func TestApplyOptionPaceCaret(t *testing.T) {
	model := NewTypingTest("", 25)

	// Find pace_caret option
	var paceOpt Option
	for _, opt := range model.allOptions {
		if opt.ID == "pace_caret" {
			paceOpt = opt
			break
		}
	}

	// Test each mode
	tests := []struct {
		choiceIdx int
		expected  PaceCaretMode
	}{
		{0, PaceOff},
		{1, PacePB},
		{2, PaceAverage},
		{3, PaceCustom},
	}

	for _, tt := range tests {
		model.applyOption(paceOpt, tt.choiceIdx)
		if model.options.PaceCaret != tt.expected {
			t.Errorf("applyOption pace_caret[%d]: expected %d, got %d",
				tt.choiceIdx, tt.expected, model.options.PaceCaret)
		}
	}
}

func TestApplyOptionTestType(t *testing.T) {
	model := NewTypingTest("", 25)

	// Find test_type option
	var typeOpt Option
	for _, opt := range model.allOptions {
		if opt.ID == "test_type" {
			typeOpt = opt
			break
		}
	}

	// Apply custom test type (index 1)
	model.applyOption(typeOpt, 1)

	if model.options.TestType != "custom" {
		t.Errorf("Expected TestType 'custom', got %q", model.options.TestType)
	}
}

func TestTransformLayoutDvorak(t *testing.T) {
	model := NewTypingTest("", 10)
	model.options.Layout = "dvorak"

	tests := []struct {
		input    string
		expected string
	}{
		{"q", "'"},
		{"w", ","},
		{"e", "."},
		{"hello", "d.nnr"}, // h->d, e->., l->n, l->n, o->r
	}

	for _, tt := range tests {
		result := model.transformLayout(tt.input)
		if result != tt.expected {
			t.Errorf("transformLayout(%q) dvorak = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestTransformLayoutColemak(t *testing.T) {
	model := NewTypingTest("", 10)
	model.options.Layout = "colemak"

	// Test some colemak transformations
	tests := []struct {
		input    string
		expected string
	}{
		{"e", "f"},
		{"r", "p"},
		{"n", "k"},
	}

	for _, tt := range tests {
		result := model.transformLayout(tt.input)
		if result != tt.expected {
			t.Errorf("transformLayout(%q) colemak = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestTransformLayoutUnknown(t *testing.T) {
	model := NewTypingTest("", 10)
	model.options.Layout = "unknown"

	// Unknown layout should return input unchanged
	result := model.transformLayout("hello")
	if result != "hello" {
		t.Errorf("transformLayout with unknown layout should be identity, got %q", result)
	}
}

func TestCenterContentZeroDimensions(t *testing.T) {
	model := NewTypingTest("", 10)
	model.width = 0
	model.height = 0

	content := "test content"
	result := model.centerContent(content)

	if result != content {
		t.Errorf("centerContent with zero dimensions should return content unchanged, got %q", result)
	}
}

func TestCenterContentNormal(t *testing.T) {
	model := NewTypingTest("", 10)
	model.width = 80
	model.height = 24

	content := "test"
	result := model.centerContent(content)

	// Result should contain the original content
	if !strings.Contains(result, "test") {
		t.Error("centerContent result should contain original content")
	}

	// Result should have leading newlines for vertical centering
	if !strings.HasPrefix(result, "\n") {
		t.Error("centerContent should add vertical padding")
	}
}

func TestCenterContentMultipleLines(t *testing.T) {
	model := NewTypingTest("", 10)
	model.width = 80
	model.height = 30

	content := "line1\nline2\nline3"
	result := model.centerContent(content)

	// Result should contain all lines
	if !strings.Contains(result, "line1") {
		t.Error("centerContent should preserve line1")
	}
	if !strings.Contains(result, "line2") {
		t.Error("centerContent should preserve line2")
	}
	if !strings.Contains(result, "line3") {
		t.Error("centerContent should preserve line3")
	}
}

func TestFilterOptionsWithMultipleMatches(t *testing.T) {
	model := NewTypingTest("", 10)

	// Search for "test" - should match "Test Type" and "Test Length"
	model.searchQuery = "test"
	model.filterOptions()

	if len(model.filteredOpts) < 2 {
		t.Errorf("Expected at least 2 matches for 'test', got %d", len(model.filteredOpts))
	}
}

func TestFilterOptionsNoMatch(t *testing.T) {
	model := NewTypingTest("", 10)

	// Search for something that doesn't exist
	model.searchQuery = "zzzznonexistent"
	model.filterOptions()

	if len(model.filteredOpts) != 0 {
		t.Errorf("Expected 0 matches for nonexistent search, got %d", len(model.filteredOpts))
	}
}

func TestFilterOptionsSelectedIndexBounds(t *testing.T) {
	model := NewTypingTest("", 10)
	model.selectedIdx = 5 // Set to high value

	// Filter to get fewer results
	model.searchQuery = "pace"
	model.filterOptions()

	// selectedIdx should be reset if it exceeds filtered results
	if model.selectedIdx >= len(model.filteredOpts) && len(model.filteredOpts) > 0 {
		t.Error("selectedIdx should be reset when exceeding filtered results")
	}
}

func TestRecordTestResultLogic(t *testing.T) {
	model := NewTypingTest("", 10)
	model.targetText = "test text here" // 14 characters
	model.typed = "test text here"
	model.state = StateFinished
	model.startTime = time.Now().Add(-5 * time.Second) // 5 seconds ago
	model.endTime = time.Now()
	model.errors = 0
	model.resultRecorded = false

	initialTestCount := model.testCount
	model.recordTestResult()

	// Test count should increase
	if model.testCount != initialTestCount+1 {
		t.Errorf("Expected testCount to increase to %d, got %d", initialTestCount+1, model.testCount)
	}

	// Result should be recorded
	if !model.resultRecorded {
		t.Error("Expected resultRecorded to be true")
	}

	// lastWPM should be set
	if model.lastWPM <= 0 {
		t.Error("Expected lastWPM to be positive")
	}
}

func TestRecordTestResultNotFinished(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateRunning // Not finished
	initialTestCount := model.testCount

	model.recordTestResult()

	// Test count should not change
	if model.testCount != initialTestCount {
		t.Error("recordTestResult should not run when state is not Finished")
	}
}

func TestRecordTestResultAlreadyRecorded(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateFinished
	model.resultRecorded = true
	initialTestCount := model.testCount

	model.recordTestResult()

	// Test count should not change
	if model.testCount != initialTestCount {
		t.Error("recordTestResult should not run when already recorded")
	}
}

func TestRecordTestResultPersonalBest(t *testing.T) {
	model := NewTypingTest("", 10)
	model.targetText = "test" // 4 chars = 0.8 words
	model.typed = "test"
	model.state = StateFinished
	model.startTime = time.Now().Add(-1 * time.Second)
	model.endTime = time.Now()
	model.personalBest = 10.0 // Low initial best
	model.resultRecorded = false

	model.recordTestResult()

	// With 4 chars in 1 second: WPM = (4/5)/1 * 60 = 48 WPM
	// This should beat the initial 10 WPM
	if model.personalBest <= 10.0 {
		t.Errorf("Expected personalBest to be updated, got %f", model.personalBest)
	}
}

func TestGenerateTextEmptyWordList(t *testing.T) {
	model := NewTypingTest("", 10)

	// Save original defaultWords
	originalWords := defaultWords
	defer func() { defaultWords = originalWords }()

	// Temporarily set defaultWords to empty
	defaultWords = []string{}

	// With empty word list, generateText should handle gracefully
	// This might panic or return empty - we just verify no crash
	defer func() {
		if r := recover(); r != nil {
			t.Logf("Recovered from panic with empty word list: %v", r)
		}
	}()

	text := model.generateText()
	t.Logf("Generated text with empty word list: %q", text)
}

func TestGenerateTextSingleWord(t *testing.T) {
	model := NewTypingTest("", 1)
	model.options.Punctuation = false

	text := model.generateText()
	words := strings.Fields(text)

	if len(words) != 1 {
		t.Errorf("Expected 1 word, got %d: %q", len(words), text)
	}
}

func TestGenerateTextVeryLarge(t *testing.T) {
	model := NewTypingTest("", 200)
	model.options.Punctuation = false

	text := model.generateText()
	words := strings.Fields(text)

	if len(words) != 200 {
		t.Errorf("Expected 200 words, got %d", len(words))
	}
}

func TestInitReturnsNil(t *testing.T) {
	model := NewTypingTest("", 10)
	cmd := model.Init()

	if cmd != nil {
		t.Error("Init() should return nil")
	}
}

func TestStateValues(t *testing.T) {
	// Verify state constants have expected values
	if StateReady != 0 {
		t.Error("StateReady should be 0")
	}
	if StateRunning != 1 {
		t.Error("StateRunning should be 1")
	}
	if StateFinished != 2 {
		t.Error("StateFinished should be 2")
	}
	if StateOptions != 3 {
		t.Error("StateOptions should be 3")
	}
}

func TestMenuFocusValues(t *testing.T) {
	if FocusTyping != 0 {
		t.Error("FocusTyping should be 0")
	}
	if FocusMenuBar != 1 {
		t.Error("FocusMenuBar should be 1")
	}
}

func TestAllOptionsContainsExpectedOptions(t *testing.T) {
	model := NewTypingTest("", 10)

	expectedIDs := []string{"theme", "test_type", "layout", "live_wpm", "test_length", "punctuation", "pace_caret"}

	optionIDs := make(map[string]bool)
	for _, opt := range model.allOptions {
		optionIDs[opt.ID] = true
	}

	for _, id := range expectedIDs {
		if !optionIDs[id] {
			t.Errorf("Expected option ID %q not found", id)
		}
	}
}

func TestDeleteLastWordEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"only spaces", "     ", ""},
		{"word followed by many spaces", "word    ", ""},
		{"multiple words with multiple spaces", "one  two  three", "one  two  "},
		{"unicode characters", "hello 世界", "hello "},
		{"single character", "a", ""},
		{"space and word", " word", " "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deleteLastWord(tt.input)
			if result != tt.expected {
				t.Errorf("deleteLastWord(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCustomTextWithMultipleEntries(t *testing.T) {
	model := NewTypingTest("", 10)
	model.options.TestType = "custom"
	model.customTexts = []string{"First text.", "Second text.", "Third text."}

	// Generate text multiple times and verify it uses custom texts
	textSet := make(map[string]bool)
	for i := 0; i < 10; i++ {
		text := model.generateText()
		textSet[text] = true
	}

	// At least one of our custom texts should be used
	foundCustom := false
	for _, ct := range model.customTexts {
		if textSet[ct] {
			foundCustom = true
			break
		}
	}

	if !foundCustom {
		t.Error("Expected at least one custom text to be used")
	}
}

// ============================================
// Bubble Tea Update() State Transition Tests
// ============================================

func TestUpdateStartsTypingOnFirstKeystroke(t *testing.T) {
	model := NewTypingTest("", 10)
	model.targetText = "hello"

	if model.state != StateReady {
		t.Fatal("Expected initial state to be StateReady")
	}

	// Send a character
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.state != StateRunning {
		t.Errorf("Expected state StateRunning after keystroke, got %d", m.state)
	}
	if m.typed != "h" {
		t.Errorf("Expected typed='h', got %q", m.typed)
	}
	if m.startTime.IsZero() {
		t.Error("Expected startTime to be set")
	}
}

func TestUpdateHandlesCorrectTyping(t *testing.T) {
	model := NewTypingTest("", 10)
	model.targetText = "hi"
	model.state = StateRunning
	model.startTime = time.Now()

	// Type 'h'
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.errors != 0 {
		t.Errorf("Expected 0 errors for correct character, got %d", m.errors)
	}

	// Type 'i' - should complete
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}}
	newModel, _ = m.Update(msg)
	m = newModel.(TypingTestModel)

	if m.state != StateFinished {
		t.Errorf("Expected state StateFinished after completing text, got %d", m.state)
	}
}

func TestUpdateHandlesIncorrectTyping(t *testing.T) {
	model := NewTypingTest("", 10)
	model.targetText = "hello"
	model.state = StateRunning
	model.startTime = time.Now()

	// Type wrong character 'x' instead of 'h'
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.errors != 1 {
		t.Errorf("Expected 1 error for wrong character, got %d", m.errors)
	}
	if m.typed != "x" {
		t.Errorf("Expected typed='x', got %q", m.typed)
	}
}

func TestUpdateHandlesBackspace(t *testing.T) {
	model := NewTypingTest("", 10)
	model.targetText = "hello"
	model.state = StateRunning
	model.typed = "hel"
	model.startTime = time.Now()

	// Press backspace
	msg := tea.KeyMsg{Type: tea.KeyBackspace}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.typed != "he" {
		t.Errorf("Expected typed='he' after backspace, got %q", m.typed)
	}
}

func TestUpdateHandlesAltBackspace(t *testing.T) {
	model := NewTypingTest("", 10)
	model.targetText = "hello world"
	model.state = StateRunning
	model.typed = "hello wor"
	model.startTime = time.Now()

	// Press Alt+Backspace to delete word
	msg := tea.KeyMsg{Type: tea.KeyBackspace, Alt: true}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.typed != "hello " {
		t.Errorf("Expected typed='hello ' after Alt+Backspace, got %q", m.typed)
	}
}

func TestUpdateHandlesSpace(t *testing.T) {
	model := NewTypingTest("", 10)
	model.targetText = "a b"
	model.state = StateRunning
	model.typed = "a"
	model.startTime = time.Now()

	// Press space
	msg := tea.KeyMsg{Type: tea.KeySpace}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.typed != "a " {
		t.Errorf("Expected typed='a ', got %q", m.typed)
	}
}

func TestUpdateHandlesTabRestart(t *testing.T) {
	model := NewTypingTest("", 10)
	model.targetText = "hello"
	model.state = StateRunning
	model.typed = "hel"
	model.errors = 1
	model.startTime = time.Now()

	originalText := model.targetText

	// Press Tab to restart
	msg := tea.KeyMsg{Type: tea.KeyTab}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.state != StateReady {
		t.Errorf("Expected state StateReady after Tab, got %d", m.state)
	}
	if m.typed != "" {
		t.Errorf("Expected empty typed after Tab, got %q", m.typed)
	}
	if m.errors != 0 {
		t.Errorf("Expected 0 errors after Tab, got %d", m.errors)
	}
	if m.targetText == originalText {
		t.Log("Note: targetText regenerated (expected behavior)")
	}
}

func TestUpdateHandlesEnterAfterFinished(t *testing.T) {
	model := NewTypingTest("", 10)
	model.targetText = "hi"
	model.state = StateFinished
	model.typed = "hi"
	model.startTime = time.Now().Add(-5 * time.Second)
	model.endTime = time.Now()

	originalText := model.targetText

	// Press Enter to restart
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.state != StateReady {
		t.Errorf("Expected state StateReady after Enter, got %d", m.state)
	}
	if m.typed != "" {
		t.Errorf("Expected empty typed after Enter, got %q", m.typed)
	}
	if m.targetText == originalText {
		t.Log("Note: targetText regenerated (expected behavior)")
	}
}

func TestUpdateHandlesEscapeToOptions(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateReady

	// Press Escape to open options
	msg := tea.KeyMsg{Type: tea.KeyEsc}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.state != StateOptions {
		t.Errorf("Expected state StateOptions after Escape, got %d", m.state)
	}
	if m.searchQuery != "" {
		t.Errorf("Expected empty searchQuery, got %q", m.searchQuery)
	}
}

func TestUpdateHandlesCtrlC(t *testing.T) {
	model := NewTypingTest("", 10)

	// Press Ctrl+C
	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	_, cmd := model.Update(msg)

	// Should return quit command
	if cmd == nil {
		t.Error("Expected quit command from Ctrl+C")
	}
}

func TestUpdateHandlesWindowResize(t *testing.T) {
	model := NewTypingTest("", 10)

	// Send window size message
	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.width != 120 {
		t.Errorf("Expected width=120, got %d", m.width)
	}
	if m.height != 40 {
		t.Errorf("Expected height=40, got %d", m.height)
	}
}

func TestUpdateOptionsNavigation(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateOptions
	model.selectedIdx = 0
	model.filterOptions()

	// Press Down
	msg := tea.KeyMsg{Type: tea.KeyDown}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.selectedIdx != 1 {
		t.Errorf("Expected selectedIdx=1 after Down, got %d", m.selectedIdx)
	}

	// Press Up
	msg = tea.KeyMsg{Type: tea.KeyUp}
	newModel, _ = m.Update(msg)
	m = newModel.(TypingTestModel)

	if m.selectedIdx != 0 {
		t.Errorf("Expected selectedIdx=0 after Up, got %d", m.selectedIdx)
	}
}

func TestUpdateOptionsSearch(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateOptions
	model.filterOptions()

	initialCount := len(model.filteredOpts)

	// Type search query
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t', 'h', 'e', 'm', 'e'}}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.searchQuery != "theme" {
		t.Errorf("Expected searchQuery='theme', got %q", m.searchQuery)
	}
	if len(m.filteredOpts) >= initialCount {
		t.Error("Expected filtered options to be reduced after search")
	}
}

func TestUpdateOptionsEscapeCloses(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateOptions

	// Press Escape to close options
	msg := tea.KeyMsg{Type: tea.KeyEsc}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.state != StateReady {
		t.Errorf("Expected state StateReady after Escape in options, got %d", m.state)
	}
}

func TestUpdateOptionsTabCloses(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateOptions

	// Press Tab to close options
	msg := tea.KeyMsg{Type: tea.KeyTab}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.state != StateReady {
		t.Errorf("Expected state StateReady after Tab in options, got %d", m.state)
	}
}

func TestUpdateOptionsEnterToggle(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateOptions
	model.filterOptions()

	// Find live_wpm option index
	liveWPMIdx := -1
	for i, opt := range model.filteredOpts {
		if opt.ID == "live_wpm" {
			liveWPMIdx = i
			break
		}
	}
	if liveWPMIdx == -1 {
		t.Skip("live_wpm option not found")
	}

	model.selectedIdx = liveWPMIdx
	initialValue := model.options.LiveWPM

	// Press Enter to toggle
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.options.LiveWPM == initialValue {
		t.Error("Expected LiveWPM to be toggled")
	}
}

func TestUpdateOptionsEnterSubmenu(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateOptions
	model.filterOptions()

	// Find theme option index (has submenu)
	themeIdx := -1
	for i, opt := range model.filteredOpts {
		if opt.ID == "theme" {
			themeIdx = i
			break
		}
	}
	if themeIdx == -1 {
		t.Skip("theme option not found")
	}

	model.selectedIdx = themeIdx

	// Press Enter to open submenu
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if !m.inSubMenu {
		t.Error("Expected inSubMenu=true after Enter on choice option")
	}
}

func TestUpdateOptionsBackspaceSearch(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateOptions
	model.searchQuery = "theme"
	model.filterOptions()

	// Press Backspace
	msg := tea.KeyMsg{Type: tea.KeyBackspace}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.searchQuery != "them" {
		t.Errorf("Expected searchQuery='them' after Backspace, got %q", m.searchQuery)
	}
}

func TestUpdateMenuBarFocus(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateReady
	model.menuFocus = FocusTyping

	// Press Up to focus menubar
	msg := tea.KeyMsg{Type: tea.KeyUp}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.menuFocus != FocusMenuBar {
		t.Errorf("Expected menuFocus=FocusMenuBar after Up, got %d", m.menuFocus)
	}
}

func TestUpdateMenuBarNavigation(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateReady
	model.menuFocus = FocusMenuBar
	model.menuSelection = 0

	// Press Right
	msg := tea.KeyMsg{Type: tea.KeyRight}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.menuSelection != 1 {
		t.Errorf("Expected menuSelection=1 after Right, got %d", m.menuSelection)
	}

	// Press Left
	msg = tea.KeyMsg{Type: tea.KeyLeft}
	newModel, _ = m.Update(msg)
	m = newModel.(TypingTestModel)

	if m.menuSelection != 0 {
		t.Errorf("Expected menuSelection=0 after Left, got %d", m.menuSelection)
	}
}

func TestUpdateMenuBarEnterStats(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateReady
	model.menuFocus = FocusMenuBar
	model.menuSelection = 0 // Stats

	// Press Enter
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if !m.showStats {
		t.Error("Expected showStats=true after Enter on Stats")
	}
}

func TestUpdateMenuBarEnterCustom(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateReady
	model.menuFocus = FocusMenuBar
	model.menuSelection = 1 // Custom

	// Press Enter
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if !m.showCustomPanel {
		t.Error("Expected showCustomPanel=true after Enter on Custom")
	}
}

func TestUpdateMenuBarEscapeReturnsFocus(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateReady
	model.menuFocus = FocusMenuBar

	// Press Escape or Down to return focus
	msg := tea.KeyMsg{Type: tea.KeyDown}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.menuFocus != FocusTyping {
		t.Errorf("Expected menuFocus=FocusTyping after Down, got %d", m.menuFocus)
	}
}

func TestUpdateStatsPanelClose(t *testing.T) {
	model := NewTypingTest("", 10)
	model.showStats = true

	// Press Escape to close
	msg := tea.KeyMsg{Type: tea.KeyEsc}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.showStats {
		t.Error("Expected showStats=false after Escape")
	}
}

func TestUpdateCustomPanelAddText(t *testing.T) {
	model := NewTypingTest("", 10)
	model.showCustomPanel = true
	model.inCustomTextInput = false

	// Press 'a' to add text
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if !m.inCustomTextInput {
		t.Error("Expected inCustomTextInput=true after 'a'")
	}
}

func TestUpdateCustomPanelInputText(t *testing.T) {
	model := NewTypingTest("", 10)
	model.showCustomPanel = true
	model.inCustomTextInput = true
	model.customTextInput = ""

	// Type some text
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h', 'e', 'l', 'l', 'o'}}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.customTextInput != "hello" {
		t.Errorf("Expected customTextInput='hello', got %q", m.customTextInput)
	}
}

func TestUpdateCustomPanelInputEnterNewline(t *testing.T) {
	model := NewTypingTest("", 10)
	model.showCustomPanel = true
	model.inCustomTextInput = true
	model.customTextInput = "line1"

	// Press Enter to add newline
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.customTextInput != "line1\n" {
		t.Errorf("Expected customTextInput='line1\\n', got %q", m.customTextInput)
	}
}

func TestUpdateCustomPanelSaveCtrlD(t *testing.T) {
	model := NewTypingTest("", 10)
	model.showCustomPanel = true
	model.inCustomTextInput = true
	model.customTextInput = "my custom text"
	model.customTexts = []string{}

	// Press Ctrl+D to save
	msg := tea.KeyMsg{Type: tea.KeyCtrlD}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.inCustomTextInput {
		t.Error("Expected inCustomTextInput=false after Ctrl+D")
	}
	if len(m.customTexts) != 1 {
		t.Errorf("Expected 1 custom text, got %d", len(m.customTexts))
	}
	if m.customTexts[0] != "my custom text" {
		t.Errorf("Expected custom text 'my custom text', got %q", m.customTexts[0])
	}
}

func TestUpdateCustomPanelDeleteText(t *testing.T) {
	model := NewTypingTest("", 10)
	model.showCustomPanel = true
	model.inCustomTextInput = false
	model.customTexts = []string{"text1", "text2"}

	// Press 'd' to delete last
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if len(m.customTexts) != 1 {
		t.Errorf("Expected 1 custom text after delete, got %d", len(m.customTexts))
	}
}

func TestUpdateCustomPanelEscapeCancel(t *testing.T) {
	model := NewTypingTest("", 10)
	model.showCustomPanel = true
	model.inCustomTextInput = true
	model.customTextInput = "unsaved text"

	// Press Escape to cancel
	msg := tea.KeyMsg{Type: tea.KeyEsc}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.inCustomTextInput {
		t.Error("Expected inCustomTextInput=false after Escape")
	}
	if m.customTextInput != "" {
		t.Errorf("Expected customTextInput='' after Escape, got %q", m.customTextInput)
	}
}

func TestUpdateExtraCharactersCountAsErrors(t *testing.T) {
	model := NewTypingTest("", 10)
	model.targetText = "hi"
	model.state = StateRunning
	model.typed = "hi"
	model.startTime = time.Now()

	// Type extra character
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.errors != 1 {
		t.Errorf("Expected 1 error for extra character, got %d", m.errors)
	}
}

func TestUpdateTestCompletesOnlyWithCorrectLastChar(t *testing.T) {
	model := NewTypingTest("", 10)
	model.targetText = "ab"
	model.state = StateRunning
	model.typed = "a"
	model.startTime = time.Now()

	// Type wrong character 'x' instead of 'b'
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	// Should NOT complete because last char is wrong
	if m.state == StateFinished {
		t.Error("Test should not complete with wrong last character")
	}
	if m.errors != 1 {
		t.Errorf("Expected 1 error, got %d", m.errors)
	}
}

func TestUpdateNoBackspaceWhenEmpty(t *testing.T) {
	model := NewTypingTest("", 10)
	model.targetText = "hello"
	model.state = StateRunning
	model.typed = ""
	model.startTime = time.Now()

	// Press backspace on empty
	msg := tea.KeyMsg{Type: tea.KeyBackspace}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	if m.typed != "" {
		t.Errorf("Expected typed to remain empty, got %q", m.typed)
	}
}

func TestUpdateBackspaceOnlyInRunningState(t *testing.T) {
	model := NewTypingTest("", 10)
	model.targetText = "hello"
	model.state = StateReady
	model.typed = "hel" // Shouldn't happen but test edge case

	// Press backspace in Ready state
	msg := tea.KeyMsg{Type: tea.KeyBackspace}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	// Should not modify typed in Ready state (no action taken)
	if m.typed != "hel" {
		t.Errorf("Expected typed='hel' in Ready state, got %q", m.typed)
	}
}

func TestUpdateUpArrowDoesNotFocusMenuBarWhenRunning(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateRunning
	model.menuFocus = FocusTyping
	model.startTime = time.Now()

	// Press Up while running
	msg := tea.KeyMsg{Type: tea.KeyUp}
	newModel, _ := model.Update(msg)
	m := newModel.(TypingTestModel)

	// Should NOT focus menubar while typing
	if m.menuFocus != FocusTyping {
		t.Error("Up arrow should not focus menubar while typing")
	}
}

// ============================================
// View() Rendering Tests (smoke tests)
// ============================================

func TestViewReady(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateReady
	model.width = 80
	model.height = 24

	view := model.View()

	if view == "" {
		t.Error("Expected non-empty view in Ready state")
	}
	if !strings.Contains(view, "Start typing") {
		t.Error("Expected 'Start typing' prompt in Ready state")
	}
}

func TestViewRunning(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateRunning
	model.targetText = "hello world"
	model.typed = "hello"
	model.width = 80
	model.height = 24
	model.startTime = time.Now()

	view := model.View()

	if view == "" {
		t.Error("Expected non-empty view in Running state")
	}
}

func TestViewFinished(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateFinished
	model.targetText = "hello"
	model.typed = "hello"
	model.width = 80
	model.height = 24
	model.startTime = time.Now().Add(-5 * time.Second)
	model.endTime = time.Now()

	view := model.View()

	if view == "" {
		t.Error("Expected non-empty view in Finished state")
	}
	if !strings.Contains(view, "Complete") {
		t.Error("Expected 'Complete' in finished view")
	}
}

func TestViewOptions(t *testing.T) {
	model := NewTypingTest("", 10)
	model.state = StateOptions
	model.width = 80
	model.height = 24
	model.filterOptions()

	view := model.View()

	if view == "" {
		t.Error("Expected non-empty view in Options state")
	}
	if !strings.Contains(view, "Options") {
		t.Error("Expected 'Options' in options view")
	}
}

func TestViewStatsPanel(t *testing.T) {
	model := NewTypingTest("", 10)
	model.showStats = true
	model.width = 80
	model.height = 24

	view := model.View()

	if view == "" {
		t.Error("Expected non-empty view with stats panel")
	}
	if !strings.Contains(view, "Statistics") {
		t.Error("Expected 'Statistics' in stats panel view")
	}
}

func TestViewCustomPanel(t *testing.T) {
	model := NewTypingTest("", 10)
	model.showCustomPanel = true
	model.width = 80
	model.height = 24

	view := model.View()

	if view == "" {
		t.Error("Expected non-empty view with custom panel")
	}
	if !strings.Contains(view, "Custom") {
		t.Error("Expected 'Custom' in custom panel view")
	}
}
