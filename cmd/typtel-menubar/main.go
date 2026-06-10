//go:build darwin
// +build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework UserNotifications

#import <Cocoa/Cocoa.h>

// Modern alert dialog using NSAlert
static int showAlert(const char* messageText, const char* informativeText, const char** buttons, int buttonCount) {
    __block int result = 0;

    void (^showAlertBlock)(void) = ^{
        @autoreleasepool {
            NSAlert *alert = [[NSAlert alloc] init];
            [alert setMessageText:[NSString stringWithUTF8String:messageText]];
            [alert setInformativeText:[NSString stringWithUTF8String:informativeText]];
            [alert setAlertStyle:NSAlertStyleInformational];

            for (int i = 0; i < buttonCount; i++) {
                [alert addButtonWithTitle:[NSString stringWithUTF8String:buttons[i]]];
            }

            NSModalResponse response = [alert runModal];
            result = (int)(response - NSAlertFirstButtonReturn);
        }
    };

    if ([NSThread isMainThread]) {
        showAlertBlock();
    } else {
        dispatch_sync(dispatch_get_main_queue(), showAlertBlock);
    }

    return result;
}

// Open URL in default browser (synchronous, returns success)
static int openURL(const char* url) {
    __block int success = 0;

    void (^openBlock)(void) = ^{
        @autoreleasepool {
            NSURL *nsurl = [NSURL URLWithString:[NSString stringWithUTF8String:url]];
            if (nsurl != nil) {
                success = [[NSWorkspace sharedWorkspace] openURL:nsurl] ? 1 : 0;
            }
        }
    };

    if ([NSThread isMainThread]) {
        openBlock();
    } else {
        dispatch_sync(dispatch_get_main_queue(), openBlock);
    }

    return success;
}

// Open file in default application (synchronous, returns success)
static int openFile(const char* path) {
    __block int success = 0;

    void (^openBlock)(void) = ^{
        @autoreleasepool {
            NSString *pathStr = [NSString stringWithUTF8String:path];
            NSURL *fileURL = [NSURL fileURLWithPath:pathStr];
            if (fileURL != nil) {
                NSWorkspaceOpenConfiguration *config = [NSWorkspaceOpenConfiguration configuration];
                dispatch_semaphore_t sem = dispatch_semaphore_create(0);

                [[NSWorkspace sharedWorkspace] openURL:fileURL
                    configuration:config
                    completionHandler:^(NSRunningApplication *app, NSError *error) {
                        success = (error == nil) ? 1 : 0;
                        dispatch_semaphore_signal(sem);
                    }];

                dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC));
            }
        }
    };

    if ([NSThread isMainThread]) {
        openBlock();
    } else {
        dispatch_sync(dispatch_get_main_queue(), openBlock);
    }

    return success;
}
*/
import "C"

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/systray"
	"github.com/aayushbajaj/typing-telemetry/internal/appfilter"
	"github.com/aayushbajaj/typing-telemetry/internal/inertia"
	"github.com/aayushbajaj/typing-telemetry/internal/keylogger"
	"github.com/aayushbajaj/typing-telemetry/internal/mousetracker"
	"github.com/aayushbajaj/typing-telemetry/internal/storage"
	"github.com/aayushbajaj/typing-telemetry/internal/wordcounter"
	"github.com/aayushbajaj/typing-telemetry/pkg/stats"
)

var (
	store          *storage.Store
	lastMenuTitle  string
	menuTitleMutex sync.Mutex
	// appFilter is shared between the keystroke loop and the settings UI so
	// that toggling strict mode or editing the allowlist takes effect live.
	appFilter = appfilter.New()
)

// Version is set at build time via ldflags: -X main.Version=$(VERSION)
var Version = "dev"

// Menu item references for dynamic updates
var (
	mTodayKeystrokes    *systray.MenuItem
	mTodayMouse         *systray.MenuItem
	mWeekKeystrokes     *systray.MenuItem
	mWeekMouse          *systray.MenuItem
	mShowKeystrokes     *systray.MenuItem
	mShowWords          *systray.MenuItem
	mShowClicks         *systray.MenuItem
	mShowDistance       *systray.MenuItem
	mShowKeyTypes       *systray.MenuItem
	mDistanceFeet       *systray.MenuItem
	mDistanceCars       *systray.MenuItem
	mDistanceFields     *systray.MenuItem
	mMouseTracking      *systray.MenuItem
	mInertiaEnabled     *systray.MenuItem
	mInertiaUltraFast   *systray.MenuItem
	mInertiaVeryFast    *systray.MenuItem
	mInertiaPrettyFast  *systray.MenuItem
	mInertiaFast        *systray.MenuItem
	mInertiaMedium      *systray.MenuItem
	mInertiaSlow        *systray.MenuItem
	mThreshold100       *systray.MenuItem
	mThreshold150       *systray.MenuItem
	mThreshold200       *systray.MenuItem
	mThreshold250       *systray.MenuItem
	mThreshold350       *systray.MenuItem
	mAccelRate025       *systray.MenuItem
	mAccelRate050       *systray.MenuItem
	mAccelRate100       *systray.MenuItem
	mAccelRate150       *systray.MenuItem
	mAccelRate200       *systray.MenuItem
	leaderboardItems    []*systray.MenuItem
	mLeaderboardHeader  *systray.MenuItem
	leaderboardSubmenus *systray.MenuItem
	// Averages submenu items
	mAverages           *systray.MenuItem
	mAvgTodayKeystrokes *systray.MenuItem
	mAvgTodayWords      *systray.MenuItem
	mAvgTodayClicks     *systray.MenuItem
	mAvgTodayDistance   *systray.MenuItem
	mAvgWeekKeystrokes  *systray.MenuItem
	mAvgWeekWords       *systray.MenuItem
	mAvgWeekClicks      *systray.MenuItem
	mAvgWeekDistance    *systray.MenuItem
	// Daily averages (per day)
	mAvgDailyKeystrokes *systray.MenuItem
	mAvgDailyWords      *systray.MenuItem
	mAvgDailyClicks     *systray.MenuItem
	mAvgDailyDistance   *systray.MenuItem
	// Typing-speed submenu items
	mSpeed          *systray.MenuItem
	mSpeedAvgToday  *systray.MenuItem
	mSpeedAvgWeek   *systray.MenuItem
	mSpeedAvgMonth  *systray.MenuItem
	mSpeedAvgYear   *systray.MenuItem
	mSpeedAvgAll    *systray.MenuItem
	mSpeedFastBurst *systray.MenuItem
	mSpeedFastWin   *systray.MenuItem
	mSpeedFastMin   *systray.MenuItem
	// Odometer submenu items
	mOdometer             *systray.MenuItem
	mOdometerStatus       *systray.MenuItem
	mOdometerToggle       *systray.MenuItem
	mOdometerReset        *systray.MenuItem
	mOdometerClearHistory *systray.MenuItem
	mOdometerHotkey       *systray.MenuItem
	// Odometer hotkey settings submenu items
	mHotkeyCmdCtrlO   *systray.MenuItem
	mHotkeyCmdShiftO  *systray.MenuItem
	mHotkeyCmdOptO    *systray.MenuItem
	mHotkeyCtrlShiftO *systray.MenuItem
)

func init() {
	runtime.LockOSThread()
}

