# Typtel

**Keystroke & typing telemetry for developers.** Typtel records how you type —
every keypress including modifiers, shortcuts, and escape sequences — to a local
SQLite database, and surfaces it as live counts, typing-speed (WPM) analytics,
and rich activity charts. **Nothing leaves your machine** unless you explicitly
opt into the [multi-device feed](multi-device.md).

It runs as a **macOS menu-bar app**, a **Linux X11 tray**, and a cross-platform
**CLI** (`typtel`) — including an interactive typing test.

![Typtel menu bar and charts](img/charts-html.png)

<div class="grid cards" markdown>

- :material-apple: **[macOS](macos.md)** — menu-bar app with live stats, charts, and settings
- :material-linux: **[Linux (X11)](linux.md)** — StatusNotifier tray with the same capture, charts, and inertia
- :material-console: **[Scripting](scripting.md)** — drive everything from i3/xmonad/polybar via the CLI
- :material-flash: **[Inertia](inertia.md)** — accelerating key-repeat with full controls

</div>

## Highlights

- **Local-first.** All data lives in `~/.local/share/typtel/typtel.db`. No network by default.
- **Typing speed.** Active-time WPM (idle auto-paused) plus fastest-pace records.
- **[Charts](charts.md).** Keystrokes/words per day, an activity heatmap, key-type breakdown, streaks, and peaks.
- **[Inertia](inertia.md).** Hold a key and watch the repeat rate accelerate — fully tunable (max speed, threshold, acceleration).
- **[Multi-device feed](multi-device.md).** Opt-in: let other machines (or a reMarkable) report into one host's combined totals over Tailscale.
- **[Typing test](getting-started.md#typing-test).** A built-in WPM test with themes and custom texts.

## Install

```sh
brew tap abaj8494/typing-telemetry
brew install --cask typtel          # macOS menu-bar app + CLI
```

On Linux, build the X11 tray from source — see **[Linux (X11)](linux.md)**.

See **[Getting started](getting-started.md)** for first-run setup (including the
macOS Accessibility grant), or jump to the **[CLI reference](reference/cli.md)**
and **[Settings reference](reference/settings.md)**.
