//go:build linux

// Command typtel-tray is the Linux counterpart to typtel-menubar: a background
// daemon that captures keystrokes into the shared SQLite store, optionally runs
// the inertia accelerating key-repeat, and surfaces live typing statistics in a
// StatusNotifier tray icon (XFCE, KDE, GNOME-with-appindicator, etc.).
//
// Capture and inertia are pure-Go X11 (internal/x11) — no root, no /dev/input,
// no special group; just a reachable $DISPLAY. The stats pipeline (wordcounter,
// speedtracker, storage) is shared verbatim with the macOS build.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"fyne.io/systray"

	"github.com/aayushbajaj/typing-telemetry/internal/charts"
	"github.com/aayushbajaj/typing-telemetry/internal/inertia"
	"github.com/aayushbajaj/typing-telemetry/internal/keylogger"
	"github.com/aayushbajaj/typing-telemetry/internal/push"
	"github.com/aayushbajaj/typing-telemetry/internal/speedtracker"
	"github.com/aayushbajaj/typing-telemetry/internal/storage"
	"github.com/aayushbajaj/typing-telemetry/internal/wordcounter"
	"github.com/aayushbajaj/typing-telemetry/pkg/stats"
)

// Version is set via -ldflags at build time.
var Version = "dev"

var (
	store *storage.Store
	speed = &speedAccumulator{}

	// Push loop state (opt-in; nil/no-op unless `typtel push enable` was run).
	pusher     *push.Client
	pushCancel context.CancelFunc
)

func main() {
	log.SetPrefix("typtel-tray: ")
	log.SetFlags(log.Ltime)

	if !keylogger.CheckAccessibilityPermissions() {
		log.Fatal("cannot reach an X display — is DISPLAY set? (this build needs an X11 session)")
	}

	var err error
	store, err = storage.New()
	if err != nil {
		log.Fatalf("failed to open store: %v", err)
	}
	speed.store = store

	keystrokeChan, err := keylogger.Start()
	if err != nil {
		log.Fatalf("failed to start keylogger: %v", err)
	}
	go processKeystrokes(keystrokeChan)

	// Start inertia if it was left enabled in settings.
	if cfg := inertiaConfig(); cfg.Enabled {
		if err := inertia.Start(cfg); err != nil {
			log.Printf("warning: inertia failed to start: %v", err)
		} else {
			log.Println("inertia enabled")
		}
	}

	// Start the device-push loop if it was opted into via `typtel push enable`.
	// Off by default: with no push settings this block is a no-op and never
	// touches the network.
	startPushLoop()

	// Restore terminal/X state on Ctrl-C or kill by routing through onExit.
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		systray.Quit()
	}()

	systray.Run(onReady, onExit)
}

// processKeystrokes is the shared keystroke pipeline: record each key, credit
// active typing time, and count completed words with their fastest-pace
// candidates. Mirrors cmd/typtel-menubar's loop, minus the macOS-only per-app
// filtering / odometer / mouse paths.
func processKeystrokes(ch <-chan keylogger.KeystrokeEvent) {
	counter := wordcounter.New()
	tracker := speedtracker.New()
	for ev := range ch {
		if err := store.RecordKeystroke(ev.Keycode); err != nil {
			log.Printf("record keystroke: %v", err)
		}
		now := time.Now()
		date := now.Format("2006-01-02")
		if ms := tracker.OnKeystroke(now); ms > 0 {
			speed.addActive(date, ms)
		}
		if counter.Observe(wordcounter.Event{
			Keycode:   ev.Keycode,
			CmdHeld:   ev.CmdHeld(),
			CtrlHeld:  ev.CtrlHeld(),
			OptHeld:   ev.OptHeld(),
			ShiftHeld: ev.ShiftHeld(),
		}) {
			if err := store.IncrementWordCount(date); err != nil {
				log.Printf("increment words: %v", err)
			}
			speed.recordSample(date, tracker.OnWord(now))
		}
	}
}

func inertiaConfig() inertia.Config {
	s := store.GetInertiaSettings()
	return inertia.Config{
		Enabled:   s.Enabled,
		MaxSpeed:  s.MaxSpeed,
		Threshold: s.Threshold,
		AccelRate: s.AccelRate,
	}
}

// startPushLoop launches the background device-push loop if push was opted into.
// It never blocks keystroke capture and is a no-op when push is disabled.
func startPushLoop() {
	cfg, enabled, err := push.LoadConfig(store)
	if err != nil || !enabled {
		return
	}
	c, err := push.New(cfg)
	if err != nil {
		log.Printf("[push] not started: %v", err)
		return
	}
	pusher = c
	var ctx context.Context
	ctx, pushCancel = context.WithCancel(context.Background())
	go push.RunLoop(ctx, store, c, push.LoopConfig{Interval: 45 * time.Second, Logf: log.Printf})
	log.Printf("[push] enabled -> %s as %s", cfg.BaseURL, cfg.DeviceID) // never log the token
}