func main() {
	// Ensure HOME is set (needed when launched via launchctl/open)
	if os.Getenv("HOME") == "" {
		if u, err := user.Current(); err == nil {
			os.Setenv("HOME", u.HomeDir)
		}
	}

	// Set up logging
	logDir, err := getLogDir()
	if err != nil {
		log.Fatalf("Failed to get log directory: %v", err)
	}

	logFile, err := os.OpenFile(filepath.Join(logDir, "menubar.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(logFile)

	log.Println("Starting typtel menu bar app...")

	// Check accessibility permissions
	if !keylogger.CheckAccessibilityPermissions() {
		showPermissionAlert()
		os.Exit(1)
	}

	// Initialize storage
	store, err = storage.New()
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	// One-time backfill of historical active typing time from raw keystroke
	// timestamps, so average-WPM has real history on first launch of v1.4.
	if err := store.BackfillActiveTime(); err != nil {
		log.Printf("Active-time backfill failed: %v", err)
	}

	// Start keylogger in background
	keystrokeChan, err := keylogger.Start()
	if err != nil {
		log.Fatalf("Failed to start keylogger: %v", err)
	}
	defer keylogger.Stop()

	// Initialize per-app filter from persisted settings.
	appFilter.SetAllowlist(store.GetWordCountAllowlist())
	appFilter.SetEnabled(store.IsStrictWordCountEnabled())

	// Process keystrokes in background
	counter := wordcounter.New()
	go func() {
		var lastSeenBundle string
		for ev := range keystrokeChan {
			// Check for odometer hotkey (uses system modifier state, not tracked)
			if checkOdometerHotkey(ev.Keycode) {
				toggleOdometer()
				continue // Don't count hotkey as regular keystroke
			}

			// Record every app we observe so the settings UI can offer it
			// in the allowlist picker. Only writes when the bundle changes.
			if bundleID := appfilter.Frontmost(); bundleID != "" && bundleID != lastSeenBundle {
				lastSeenBundle = bundleID
				if err := store.RecordSeenApp(bundleID); err != nil {
					log.Printf("Failed to record seen app: %v", err)
				}
			}

			// Strict mode (opt-in): drop keystrokes from apps not on the
			// allowlist. We drop the keystroke entirely — not just the word
			// boundary — to match Feather's "this app's typing doesn't count"
			// semantics.
			if !appFilter.IsAllowed(lastSeenBundle) {
				counter.Reset()
				continue
			}

			if err := store.RecordKeystroke(ev.Keycode); err != nil {
				log.Printf("Failed to record keystroke: %v", err)
			}

			// Typing speed: credit active time (idle gaps auto-paused) and,
			// on a completed word, fold in the fastest-pace candidates. Both
			// are batched in speedAcc and flushed by the stats ticker.
			now := time.Now()
			date := now.Format("2006-01-02")
			if ms := speedTracker.OnKeystroke(now); ms > 0 {
				speedAcc.addActive(date, ms)
			}

			if counter.Observe(wordcounter.Event{
				Keycode:   ev.Keycode,
				CmdHeld:   ev.CmdHeld(),
				CtrlHeld:  ev.CtrlHeld(),
				OptHeld:   ev.OptHeld(),
				ShiftHeld: ev.ShiftHeld(),
			}) {
				if err := store.IncrementWordCount(date); err != nil {
					log.Printf("Failed to increment word count: %v", err)
				}
				speedAcc.recordSample(date, speedTracker.OnWord(now))
			}

			// Update odometer if active
			updateOdometerIfActive()
		}
	}()

	// Start mouse tracker if enabled
	mouseTrackingEnabled := store.IsMouseTrackingEnabled()
	if mouseTrackingEnabled {
		mouseChan, clickChan, err := mousetracker.Start()
		if err != nil {
			log.Printf("Warning: Failed to start mouse tracker: %v", err)
		} else {
			defer mousetracker.Stop()

			pos := mousetracker.GetCurrentPosition()
			date := time.Now().Format("2006-01-02")
			if err := store.SetMidnightPosition(date, pos.X, pos.Y); err != nil {
				log.Printf("Failed to set midnight position: %v", err)
			}

			go func() {
				currentDate := time.Now().Format("2006-01-02")
				for movement := range mouseChan {
					newDate := time.Now().Format("2006-01-02")
					if newDate != currentDate {
						currentDate = newDate
						if err := store.SetMidnightPosition(currentDate, movement.X, movement.Y); err != nil {
							log.Printf("Failed to set midnight position: %v", err)
						}
					}
					if err := store.RecordMouseMovement(movement.X, movement.Y, movement.Distance); err != nil {
						log.Printf("Failed to record mouse movement: %v", err)
					}
				}
			}()

			go func() {
				for range clickChan {
					if err := store.RecordMouseClick(); err != nil {
						log.Printf("Failed to record mouse click: %v", err)
					}
				}
			}()
		}
	} else {
		log.Println("Mouse tracking is disabled")
	}

	// Start inertia system if enabled
	inertiaSettings := store.GetInertiaSettings()
	if inertiaSettings.Enabled {
		inertiaCfg := inertia.Config{
			Enabled:   true,
			MaxSpeed:  inertiaSettings.MaxSpeed,
			Threshold: inertiaSettings.Threshold,
			AccelRate: inertiaSettings.AccelRate,
		}
		if err := inertia.Start(inertiaCfg); err != nil {
			log.Printf("Warning: Failed to start inertia: %v", err)
		} else {
			log.Println("Inertia system started")
			defer inertia.Stop()
		}
	} else {
		log.Println("Inertia is disabled")
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down...")
		systray.Quit()
	}()

	log.Println("Menu bar app starting...")

	// Run the systray app
	systray.Run(onReady, onExit)
}

func onReady() {
	// Set initial title
	systray.SetTitle("⌨️")
	systray.SetTooltip("Typing Telemetry")

	// Build the menu structure
	buildMenu()

	// Start update loop
	go func() {
		// Initial update after a short delay
		time.Sleep(1 * time.Second)
		updateMenuBarTitle()
		updateStatsDisplay()

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			speedAcc.flush(store)
			updateMenuBarTitle()
			updateStatsDisplay()
		}
	}()
}

func onExit() {
	log.Println("Systray exiting...")
	// Persist any speed measurements still buffered in memory.
	if store != nil {
		speedAcc.flush(store)
	}
}

func buildMenu() {
	// Today's stats
	mTodayKeystrokes = systray.AddMenuItem("Today: -- keystrokes (-- words)", "")
	mTodayKeystrokes.Disable()
	mTodayMouse = systray.AddMenuItem("Today: 🖱️ -- clicks, -- distance", "")
	mTodayMouse.Disable()

	systray.AddSeparator()

	// Week stats
	mWeekKeystrokes = systray.AddMenuItem("This Week: -- keystrokes (-- words)", "")
	mWeekKeystrokes.Disable()
	mWeekMouse = systray.AddMenuItem("This Week: 🖱️ -- clicks, -- distance", "")
	mWeekMouse.Disable()

	systray.AddSeparator()

	// Averages submenu
	mAverages = systray.AddMenuItem("📊 Averages", "Hourly averages for today and this week")

	// Today's averages header
	mTodayAvgHeader := mAverages.AddSubMenuItem("Today's Averages:", "")
	mTodayAvgHeader.Disable()

	mAvgTodayKeystrokes = mAverages.AddSubMenuItem("   -- keystrokes/hr", "")
	mAvgTodayKeystrokes.Disable()
	mAvgTodayWords = mAverages.AddSubMenuItem("   -- words/hr", "")
	mAvgTodayWords.Disable()
	mAvgTodayClicks = mAverages.AddSubMenuItem("   -- clicks/hr", "")
	mAvgTodayClicks.Disable()
	mAvgTodayDistance = mAverages.AddSubMenuItem("   -- distance/hr", "")
	mAvgTodayDistance.Disable()

	// Separator in submenu
	mAverages.AddSubMenuItem("", "").Disable()

	// Week's averages header
	mWeekAvgHeader := mAverages.AddSubMenuItem("This Week's Averages:", "")
	mWeekAvgHeader.Disable()

	mAvgWeekKeystrokes = mAverages.AddSubMenuItem("   -- keystrokes/hr", "")
	mAvgWeekKeystrokes.Disable()
	mAvgWeekWords = mAverages.AddSubMenuItem("   -- words/hr", "")
	mAvgWeekWords.Disable()
	mAvgWeekClicks = mAverages.AddSubMenuItem("   -- clicks/hr", "")
	mAvgWeekClicks.Disable()
	mAvgWeekDistance = mAverages.AddSubMenuItem("   -- distance/hr", "")
	mAvgWeekDistance.Disable()

	// Separator in submenu
	mAverages.AddSubMenuItem("", "").Disable()

	// Daily averages header
	mDailyAvgHeader := mAverages.AddSubMenuItem("Daily Averages (per day):", "")
	mDailyAvgHeader.Disable()

	mAvgDailyKeystrokes = mAverages.AddSubMenuItem("   -- keystrokes/day", "")
	mAvgDailyKeystrokes.Disable()
	mAvgDailyWords = mAverages.AddSubMenuItem("   -- words/day", "")
	mAvgDailyWords.Disable()
	mAvgDailyClicks = mAverages.AddSubMenuItem("   -- clicks/day", "")
	mAvgDailyClicks.Disable()
	mAvgDailyDistance = mAverages.AddSubMenuItem("   -- distance/day", "")
	mAvgDailyDistance.Disable()

	// Typing Speed submenu (WPM averages + Garmin-style fastest pace)
	mSpeed = systray.AddMenuItem("⚡ Typing Speed", "Average typing speed and fastest pace")

	mSpeedAvgHeader := mSpeed.AddSubMenuItem("Average WPM:", "")
	mSpeedAvgHeader.Disable()
	mSpeedAvgToday = mSpeed.AddSubMenuItem("   Today: -- WPM", "")
	mSpeedAvgToday.Disable()
	mSpeedAvgWeek = mSpeed.AddSubMenuItem("   This Week: -- WPM", "")
	mSpeedAvgWeek.Disable()
	mSpeedAvgMonth = mSpeed.AddSubMenuItem("   This Month: -- WPM", "")
	mSpeedAvgMonth.Disable()
	mSpeedAvgYear = mSpeed.AddSubMenuItem("   This Year: -- WPM", "")
	mSpeedAvgYear.Disable()
	mSpeedAvgAll = mSpeed.AddSubMenuItem("   All-Time: -- WPM", "")
	mSpeedAvgAll.Disable()

	mSpeed.AddSubMenuItem("", "").Disable() // Separator

	mSpeedFastHeader := mSpeed.AddSubMenuItem("Fastest Pace (all-time):", "")
	mSpeedFastHeader.Disable()
	mSpeedFastBurst = mSpeed.AddSubMenuItem("   10-word burst: -- WPM", "Fastest 10 consecutive words")
	mSpeedFastBurst.Disable()
	mSpeedFastWin = mSpeed.AddSubMenuItem("   60-second: -- WPM", "Most words in any rolling 60s window")
	mSpeedFastWin.Disable()
	mSpeedFastMin = mSpeed.AddSubMenuItem("   Best minute: -- WPM", "Most words in a single clock-minute")
	mSpeedFastMin.Disable()

	// Odometer submenu
	mOdometer = systray.AddMenuItem("⏱️ Odometer", "Track session metrics")

	// Get current odometer state
	odometerSession, _ := store.GetOdometerSession()
	odometerStatusText := "Inactive"
	toggleText := "Start Odometer"
	if odometerSession != nil && odometerSession.IsActive {
		odometerStatusText = "Active"
		toggleText = "Stop Odometer"
	}

	mOdometerStatus = mOdometer.AddSubMenuItem(fmt.Sprintf("Status: %s", odometerStatusText), "")
	mOdometerStatus.Disable()

	currentHotkey := store.GetOdometerHotkey()
	hotkeyDisplay := formatHotkeyDisplay(currentHotkey)
	mOdometerToggle = mOdometer.AddSubMenuItem(fmt.Sprintf("%s (%s)", toggleText, hotkeyDisplay), "")

	mOdometerReset = mOdometer.AddSubMenuItem("Reset Odometer", "")
	mOdometerClearHistory = mOdometer.AddSubMenuItem("Clear History", "")

	mOdometer.AddSubMenuItem("", "").Disable() // Separator

	// Hotkey configuration submenu
	mOdometerHotkey = mOdometer.AddSubMenuItem("Configure Hotkey", "")
	mHotkeyCmdCtrlO = mOdometerHotkey.AddSubMenuItemCheckbox("Cmd+Ctrl+O", "", currentHotkey == "cmd+ctrl+o")
	mHotkeyCmdShiftO = mOdometerHotkey.AddSubMenuItemCheckbox("Cmd+Shift+O", "", currentHotkey == "cmd+shift+o")
	mHotkeyCmdOptO = mOdometerHotkey.AddSubMenuItemCheckbox("Cmd+Opt+O", "", currentHotkey == "cmd+opt+o")
	mHotkeyCtrlShiftO = mOdometerHotkey.AddSubMenuItemCheckbox("Ctrl+Shift+O", "", currentHotkey == "ctrl+shift+o")

	// Handle odometer clicks
	go handleOdometerClicks()

	systray.AddSeparator()

	// View Charts
	mCharts := systray.AddMenuItem("View Charts", "Open statistics charts")
	go func() {
		for range mCharts.ClickedCh {
			openCharts()
		}
	}()

	// Leaderboard submenu
	leaderboardSubmenus = systray.AddMenuItem("🏆 Stillness Leaderboard", "Days with least mouse movement")
	mLeaderboardHeader = leaderboardSubmenus.AddSubMenuItem("🧘 Days You Didn't Move The Mouse", "")
	mLeaderboardHeader.Disable()
	// Pre-allocate leaderboard slots
	for i := 0; i < 10; i++ {
		item := leaderboardSubmenus.AddSubMenuItem("", "")
		item.Hide()
		leaderboardItems = append(leaderboardItems, item)
	}
	go func() {
		for range leaderboardSubmenus.ClickedCh {
			showLeaderboard()
		}
	}()

	systray.AddSeparator()

	// Settings submenu
	mSettings := systray.AddMenuItem("⚙️ Settings", "Configure display options")

	// Menu Bar Display section
	mDisplayLabel := mSettings.AddSubMenuItem("Menu Bar Display:", "")
	mDisplayLabel.Disable()

	settings := store.GetMenubarSettings()

	mShowKeystrokes = mSettings.AddSubMenuItemCheckbox("Show Keystrokes", "", settings.ShowKeystrokes)
	mShowWords = mSettings.AddSubMenuItemCheckbox("Show Words", "", settings.ShowWords)
	mShowClicks = mSettings.AddSubMenuItemCheckbox("Show Mouse Clicks", "", settings.ShowClicks)
	mShowDistance = mSettings.AddSubMenuItemCheckbox("Show Mouse Distance", "", settings.ShowDistance)

	// Distance Unit submenu
	mDistanceUnit := mSettings.AddSubMenuItem("   Distance Unit", "")
	currentUnit := store.GetDistanceUnit()
	mDistanceFeet = mDistanceUnit.AddSubMenuItemCheckbox("Feet / Miles", "", currentUnit == storage.DistanceUnitFeet)
	mDistanceCars = mDistanceUnit.AddSubMenuItemCheckbox("Cars (15ft each)", "", currentUnit == storage.DistanceUnitCars)
	mDistanceFields = mDistanceUnit.AddSubMenuItemCheckbox("Frisbee Fields (330ft)", "", currentUnit == storage.DistanceUnitFrisbee)

	mSettings.AddSubMenuItem("", "").Disable() // Separator

	// Tracking section
	mTrackingLabel := mSettings.AddSubMenuItem("Tracking:", "")
	mTrackingLabel.Disable()

	mouseTrackingEnabled := store.IsMouseTrackingEnabled()
	mMouseTracking = mSettings.AddSubMenuItemCheckbox("Enable Mouse Distance", "", mouseTrackingEnabled)

	mSettings.AddSubMenuItem("", "").Disable() // Separator

	// Charts section
	mChartsLabel := mSettings.AddSubMenuItem("📊 Charts:", "")
	mChartsLabel.Disable()

	showKeyTypes := store.IsShowKeyTypesEnabled()
	mShowKeyTypes = mSettings.AddSubMenuItemCheckbox("Show Key Types (Letters/Modifiers/Special)", "", showKeyTypes)

	mSettings.AddSubMenuItem("", "").Disable() // Separator

	// Word Counting section — controls strict-mode per-app filtering.
	// (The smarter keystroke heuristics in wordcounter.Counter always apply.)
	mWordCountLabel := mSettings.AddSubMenuItem("🎯 Word Counting:", "")
	mWordCountLabel.Disable()

	strictEnabled := store.IsStrictWordCountEnabled()
	mWordStrict := mSettings.AddSubMenuItemCheckbox("Strict Mode (filter by app)", "Only count keystrokes in allowlisted apps", strictEnabled)
	go func() {
		for range mWordStrict.ClickedCh {
			newState := !store.IsStrictWordCountEnabled()
			if err := store.SetStrictWordCountEnabled(newState); err != nil {
				log.Printf("Failed to save strict mode setting: %v", err)
				continue
			}
			appFilter.SetEnabled(newState)
			if newState {
				mWordStrict.Check()
			} else {
				mWordStrict.Uncheck()
			}
		}
	}()

	// Allowed apps sub-menu: one checkbox per observed bundle ID.
	mAllowedApps := mSettings.AddSubMenuItem("   Allowed Apps", "Pick which apps' typing counts in strict mode")
	appsSeen := store.GetWordCountAppsSeen()
	if len(appsSeen) == 0 {
		hint := mAllowedApps.AddSubMenuItem("(no apps observed yet — type for a while)", "")
		hint.Disable()
	} else {
		allowSet := map[string]bool{}
		for _, id := range store.GetWordCountAllowlist() {
			allowSet[id] = true
		}
		sort.Strings(appsSeen)
		for _, bundleID := range appsSeen {
			bundleID := bundleID // capture
			item := mAllowedApps.AddSubMenuItemCheckbox(bundleID, "", allowSet[bundleID])
			go func() {
				for range item.ClickedCh {
					current := store.GetWordCountAllowlist()
					found := false
					out := make([]string, 0, len(current)+1)
					for _, id := range current {
						if id == bundleID {
							found = true
							continue
						}
						out = append(out, id)
					}
					if !found {
						out = append(out, bundleID)
					}
					if err := store.SetWordCountAllowlist(out); err != nil {
						log.Printf("Failed to save allowlist: %v", err)
						continue
					}
					appFilter.SetAllowlist(out)
					if found {
						item.Uncheck()
					} else {
						item.Check()
					}
				}
			}()
		}
	}

	mSettings.AddSubMenuItem("", "").Disable() // Separator

	// Inertia section
	mInertiaLabel := mSettings.AddSubMenuItem("⚡ Inertia (Key Acceleration):", "")
	mInertiaLabel.Disable()

	inertiaSettings := store.GetInertiaSettings()
	mInertiaEnabled = mSettings.AddSubMenuItemCheckbox("Enable Inertia", "", inertiaSettings.Enabled)

	// Max Speed submenu
	mMaxSpeed := mSettings.AddSubMenuItem("   Max Speed", "")
	mInertiaUltraFast = mMaxSpeed.AddSubMenuItemCheckbox("Ultra Fast (~140 keys/sec)", "", inertiaSettings.MaxSpeed == storage.InertiaSpeedUltraFast)
	mInertiaVeryFast = mMaxSpeed.AddSubMenuItemCheckbox("Very Fast (~125 keys/sec)", "", inertiaSettings.MaxSpeed == storage.InertiaSpeedVeryFast)
	mInertiaPrettyFast = mMaxSpeed.AddSubMenuItemCheckbox("Pretty Fast (~100 keys/sec)", "", inertiaSettings.MaxSpeed == storage.InertiaSpeedPrettyFast)
	mInertiaFast = mMaxSpeed.AddSubMenuItemCheckbox("Fast (~83 keys/sec)", "", inertiaSettings.MaxSpeed == storage.InertiaSpeedFast)
	mInertiaMedium = mMaxSpeed.AddSubMenuItemCheckbox("Medium (~50 keys/sec)", "", inertiaSettings.MaxSpeed == storage.InertiaSpeedMedium)
	mInertiaSlow = mMaxSpeed.AddSubMenuItemCheckbox("Slow (~20 keys/sec)", "", inertiaSettings.MaxSpeed == storage.InertiaSpeedSlow)

	// Threshold submenu
	mThreshold := mSettings.AddSubMenuItem("   Threshold (ms)", "")
	mThreshold100 = mThreshold.AddSubMenuItemCheckbox("100ms (instant)", "", inertiaSettings.Threshold == 100)
	mThreshold150 = mThreshold.AddSubMenuItemCheckbox("150ms (fast)", "", inertiaSettings.Threshold == 150)
	mThreshold200 = mThreshold.AddSubMenuItemCheckbox("200ms (default)", "", inertiaSettings.Threshold == 200)
	mThreshold250 = mThreshold.AddSubMenuItemCheckbox("250ms (slow)", "", inertiaSettings.Threshold == 250)
	mThreshold350 = mThreshold.AddSubMenuItemCheckbox("350ms (very slow)", "", inertiaSettings.Threshold == 350)

	// Acceleration Rate submenu
	mAccelRate := mSettings.AddSubMenuItem("   Acceleration Rate", "")
	mAccelRate025 = mAccelRate.AddSubMenuItemCheckbox("0.25x (very gentle)", "", inertiaSettings.AccelRate == 0.25)
	mAccelRate050 = mAccelRate.AddSubMenuItemCheckbox("0.5x (gentle)", "", inertiaSettings.AccelRate == 0.5)
	mAccelRate100 = mAccelRate.AddSubMenuItemCheckbox("1.0x (default)", "", inertiaSettings.AccelRate == 1.0)
	mAccelRate150 = mAccelRate.AddSubMenuItemCheckbox("1.5x (faster)", "", inertiaSettings.AccelRate == 1.5)
	mAccelRate200 = mAccelRate.AddSubMenuItemCheckbox("2.0x (aggressive)", "", inertiaSettings.AccelRate == 2.0)

	mSettings.AddSubMenuItem("", "").Disable() // Separator

	// Debug section
	mDebugLabel := mSettings.AddSubMenuItem("🔧 Debug:", "")
	mDebugLabel.Disable()
	mDisplayInfo := mSettings.AddSubMenuItem("   Show Display Info", "")
	go func() {
		for range mDisplayInfo.ClickedCh {
			showDisplayDebugInfo()
		}
	}()

	// About
	mAbout := systray.AddMenuItem("About", "About Typing Telemetry")
	go func() {
		for range mAbout.ClickedCh {
			showAbout()
		}
	}()

	// Quit
	mQuit := systray.AddMenuItem("Quit", "Quit application")
	go func() {
		for range mQuit.ClickedCh {
			quit()
		}
	}()

	// Start click handlers for settings
	go handleSettingsClicks()
}

func handleSettingsClicks() {
	for {
		select {
		case <-mShowKeystrokes.ClickedCh:
			s := store.GetMenubarSettings()
			s.ShowKeystrokes = !s.ShowKeystrokes
			store.SaveMenubarSettings(s)
			if s.ShowKeystrokes {
				mShowKeystrokes.Check()
			} else {
				mShowKeystrokes.Uncheck()
			}
			updateMenuBarTitle()

		case <-mShowWords.ClickedCh:
			s := store.GetMenubarSettings()
			s.ShowWords = !s.ShowWords
			store.SaveMenubarSettings(s)
			if s.ShowWords {
				mShowWords.Check()
			} else {
				mShowWords.Uncheck()
			}
			updateMenuBarTitle()

		case <-mShowClicks.ClickedCh:
			s := store.GetMenubarSettings()
			s.ShowClicks = !s.ShowClicks
			store.SaveMenubarSettings(s)
			if s.ShowClicks {
				mShowClicks.Check()
			} else {
				mShowClicks.Uncheck()
			}
			updateMenuBarTitle()

		case <-mShowDistance.ClickedCh:
			s := store.GetMenubarSettings()
			s.ShowDistance = !s.ShowDistance
			store.SaveMenubarSettings(s)
			if s.ShowDistance {
				mShowDistance.Check()
			} else {
				mShowDistance.Uncheck()
			}
			updateMenuBarTitle()

		case <-mDistanceFeet.ClickedCh:
			store.SetDistanceUnit(storage.DistanceUnitFeet)
			mDistanceFeet.Check()
			mDistanceCars.Uncheck()
			mDistanceFields.Uncheck()
			updateMenuBarTitle()

		case <-mDistanceCars.ClickedCh:
			store.SetDistanceUnit(storage.DistanceUnitCars)
			mDistanceFeet.Uncheck()
			mDistanceCars.Check()
			mDistanceFields.Uncheck()
			updateMenuBarTitle()

		case <-mDistanceFields.ClickedCh:
			store.SetDistanceUnit(storage.DistanceUnitFrisbee)
			mDistanceFeet.Uncheck()
			mDistanceCars.Uncheck()
			mDistanceFields.Check()
			updateMenuBarTitle()

		case <-mMouseTracking.ClickedCh:
			enabled := store.IsMouseTrackingEnabled()
			store.SetMouseTrackingEnabled(!enabled)
			if !enabled {
				mMouseTracking.Check()
			} else {
				mMouseTracking.Uncheck()
			}
			showAlertDialog("Mouse Distance "+map[bool]string{true: "Disabled", false: "Enabled"}[enabled],
				"Restart the app for changes to take effect.",
				[]string{"OK"})

		case <-mShowKeyTypes.ClickedCh:
			enabled := store.IsShowKeyTypesEnabled()
			store.SetShowKeyTypesEnabled(!enabled)
			if !enabled {
				mShowKeyTypes.Check()
			} else {
				mShowKeyTypes.Uncheck()
			}

		case <-mInertiaEnabled.ClickedCh:
			s := store.GetInertiaSettings()
			newEnabled := !s.Enabled
			store.SetInertiaEnabled(newEnabled)
			if newEnabled {
				mInertiaEnabled.Check()
				cfg := inertia.Config{
					Enabled:   true,
					MaxSpeed:  s.MaxSpeed,
					Threshold: s.Threshold,
					AccelRate: s.AccelRate,
				}
				inertia.Start(cfg)
			} else {
				mInertiaEnabled.Uncheck()
				inertia.Stop()
			}
			showAlertDialog("Inertia "+map[bool]string{true: "Enabled", false: "Disabled"}[newEnabled],
				"Key acceleration is now "+map[bool]string{true: "active", false: "inactive"}[newEnabled]+".\n\nHold any key to accelerate repeat speed.\nPressing any other key resets acceleration.",
				[]string{"OK"})

		case <-mInertiaUltraFast.ClickedCh:
			store.SetInertiaMaxSpeed(storage.InertiaSpeedUltraFast)
			updateInertiaSpeedChecks(storage.InertiaSpeedUltraFast)
			updateInertiaConfig()

		case <-mInertiaVeryFast.ClickedCh:
			store.SetInertiaMaxSpeed(storage.InertiaSpeedVeryFast)
			updateInertiaSpeedChecks(storage.InertiaSpeedVeryFast)
			updateInertiaConfig()

		case <-mInertiaPrettyFast.ClickedCh:
			store.SetInertiaMaxSpeed(storage.InertiaSpeedPrettyFast)
			updateInertiaSpeedChecks(storage.InertiaSpeedPrettyFast)
			updateInertiaConfig()

		case <-mInertiaFast.ClickedCh:
			store.SetInertiaMaxSpeed(storage.InertiaSpeedFast)
			updateInertiaSpeedChecks(storage.InertiaSpeedFast)
			updateInertiaConfig()

		case <-mInertiaMedium.ClickedCh:
			store.SetInertiaMaxSpeed(storage.InertiaSpeedMedium)
			updateInertiaSpeedChecks(storage.InertiaSpeedMedium)
			updateInertiaConfig()

		case <-mInertiaSlow.ClickedCh:
			store.SetInertiaMaxSpeed(storage.InertiaSpeedSlow)
			updateInertiaSpeedChecks(storage.InertiaSpeedSlow)
			updateInertiaConfig()

		case <-mThreshold100.ClickedCh:
			store.SetInertiaThreshold(100)
			updateThresholdChecks(100)
			updateInertiaConfig()

		case <-mThreshold150.ClickedCh:
			store.SetInertiaThreshold(150)
			updateThresholdChecks(150)
			updateInertiaConfig()

		case <-mThreshold200.ClickedCh:
			store.SetInertiaThreshold(200)
			updateThresholdChecks(200)
			updateInertiaConfig()

		case <-mThreshold250.ClickedCh:
			store.SetInertiaThreshold(250)
			updateThresholdChecks(250)
			updateInertiaConfig()

		case <-mThreshold350.ClickedCh:
			store.SetInertiaThreshold(350)
			updateThresholdChecks(350)
			updateInertiaConfig()

		case <-mAccelRate025.ClickedCh:
			store.SetInertiaAccelRate(0.25)
			updateAccelRateChecks(0.25)
			updateInertiaConfig()

		case <-mAccelRate050.ClickedCh:
			store.SetInertiaAccelRate(0.5)
			updateAccelRateChecks(0.5)
			updateInertiaConfig()

		case <-mAccelRate100.ClickedCh:
			store.SetInertiaAccelRate(1.0)
			updateAccelRateChecks(1.0)
			updateInertiaConfig()

		case <-mAccelRate150.ClickedCh:
			store.SetInertiaAccelRate(1.5)
			updateAccelRateChecks(1.5)
			updateInertiaConfig()

		case <-mAccelRate200.ClickedCh:
			store.SetInertiaAccelRate(2.0)
			updateAccelRateChecks(2.0)
			updateInertiaConfig()
		}
	}
}

func updateInertiaSpeedChecks(speed string) {
	mInertiaUltraFast.Uncheck()
	mInertiaVeryFast.Uncheck()
	mInertiaPrettyFast.Uncheck()
	mInertiaFast.Uncheck()
	mInertiaMedium.Uncheck()
	mInertiaSlow.Uncheck()
	switch speed {
	case storage.InertiaSpeedUltraFast:
		mInertiaUltraFast.Check()
	case storage.InertiaSpeedVeryFast:
		mInertiaVeryFast.Check()
	case storage.InertiaSpeedPrettyFast:
		mInertiaPrettyFast.Check()
	case storage.InertiaSpeedFast:
		mInertiaFast.Check()
	case storage.InertiaSpeedMedium:
		mInertiaMedium.Check()
	case storage.InertiaSpeedSlow:
		mInertiaSlow.Check()
	}
}

func updateThresholdChecks(threshold int) {
	mThreshold100.Uncheck()
	mThreshold150.Uncheck()
	mThreshold200.Uncheck()
	mThreshold250.Uncheck()
	mThreshold350.Uncheck()
	switch threshold {
	case 100:
		mThreshold100.Check()
	case 150:
		mThreshold150.Check()
	case 200:
		mThreshold200.Check()
	case 250:
		mThreshold250.Check()
	case 350:
		mThreshold350.Check()
	}
}

func updateAccelRateChecks(rate float64) {
	mAccelRate025.Uncheck()
	mAccelRate050.Uncheck()
	mAccelRate100.Uncheck()
	mAccelRate150.Uncheck()
	mAccelRate200.Uncheck()
	switch rate {
	case 0.25:
		mAccelRate025.Check()
	case 0.5:
		mAccelRate050.Check()
	case 1.0:
		mAccelRate100.Check()
	case 1.5:
		mAccelRate150.Check()
	case 2.0:
		mAccelRate200.Check()
	}
}

func updateInertiaConfig() {
	s := store.GetInertiaSettings()
	if s.Enabled {
		cfg := inertia.Config{
			Enabled:   true,
			MaxSpeed:  s.MaxSpeed,
			Threshold: s.Threshold,
			AccelRate: s.AccelRate,
		}
		inertia.UpdateConfig(cfg)
	}
}

func updateMenuBarTitle() {
	stats, err := store.GetTodayStats()
	if err != nil {
		setMenuTitle("⌨️ --")
		return
	}

	settings := store.GetMenubarSettings()
	mouseStats, _ := store.GetTodayMouseStats()

	var parts []string

	if settings.ShowKeystrokes {
		parts = append(parts, fmt.Sprintf("⌨️%s", formatAbsolute(stats.Keystrokes)))
	}
	if settings.ShowWords {
		parts = append(parts, fmt.Sprintf("%sw", formatAbsolute(stats.Words)))
	}
	if settings.ShowClicks && mouseStats != nil {
		parts = append(parts, fmt.Sprintf("🖱️%s", formatAbsolute(mouseStats.ClickCount)))
	}
	if settings.ShowDistance && mouseStats != nil && mouseStats.TotalDistance > 0 {
		parts = append(parts, formatDistance(mouseStats.TotalDistance))
	}

	title := "⌨️"
	if len(parts) > 0 {
		title = strings.Join(parts, " | ")
	}

	setMenuTitle(title)
}

func setMenuTitle(title string) {
	menuTitleMutex.Lock()
	defer menuTitleMutex.Unlock()

	if title == lastMenuTitle {
		return
	}

	lastMenuTitle = title
	systray.SetTitle(title)
}

func updateStatsDisplay() {
	stats, _ := store.GetTodayStats()
	weekStats, _ := store.GetWeekStats()
	mouseStats, _ := store.GetTodayMouseStats()
	weekMouseStats, _ := store.GetWeekMouseStats()

	var weekKeystrokes, weekWords int64
	var weekMouseDistance float64
	var weekClicks int64
	if weekStats != nil {
		for _, day := range weekStats {
			weekKeystrokes += day.Keystrokes
			weekWords += day.Words
		}
	}
	if weekMouseStats != nil {
		for _, day := range weekMouseStats {
			weekMouseDistance += day.TotalDistance
			weekClicks += day.ClickCount
		}
	}

	keystrokeCount := int64(0)
	todayWords := int64(0)
	if stats != nil {
		keystrokeCount = stats.Keystrokes
		todayWords = stats.Words
	}

	todayMouseDistance := float64(0)
	todayClicks := int64(0)
	if mouseStats != nil {
		todayMouseDistance = mouseStats.TotalDistance
		todayClicks = mouseStats.ClickCount
	}

	// Update menu items
	mTodayKeystrokes.SetTitle(fmt.Sprintf("Today: %s keystrokes (%s words)", formatAbsolute(keystrokeCount), formatAbsolute(todayWords)))
	mTodayMouse.SetTitle(fmt.Sprintf("Today: 🖱️ %s clicks, %s distance", formatAbsolute(todayClicks), formatDistance(todayMouseDistance)))
	mWeekKeystrokes.SetTitle(fmt.Sprintf("This Week: %s keystrokes (%s words)", formatAbsolute(weekKeystrokes), formatAbsolute(weekWords)))
	mWeekMouse.SetTitle(fmt.Sprintf("This Week: 🖱️ %s clicks, %s distance", formatAbsolute(weekClicks), formatDistance(weekMouseDistance)))

	// Calculate today's averages
	todayDate := time.Now().Format("2006-01-02")
	todayActiveHours := calculateActiveHours(todayDate)

	avgKeystrokesToday := float64(keystrokeCount) / todayActiveHours
	avgWordsToday := float64(todayWords) / todayActiveHours
	avgClicksToday := float64(todayClicks) / todayActiveHours
	avgDistanceToday := todayMouseDistance / todayActiveHours

	mAvgTodayKeystrokes.SetTitle(fmt.Sprintf("   %.0f keystrokes/hr", avgKeystrokesToday))
	mAvgTodayWords.SetTitle(fmt.Sprintf("   %.0f words/hr", avgWordsToday))
	mAvgTodayClicks.SetTitle(fmt.Sprintf("   %.0f clicks/hr", avgClicksToday))
	mAvgTodayDistance.SetTitle(fmt.Sprintf("   %s/hr", formatDistance(avgDistanceToday)))

	// Calculate week's averages (sum active hours across all days)
	var weekActiveHours float64
	now := time.Now()
	for i := 0; i < 7; i++ {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		weekActiveHours += calculateActiveHours(date)
	}
	if weekActiveHours == 0 {
		weekActiveHours = 1.0
	}

	avgKeystrokesWeek := float64(weekKeystrokes) / weekActiveHours
	avgWordsWeek := float64(weekWords) / weekActiveHours
	avgClicksWeek := float64(weekClicks) / weekActiveHours
	avgDistanceWeek := weekMouseDistance / weekActiveHours

	mAvgWeekKeystrokes.SetTitle(fmt.Sprintf("   %.0f keystrokes/hr", avgKeystrokesWeek))
	mAvgWeekWords.SetTitle(fmt.Sprintf("   %.0f words/hr", avgWordsWeek))
	mAvgWeekClicks.SetTitle(fmt.Sprintf("   %.0f clicks/hr", avgClicksWeek))
	mAvgWeekDistance.SetTitle(fmt.Sprintf("   %s/hr", formatDistance(avgDistanceWeek)))

	// Calculate daily averages (per day over the week)
	// Count active days (days with any keystroke or mouse activity)
	activeDays := 0
	for i := 0; i < 7; i++ {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		hours := calculateActiveHours(date)
		if hours > 0 {
			activeDays++
		}
	}
	if activeDays == 0 {
		activeDays = 1
	}

	avgKeystrokesDaily := float64(weekKeystrokes) / float64(activeDays)
	avgWordsDaily := float64(weekWords) / float64(activeDays)
	avgClicksDaily := float64(weekClicks) / float64(activeDays)
	avgDistanceDaily := weekMouseDistance / float64(activeDays)

	mAvgDailyKeystrokes.SetTitle(fmt.Sprintf("   %s keystrokes/day", formatAbsolute(int64(avgKeystrokesDaily))))
	mAvgDailyWords.SetTitle(fmt.Sprintf("   %s words/day", formatAbsolute(int64(avgWordsDaily))))
	mAvgDailyClicks.SetTitle(fmt.Sprintf("   %s clicks/day", formatAbsolute(int64(avgClicksDaily))))
	mAvgDailyDistance.SetTitle(fmt.Sprintf("   %s/day", formatDistance(avgDistanceDaily)))

	// Update typing-speed submenu
	updateSpeedDisplay()

	// Update leaderboard
	updateLeaderboard()
}

// updateSpeedDisplay refreshes the ⚡ Typing Speed submenu: average WPM over
// rolling windows (today / 7d / 30d / 365d / all-time) plus the all-time
// fastest paces. Periods are rolling windows for consistency with the rest of
// the menu (This Week is the trailing 7 days, etc.).
func updateSpeedDisplay() {
	now := time.Now()
	setAvg := func(item *systray.MenuItem, label, since string) {
		agg, err := store.GetSpeedAggregate(since)
		if err != nil {
			return
		}
		item.SetTitle(fmt.Sprintf("   %s: %s", label, formatWPM(stats.AverageWPM(agg.Words, agg.ActiveMs))))
	}
	setAvg(mSpeedAvgToday, "Today", now.Format("2006-01-02"))
	setAvg(mSpeedAvgWeek, "This Week", now.AddDate(0, 0, -6).Format("2006-01-02"))
	setAvg(mSpeedAvgMonth, "This Month", now.AddDate(0, 0, -29).Format("2006-01-02"))
	setAvg(mSpeedAvgYear, "This Year", now.AddDate(0, 0, -364).Format("2006-01-02"))
	setAvg(mSpeedAvgAll, "All-Time", "")

	if all, err := store.GetSpeedAggregate(""); err == nil {
		mSpeedFastBurst.SetTitle(fmt.Sprintf("   10-word burst: %s", formatWPM(all.FastestBurstWPM)))
		mSpeedFastWin.SetTitle(fmt.Sprintf("   60-second: %s", formatWPM(all.FastestWindowWPM)))
		mSpeedFastMin.SetTitle(fmt.Sprintf("   Best minute: %s", formatWPM(all.FastestMinuteWPM)))
	}
}

func updateLeaderboard() {
	entries, err := store.GetMouseLeaderboard(10)
	if err != nil || len(entries) == 0 {
		for _, item := range leaderboardItems {
			item.Hide()
		}
		return
	}

	for i, item := range leaderboardItems {
		if i < len(entries) {
			entry := entries[i]
			t, _ := time.Parse("2006-01-02", entry.Date)
			medal := ""
			switch entry.Rank {
			case 1:
				medal = "🥇 "
			case 2:
				medal = "🥈 "
			case 3:
				medal = "🥉 "
			}
			item.SetTitle(fmt.Sprintf("%s#%d: %s - %s", medal, entry.Rank, t.Format("Jan 2, 2006"), formatDistance(entry.TotalDistance)))
			item.Show()
		} else {
			item.Hide()
		}
	}
}

func showAbout() {
	response := showAlertDialog("Typing Telemetry",
		fmt.Sprintf("Version %s\n\nTrack your keystrokes and typing speed.\n\nGitHub: github.com/abaj8494/typing-telemetry", Version),
		[]string{"OK", "Open GitHub"})

	if response == 1 {
		if !openURLNative("https://github.com/abaj8494/typing-telemetry") {
			log.Printf("Failed to open GitHub URL")
		}
	}
}

func quit() {
	response := showAlertDialog("Quit Typing Telemetry",
		"This will stop keystroke tracking and close the menu bar app.\n\nTo restart, run: open -a typtel-menubar",
		[]string{"Cancel", "Quit"})

	if response == 1 {
		log.Println("User requested quit")
		keylogger.Stop()
		mousetracker.Stop()
		inertia.Stop()
		if store != nil {
			store.Close()
		}
		systray.Quit()
	}
}

func showPermissionAlert() {
	fmt.Println("ERROR: Accessibility permissions not granted.")
	fmt.Println("")
	fmt.Println("To enable:")
	fmt.Println("1. Open System Preferences > Privacy & Security > Accessibility")
	fmt.Println("2. Click the lock to make changes")
	fmt.Println("3. Add this application to the list")
	fmt.Println("4. Restart the application")
}

func showDisplayDebugInfo() {
	ppi := mousetracker.GetAveragePPI()
	displayCount := mousetracker.GetDisplayCount()

	info := fmt.Sprintf("Displays Detected: %d\nAverage PPI: %.1f\n\n", displayCount, ppi)

	if ppi == mousetracker.DefaultPPI {
		info += "Note: Using fallback PPI (100).\nYour displays may not report physical dimensions."
	} else {
		info += fmt.Sprintf("Mouse distance is calculated using %.1f pixels per inch.", ppi)
	}

	showAlertDialog("Display Information", info, []string{"OK"})
}

// Native alert dialog using Cocoa
func showAlertDialog(messageText, informativeText string, buttons []string) int {
	cMessage := C.CString(messageText)
	cInfo := C.CString(informativeText)
	defer C.free(unsafe.Pointer(cMessage))
	defer C.free(unsafe.Pointer(cInfo))

	cButtons := make([]*C.char, len(buttons))
	for i, b := range buttons {
		cButtons[i] = C.CString(b)
		defer C.free(unsafe.Pointer(cButtons[i]))
	}

	return int(C.showAlert(cMessage, cInfo, &cButtons[0], C.int(len(buttons))))
}

// Open URL in default browser using native macOS API
func openURLNative(url string) bool {
	cURL := C.CString(url)
	defer C.free(unsafe.Pointer(cURL))
	return C.openURL(cURL) == 1
}

// Open file using native macOS API
func openFileNative(path string) bool {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	return C.openFile(cPath) == 1
}

func getLogDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	logDir := filepath.Join(home, ".local", "share", "typtel", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return "", err
	}
	return logDir, nil
}

