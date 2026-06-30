//go:build darwin
// +build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework UserNotifications

#import <Cocoa/Cocoa.h>
#include <stdlib.h>

// fyne/systray hides its NSStatusItem inside its private SystrayAppDelegate, so
// we reach it via KVC on the app delegate (ivars `statusItem` and `menu`). This
// lets us (a) draw a colored attributedTitle — fyne's SetTitle only sets a plain
// NSString — and (b) pop up fyne's own menu ourselves, since taking over the
// left-tap (SetOnTapped) suppresses fyne's automatic show_menu. Both are best
// effort: if the delegate shape ever changes, the @try/nil guards no-op safely.
static id systrayDelegateValue(NSString *key) {
    id delegate = [NSApp delegate];
    if (delegate == nil) return nil;
    @try {
        return [delegate valueForKey:key];
    } @catch (NSException *e) {
        return nil;
    }
}

// setMenuBarTitle sets the status-item button title. When colored != 0 it draws
// a high-contrast attributedTitle (system orange); otherwise a default-attributed
// string that follows the menu bar's own light/dark text color.
static void setMenuBarTitle(const char* ctitle, int colored) {
    NSString *title = [NSString stringWithUTF8String:ctitle];
    void (^block)(void) = ^{
        @autoreleasepool {
            NSStatusItem *si = (NSStatusItem *)systrayDelegateValue(@"statusItem");
            if (si == nil) return;
            NSStatusBarButton *button = si.button;
            if (button == nil) return;
            if (colored) {
                NSDictionary *attrs = @{ NSForegroundColorAttributeName: [NSColor systemOrangeColor] };
                button.attributedTitle = [[NSAttributedString alloc] initWithString:title attributes:attrs];
            } else {
                button.attributedTitle = [[NSAttributedString alloc] initWithString:title];
            }
        }
    };
    if ([NSThread isMainThread]) {
        block();
    } else {
        dispatch_async(dispatch_get_main_queue(), block);
    }
}

// popUpSystrayMenu pops fyne's status-bar menu under the button. It blocks (runs
// a modal tracking loop) until the menu is dismissed, so the caller can revert
// the title once it returns. Must be called from the main thread (the tap
// handler already is); a background caller is dispatched synchronously.
static void popUpSystrayMenu(void) {
    void (^block)(void) = ^{
        @autoreleasepool {
            NSStatusItem *si = (NSStatusItem *)systrayDelegateValue(@"statusItem");
            NSMenu *menu = (NSMenu *)systrayDelegateValue(@"menu");
            if (si == nil || menu == nil) return;
            NSStatusBarButton *button = si.button;
            if (button == nil) return;
            [menu popUpMenuPositioningItem:nil
                                atLocation:NSMakePoint(0, button.bounds.size.height + 6)
                                    inView:button];
        }
    };
    if ([NSThread isMainThread]) {
        block();
    } else {
        dispatch_sync(dispatch_get_main_queue(), block);
    }
}

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
	"context"
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
	"github.com/aayushbajaj/typing-telemetry/internal/charts"
	"github.com/aayushbajaj/typing-telemetry/internal/inertia"
	"github.com/aayushbajaj/typing-telemetry/internal/ingest"
	"github.com/aayushbajaj/typing-telemetry/internal/keylogger"
	"github.com/aayushbajaj/typing-telemetry/internal/mousetracker"
	"github.com/aayushbajaj/typing-telemetry/internal/storage"
	"github.com/aayushbajaj/typing-telemetry/internal/wordcounter"
	"github.com/aayushbajaj/typing-telemetry/pkg/stats"
)

