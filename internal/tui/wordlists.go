package tui

import (
	_ "embed"
	"strings"
)

// Language constants
const (
	LanguageUS = "us"
	LanguageAU = "au"
)

//go:embed wordlists/english_common.txt
var englishCommonWords string

//go:embed wordlists/eff_words.txt
var effWords string

//go:embed wordlists/programming.txt
var programmingWords string

// LoadEmbeddedWordLists returns the combined word list from embedded files
func LoadEmbeddedWordLists() []string {
	var words []string

	// Parse english common words (min 4 chars to avoid abbreviations like "vpn", "pf", "gg")
	for _, line := range strings.Split(englishCommonWords, "\n") {
		word := strings.TrimSpace(line)
		if word != "" && len(word) >= 4 && len(word) <= 15 {
			words = append(words, word)
		}
	}

	// Parse EFF words (good quality, no offensive content)
	for _, line := range strings.Split(effWords, "\n") {
		word := strings.TrimSpace(line)
		if word != "" && len(word) >= 4 {
			words = append(words, word)
		}
	}

	// Parse programming words
	for _, line := range strings.Split(programmingWords, "\n") {
		word := strings.TrimSpace(line)
		if word != "" && len(word) >= 4 {
			words = append(words, word)
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	unique := make([]string, 0, len(words))
	for _, word := range words {
		lower := strings.ToLower(word)
		if !seen[lower] {
			seen[lower] = true
			unique = append(unique, lower)
		}
	}

	return unique
}

// TransformToAU converts US English spelling to AU English
func TransformToAU(word string) string {
	// -ize -> -ise (realize -> realise)
	if strings.HasSuffix(word, "ize") {
		word = word[:len(word)-3] + "ise"
	} else if strings.HasSuffix(word, "izing") {
		word = word[:len(word)-5] + "ising"
	} else if strings.HasSuffix(word, "ized") {
		word = word[:len(word)-4] + "ised"
	} else if strings.HasSuffix(word, "ization") {
		word = word[:len(word)-7] + "isation"
	}

	// -or -> -our (color -> colour)
	orToOur := map[string]string{
		"color": "colour", "colors": "colours", "colored": "coloured", "coloring": "colouring",
		"favor": "favour", "favors": "favours", "favored": "favoured", "favorite": "favourite",
		"honor": "honour", "honors": "honours", "honored": "honoured", "honoring": "honouring",
		"labor": "labour", "labors": "labours", "labored": "laboured", "laboring": "labouring",
		"humor": "humour", "humors": "humours", "neighbor": "neighbour", "neighbors": "neighbours",
		"behavior": "behaviour", "behaviors": "behaviours", "flavor": "flavour", "flavors": "flavours",
	}
	if au, ok := orToOur[word]; ok {
		return au
	}

	// -er -> -re (center -> centre)
	erToRe := map[string]string{
		"center": "centre", "centers": "centres", "centered": "centred",
		"theater": "theatre", "theaters": "theatres",
		"meter": "metre", "meters": "metres",
		"liter": "litre", "liters": "litres",
		"fiber": "fibre", "fibers": "fibres",
	}
	if au, ok := erToRe[word]; ok {
		return au
	}

	// -og -> -ogue (catalog -> catalogue)
	ogToOgue := map[string]string{
		"catalog": "catalogue", "catalogs": "catalogues",
		"dialog": "dialogue", "dialogs": "dialogues",
		"analog": "analogue", "analogs": "analogues",
		"prolog": "prologue", "prologs": "prologues",
		"epilog": "epilogue", "epilogs": "epilogues",
	}
	if au, ok := ogToOgue[word]; ok {
		return au
	}

	return word
}

// LoadWordListsForLanguage returns word list transformed for the specified language
func LoadWordListsForLanguage(language string) []string {
	words := LoadEmbeddedWordLists()
	if language == LanguageAU {
		for i, word := range words {
			words[i] = TransformToAU(word)
		}
	}
	return words
}

func init() {
	// Load word lists from embedded files (default to US English)
	defaultWords = LoadEmbeddedWordLists()
}
