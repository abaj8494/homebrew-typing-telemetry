package main

import (
	"context"
	"fmt"

	"github.com/aayushbajaj/typing-telemetry/internal/push"
	"github.com/aayushbajaj/typing-telemetry/internal/storage"
	"github.com/spf13/cobra"
)

// Flags for the push subcommands.
var (
	pushURL   string
	pushToken string
	pushID    string
	pushName  string
)

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push THIS machine's typing stats to a host typtel (opt-in)",
	Long: `Push is the outbound counterpart to "typtel devices": it sends this
machine's own daily aggregates to a host typtel's ingest API over Tailscale,
so the host can show a combined cross-device total.

It is OFF by default and does nothing until you run "typtel push enable".
A single-device user can ignore this command entirely.

With no subcommand, prints the current push status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPushStatus()
	},
}

var pushEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable pushing to a host (e.g. --url http://100.x.y.z:8889 --token <t> --id <id>)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPushEnable()
	},
}

var pushDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Stop pushing (settings are kept; re-enable with 'typtel push enable')",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPushDisable()
	},
}

var pushStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current push configuration (token masked)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPushStatus()
	},
}

var pushNowCmd = &cobra.Command{
	Use:   "now",
	Short: "Push today's stats once now (flags override stored config; ignores enabled state)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPushNow()
	},
}

func init() {
	for _, c := range []*cobra.Command{pushEnableCmd, pushNowCmd} {
		c.Flags().StringVar(&pushURL, "url", "", "Host base URL, e.g. http://100.93.238.15:8889")
		c.Flags().StringVar(&pushToken, "token", "", "Bearer token from the host ('typtel devices token')")
		c.Flags().StringVar(&pushID, "id", "", "This device's id (must match [a-z0-9-]{1,32})")
		c.Flags().StringVar(&pushName, "name", "", "Friendly name shown on the host (optional)")
	}
	pushCmd.AddCommand(pushEnableCmd, pushDisableCmd, pushStatusCmd, pushNowCmd)
}

// effectiveConfig merges stored push settings with any flags supplied this run
// (flags win). Used by both `enable` (persist) and `now` (transient).
func effectiveConfig(store *storage.Store) push.Config {
	cfg, _, _ := push.LoadConfig(store)
	if pushURL != "" {
		cfg.BaseURL = pushURL
	}
	if pushToken != "" {
		cfg.Token = pushToken
	}
	if pushID != "" {
		cfg.DeviceID = pushID
	}
	if pushName != "" {
		cfg.Name = pushName
	}
	return cfg
}

func runPushEnable() error {
	store, err := storage.New()
	if err != nil {
		return err
	}
	defer store.Close()

	cfg := effectiveConfig(store)
	// Validate the merged config before persisting anything.
	if _, err := push.New(cfg); err != nil {
		return err
	}

	store.SetSetting(storage.SettingPushBaseURL, cfg.BaseURL)
	store.SetSetting(storage.SettingPushToken, cfg.Token)
	store.SetSetting(storage.SettingPushDeviceID, cfg.DeviceID)
	store.SetSetting(storage.SettingPushDeviceName, cfg.Name)
	if err := store.SetSettingBool(storage.SettingPushEnabled, true); err != nil {
		return err
	}

	fmt.Println("Push enabled.")
	fmt.Printf("  host:   %s\n", cfg.BaseURL)
	fmt.Printf("  id:     %s\n", cfg.DeviceID)
	if cfg.Name != "" {
		fmt.Printf("  name:   %s\n", cfg.Name)
	}
	fmt.Printf("  token:  %s\n", maskToken(cfg.Token))
	fmt.Println("\nRestart the typtel daemon (typtel-tray on Linux, the menubar app on macOS) to start pushing.")
	fmt.Println("Tip: 'typtel push now' sends one push immediately to confirm the host is reachable.")
	return nil
}

func runPushDisable() error {
	store, err := storage.New()
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.SetSettingBool(storage.SettingPushEnabled, false); err != nil {
		return err
	}
	fmt.Println("Push disabled. (Stored host/token/id are kept; re-enable with 'typtel push enable'.)")
	return nil
}

func runPushStatus() error {
	store, err := storage.New()
	if err != nil {
		return err
	}
	defer store.Close()

	cfg, enabled, _ := push.LoadConfig(store)
	state := "disabled"
	if enabled {
		state = "enabled"
	}
	fmt.Printf("push: %s\n", state)
	if cfg.BaseURL == "" && cfg.DeviceID == "" {
		fmt.Println("  (not configured — run 'typtel push enable --url … --token … --id …')")
		return nil
	}
	fmt.Printf("  host:  %s\n", orDash(cfg.BaseURL))
	fmt.Printf("  id:    %s\n", orDash(cfg.DeviceID))
	fmt.Printf("  name:  %s\n", orDash(cfg.Name))
	fmt.Printf("  token: %s\n", maskToken(cfg.Token))
	return nil
}

func runPushNow() error {
	store, err := storage.New()
	if err != nil {
		return err
	}
	defer store.Close()

	cfg := effectiveConfig(store)
	client, err := push.New(cfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*push.DefaultTimeout/10)
	defer cancel()
	if err := client.Health(ctx); err != nil {
		return fmt.Errorf("host not reachable: %w", err)
	}
	if err := client.PushToday(ctx, store); err != nil {
		return fmt.Errorf("push failed: %w", err)
	}
	fmt.Printf("Pushed today's stats to %s as %s.\n", cfg.BaseURL, cfg.DeviceID)
	return nil
}

// maskToken shows only the last 4 characters of a token.
func maskToken(t string) string {
	if t == "" {
		return "(none)"
	}
	if len(t) <= 4 {
		return "****"
	}
	return "****" + t[len(t)-4:]
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
