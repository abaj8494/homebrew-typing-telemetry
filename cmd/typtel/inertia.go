package main

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/aayushbajaj/typing-telemetry/internal/storage"
	"github.com/spf13/cobra"
)

// validSpeeds lists the accepted Max Speed values (and their approximate caps),
// in order, for help text and validation.
var validSpeeds = []struct{ val, desc string }{
	{storage.InertiaSpeedUltraFast, "~140 keys/s"},
	{storage.InertiaSpeedVeryFast, "~125 keys/s"},
	{storage.InertiaSpeedPrettyFast, "~100 keys/s"},
	{storage.InertiaSpeedFast, "~83 keys/s"},
	{storage.InertiaSpeedMedium, "~50 keys/s"},
	{storage.InertiaSpeedSlow, "~20 keys/s"},
}

var inertiaCmd = &cobra.Command{
	Use:   "inertia",
	Short: "Inspect and control accelerating key-repeat (scriptable)",
	Long: `Read and control inertia (accelerating key-repeat) from the shell — the
control surface for window-manager users (i3, xmonad, sway, …) who script their
own status bars and keybindings instead of using a tray menu.

A running typtel daemon (typtel-tray on Linux) applies changes live within a
couple of seconds; otherwise they take effect the next time it starts.

  typtel inertia                 # status (human-readable)
  typtel inertia status --json   # status as JSON (for polybar/i3blocks/jq)
  typtel inertia toggle          # bind to a key, e.g. i3 'bindsym $mod+i'
  typtel inertia on | off
  typtel inertia speed fast      # ultra_fast|very_fast|pretty_fast|fast|medium|slow
  typtel inertia threshold 200   # ms before acceleration starts
  typtel inertia accel 1.0       # acceleration-rate multiplier`,
	RunE: func(cmd *cobra.Command, args []string) error { return runInertiaStatus() },
}

var inertiaStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show inertia settings (--json for machine output)",
	RunE:  func(cmd *cobra.Command, args []string) error { return runInertiaStatus() },
}

var inertiaOnCmd = &cobra.Command{
	Use: "on", Short: "Enable inertia",
	RunE: func(cmd *cobra.Command, args []string) error { return setInertiaEnabled(true) },
}

var inertiaOffCmd = &cobra.Command{
	Use: "off", Short: "Disable inertia",
	RunE: func(cmd *cobra.Command, args []string) error { return setInertiaEnabled(false) },
}

var inertiaToggleCmd = &cobra.Command{
	Use: "toggle", Short: "Toggle inertia on/off",
	RunE: func(cmd *cobra.Command, args []string) error {
		return withStore(func(s *storage.Store) error {
			return applyEnabled(s, !s.GetInertiaSettings().Enabled)
		})
	},
}

var inertiaSpeedCmd = &cobra.Command{
	Use: "speed <name>", Short: "Set max speed cap", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, s := range validSpeeds {
			if args[0] == s.val {
				return withStore(func(st *storage.Store) error {
					if err := st.SetInertiaMaxSpeed(args[0]); err != nil {
						return err
					}
					return printInertia(st)
				})
			}
		}
		return fmt.Errorf("invalid speed %q; valid: %s", args[0], speedNames())
	},
}

var inertiaThresholdCmd = &cobra.Command{
	Use: "threshold <ms>", Short: "Set ms before acceleration starts", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ms, err := strconv.Atoi(args[0])
		if err != nil || ms <= 0 {
			return fmt.Errorf("threshold must be a positive integer (ms), got %q", args[0])
		}
		return withStore(func(st *storage.Store) error {
			if err := st.SetInertiaThreshold(ms); err != nil {
				return err
			}
			return printInertia(st)
		})
	},
}

var inertiaAccelCmd = &cobra.Command{
	Use: "accel <rate>", Short: "Set acceleration-rate multiplier", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rate, err := strconv.ParseFloat(args[0], 64)
		if err != nil || rate <= 0 {
			return fmt.Errorf("accel must be a positive number, got %q", args[0])
		}
		return withStore(func(st *storage.Store) error {
			if err := st.SetInertiaAccelRate(rate); err != nil {
				return err
			}
			return printInertia(st)
		})
	},
}

func init() {
	inertiaStatusCmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON")
	inertiaCmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON")
	inertiaCmd.AddCommand(inertiaStatusCmd, inertiaOnCmd, inertiaOffCmd, inertiaToggleCmd,
		inertiaSpeedCmd, inertiaThresholdCmd, inertiaAccelCmd)
}

// withStore opens the store, runs fn, and closes it.
func withStore(fn func(*storage.Store) error) error {
	store, err := storage.New()
	if err != nil {
		return err
	}
	defer store.Close()
	return fn(store)
}

func setInertiaEnabled(v bool) error {
	return withStore(func(s *storage.Store) error { return applyEnabled(s, v) })
}

func applyEnabled(s *storage.Store, v bool) error {
	if err := s.SetInertiaEnabled(v); err != nil {
		return err
	}
	return printInertia(s)
}

func runInertiaStatus() error { return withStore(printInertia) }

func printInertia(s *storage.Store) error {
	is := s.GetInertiaSettings()
	if jsonOutput {
		b, _ := json.MarshalIndent(map[string]any{
			"enabled":    is.Enabled,
			"max_speed":  is.MaxSpeed,
			"threshold":  is.Threshold,
			"accel_rate": is.AccelRate,
		}, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	state := "off"
	if is.Enabled {
		state = "on"
	}
	fmt.Printf("inertia: %s\n", state)
	fmt.Printf("  max speed: %s\n", is.MaxSpeed)
	fmt.Printf("  threshold: %dms\n", is.Threshold)
	fmt.Printf("  accel:     %gx\n", is.AccelRate)
	return nil
}

func speedNames() string {
	out := ""
	for i, s := range validSpeeds {
		if i > 0 {
			out += ", "
		}
		out += s.val
	}
	return out
}