func formatAbsolute(n int64) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		return s
	}

	result := ""
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}

func formatDistance(pixels float64) string {
	feet := mousetracker.PixelsToFeet(pixels)

	unit := store.GetDistanceUnit()
	switch unit {
	case storage.DistanceUnitCars:
		cars := feet / 15.0
		if cars >= 1000 {
			return fmt.Sprintf("%.1fk cars", cars/1000)
		} else if cars >= 1 {
			return fmt.Sprintf("%.0f cars", cars)
		}
		return fmt.Sprintf("%.1f cars", cars)

	case storage.DistanceUnitFrisbee:
		fields := feet / 330.0
		if fields >= 100 {
			return fmt.Sprintf("%.0f fields", fields)
		} else if fields >= 1 {
			return fmt.Sprintf("%.1f fields", fields)
		}
		return fmt.Sprintf("%.2f fields", fields)

	default:
		if feet >= 5280 {
			return fmt.Sprintf("%.1fmi", feet/5280)
		} else if feet >= 1 {
			return fmt.Sprintf("%.0fft", feet)
		}
		inches := feet * 12
		return fmt.Sprintf("%.0fin", inches)
	}
}

// formatHotkeyDisplay converts internal hotkey format to display format
func formatHotkeyDisplay(hotkey string) string {
	switch hotkey {
	case "cmd+ctrl+o":
		return "⌘⌃O"
	case "cmd+shift+o":
		return "⌘⇧O"
	case "cmd+opt+o":
		return "⌘⌥O"
	case "ctrl+shift+o":
		return "⌃⇧O"
	default:
		return "⌘⌃O"
	}
}