// Inertia radio-group menu items, keyed by their setting value, so a generic
// radio helper can tick exactly one and untick the rest (systray has only
// checkboxes — same emulation as the macOS menubar).
var (
	miSpeed  = map[string]*systray.MenuItem{}
	miThresh = map[int]*systray.MenuItem{}
	miAccel  = map[float64]*systray.MenuItem{}
)

// Ordered option tables (maps don't preserve order for the menu).
var (
	speedOpts = []struct{ val, label string }{
		{storage.InertiaSpeedUltraFast, "Ultra Fast (~140/s)"},
		{storage.InertiaSpeedVeryFast, "Very Fast (~125/s)"},
		{storage.InertiaSpeedPrettyFast, "Pretty Fast (~100/s)"},
		{storage.InertiaSpeedFast, "Fast (~83/s)"},
		{storage.InertiaSpeedMedium, "Medium (~50/s)"},
		{storage.InertiaSpeedSlow, "Slow (~20/s)"},
	}
	threshOpts = []struct {
		val   int
		label string
	}{
		{100, "100ms (instant)"}, {150, "150ms (fast)"}, {200, "200ms (default)"},
		{250, "250ms (slow)"}, {350, "350ms (very slow)"},
	}
	accelOpts = []struct {
		val   float64
		label string
	}{
		{0.25, "0.25x (very gentle)"}, {0.5, "0.5x (gentle)"}, {1.0, "1.0x (default)"},
		{1.5, "1.5x (faster)"}, {2.0, "2.0x (aggressive)"},
	}
)

func onReady() {
	systray.SetIcon(trayIcon())
	systray.SetTitle("typtel")
	systray.SetTooltip("typing-telemetry")

	// --- live stats (disabled, display-only) ---
	mKeys := systray.AddMenuItem("Keystrokes today: —", "")
	mWords := systray.AddMenuItem("Words today: —", "")
	mWPM := systray.AddMenuItem("Avg WPM: —", "")
	mFast := systray.AddMenuItem("Fastest WPM: —", "")
	for _, m := range []*systray.MenuItem{mKeys, mWords, mWPM, mFast} {
		m.Disable()
	}

	systray.AddSeparator()
	mCharts := systray.AddMenuItem("View Charts…", "Open the stats dashboard in your browser")

	// --- Inertia settings submenu ---
	is := store.GetInertiaSettings()
	mInertia := systray.AddMenuItem("Inertia", "Accelerating key-repeat settings")
	mInertiaEnable := mInertia.AddSubMenuItemCheckbox("Enable Inertia", "Toggle accelerating key-repeat", is.Enabled)

	mMaxSpeed := mInertia.AddSubMenuItem("Max Speed", "Top repeat speed cap")
	for _, o := range speedOpts {
		miSpeed[o.val] = mMaxSpeed.AddSubMenuItemCheckbox(o.label, "", is.MaxSpeed == o.val)
	}
	mThreshold := mInertia.AddSubMenuItem("Threshold", "Delay before acceleration starts")
	for _, o := range threshOpts {
		miThresh[o.val] = mThreshold.AddSubMenuItemCheckbox(o.label, "", is.Threshold == o.val)
	}
	mAccel := mInertia.AddSubMenuItem("Acceleration Rate", "How quickly speed ramps up")
	for _, o := range accelOpts {
		miAccel[o.val] = mAccel.AddSubMenuItemCheckbox(o.label, "", is.AccelRate == o.val)
	}

	// --- Charts settings ---
	mChartsSettings := systray.AddMenuItem("Chart Settings", "")
	mShowKeyTypes := mChartsSettings.AddSubMenuItemCheckbox(
		"Show Key Types (letters/modifiers/special)", "", store.IsShowKeyTypesEnabled())

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit typtel", "Stop capture and exit")

	refresh := func() {
		speed.flush()
		st, err := store.GetTodayStats()
		if err != nil || st == nil {
			return
		}
		wpm := stats.AverageWPM(st.Words, st.ActiveMs)
		fastest := st.FastestWindowWPM
		mKeys.SetTitle(fmt.Sprintf("Keystrokes today: %d", st.Keystrokes))
		mWords.SetTitle(fmt.Sprintf("Words today: %d", st.Words))
		mWPM.SetTitle(fmt.Sprintf("Avg WPM: %.0f", wpm))
		mFast.SetTitle(fmt.Sprintf("Fastest WPM: %.0f", fastest))
		systray.SetTooltip(fmt.Sprintf("typtel — %d keys · %d words · %.0f wpm",
			st.Keystrokes, st.Words, wpm))
	}

	go func() {
		refresh()
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for range t.C {
			refresh()
		}
	}()

	// --- click handlers (one goroutine per item; menu items are static) ---
	go func() {
		for range mCharts.ClickedCh {
			openCharts()
		}
	}()
	go func() {
		for range mInertiaEnable.ClickedCh {
			toggleInertiaEnabled(mInertiaEnable)
		}
	}()
	go func() {
		for range mShowKeyTypes.ClickedCh {
			toggleSetting(mShowKeyTypes, store.IsShowKeyTypesEnabled, store.SetShowKeyTypesEnabled)
		}
	}()
	for _, o := range speedOpts {
		val := o.val
		go func() {
			for range miSpeed[val].ClickedCh {
				store.SetInertiaMaxSpeed(val)
				radioCheck(miSpeed, val)
				applyInertia()
			}
		}()
	}
	for _, o := range threshOpts {
		val := o.val
		go func() {
			for range miThresh[val].ClickedCh {
				store.SetInertiaThreshold(val)
				radioCheck(miThresh, val)
				applyInertia()
			}
		}()
	}
	for _, o := range accelOpts {
		val := o.val
		go func() {
			for range miAccel[val].ClickedCh {
				store.SetInertiaAccelRate(val)
				radioCheck(miAccel, val)
				applyInertia()
			}
		}()
	}
	go func() {
		<-mQuit.ClickedCh
		systray.Quit()
	}()
}

