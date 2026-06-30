package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/aayushbajaj/typing-telemetry/internal/charts"
	"github.com/aayushbajaj/typing-telemetry/internal/storage"
	"github.com/aayushbajaj/typing-telemetry/internal/tui"
	"github.com/aayushbajaj/typing-telemetry/pkg/stats"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags: -X main.Version=$(VERSION)
var Version = "dev"

var (
	// Flags for test command
	testFile      string
	testWordCount int
	testLanguage  string

	// JSON output flag for `today` and `stats` (machine-readable surface
	// consumed by other tools like macos-watchdog).
	jsonOutput bool

	// deviceFilter (--device) reroutes `today`/`stats` to read an external
	// device's tables instead of this Mac's daily_summary.
	deviceFilter string
)

var rootCmd = &cobra.Command{
	Use:   "typtel",
	Short: "Typing telemetry - track your keystrokes",
	Long: `Typtel — keystroke & typing telemetry for developers.

Run with no arguments to open the interactive dashboard (TUI).

SEE YOUR TYPING STATS
  typtel today                 Today's keystroke count
  typtel today --json          Today's full breakdown (letters/modifiers/special/words)
  typtel stats                 Today + this week, plus typing speed (WPM)
  typtel devices show <id>     Per-day table for an external device,
                               with letters/modifiers/special/words/active time
  typtel v                     Open the charts/heatmap in a browser

PRACTICE
  typtel test                  Interactive typing speed test (25 words)
  typtel test -w 50            Longer test (50 words)
  typtel test -l au            Use AU English spelling (saved as default)
    in-test keys: tab=new words  esc=options  enter=start  ctrl+c=quit

DEVICE FEEDS (optional — external devices that push their own stats)
  typtel devices               List registered devices and today's count
  typtel devices token         Print the ingest bearer token
  typtel devices enable        Enable the device ingest API

  typtel help <command>        Detailed help for any command
  typtel version               Version info`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI()
	},
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show typing statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		if deviceFilter != "" {
			return runDevicesShow(deviceFilter)
		}
		if jsonOutput {
			return runStatsJSON()
		}
		return showStats()
	},
}

var todayCmd = &cobra.Command{
	Use:   "today",
	Short: "Show today's keystroke count (for menu bar)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if deviceFilter != "" {
			if jsonOutput {
				return runDeviceTodayJSON(deviceFilter)
			}
			return runDeviceTodayText(deviceFilter)
		}
		if jsonOutput {
			return runTodayJSON()
		}
		return showToday()
	},
}

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Start a typing test",
	Long: `Start an interactive typing test to measure your WPM and accuracy.

Examples:
  typtel test                    # Default 25-word test
  typtel test -w 50              # 50-word test
  typtel test -f words.txt       # Use custom word list
  typtel test -f passage.txt -w 100  # 100 words from custom file`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTypingTest()
	},
}

var viewCmd = &cobra.Command{
	Use:     "v",
	Aliases: []string{"view", "charts"},
	Short:   "View typing statistics charts in browser",
	RunE: func(cmd *cobra.Command, args []string) error {
		return viewCharts()
	},
}

var versionCmd = &cobra.Command{
	Use:     "version",
	Aliases: []string{"info"},
	Short:   "Show version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Typtel v%s\n", Version)
		fmt.Println("Keystroke and mouse distance metrics for developers")
		fmt.Println()
		fmt.Println("Homepage: https://github.com/abaj8494/typing-telemetry")
		fmt.Println("License:  MIT")
	},
}

