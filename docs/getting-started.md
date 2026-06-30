# Getting started

**typtel** records keystroke and typing metrics locally and gives you a CLI, a
TUI dashboard, and a built-in typing test. Everything stays on your machine.

This page gets you from install to your first stats in a few minutes. For the
deep dives, see [macOS](macos.md), [Linux](linux.md), [inertia](inertia.md),
[charts](charts.md), and the [multi-device feed](multi-device.md).

## Install

### macOS

The menu-bar app and the `typtel` CLI ship together in the cask — the CLI is
symlinked out of the app bundle onto your `PATH`.

```sh
brew tap abaj8494/typing-telemetry
brew install --cask typtel
```

That gives you both the background menu-bar daemon and the `typtel` command.

!!! tip "CLI without the menu-bar app"
    If you only want the cross-platform CLI (stats, typing test, charts), there
    is a separate formula:

    ```sh
    brew install abaj8494/typing-telemetry/typing-telemetry
    ```

### Linux

The Homebrew cask is macOS-only. On X11 Linux (tested on Debian/Kali), build the
`typtel-tray` daemon and the `typtel` CLI from source. See [Linux](linux.md) for
the full walkthrough.

## First run

### macOS — grant Accessibility

Capturing keystrokes needs Accessibility permission for the CGEventTap:

1. Open **System Settings → Privacy & Security → Accessibility**.
2. Click **+** and add `/Applications/Typtel.app`.
3. Enable the checkbox for Typtel.
4. Relaunch from the menu bar, or `open /Applications/Typtel.app`.

!!! warning "Re-grant after every upgrade"
    A `brew upgrade --cask typtel` replaces the binary, which changes its
    cdhash. macOS treats the new binary as a different app and silently drops
    the existing grant. If Typtel won't capture input after an upgrade, remove
    its entry from the Accessibility list and add it back.

### Linux — start the tray

Run the tray daemon inside an X11 session (Wayland is not supported):

```sh
typtel-tray &
```

The tray icon shows today's keystrokes, words, and WPM. See [Linux](linux.md)
for autostart and window-manager setup.

## Where data lives

All data is stored locally under `~/.local/share/typtel/`:

| Path | Contents |
|------|----------|
| `~/.local/share/typtel/typtel.db` | SQLite database (keystrokes, daily summaries, settings) |
| `~/.local/share/typtel/logs/` | Application logs and generated charts (`charts.html`) |

!!! note "Nothing leaves your machine"
    typtel never sends data off the device. The only exception is the
    [multi-device feed](multi-device.md), which is opt-in and disabled by
    default — a fresh install never opens a port or talks to any device.

## A quick CLI tour

```sh
typtel              # interactive TUI dashboard (run with no arguments)
typtel today        # today's keystroke count (a single number, for status bars)
typtel stats        # today + this week, plus typing speed (WPM)
typtel stats --json # the same data as machine-readable JSON
typtel v            # generate and open the charts/heatmap in your browser
typtel version      # version info
```

`typtel today` and `typtel stats` both accept `--json` for scripting, and `v`
is aliased to `view` and `charts`. For inertia control and the device feed, and
the full flag list, see the [CLI reference](reference/cli.md), [charts](charts.md),
[inertia](inertia.md), [multi-device feed](multi-device.md), and
[scripting](scripting.md).

## Typing test

Launch the built-in test with `typtel test`. It measures net WPM, raw WPM,
accuracy, and CPM, and saves your results.

```sh
typtel test            # default 25-word test
typtel test -w 50      # 50 words (--words)
typtel test -f file.txt # custom text from a file (--file)
typtel test -l au      # AU English spelling, saved as the new default (--language)
```

`-l` accepts `us` or `au`; whichever you pass is persisted as your default for
future tests.

### In-test keys

| Key | Action |
|-----|--------|
| `Tab` | Restart with new words |
| `Esc` | Open the options menu (theme, layout, length, punctuation, pace caret) |
| `Enter` | Start / restart the test |
| `Backspace` | Delete one character (also `Ctrl-H` / `Delete` on Linux terminals) |
| `Ctrl+C` | Quit |

!!! tip
    Start typing to begin — the timer starts on your first keystroke. From the
    options menu you can search by typing, and close it with `q`, `Esc`, or
    `Tab`.