// radioCheck ticks the item whose key equals sel and unticks the others.
func radioCheck[K comparable](items map[K]*systray.MenuItem, sel K) {
	for k, it := range items {
		if k == sel {
			it.Check()
		} else {
			it.Uncheck()
		}
	}
}

// applyInertia pushes the current persisted inertia settings to the running
// system (starts/stops/updates as the Enabled flag dictates).
func applyInertia() { inertia.UpdateConfig(inertiaConfig()) }

func toggleInertiaEnabled(item *systray.MenuItem) {
	enable := !store.GetInertiaSettings().Enabled
	if err := store.SetInertiaEnabled(enable); err != nil {
		log.Printf("persist inertia setting: %v", err)
	}
	applyInertia()
	if inertia.IsRunning() {
		item.Check()
		log.Println("inertia enabled")
	} else {
		item.Uncheck()
		log.Println("inertia disabled")
	}
}

// toggleSetting flips a boolean setting and syncs the checkbox.
func toggleSetting(item *systray.MenuItem, get func() bool, set func(bool) error) {
	v := !get()
	if err := set(v); err != nil {
		log.Printf("persist setting: %v", err)
		return
	}
	if v {
		item.Check()
	} else {
		item.Uncheck()
	}
}

// openCharts generates the rich charts dashboard and opens it in a browser.
func openCharts() {
	path, err := charts.Generate(store, charts.Options{})
	if err != nil {
		log.Printf("charts: %v", err)
		return
	}
	if err := exec.Command("xdg-open", path).Start(); err != nil {
		log.Printf("open charts: %v", err)
	}
}

func onExit() {
	speed.flush()
	// Final synchronous push so the day's last counts land before we close the
	// store. Runs after speed.flush() so active_ms is current.
	if pushCancel != nil {
		pushCancel()
	}
	if pusher != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := pusher.PushToday(ctx, store); err != nil {
			log.Printf("[push] final: %v", err)
		}
		cancel()
	}
	keylogger.Stop()
	inertia.Stop() // restores X auto-repeat
	if store != nil {
		store.Close()
	}
	log.Println("stopped")
}

// speedAccumulator batches active-time and fastest-pace writes so the keystroke
// goroutine never hits the DB for speed on every key (mirrors the menubar's
// design note: do not write speed per keystroke).
type speedAccumulator struct {
	store *storage.Store

	mu       sync.Mutex
	date     string
	activeMs int64
	burst    float64
	window   float64
	minute   float64
	dirty    bool
}

func (a *speedAccumulator) addActive(date string, ms int64) {
	a.mu.Lock()
	a.rollIfNeededLocked(date)
	a.activeMs += ms
	a.dirty = true
	a.mu.Unlock()
}

func (a *speedAccumulator) recordSample(date string, s speedtracker.Sample) {
	a.mu.Lock()
	a.rollIfNeededLocked(date)
	a.burst = maxf(a.burst, s.Burst)
	a.window = maxf(a.window, s.Window)
	a.minute = maxf(a.minute, s.Minute)
	a.dirty = true
	a.mu.Unlock()
}

// rollIfNeededLocked flushes the previous day's pending totals when the date
// rolls over (e.g. across midnight). Caller holds a.mu.
func (a *speedAccumulator) rollIfNeededLocked(date string) {
	if a.date != "" && a.date != date {
		a.flushLocked()
	}
	a.date = date
}

func (a *speedAccumulator) flush() {
	a.mu.Lock()
	a.flushLocked()
	a.mu.Unlock()
}

func (a *speedAccumulator) flushLocked() {
	if !a.dirty || a.date == "" || a.store == nil {
		return
	}
	if a.activeMs > 0 {
		if err := a.store.AddActiveTime(a.date, a.activeMs); err != nil {
			log.Printf("flush active time: %v", err)
		}
	}
	if a.burst > 0 || a.window > 0 || a.minute > 0 {
		if err := a.store.UpdateFastest(a.date, a.burst, a.window, a.minute); err != nil {
			log.Printf("flush fastest: %v", err)
		}
	}
	a.activeMs, a.burst, a.window, a.minute = 0, 0, 0, 0
	a.dirty = false
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