// updateOdometerDisplay updates the odometer menu items
func updateOdometerDisplay() {
	session, _ := store.GetOdometerSession()
	if session == nil {
		return
	}

	statusText := "Inactive"
	toggleText := "Start Odometer"
	if session.IsActive {
		statusText = "Active"
		toggleText = "Stop Odometer"
	}

	mOdometerStatus.SetTitle(fmt.Sprintf("Status: %s", statusText))

	currentHotkey := store.GetOdometerHotkey()
	hotkeyDisplay := formatHotkeyDisplay(currentHotkey)
	mOdometerToggle.SetTitle(fmt.Sprintf("%s (%s)", toggleText, hotkeyDisplay))
}

// toggleOdometer starts or stops the odometer
func toggleOdometer() {
	session, _ := store.GetOdometerSession()
	if session != nil && session.IsActive {
		store.StopOdometer()
		keylogger.PlaySound(keylogger.SoundPurr) // Deactivation sound
		log.Println("Odometer stopped")
	} else {
		store.StartOdometer()
		keylogger.PlaySound(keylogger.SoundGlass) // Activation sound
		log.Println("Odometer started")
	}
	updateOdometerDisplay()
}

// handleOdometerClicks handles clicks on odometer menu items
func handleOdometerClicks() {
	for {
		select {
		case <-mOdometerToggle.ClickedCh:
			toggleOdometer()

		case <-mOdometerReset.ClickedCh:
			store.ResetOdometer()
			updateOdometerDisplay()
			log.Println("Odometer reset")

		case <-mOdometerClearHistory.ClickedCh:
			store.ClearOdometerHistory()
			log.Println("Odometer history cleared")

		case <-mHotkeyCmdCtrlO.ClickedCh:
			store.SetOdometerHotkey("cmd+ctrl+o")
			updateHotkeyChecks("cmd+ctrl+o")
			updateOdometerDisplay()

		case <-mHotkeyCmdShiftO.ClickedCh:
			store.SetOdometerHotkey("cmd+shift+o")
			updateHotkeyChecks("cmd+shift+o")
			updateOdometerDisplay()

		case <-mHotkeyCmdOptO.ClickedCh:
			store.SetOdometerHotkey("cmd+opt+o")
			updateHotkeyChecks("cmd+opt+o")
			updateOdometerDisplay()

		case <-mHotkeyCtrlShiftO.ClickedCh:
			store.SetOdometerHotkey("ctrl+shift+o")
			updateHotkeyChecks("ctrl+shift+o")
			updateOdometerDisplay()
		}
	}
}