func init() {
	testCmd.Flags().StringVarP(&testFile, "file", "f", "", "Path to text file with words/passages")
	testCmd.Flags().IntVarP(&testWordCount, "words", "w", 25, "Number of words in the test")
	testCmd.Flags().StringVarP(&testLanguage, "language", "l", "", "Language variant: us, au (saved as default)")

	todayCmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON instead of text")
	statsCmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON instead of text")
	todayCmd.Flags().StringVar(&deviceFilter, "device", "", "Read an external device's stats instead of this Mac's")
	statsCmd.Flags().StringVar(&deviceFilter, "device", "", "Read an external device's stats instead of this Mac's")

	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(todayCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(viewCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(devicesCmd)
	rootCmd.AddCommand(pushCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runTUI() error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	p := tea.NewProgram(tui.New(store), tea.WithAltScreen())
	model, err := p.Run()
	if err != nil {
		return err
	}

	// Check if user wants to switch to typing test or charts
	if m, ok := model.(tui.Model); ok {
		if m.SwitchToTypingTest {
			return runTypingTest()
		}
		if m.SwitchToCharts {
			return viewCharts()
		}
	}

	return nil
}

func runTypingTest() error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	// If language specified via CLI, save it as the new default
	if testLanguage != "" {
		lang := testLanguage
		if lang == "au" || lang == "AU" {
			lang = tui.LanguageAU
		} else {
			lang = tui.LanguageUS
		}
		store.SetTypingTestLanguage(lang)
	}

	p := tea.NewProgram(
		tui.NewTypingTestWithStore(testFile, testWordCount, store),
		tea.WithAltScreen(),
	)
	_, err = p.Run()
	return err
}

func showStats() error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	// Ensure historical active time exists so speed stats are meaningful even
	// if the menubar hasn't run since upgrading. Guarded — runs at most once.
	if err := store.BackfillActiveTime(); err != nil {
		return fmt.Errorf("backfill active time: %w", err)
	}

	today, err := store.GetTodayStats()
	if err != nil {
		return fmt.Errorf("failed to get today's stats: %w", err)
	}

	week, err := store.GetWeekStats()
	if err != nil {
		return fmt.Errorf("failed to get week stats: %w", err)
	}

	var weekTotal int64
	for _, day := range week {
		weekTotal += day.Keystrokes
	}

	// Calculate week words from daily stats
	var weekWords int64
	for _, day := range week {
		weekWords += day.Words
	}

	fmt.Println("📊 Typing Statistics")
	fmt.Println("────────────────────")
	fmt.Printf("Today:     %s keystrokes (%s words)\n", formatNum(today.Keystrokes), formatNum(today.Words))
	fmt.Printf("This week: %s keystrokes (%s words)\n", formatNum(weekTotal), formatNum(weekWords))
	fmt.Printf("Daily avg: %s keystrokes (%s words)\n", formatNum(weekTotal/7), formatNum(weekWords/7))

	// Typing speed: today's average plus all-time average and fastest pace.
	speedToday, err := store.GetSpeedAggregate(today.Date)
	if err != nil {
		return fmt.Errorf("failed to get speed stats: %w", err)
	}
	speedAll, err := store.GetSpeedAggregate("")
	if err != nil {
		return fmt.Errorf("failed to get speed stats: %w", err)
	}
	fmt.Printf("Speed:     %s today, %s all-time (fastest %s)\n",
		formatWPM(stats.AverageWPM(speedToday.Words, speedToday.ActiveMs)),
		formatWPM(stats.AverageWPM(speedAll.Words, speedAll.ActiveMs)),
		formatWPM(bestFastest(speedAll)))

	return nil
}

// formatWPM renders a words-per-minute value, with a dash before any pace has
// been recorded.
func formatWPM(wpm float64) string {
	if wpm <= 0 {
		return "-- WPM"
	}
	return fmt.Sprintf("%.0f WPM", wpm)
}

// bestFastest returns the highest of the three tracked fastest-pace metrics.
func bestFastest(a storage.SpeedAggregate) float64 {
	best := a.FastestBurstWPM
	if a.FastestWindowWPM > best {
		best = a.FastestWindowWPM
	}
	if a.FastestMinuteWPM > best {
		best = a.FastestMinuteWPM
	}
	return best
}

func formatNum(n int64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func showToday() error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	today, err := store.GetTodayStats()
	if err != nil {
		return fmt.Errorf("failed to get today's stats: %w", err)
	}

	// Output format suitable for menu bar scripts
	fmt.Printf("%d\n", today.Keystrokes)
	return nil
}

func viewCharts() error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	htmlPath, err := charts.Generate(store, charts.Options{})
	if err != nil {
		return fmt.Errorf("failed to generate charts: %w", err)
	}

	fmt.Printf("Opening charts: %s\n", htmlPath)
	return openInBrowser(htmlPath)
}

// openInBrowser opens path (a local file or URL) in the default browser,
// choosing the right launcher per OS so charts work on Linux as well as macOS.
func openInBrowser(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
	default: // linux, *bsd, …
		return exec.Command("xdg-open", path).Start()
	}
}

