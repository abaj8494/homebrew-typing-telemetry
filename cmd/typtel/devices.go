package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aayushbajaj/typing-telemetry/internal/storage"
	"github.com/spf13/cobra"
)

// rotateToken is the flag for `typtel devices token --rotate`.
var rotateToken bool

var devicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "Manage external-device keystroke feeds (e.g. a reMarkable tablet)",
	Long: `Device feeds are a separate source from this Mac's own typing.

External devices PUT absolute daily aggregates to the opt-in, Tailscale-bound
ingest API (see "typtel devices enable"). Their stats are stored in dedicated
tables and never mix into the Mac's daily_summary.

With no subcommand, lists registered devices.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDevicesList()
	},
}

var devicesShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show recent days reported by a device",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDevicesShow(args[0])
	},
}

var devicesForgetCmd = &cobra.Command{
	Use:   "forget <id>",
	Short: "Delete a device and all of its recorded days",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDevicesForget(args[0])
	},
}

var devicesEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable the device ingest API (generates a token if absent)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDevicesEnable()
	},
}

var devicesDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable the device ingest API",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDevicesDisable()
	},
}

var devicesTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Print the bearer token (use --rotate to regenerate it)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDevicesToken()
	},
}

func init() {
	devicesShowCmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON instead of text")
	devicesTokenCmd.Flags().BoolVar(&rotateToken, "rotate", false, "Regenerate the bearer token")

	devicesCmd.AddCommand(devicesShowCmd)
	devicesCmd.AddCommand(devicesForgetCmd)
	devicesCmd.AddCommand(devicesEnableCmd)
	devicesCmd.AddCommand(devicesDisableCmd)
	devicesCmd.AddCommand(devicesTokenCmd)
}

// generateToken returns 32 hex characters (16 random bytes) for use as the
// ingest bearer token.
func generateToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func runDevicesList() error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	devices, err := store.ListDevices()
	if err != nil {
		return fmt.Errorf("list devices: %w", err)
	}
	if len(devices) == 0 {
		fmt.Println("No devices registered. Run 'typtel devices enable' to start the ingest API.")
		return nil
	}

	today := time.Now().Format("2006-01-02")
	// The KEYS/WORDS/MODS/SPECIAL columns are today's counts for each device.
	fmt.Printf("%-14s %-16s %9s %8s %8s %8s  %s\n",
		"DEVICE_ID", "NAME", "KEYS", "WORDS", "MODS", "SPECIAL", "LAST_SEEN")
	for _, d := range devices {
		var keys, words, mods, special int64
		if c, err := store.GetDeviceDay(d.DeviceID, today); err == nil && c != nil {
			keys, words, mods, special = c.Keystrokes, c.Words, c.Modifiers, c.Special
		}
		fmt.Printf("%-14s %-16s %9s %8s %8s %8s  %s\n",
			d.DeviceID, truncate(d.Name, 16),
			formatNum(keys), formatNum(words), formatNum(mods), formatNum(special),
			dashIfEmpty(d.LastSeen))
	}
	return nil
}

func runDevicesShow(id string) error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	days, err := store.GetDeviceDays(id, "")
	if err != nil {
		return fmt.Errorf("get device days: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if days == nil {
			days = []storage.DeviceDay{}
		}
		return enc.Encode(days)
	}

	if len(days) == 0 {
		fmt.Printf("No days recorded for device %q.\n", id)
		return nil
	}
	fmt.Printf("Recent days for %s:\n", id)
	fmt.Printf("%-12s %12s %10s %10s %10s %10s %12s\n",
		"DATE", "KEYSTROKES", "LETTERS", "MODIFIERS", "SPECIAL", "WORDS", "ACTIVE_MS")
	for _, d := range days {
		fmt.Printf("%-12s %12d %10d %10d %10d %10d %12d\n",
			d.Date, d.Keystrokes, d.Letters, d.Modifiers, d.Special, d.Words, d.ActiveMs)
	}
	return nil
}

func runDevicesForget(id string) error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	fmt.Printf("Delete device %q and all of its recorded days? [y/N]: ", id)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	if ans := strings.ToLower(strings.TrimSpace(line)); ans != "y" && ans != "yes" {
		fmt.Println("Aborted.")
		return nil
	}
	if err := store.DeleteDevice(id); err != nil {
		return fmt.Errorf("delete device: %w", err)
	}
	fmt.Printf("Forgot device %q.\n", id)
	return nil
}

func runDevicesEnable() error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	if err := store.SetSettingBool(storage.SettingDeviceIngestEnabled, true); err != nil {
		return fmt.Errorf("enable ingest: %w", err)
	}

	token, _ := store.GetSetting(storage.SettingDeviceIngestToken)
	if token == "" {
		token, err = generateToken()
		if err != nil {
			return fmt.Errorf("generate token: %w", err)
		}
		if err := store.SetSetting(storage.SettingDeviceIngestToken, token); err != nil {
			return fmt.Errorf("save token: %w", err)
		}
	}

	addr := store.GetSettingOr(storage.SettingDeviceIngestBindAddr, "127.0.0.1:8889")

	fmt.Println("Device ingest API enabled.")
	fmt.Printf("  Bearer token: %s\n", token)
	fmt.Printf("  Bind address: %s (loopback; reached over the tailnet via 'tailscale serve')\n", addr)
	fmt.Println()
	fmt.Println("⚠️  Restart the menubar app for this to take effect.")
	return nil
}

func runDevicesDisable() error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	if err := store.SetSettingBool(storage.SettingDeviceIngestEnabled, false); err != nil {
		return fmt.Errorf("disable ingest: %w", err)
	}
	fmt.Println("Device ingest API disabled. Restart the menubar app for this to take effect.")
	return nil
}

func runDevicesToken() error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	token, _ := store.GetSetting(storage.SettingDeviceIngestToken)
	if rotateToken || token == "" {
		token, err = generateToken()
		if err != nil {
			return fmt.Errorf("generate token: %w", err)
		}
		if err := store.SetSetting(storage.SettingDeviceIngestToken, token); err != nil {
			return fmt.Errorf("save token: %w", err)
		}
		if rotateToken {
			fmt.Println("Token rotated. Restart the menubar app and update the device.")
		}
	}
	fmt.Println(token)
	return nil
}

// runDeviceTodayText prints a device's today keystroke count, mirroring the
// menu-bar-friendly bare-integer output of `typtel today`.
func runDeviceTodayText(id string) error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	today := time.Now().Format("2006-01-02")
	c, err := store.GetDeviceDay(id, today)
	if err != nil {
		return fmt.Errorf("get device day: %w", err)
	}
	if c == nil {
		fmt.Println("0")
		return nil
	}
	fmt.Printf("%d\n", c.Keystrokes)
	return nil
}

// runDeviceTodayJSON emits a device's today counts as JSON. Absent days yield
// a zero-valued document (with the date filled in) so callers always get a
// well-formed object.
func runDeviceTodayJSON(id string) error {
	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer store.Close()

	today := time.Now().Format("2006-01-02")
	c, err := store.GetDeviceDay(id, today)
	if err != nil {
		return fmt.Errorf("get device day: %w", err)
	}
	out := storage.DeviceDay{Date: today}
	if c != nil {
		out.DeviceDayCounts = *c
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