// updateHotkeyChecks updates the checkmarks on hotkey menu items
func updateHotkeyChecks(selected string) {
	mHotkeyCmdCtrlO.Uncheck()
	mHotkeyCmdShiftO.Uncheck()
	mHotkeyCmdOptO.Uncheck()
	mHotkeyCtrlShiftO.Uncheck()
	switch selected {
	case "cmd+ctrl+o":
		mHotkeyCmdCtrlO.Check()
	case "cmd+shift+o":
		mHotkeyCmdShiftO.Check()
	case "cmd+opt+o":
		mHotkeyCmdOptO.Check()
	case "ctrl+shift+o":
		mHotkeyCtrlShiftO.Check()
	}
}

// checkOdometerHotkey checks if the current keycode + modifiers match the odometer hotkey
func checkOdometerHotkey(keycode int) bool {
	// Only trigger on 'O' key (keycode 31)
	if keycode != 31 {
		return false
	}

	// Query actual system modifier state (not our tracked state which can be unreliable)
	mods := keylogger.GetCurrentModifiers()
	hotkey := store.GetOdometerHotkey()

	switch hotkey {
	case "cmd+ctrl+o":
		return mods.Cmd && mods.Ctrl && !mods.Opt && !mods.Shift
	case "cmd+shift+o":
		return mods.Cmd && mods.Shift && !mods.Ctrl && !mods.Opt
	case "cmd+opt+o":
		return mods.Cmd && mods.Opt && !mods.Ctrl && !mods.Shift
	case "ctrl+shift+o":
		return mods.Ctrl && mods.Shift && !mods.Cmd && !mods.Opt
	default:
		return mods.Cmd && mods.Ctrl && !mods.Opt && !mods.Shift
	}
}

// updateOdometerIfActive updates the odometer's current values if it's active
func updateOdometerIfActive() {
	session, _ := store.GetOdometerSession()
	if session == nil || !session.IsActive {
		return
	}

	// Get current totals
	todayStats, _ := store.GetTodayStats()
	mouseStats, _ := store.GetTodayMouseStats()

	keystrokes := int64(0)
	words := int64(0)
	clicks := int64(0)
	distance := float64(0)

	if todayStats != nil {
		keystrokes = todayStats.Keystrokes
		words = todayStats.Words
	}
	if mouseStats != nil {
		clicks = mouseStats.ClickCount
		distance = mouseStats.TotalDistance
	}

	store.UpdateOdometerCurrent(keystrokes, words, clicks, distance)
}

// calculateActiveHours returns the number of hours with activity for a given date
func calculateActiveHours(date string) float64 {
	hourlyStats, err := store.GetHourlyStats(date)
	if err != nil {
		return 1.0 // Avoid division by zero
	}
	activeHours := 0.0
	for _, h := range hourlyStats {
		if h.Keystrokes > 0 {
			activeHours++
		}
	}
	if activeHours == 0 {
		activeHours = 1.0 // Avoid division by zero
	}
	return activeHours
}

func showLeaderboard() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Panic in showLeaderboard: %v", r)
			}
		}()

		htmlPath, err := generateLeaderboardHTML()
		if err != nil {
			log.Printf("Failed to generate leaderboard: %v", err)
			showAlertDialog("Error", fmt.Sprintf("Failed to generate leaderboard: %v", err), []string{"OK"})
			return
		}

		if !openFileNative(htmlPath) {
			log.Printf("Failed to open leaderboard file: %s", htmlPath)
			showAlertDialog("Error", "Failed to open leaderboard in browser. The file was saved to:\n"+htmlPath, []string{"OK"})
		}
	}()
}