var (
	store           *storage.Store
	lastMenuTitle   string
	lastMenuColored bool
	menuTitleMutex  sync.Mutex
	// deviceSumActive is true while the menu is open from a menu-bar tap: the
	// title then shows the Mac's stats summed with every connected device's
	// today totals, drawn in a high-contrast color. It reverts when the menu
	// closes. See onMenuBarTapped / updateMenuBarTitle.
	deviceSumActive bool
	deviceSumMutex  sync.Mutex
	// appFilter is shared between the keystroke loop and the settings UI so
	// that toggling strict mode or editing the allowlist takes effect live.
	appFilter = appfilter.New()
	// singletonLock holds the flock'd lockfile open for the process lifetime.
	// It must stay referenced so the fd isn't closed (closing releases the
	// lock); see acquireSingletonLock.
	singletonLock *os.File
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

	// Enforce a single running instance. A LaunchAgent-started daemon plus a
	// manual launch (or a leftover process after make install-app) otherwise
	// show two menu-bar icons and open the DB twice. First instance wins; a
	// duplicate exits quietly before touching storage or the keylogger.
	if ok, err := acquireSingletonLock(); err != nil {
		log.Printf("Singleton lock failed, continuing without it: %v", err)
	} else if !ok {
		log.Println("Another typtel menu bar instance is already running; exiting.")
		os.Exit(0)
	}

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

	// Lifetime context, cancelled on shutdown to gracefully stop background
	// goroutines such as the device-ingest listener.
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	// Device-ingest HTTP API (v1.4142). Opt-in and disabled by default. When
	// enabled it accepts absolute daily aggregates from external devices over a
	// Tailscale-bound, token-gated listener into the dedicated device_* tables —
	// the macOS capture path is untouched. Toggling this requires a restart.
	if store.GetSettingBool(storage.SettingDeviceIngestEnabled) {
		token, _ := store.GetSetting(storage.SettingDeviceIngestToken)
		addr := store.GetSettingOr(storage.SettingDeviceIngestBindAddr, defaultBindAddr)
		peersRaw, _ := store.GetSetting(storage.SettingDeviceIngestPeers)
		peers := splitCSV(peersRaw)
		srv := ingest.New(store, token, addr, peers, Version)
		go func() {
			if err := srv.Start(ctx); err != nil {
				log.Printf("[ingest] stopped: %v", err)
			}
		}()
		log.Printf("[ingest] listening on %s", addr)
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

	// Taking over the left-tap means fyne no longer auto-opens the menu, so the
	// handler opens it itself (and reverts the device-sum reveal on close).
	systray.SetOnTapped(onMenuBarTapped)

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

// maxDeviceSlots caps how many external devices the menu can show. Slots are
// pre-allocated at build time (systray menus are static) and shown/hidden on
// the stats ticker, mirroring the leaderboard pattern.
const maxDeviceSlots = 5

// deviceMenuSlot holds the menu items for one external device: a per-device
// submenu (titled with the device name) plus its stat rows.
type deviceMenuSlot struct {
	root       *systray.MenuItem
	keystrokes *systray.MenuItem
	words      *systray.MenuItem
	breakdown  *systray.MenuItem
	active     *systray.MenuItem
	lastSeen   *systray.MenuItem
}

// mDevices is the "📱 Devices" parent (hidden when no device has reported);
// deviceSlots are its pre-allocated per-device rows.
var (
	mDevices    *systray.MenuItem
	deviceSlots []deviceMenuSlot
)

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

	// Devices submenu (v1.4142): keystroke stats pushed from external devices
	// (e.g. a reMarkable tablet) via the opt-in ingest API. These are a separate
	// source and never mix into the Mac's own totals. The parent is hidden
	// entirely until a device registers, so non-users see no change. Slots are
	// pre-allocated and populated on the stats ticker (updateDevicesDisplay).
	mDevices = systray.AddMenuItem("📱 Devices", "Keystroke stats from external devices")
	for i := 0; i < maxDeviceSlots; i++ {
		root := mDevices.AddSubMenuItem("device", "")
		slot := deviceMenuSlot{
			root:       root,
			keystrokes: root.AddSubMenuItem("   Today: -- keystrokes", ""),
			words:      root.AddSubMenuItem("   -- words", ""),
			breakdown:  root.AddSubMenuItem("   -- letters / -- mod / -- special", ""),
			active:     root.AddSubMenuItem("   -- active", ""),
			lastSeen:   root.AddSubMenuItem("   last seen --", ""),
		}
		slot.keystrokes.Disable()
		slot.words.Disable()
		slot.breakdown.Disable()
		slot.active.Disable()
		slot.lastSeen.Disable()
		root.Hide()
		deviceSlots = append(deviceSlots, slot)
	}
	mDevices.Hide()

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
		setMenuTitle("⌨️ --", false)
		return
	}

	settings := store.GetMenubarSettings()
	mouseStats, _ := store.GetTodayMouseStats()

	keystrokes := stats.Keystrokes
	words := stats.Words

	// While the menu is open from a tap, fold every connected device's today
	// totals into the keystroke/word counts and draw the title highlighted.
	// Clicks and distance stay Mac-only — devices report neither.
	colored := deviceSumEnabled()
	if colored {
		dk, dw := deviceTotalsToday()
		keystrokes += dk
		words += dw
	}

	var parts []string

	if settings.ShowKeystrokes {
		parts = append(parts, fmt.Sprintf("⌨️%s", formatAbsolute(keystrokes)))
	}
	if settings.ShowWords {
		parts = append(parts, fmt.Sprintf("%sw", formatAbsolute(words)))
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

	setMenuTitle(title, colored)
}

// deviceTotalsToday sums every registered device's today keystrokes and words.
// Device stats are a separate source from the Mac's daily_summary; this is the
// only place they fold into the menu-bar figure (the tap reveal).
func deviceTotalsToday() (keystrokes, words int64) {
	devices, err := store.ListDevices()
	if err != nil {
		return 0, 0
	}
	today := time.Now().Format("2006-01-02")
	for _, d := range devices {
		if c, _ := store.GetDeviceDay(d.DeviceID, today); c != nil {
			keystrokes += c.Keystrokes
			words += c.Words
		}
	}
	return keystrokes, words
}

func deviceSumEnabled() bool {
	deviceSumMutex.Lock()
	defer deviceSumMutex.Unlock()
	return deviceSumActive
}

func setDeviceSumActive(on bool) {
	deviceSumMutex.Lock()
	deviceSumActive = on
	deviceSumMutex.Unlock()
}

// onMenuBarTapped runs on a left-tap of the status item (we took over the tap
// from fyne via SetOnTapped). It reveals the device-summed, highlighted title,
// then opens fyne's menu — popUpSystrayMenu blocks until the menu is dismissed
// (selection or click-away), at which point the title reverts to Mac-only.
func onMenuBarTapped() {
	setDeviceSumActive(true)
	updateMenuBarTitle()
	C.popUpSystrayMenu()
	setDeviceSumActive(false)
	updateMenuBarTitle()
}

func setMenuTitle(title string, colored bool) {
	menuTitleMutex.Lock()
	defer menuTitleMutex.Unlock()

	if title == lastMenuTitle && colored == lastMenuColored {
		return
	}

	lastMenuTitle = title
	lastMenuColored = colored

	ctitle := C.CString(title)
	defer C.free(unsafe.Pointer(ctitle))
	c := C.int(0)
	if colored {
		c = C.int(1)
	}
	C.setMenuBarTitle(ctitle, c)
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

	// Update external-device submenu
	updateDevicesDisplay()

	// Update leaderboard
	updateLeaderboard()
}

// updateDevicesDisplay refreshes the 📱 Devices submenu from the device tables.
// The parent is hidden when no device has reported; otherwise each registered
// device gets a row showing today's absolute counts plus when it last reported.
// Device stats are a separate source — they never fold into the Mac's totals.
func updateDevicesDisplay() {
	if mDevices == nil {
		return
	}
	devices, err := store.ListDevices()
	if err != nil || len(devices) == 0 {
		mDevices.Hide()
		for _, slot := range deviceSlots {
			slot.root.Hide()
		}
		return
	}

	mDevices.Show()
	today := time.Now().Format("2006-01-02")
	for i, slot := range deviceSlots {
		if i >= len(devices) {
			slot.root.Hide()
			continue
		}
		d := devices[i]
		name := d.Name
		if name == "" {
			name = d.DeviceID
		}
		slot.root.SetTitle(name)

		var ks, w, l, mod, sp, act int64
		if c, _ := store.GetDeviceDay(d.DeviceID, today); c != nil {
			ks, w, l, mod, sp, act = c.Keystrokes, c.Words, c.Letters, c.Modifiers, c.Special, c.ActiveMs
		}
		slot.keystrokes.SetTitle(fmt.Sprintf("   Today: %s keystrokes", formatAbsolute(ks)))
		slot.words.SetTitle(fmt.Sprintf("   %s words", formatAbsolute(w)))
		slot.breakdown.SetTitle(fmt.Sprintf("   %s letters / %s mod / %s special",
			formatAbsolute(l), formatAbsolute(mod), formatAbsolute(sp)))
		slot.active.SetTitle(fmt.Sprintf("   %s active", formatActiveMs(act)))
		slot.lastSeen.SetTitle(fmt.Sprintf("   last seen %s", formatLastSeen(d.LastSeen)))
		slot.root.Show()
	}
}

// formatActiveMs renders active-typing milliseconds as a compact duration.
func formatActiveMs(ms int64) string {
	if ms <= 0 {
		return "0m"
	}
	secs := ms / 1000
	if h := secs / 3600; h > 0 {
		return fmt.Sprintf("%dh %dm", h, (secs%3600)/60)
	}
	if m := secs / 60; m > 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%ds", secs)
}

// formatLastSeen renders an RFC3339 last_seen timestamp in the local zone, or a
// dash when it is empty/unparseable.
func formatLastSeen(s string) string {
	if s == "" {
		return "--"
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.Local().Format("Jan 2 15:04")
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
		fmt.Sprintf("Version %s\nPID: %d\n\nTrack your keystrokes and typing speed.\n\nGitHub: github.com/abaj8494/typing-telemetry", Version, os.Getpid()),
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

// acquireSingletonLock takes an exclusive, non-blocking flock on a lockfile in
// the data dir so only one menu-bar instance ever runs — otherwise a daemon
// (LaunchAgent) plus a manual launch leave two icons in the menu bar. It returns
// true when this process won the lock; false means another instance already
// holds it and the caller should exit. The lock is held for the process lifetime
// via the package-level singletonLock fd and is released automatically by the OS
// on exit or crash, so there is no stale-lock to clean up.
func acquireSingletonLock() (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	dir := filepath.Join(home, ".local", "share", "typtel")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "menubar.lock"), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK {
			return false, nil // another instance holds the lock
		}
		return false, err
	}
	singletonLock = f // keep open for the process lifetime
	return true, nil
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

		htmlPath, err := charts.Generate(store, charts.Options{PixelsToFeet: mousetracker.PixelsToFeet})
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