func generateLeaderboardHTML() (string, error) {
	entries, err := store.GetMouseLeaderboard(30)
	if err != nil {
		return "", err
	}

	var rows strings.Builder
	for _, entry := range entries {
		t, _ := time.Parse("2006-01-02", entry.Date)
		medal := ""
		switch entry.Rank {
		case 1:
			medal = "🥇"
		case 2:
			medal = "🥈"
		case 3:
			medal = "🥉"
		default:
			medal = fmt.Sprintf("#%d", entry.Rank)
		}
		rows.WriteString(fmt.Sprintf(`
			<tr>
				<td class="rank">%s</td>
				<td class="date">%s</td>
				<td class="distance">%s</td>
			</tr>`, medal, t.Format("Monday, Jan 2, 2006"), formatDistance(entry.TotalDistance)))
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Typtel - Stillness Leaderboard</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%%, #16213e 100%%);
            color: #eee;
            min-height: 100vh;
            padding: 30px;
        }
        h1 {
            text-align: center;
            margin-bottom: 10px;
            font-size: 2.5em;
            background: linear-gradient(90deg, #ff6b6b, #feca57);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .subtitle {
            text-align: center;
            color: #888;
            margin-bottom: 30px;
            font-size: 1.1em;
        }
        .leaderboard-container {
            max-width: 800px;
            margin: 0 auto;
            background: rgba(255,255,255,0.05);
            border-radius: 16px;
            padding: 25px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255,255,255,0.1);
        }
        table {
            width: 100%%;
            border-collapse: collapse;
        }
        th {
            text-align: left;
            padding: 15px;
            border-bottom: 2px solid rgba(255,255,255,0.2);
            color: #888;
            font-size: 0.9em;
            text-transform: uppercase;
        }
        td {
            padding: 15px;
            border-bottom: 1px solid rgba(255,255,255,0.05);
        }
        tr:hover {
            background: rgba(255,255,255,0.05);
        }
        .rank {
            font-size: 1.5em;
            width: 80px;
        }
        .date {
            color: #aaa;
        }
        .distance {
            text-align: right;
            font-weight: bold;
            background: linear-gradient(90deg, #00d2ff, #3a7bd5);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .explanation {
            margin-top: 30px;
            padding: 20px;
            background: rgba(255,255,255,0.03);
            border-radius: 10px;
            color: #888;
            font-size: 0.9em;
            line-height: 1.6;
        }
    </style>
</head>
<body>
    <h1>🧘 Stillness Leaderboard</h1>
    <p class="subtitle">Days You Didn't Move The Mouse (Much)</p>

    <div class="leaderboard-container">
        <table>
            <thead>
                <tr>
                    <th>Rank</th>
                    <th>Date</th>
                    <th>Distance</th>
                </tr>
            </thead>
            <tbody>
                %s
            </tbody>
        </table>

        <div class="explanation">
            <strong>What is this?</strong><br>
            This leaderboard tracks the days when you moved your mouse the least. Less mouse movement
            could indicate focused keyboard work, reading, or meditation sessions. The distance is
            calculated as the total Euclidean distance your cursor traveled throughout the day,
            converted to approximate real-world measurements in feet (assuming ~100 DPI display).
        </div>
    </div>
</body>
</html>`, rows.String())

	dataDir, err := getLogDir()
	if err != nil {
		return "", err
	}
	htmlPath := filepath.Join(dataDir, "leaderboard.html")
	if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
		return "", err
	}

	return htmlPath, nil
}

func openCharts() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Panic in openCharts: %v", r)
			}
		}()

		htmlPath, err := generateChartsHTML()
		if err != nil {
			log.Printf("Failed to generate charts: %v", err)
			showAlertDialog("Error", fmt.Sprintf("Failed to generate charts: %v", err), []string{"OK"})
			return
		}

		if !openFileNative(htmlPath) {
			log.Printf("Failed to open charts file: %s", htmlPath)
			showAlertDialog("Error", "Failed to open charts in browser. The file was saved to:\n"+htmlPath, []string{"OK"})
		}
	}()
}

func generateChartsHTML() (string, error) {
	// Check if key types should be shown
	showKeyTypes := store.IsShowKeyTypesEnabled()

	type chartData struct {
		labels          []string
		keystrokeData   []string
		wordData        []string
		mouseDataFeet   []float64
		letterData      []string
		modifierData    []string
		specialData     []string
		totalKeystrokes int64
		totalWords      int64
		totalMouseDist  float64
		totalLetters    int64
		totalModifiers  int64
		totalSpecial    int64
		heatmapHTML     string
		// Derived stats
		avgKeystrokes   float64
		avgWordsActive  float64
		peakDayLabel    string
		peakDayValue    int64
		peakWordsLabel  string
		peakWordsValue  int64
		peakHourLabel   string
		peakHourValue   int64
		currentStreak   int
		longestStreak   int
		activeDays      int
		totalClicks     int64
	}

	prepareChartData := func(days int) (*chartData, error) {
		data := &chartData{}

		histStats, err := store.GetHistoricalStats(days)
		if err != nil {
			return nil, err
		}

		mouseStats, err := store.GetMouseHistoricalStats(days)
		if err != nil {
			return nil, err
		}

		hourlyData, err := store.GetAllHourlyStatsForDays(days)
		if err != nil {
			return nil, err
		}

		dayData := make([]stats.DayData, 0, len(histStats))
		var peakWords stats.DayData
		hourTotals := make([]int64, 24)

		for i, stat := range histStats {
			t, _ := time.Parse("2006-01-02", stat.Date)
			data.labels = append(data.labels, fmt.Sprintf("'%s'", t.Format("Jan 2")))
			data.keystrokeData = append(data.keystrokeData, fmt.Sprintf("%d", stat.Keystrokes))
			data.wordData = append(data.wordData, fmt.Sprintf("%d", stat.Words))
			data.letterData = append(data.letterData, fmt.Sprintf("%d", stat.Letters))
			data.modifierData = append(data.modifierData, fmt.Sprintf("%d", stat.Modifiers))
			data.specialData = append(data.specialData, fmt.Sprintf("%d", stat.Special))
			data.totalKeystrokes += stat.Keystrokes
			data.totalWords += stat.Words
			data.totalLetters += stat.Letters
			data.totalModifiers += stat.Modifiers
			data.totalSpecial += stat.Special

			dd := stats.DayData{Date: t, Keystrokes: stat.Keystrokes, Words: stat.Words}
			dayData = append(dayData, dd)
			if dd.Words > peakWords.Words {
				peakWords = dd
			}

			if hours, ok := hourlyData[stat.Date]; ok {
				for _, h := range hours {
					if h.Hour >= 0 && h.Hour < 24 {
						hourTotals[h.Hour] += h.Keystrokes
					}
				}
			}

			if i < len(mouseStats) {
				feet := mousetracker.PixelsToFeet(mouseStats[i].TotalDistance)
				data.mouseDataFeet = append(data.mouseDataFeet, feet)
				data.totalMouseDist += mouseStats[i].TotalDistance
				data.totalClicks += mouseStats[i].ClickCount
			} else {
				data.mouseDataFeet = append(data.mouseDataFeet, 0)
			}
		}

		// Derived stats
		if days > 0 {
			data.avgKeystrokes = float64(data.totalKeystrokes) / float64(days)
		}
		data.avgWordsActive = stats.CalculateAverageWordsActive(dayData)
		data.activeDays = stats.CountActiveDays(dayData)
		data.currentStreak = stats.CurrentStreak(dayData)
		data.longestStreak = stats.LongestStreak(dayData)

		if peak, ok := stats.FindPeakDay(dayData); ok {
			data.peakDayLabel = peak.Date.Format("Jan 2")
			data.peakDayValue = peak.Keystrokes
		} else {
			data.peakDayLabel = "—"
		}
		if peakWords.Words > 0 {
			data.peakWordsLabel = peakWords.Date.Format("Jan 2")
			data.peakWordsValue = peakWords.Words
		} else {
			data.peakWordsLabel = "—"
		}

		hour, count := stats.FindPeakHour(hourTotals)
		if count > 0 {
			data.peakHourLabel = stats.FormatHour(hour)
			data.peakHourValue = count
		} else {
			data.peakHourLabel = "—"
		}

		data.heatmapHTML = generateHeatmapHTML(hourlyData, days)
		return data, nil
	}

	weeklyData, err := prepareChartData(7)
	if err != nil {
		return "", err
	}

	monthlyData, err := prepareChartData(30)
	if err != nil {
		return "", err
	}

	yearlyData, err := prepareChartData(365)
	if err != nil {
		return "", err
	}

	formatMouseData := func(feetData []float64, divisor float64) string {
		var result []string
		for _, f := range feetData {
			result = append(result, fmt.Sprintf("%.2f", f/divisor))
		}
		return strings.Join(result, ",")
	}

	weeklyMouseFeetStr := formatMouseData(weeklyData.mouseDataFeet, 1.0)
	weeklyMouseCarsStr := formatMouseData(weeklyData.mouseDataFeet, 15.0)
	weeklyMouseFieldsStr := formatMouseData(weeklyData.mouseDataFeet, 330.0)
	monthlyMouseFeetStr := formatMouseData(monthlyData.mouseDataFeet, 1.0)
	monthlyMouseCarsStr := formatMouseData(monthlyData.mouseDataFeet, 15.0)
	monthlyMouseFieldsStr := formatMouseData(monthlyData.mouseDataFeet, 330.0)
	yearlyMouseFeetStr := formatMouseData(yearlyData.mouseDataFeet, 1.0)
	yearlyMouseCarsStr := formatMouseData(yearlyData.mouseDataFeet, 15.0)
	yearlyMouseFieldsStr := formatMouseData(yearlyData.mouseDataFeet, 330.0)

	// Get odometer data
	odometerSession, _ := store.GetOdometerSession()
	odometerIsActive := false
	odometerStartTime := ""
	odometerKeystrokes := int64(0)
	odometerWords := int64(0)
	odometerClicks := int64(0)
	odometerDistanceFeet := float64(0)

	if odometerSession != nil {
		odometerIsActive = odometerSession.IsActive
		if !odometerSession.StartTime.IsZero() {
			odometerStartTime = odometerSession.StartTime.Format("Jan 2, 2006 3:04 PM")
		}
		odometerKeystrokes = odometerSession.CurrentKeystrokes - odometerSession.StartKeystrokes
		odometerWords = odometerSession.CurrentWords - odometerSession.StartWords
		odometerClicks = odometerSession.CurrentClicks - odometerSession.StartClicks
		odometerDistanceFeet = mousetracker.PixelsToFeet(odometerSession.CurrentDistance - odometerSession.StartDistance)
	}

	// Get odometer history
	odometerHistory, _ := store.GetOdometerHistory()
	var historyJSON strings.Builder
	historyJSON.WriteString("[")
	for i, entry := range odometerHistory {
		if i > 0 {
			historyJSON.WriteString(",")
		}
		duration := entry.EndTime.Sub(entry.StartTime)
		historyJSON.WriteString(fmt.Sprintf(`{"id":%d,"startTime":"%s","endTime":"%s","keystrokes":%d,"words":%d,"clicks":%d,"distanceFeet":%.2f,"durationSecs":%d}`,
			entry.ID,
			entry.StartTime.Format("Jan 2, 2006 3:04 PM"),
			entry.EndTime.Format("Jan 2, 2006 3:04 PM"),
			entry.Keystrokes,
			entry.Words,
			entry.Clicks,
			mousetracker.PixelsToFeet(entry.Distance),
			int64(duration.Seconds()),
		))
	}
	historyJSON.WriteString("]")

	// Determine if key types section should be visible
	keyTypesDisplay := "none"
	if showKeyTypes {
		keyTypesDisplay = "flex"
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Typtel - Typing Statistics</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%%, #16213e 100%%);
            color: #eee;
            min-height: 100vh;
            padding: 30px;
        }
        h1 {
            text-align: center;
            margin-bottom: 10px;
            font-size: 2.5em;
            background: linear-gradient(90deg, #00d2ff, #3a7bd5);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .controls {
            display: flex;
            justify-content: center;
            gap: 20px;
            margin-bottom: 30px;
        }
        .control-group {
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .control-group label {
            color: #888;
            font-size: 0.9em;
        }
        select {
            background: rgba(255,255,255,0.1);
            border: 1px solid rgba(255,255,255,0.2);
            border-radius: 8px;
            color: #eee;
            padding: 8px 16px;
            font-size: 0.9em;
            cursor: pointer;
        }
        select:hover {
            background: rgba(255,255,255,0.15);
        }
        .charts-container {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 30px;
            max-width: 1400px;
            margin: 0 auto 40px;
        }
        .chart-box {
            background: rgba(255,255,255,0.05);
            border-radius: 16px;
            padding: 25px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255,255,255,0.1);
        }
        .chart-box h2 {
            margin-bottom: 20px;
            font-size: 1.3em;
            color: #aaa;
        }
        .heatmap-container {
            max-width: 1400px;
            margin: 0 auto;
        }
        .heatmap-box {
            background: rgba(255,255,255,0.05);
            border-radius: 16px;
            padding: 25px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255,255,255,0.1);
        }
        .heatmap-box h2 {
            margin-bottom: 20px;
            font-size: 1.3em;
            color: #aaa;
        }
        .heatmap {
            display: flex;
            flex-direction: column;
            gap: 3px;
        }
        .heatmap-row {
            display: flex;
            align-items: center;
            gap: 3px;
        }
        .heatmap-label {
            width: 70px;
            font-size: 11px;
            color: #888;
            text-align: right;
            padding-right: 10px;
        }
        .heatmap-cell {
            width: 20px;
            height: 20px;
            border-radius: 3px;
            transition: transform 0.2s;
        }
        .heatmap-cell:hover {
            transform: scale(1.3);
            z-index: 10;
        }
        .hour-labels {
            display: flex;
            gap: 3px;
            margin-left: 80px;
            margin-bottom: 5px;
        }
        .hour-label {
            width: 20px;
            font-size: 10px;
            color: #666;
            text-align: center;
        }
        .legend {
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 8px;
            margin-top: 20px;
        }
        .legend-text { color: #666; font-size: 12px; }
        .legend-box {
            width: 15px;
            height: 15px;
            border-radius: 2px;
        }
        .stats-summary {
            display: flex;
            justify-content: center;
            flex-wrap: wrap;
            gap: 40px;
            margin: 30px 0;
        }
        .stats-summary-secondary {
            margin-top: -10px;
            padding-top: 20px;
            border-top: 1px solid rgba(255,255,255,0.08);
            max-width: 1400px;
            margin-left: auto;
            margin-right: auto;
        }
        .stat-item {
            text-align: center;
            min-width: 110px;
        }
        .stat-value {
            font-size: 2.5em;
            font-weight: bold;
            background: linear-gradient(90deg, #00d2ff, #3a7bd5);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .stat-secondary {
            font-size: 1.6em;
            background: linear-gradient(90deg, #ffd166, #f08a5d);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .stat-label {
            color: #888;
            font-size: 0.9em;
        }
        .tooltip-container {
            display: inline-flex;
            align-items: center;
            gap: 6px;
        }
        .tooltip-help {
            display: inline-flex;
            align-items: center;
            justify-content: center;
            width: 16px;
            height: 16px;
            border-radius: 50%%;
            background: rgba(255,255,255,0.15);
            color: #888;
            font-size: 11px;
            cursor: help;
            position: relative;
        }
        .tooltip-help:hover {
            background: rgba(255,255,255,0.25);
            color: #fff;
        }
        .tooltip-content {
            display: none;
            position: absolute;
            bottom: 130%%;
            left: 50%%;
            transform: translateX(-50%%);
            background: rgba(30,30,50,0.98);
            border: 1px solid rgba(255,255,255,0.2);
            border-radius: 8px;
            padding: 10px 14px;
            min-width: 220px;
            font-size: 12px;
            color: #ddd;
            text-align: left;
            z-index: 100;
            line-height: 1.5;
            box-shadow: 0 4px 12px rgba(0,0,0,0.3);
        }
        .tooltip-content::after {
            content: '';
            position: absolute;
            top: 100%%;
            left: 50%%;
            transform: translateX(-50%%);
            border: 6px solid transparent;
            border-top-color: rgba(30,30,50,0.98);
        }
        .tooltip-help:hover .tooltip-content {
            display: block;
        }
        .tooltip-content strong {
            color: #fff;
        }
        .odometer-display {
            display: none;
            max-width: 800px;
            margin: 30px auto;
        }
        .odometer-box {
            background: rgba(255,255,255,0.05);
            border-radius: 16px;
            padding: 25px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255,255,255,0.1);
        }
        .odometer-box h2 {
            margin-bottom: 20px;
            font-size: 1.3em;
            color: #aaa;
        }
        .odometer-status {
            text-align: center;
            padding: 15px;
            margin-bottom: 20px;
            border-radius: 8px;
            font-size: 1.2em;
            font-weight: bold;
        }
        .odometer-status.active {
            background: rgba(122, 201, 111, 0.2);
            color: #7bc96f;
        }
        .odometer-status.inactive {
            background: rgba(255, 107, 107, 0.2);
            color: #ff6b6b;
        }
        .odometer-table {
            width: 100%%;
            border-collapse: collapse;
        }
        .odometer-table th {
            text-align: left;
            padding: 12px;
            border-bottom: 2px solid rgba(255,255,255,0.2);
            color: #888;
            font-size: 0.9em;
        }
        .odometer-table td {
            padding: 12px;
            border-bottom: 1px solid rgba(255,255,255,0.05);
        }
        .odometer-table tr:hover {
            background: rgba(255,255,255,0.05);
        }
        .odometer-value {
            font-weight: bold;
            background: linear-gradient(90deg, #00d2ff, #3a7bd5);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
    </style>
</head>
<body>
    <h1>Typtel Statistics</h1>

    <div class="controls">
        <div class="control-group">
            <label>Time Period:</label>
            <select id="periodSelect" onchange="updateCharts()">
                <option value="weekly">Weekly (7 days)</option>
                <option value="monthly">Monthly (30 days)</option>
                <option value="yearly">Yearly (365 days)</option>
                <option value="odometer">Odometer</option>
            </select>
        </div>
        <div class="control-group">
            <label>Distance Unit:</label>
            <select id="unitSelect" onchange="updateCharts()">
                <option value="feet">Feet</option>
                <option value="cars">Car Lengths (~15ft)</option>
                <option value="fields">Frisbee Fields (~330ft)</option>
            </select>
        </div>
    </div>

    <div class="stats-summary">
        <div class="stat-item">
            <div class="stat-value" id="totalKeystrokes">-</div>
            <div class="stat-label">Total Keystrokes</div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="totalWords">-</div>
            <div class="stat-label">Words</div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="avgKeystrokes">-</div>
            <div class="stat-label">Avg Keystrokes/Day</div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="avgWords">-</div>
            <div class="stat-label tooltip-container">Avg Words / Active Day <span class="tooltip-help">?<div class="tooltip-content"><strong>Average Words per Active Day</strong><br>Mean words on days with at least one keystroke. Excludes idle days so the figure reflects how much you type when you're actually typing.</div></span></div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="totalMouse">-</div>
            <div class="stat-label">Mouse Distance</div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="totalClicks">-</div>
            <div class="stat-label">Mouse Clicks</div>
        </div>
    </div>

    <div class="stats-summary stats-summary-secondary">
        <div class="stat-item">
            <div class="stat-value stat-secondary" id="peakDay">-</div>
            <div class="stat-label" id="peakDayLabel">Peak Day</div>
        </div>
        <div class="stat-item">
            <div class="stat-value stat-secondary" id="peakWords">-</div>
            <div class="stat-label" id="peakWordsLabel">Peak Words Day</div>
        </div>
        <div class="stat-item">
            <div class="stat-value stat-secondary" id="peakHour">-</div>
            <div class="stat-label" id="peakHourLabel">Most Active Hour</div>
        </div>
        <div class="stat-item">
            <div class="stat-value stat-secondary" id="currentStreak">-</div>
            <div class="stat-label tooltip-container">Current Streak <span class="tooltip-help">?<div class="tooltip-content"><strong>Current Streak</strong><br>Number of consecutive days, ending today, with at least one keystroke recorded.</div></span></div>
        </div>
        <div class="stat-item">
            <div class="stat-value stat-secondary" id="longestStreak">-</div>
            <div class="stat-label">Longest Streak</div>
        </div>
        <div class="stat-item">
            <div class="stat-value stat-secondary" id="activeDays">-</div>
            <div class="stat-label" id="activeDaysLabel">Active Days</div>
        </div>
    </div>

    <div class="stats-summary" id="keyTypesStats" style="display: %[1]s;">
        <div class="stat-item">
            <div class="stat-value" id="totalLetters" style="background: linear-gradient(90deg, #7bc96f, #4caf50); -webkit-background-clip: text; -webkit-text-fill-color: transparent;">-</div>
            <div class="stat-label">Letters (A-Z)</div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="totalModifiers" style="background: linear-gradient(90deg, #ff9800, #f57c00); -webkit-background-clip: text; -webkit-text-fill-color: transparent;">-</div>
            <div class="stat-label tooltip-container">Modifiers <span class="tooltip-help">?<div class="tooltip-content"><strong>Modifier Keys:</strong><br>Shift (Left/Right), Control (Left/Right), Option/Alt (Left/Right), Command (Left/Right), Fn, Caps Lock</div></span></div>
        </div>
        <div class="stat-item">
            <div class="stat-value" id="totalSpecial" style="background: linear-gradient(90deg, #e91e63, #c2185b); -webkit-background-clip: text; -webkit-text-fill-color: transparent;">-</div>
            <div class="stat-label tooltip-container">Special Keys <span class="tooltip-help">?<div class="tooltip-content"><strong>Special Keys:</strong><br>Numbers (0-9), punctuation (!@#$%%), function keys (F1-F12), arrow keys, Tab, Return/Enter, Space, Backspace, Delete, Escape, etc.</div></span></div>
        </div>
    </div>

    <div class="charts-container">
        <div class="chart-box">
            <h2>Keystrokes per Day</h2>
            <canvas id="keystrokesChart"></canvas>
        </div>
        <div class="chart-box">
            <h2>Words per Day</h2>
            <canvas id="wordsChart"></canvas>
        </div>
    </div>

    <div class="charts-container" id="keyTypesCharts" style="display: %[1]s;">
        <div class="chart-box" style="grid-column: span 2;">
            <h2>Key Type Breakdown per Day</h2>
            <canvas id="keyTypesChart"></canvas>
        </div>
    </div>

    <div class="charts-container">
        <div class="chart-box" style="grid-column: span 2;">
            <h2 id="mouseChartTitle">Mouse Distance per Day</h2>
            <canvas id="mouseChart"></canvas>
        </div>
    </div>

    <div class="heatmap-container" id="heatmapSection">
        <div class="heatmap-box">
            <h2>Activity Heatmap (Hourly)</h2>
            <div class="hour-labels">
                %[2]s
            </div>
            <div class="heatmap" id="heatmapContainer">
            </div>
            <div class="legend">
                <span class="legend-text">Less</span>
                <div class="legend-box" style="background: #1a1a2e;"></div>
                <div class="legend-box" style="background: #2d4a3e;"></div>
                <div class="legend-box" style="background: #3d6b4f;"></div>
                <div class="legend-box" style="background: #5a9a6f;"></div>
                <div class="legend-box" style="background: #7bc96f;"></div>
                <span class="legend-text">More</span>
            </div>
        </div>
    </div>

    <div class="odometer-display" id="odometerDisplay">
        <div class="odometer-box">
            <h2>⏱️ Current Session</h2>
            <div class="odometer-status" id="odometerStatusBox">Inactive</div>
            <table class="odometer-table" id="currentSessionTable">
                <thead>
                    <tr>
                        <th>Metric</th>
                        <th>Value</th>
                    </tr>
                </thead>
                <tbody>
                    <tr>
                        <td>Start Time</td>
                        <td class="odometer-value" id="odometerStartTime">-</td>
                    </tr>
                    <tr>
                        <td>Keystrokes</td>
                        <td class="odometer-value" id="odometerKeystrokes">-</td>
                    </tr>
                    <tr>
                        <td>Words</td>
                        <td class="odometer-value" id="odometerWords">-</td>
                    </tr>
                    <tr>
                        <td>Mouse Clicks</td>
                        <td class="odometer-value" id="odometerClicks">-</td>
                    </tr>
                    <tr>
                        <td>Mouse Distance</td>
                        <td class="odometer-value" id="odometerDistance">-</td>
                    </tr>
                    <tr>
                        <td>Duration</td>
                        <td class="odometer-value" id="odometerDuration">-</td>
                    </tr>
                </tbody>
            </table>
        </div>
        <div class="odometer-box" style="margin-top: 20px;">
            <h2>📜 Session History</h2>
            <div id="historyTableContainer">
                <table class="odometer-table" id="historyTable">
                    <thead>
                        <tr>
                            <th>Start</th>
                            <th>End</th>
                            <th>Duration</th>
                            <th>Keystrokes</th>
                            <th>Words</th>
                            <th>Clicks</th>
                            <th>Distance</th>
                        </tr>
                    </thead>
                    <tbody id="historyTableBody">
                    </tbody>
                </table>
                <p id="noHistoryMsg" style="color: #888; text-align: center; padding: 20px;">No history yet. Complete an odometer session to see it here.</p>
            </div>
        </div>
    </div>

    <script>
        const data = {
            weekly: {
                labels: [%s],
                keystrokes: [%s],
                words: [%s],
                mouse: { feet: [%s], cars: [%s], fields: [%s] },
                letters: [%s],
                modifiers: [%s],
                special: [%s],
                totalKeystrokes: %d,
                totalWords: %d,
                totalMouseFeet: %.2f,
                totalClicks: %d,
                totalLetters: %d,
                totalModifiers: %d,
                totalSpecial: %d,
                avgKeystrokes: %.2f,
                avgWordsActive: %.2f,
                peakDayLabel: '%s',
                peakDayValue: %d,
                peakWordsLabel: '%s',
                peakWordsValue: %d,
                peakHourLabel: '%s',
                peakHourValue: %d,
                currentStreak: %d,
                longestStreak: %d,
                activeDays: %d,
                days: 7,
                heatmap: `+"`%s`"+`
            },
            monthly: {
                labels: [%s],
                keystrokes: [%s],
                words: [%s],
                mouse: { feet: [%s], cars: [%s], fields: [%s] },
                letters: [%s],
                modifiers: [%s],
                special: [%s],
                totalKeystrokes: %d,
                totalWords: %d,
                totalMouseFeet: %.2f,
                totalClicks: %d,
                totalLetters: %d,
                totalModifiers: %d,
                totalSpecial: %d,
                avgKeystrokes: %.2f,
                avgWordsActive: %.2f,
                peakDayLabel: '%s',
                peakDayValue: %d,
                peakWordsLabel: '%s',
                peakWordsValue: %d,
                peakHourLabel: '%s',
                peakHourValue: %d,
                currentStreak: %d,
                longestStreak: %d,
                activeDays: %d,
                days: 30,
                heatmap: `+"`%s`"+`
            },
            yearly: {
                labels: [%s],
                keystrokes: [%s],
                words: [%s],
                mouse: { feet: [%s], cars: [%s], fields: [%s] },
                letters: [%s],
                modifiers: [%s],
                special: [%s],
                totalKeystrokes: %d,
                totalWords: %d,
                totalMouseFeet: %.2f,
                totalClicks: %d,
                totalLetters: %d,
                totalModifiers: %d,
                totalSpecial: %d,
                avgKeystrokes: %.2f,
                avgWordsActive: %.2f,
                peakDayLabel: '%s',
                peakDayValue: %d,
                peakWordsLabel: '%s',
                peakWordsValue: %d,
                peakHourLabel: '%s',
                peakHourValue: %d,
                currentStreak: %d,
                longestStreak: %d,
                activeDays: %d,
                days: 365,
                heatmap: `+"`%s`"+`
            },
            odometer: {
                isActive: %t,
                startTime: '%s',
                keystrokes: %d,
                words: %d,
                clicks: %d,
                distanceFeet: %.2f,
                history: %s
            }
        };

        const unitLabels = { feet: 'feet', cars: 'car lengths', fields: 'frisbee fields' };

        let keystrokesChart, wordsChart, mouseChart, keyTypesChart;

        const chartConfig = {
            responsive: true,
            plugins: { legend: { display: false } },
            scales: {
                y: { beginAtZero: true, grid: { color: 'rgba(255,255,255,0.1)' }, ticks: { color: '#888' } },
                x: { grid: { display: false }, ticks: { color: '#888' } }
            }
        };

        const stackedChartConfig = {
            responsive: true,
            plugins: { legend: { display: true, labels: { color: '#888' } } },
            scales: {
                y: { beginAtZero: true, stacked: true, grid: { color: 'rgba(255,255,255,0.1)' }, ticks: { color: '#888' } },
                x: { stacked: true, grid: { display: false }, ticks: { color: '#888' } }
            }
        };

        function formatNumber(n) {
            if (n >= 1000000) return (n/1000000).toFixed(1) + 'M';
            if (n >= 1000) return (n/1000).toFixed(1) + 'K';
            return n.toString();
        }

        function formatDistance(feet, unit) {
            unit = unit || 'feet';
            switch(unit) {
                case 'cars':
                    const cars = feet / 15.0;
                    if (cars >= 1000) return (cars/1000).toFixed(1) + 'k cars';
                    if (cars >= 1) return cars.toFixed(0) + ' cars';
                    return cars.toFixed(1) + ' cars';
                case 'fields':
                    const fields = feet / 330.0;
                    if (fields >= 100) return fields.toFixed(0) + ' fields';
                    if (fields >= 1) return fields.toFixed(1) + ' fields';
                    return fields.toFixed(2) + ' fields';
                default:
                    if (feet >= 5280) return (feet/5280).toFixed(2) + ' mi';
                    if (feet >= 1) return feet.toFixed(0) + ' ft';
                    return (feet * 12).toFixed(0) + ' in';
            }
        }

        function updateCharts() {
            const period = document.getElementById('periodSelect').value;
            const unit = document.getElementById('unitSelect').value;

            // Handle odometer display separately
            if (period === 'odometer') {
                document.querySelectorAll('.stats-summary').forEach(el => el.style.display = 'none');
                document.getElementById('keyTypesStats').style.display = 'none';
                document.querySelectorAll('.charts-container').forEach(el => el.style.display = 'none');
                document.getElementById('heatmapSection').style.display = 'none';
                document.getElementById('odometerDisplay').style.display = 'block';
                updateOdometerDisplay();
                return;
            }

            // Show regular charts, hide odometer
            document.querySelectorAll('.stats-summary').forEach(el => el.style.display = 'flex');
            const keyTypesStats = document.getElementById('keyTypesStats');
            if (keyTypesStats) keyTypesStats.style.display = keyTypesStats.getAttribute('data-visible') === 'true' ? 'flex' : 'none';
            document.querySelectorAll('.charts-container').forEach(el => el.style.display = 'grid');
            document.getElementById('heatmapSection').style.display = 'block';
            document.getElementById('odometerDisplay').style.display = 'none';

            const d = data[period];

            document.getElementById('totalKeystrokes').textContent = formatNumber(d.totalKeystrokes);
            document.getElementById('totalWords').textContent = formatNumber(d.totalWords);
            document.getElementById('avgKeystrokes').textContent = formatNumber(Math.round(d.avgKeystrokes));
            document.getElementById('avgWords').textContent = formatNumber(Math.round(d.avgWordsActive));
            document.getElementById('totalMouse').textContent = formatDistance(d.totalMouseFeet, unit);
            document.getElementById('totalClicks').textContent = formatNumber(d.totalClicks);

            // Secondary stats row
            const peakDayEl = document.getElementById('peakDay');
            const peakDayLabelEl = document.getElementById('peakDayLabel');
            if (d.peakDayValue > 0) {
                peakDayEl.textContent = formatNumber(d.peakDayValue);
                peakDayLabelEl.textContent = 'Peak Day (' + d.peakDayLabel + ')';
            } else {
                peakDayEl.textContent = '—';
                peakDayLabelEl.textContent = 'Peak Day';
            }

            const peakWordsEl = document.getElementById('peakWords');
            const peakWordsLabelEl = document.getElementById('peakWordsLabel');
            if (d.peakWordsValue > 0) {
                peakWordsEl.textContent = formatNumber(d.peakWordsValue);
                peakWordsLabelEl.textContent = 'Peak Words (' + d.peakWordsLabel + ')';
            } else {
                peakWordsEl.textContent = '—';
                peakWordsLabelEl.textContent = 'Peak Words Day';
            }

            const peakHourEl = document.getElementById('peakHour');
            const peakHourLabelEl = document.getElementById('peakHourLabel');
            if (d.peakHourValue > 0) {
                peakHourEl.textContent = d.peakHourLabel;
                peakHourLabelEl.textContent = 'Most Active Hour (' + formatNumber(d.peakHourValue) + ')';
            } else {
                peakHourEl.textContent = '—';
                peakHourLabelEl.textContent = 'Most Active Hour';
            }

            document.getElementById('currentStreak').textContent = d.currentStreak + (d.currentStreak === 1 ? ' day' : ' days');
            document.getElementById('longestStreak').textContent = d.longestStreak + (d.longestStreak === 1 ? ' day' : ' days');
            document.getElementById('activeDays').textContent = d.activeDays;
            document.getElementById('activeDaysLabel').textContent = 'Active Days / ' + d.days;

            // Update key types stats if visible
            if (keyTypesStats && keyTypesStats.style.display !== 'none') {
                document.getElementById('totalLetters').textContent = formatNumber(d.totalLetters);
                document.getElementById('totalModifiers').textContent = formatNumber(d.totalModifiers);
                document.getElementById('totalSpecial').textContent = formatNumber(d.totalSpecial);
            }

            document.getElementById('mouseChartTitle').textContent = 'Mouse Distance per Day (' + unitLabels[unit] + ')';

            if (keystrokesChart) keystrokesChart.destroy();
            if (wordsChart) wordsChart.destroy();
            if (mouseChart) mouseChart.destroy();
            if (keyTypesChart) keyTypesChart.destroy();

            keystrokesChart = new Chart(document.getElementById('keystrokesChart'), {
                type: 'bar',
                data: {
                    labels: d.labels,
                    datasets: [{ data: d.keystrokes, backgroundColor: 'rgba(0, 210, 255, 0.6)', borderColor: 'rgba(0, 210, 255, 1)', borderWidth: 1, borderRadius: 4 }]
                },
                options: chartConfig
            });

            wordsChart = new Chart(document.getElementById('wordsChart'), {
                type: 'line',
                data: {
                    labels: d.labels,
                    datasets: [{ data: d.words, borderColor: 'rgba(122, 201, 111, 1)', backgroundColor: 'rgba(122, 201, 111, 0.2)', fill: true, tension: 0.4, pointRadius: 4, pointBackgroundColor: 'rgba(122, 201, 111, 1)' }]
                },
                options: chartConfig
            });

            // Key types chart (stacked bar) - only create if element exists
            const keyTypesCanvas = document.getElementById('keyTypesChart');
            if (keyTypesCanvas) {
                keyTypesChart = new Chart(keyTypesCanvas, {
                    type: 'bar',
                    data: {
                        labels: d.labels,
                        datasets: [
                            { label: 'Letters', data: d.letters, backgroundColor: 'rgba(122, 201, 111, 0.7)', borderColor: 'rgba(122, 201, 111, 1)', borderWidth: 1 },
                            { label: 'Modifiers', data: d.modifiers, backgroundColor: 'rgba(255, 152, 0, 0.7)', borderColor: 'rgba(255, 152, 0, 1)', borderWidth: 1 },
                            { label: 'Special', data: d.special, backgroundColor: 'rgba(233, 30, 99, 0.7)', borderColor: 'rgba(233, 30, 99, 1)', borderWidth: 1 }
                        ]
                    },
                    options: stackedChartConfig
                });
            }

            mouseChart = new Chart(document.getElementById('mouseChart'), {
                type: 'bar',
                data: {
                    labels: d.labels,
                    datasets: [{ data: d.mouse[unit], backgroundColor: 'rgba(255, 107, 107, 0.6)', borderColor: 'rgba(255, 107, 107, 1)', borderWidth: 1, borderRadius: 4 }]
                },
                options: chartConfig
            });

            document.getElementById('heatmapContainer').innerHTML = d.heatmap;
        }

        function formatDuration(totalSeconds) {
            const hours = Math.floor(totalSeconds / 3600);
            const minutes = Math.floor((totalSeconds %% 3600) / 60);
            const seconds = totalSeconds %% 60;
            let durationStr = '';
            if (hours > 0) durationStr += hours + 'h ';
            if (minutes > 0 || hours > 0) durationStr += minutes + 'm ';
            durationStr += seconds + 's';
            return durationStr || '0s';
        }

        function updateOdometerDisplay() {
            const od = data.odometer;
            const unit = document.getElementById('unitSelect').value;

            const statusBox = document.getElementById('odometerStatusBox');
            if (od.isActive) {
                statusBox.textContent = 'Active';
                statusBox.className = 'odometer-status active';
            } else {
                statusBox.textContent = 'Inactive';
                statusBox.className = 'odometer-status inactive';
            }

            document.getElementById('odometerStartTime').textContent = od.startTime || '-';
            document.getElementById('odometerKeystrokes').textContent = formatNumber(od.keystrokes);
            document.getElementById('odometerWords').textContent = formatNumber(od.words);
            document.getElementById('odometerClicks').textContent = formatNumber(od.clicks);
            document.getElementById('odometerDistance').textContent = formatDistance(od.distanceFeet, unit);

            // Calculate duration if active
            if (od.isActive && od.startTime) {
                const start = new Date(od.startTime);
                const now = new Date();
                const totalSeconds = Math.floor((now - start) / 1000);
                document.getElementById('odometerDuration').textContent = formatDuration(totalSeconds);
            } else {
                document.getElementById('odometerDuration').textContent = '-';
            }

            // Populate history table
            const historyBody = document.getElementById('historyTableBody');
            const noHistoryMsg = document.getElementById('noHistoryMsg');
            const historyTable = document.getElementById('historyTable');

            historyBody.innerHTML = '';
            if (od.history && od.history.length > 0) {
                historyTable.style.display = 'table';
                noHistoryMsg.style.display = 'none';
                od.history.forEach(function(entry) {
                    var row = document.createElement('tr');
                    row.innerHTML = '<td>' + entry.startTime + '</td>' +
                        '<td>' + entry.endTime + '</td>' +
                        '<td class="odometer-value">' + formatDuration(entry.durationSecs) + '</td>' +
                        '<td class="odometer-value">' + formatNumber(entry.keystrokes) + '</td>' +
                        '<td class="odometer-value">' + formatNumber(entry.words) + '</td>' +
                        '<td class="odometer-value">' + formatNumber(entry.clicks) + '</td>' +
                        '<td class="odometer-value">' + formatDistance(entry.distanceFeet, unit) + '</td>';
                    historyBody.appendChild(row);
                });
            } else {
                historyTable.style.display = 'none';
                noHistoryMsg.style.display = 'block';
            }
        }

        // Store initial visibility state for keyTypesStats
        (function() {
            const keyTypesStats = document.getElementById('keyTypesStats');
            if (keyTypesStats) {
                keyTypesStats.setAttribute('data-visible', keyTypesStats.style.display !== 'none');
            }
        })();

        updateCharts();
    </script>
</body>
</html>`,
		keyTypesDisplay,      // %[1]s - key types stats and charts display
		generateHourLabels(), // %[2]s - hour labels
		strings.Join(weeklyData.labels, ","),
		strings.Join(weeklyData.keystrokeData, ","),
		strings.Join(weeklyData.wordData, ","),
		weeklyMouseFeetStr,
		weeklyMouseCarsStr,
		weeklyMouseFieldsStr,
		strings.Join(weeklyData.letterData, ","),
		strings.Join(weeklyData.modifierData, ","),
		strings.Join(weeklyData.specialData, ","),
		weeklyData.totalKeystrokes,
		weeklyData.totalWords,
		mousetracker.PixelsToFeet(weeklyData.totalMouseDist),
		weeklyData.totalClicks,
		weeklyData.totalLetters,
		weeklyData.totalModifiers,
		weeklyData.totalSpecial,
		weeklyData.avgKeystrokes,
		weeklyData.avgWordsActive,
		weeklyData.peakDayLabel,
		weeklyData.peakDayValue,
		weeklyData.peakWordsLabel,
		weeklyData.peakWordsValue,
		weeklyData.peakHourLabel,
		weeklyData.peakHourValue,
		weeklyData.currentStreak,
		weeklyData.longestStreak,
		weeklyData.activeDays,
		weeklyData.heatmapHTML,
		strings.Join(monthlyData.labels, ","),
		strings.Join(monthlyData.keystrokeData, ","),
		strings.Join(monthlyData.wordData, ","),
		monthlyMouseFeetStr,
		monthlyMouseCarsStr,
		monthlyMouseFieldsStr,
		strings.Join(monthlyData.letterData, ","),
		strings.Join(monthlyData.modifierData, ","),
		strings.Join(monthlyData.specialData, ","),
		monthlyData.totalKeystrokes,
		monthlyData.totalWords,
		mousetracker.PixelsToFeet(monthlyData.totalMouseDist),
		monthlyData.totalClicks,
		monthlyData.totalLetters,
		monthlyData.totalModifiers,
		monthlyData.totalSpecial,
		monthlyData.avgKeystrokes,
		monthlyData.avgWordsActive,
		monthlyData.peakDayLabel,
		monthlyData.peakDayValue,
		monthlyData.peakWordsLabel,
		monthlyData.peakWordsValue,
		monthlyData.peakHourLabel,
		monthlyData.peakHourValue,
		monthlyData.currentStreak,
		monthlyData.longestStreak,
		monthlyData.activeDays,
		monthlyData.heatmapHTML,
		strings.Join(yearlyData.labels, ","),
		strings.Join(yearlyData.keystrokeData, ","),
		strings.Join(yearlyData.wordData, ","),
		yearlyMouseFeetStr,
		yearlyMouseCarsStr,
		yearlyMouseFieldsStr,
		strings.Join(yearlyData.letterData, ","),
		strings.Join(yearlyData.modifierData, ","),
		strings.Join(yearlyData.specialData, ","),
		yearlyData.totalKeystrokes,
		yearlyData.totalWords,
		mousetracker.PixelsToFeet(yearlyData.totalMouseDist),
		yearlyData.totalClicks,
		yearlyData.totalLetters,
		yearlyData.totalModifiers,
		yearlyData.totalSpecial,
		yearlyData.avgKeystrokes,
		yearlyData.avgWordsActive,
		yearlyData.peakDayLabel,
		yearlyData.peakDayValue,
		yearlyData.peakWordsLabel,
		yearlyData.peakWordsValue,
		yearlyData.peakHourLabel,
		yearlyData.peakHourValue,
		yearlyData.currentStreak,
		yearlyData.longestStreak,
		yearlyData.activeDays,
		yearlyData.heatmapHTML,
		odometerIsActive,
		odometerStartTime,
		odometerKeystrokes,
		odometerWords,
		odometerClicks,
		odometerDistanceFeet,
		historyJSON.String(),
	)

	dataDir, err := getLogDir()
	if err != nil {
		return "", err
	}
	htmlPath := filepath.Join(dataDir, "charts.html")
	if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
		return "", err
	}

	return htmlPath, nil
}

func generateHourLabels() string {
	var labels []string
	for h := 0; h < 24; h++ {
		if h%3 == 0 {
			labels = append(labels, fmt.Sprintf(`<div class="hour-label">%d</div>`, h))
		} else {
			labels = append(labels, `<div class="hour-label"></div>`)
		}
	}
	return strings.Join(labels, "\n                ")
}

func generateHeatmapHTML(hourlyData map[string][]HourlyStats, days int) string {
	var maxVal int64 = 1
	for _, hours := range hourlyData {
		for _, h := range hours {
			if h.Keystrokes > maxVal {
				maxVal = h.Keystrokes
			}
		}
	}

	dates := make([]string, 0, len(hourlyData))
	for date := range hourlyData {
		dates = append(dates, date)
	}
	sort.Strings(dates)

	var rows []string
	for _, date := range dates {
		hours := hourlyData[date]
		t, _ := time.Parse("2006-01-02", date)
		dateLabel := t.Format("Mon Jan 2")

		var cells []string
		for _, h := range hours {
			color := getHeatmapColor(h.Keystrokes, maxVal)
			title := fmt.Sprintf("%s %d:00 - %d keystrokes", dateLabel, h.Hour, h.Keystrokes)
			cells = append(cells, fmt.Sprintf(
				`<div class="heatmap-cell" style="background: %s;" title="%s"></div>`,
				color, title,
			))
		}

		rows = append(rows, fmt.Sprintf(
			`<div class="heatmap-row"><div class="heatmap-label">%s</div>%s</div>`,
			dateLabel,
			strings.Join(cells, ""),
		))
	}

	return strings.Join(rows, "\n                ")
}

func getHeatmapColor(value, max int64) string {
	if value == 0 {
		return "#1a1a2e"
	}
	ratio := float64(value) / float64(max)
	if ratio < 0.25 {
		return "#2d4a3e"
	} else if ratio < 0.5 {
		return "#3d6b4f"
	} else if ratio < 0.75 {
		return "#5a9a6f"
	}
	return "#7bc96f"
}

type HourlyStats = storage.HourlyStats
