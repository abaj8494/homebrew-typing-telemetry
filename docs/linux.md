# Linux (X11)

On Linux, typtel runs as **`typtel-tray`** — the counterpart to the macOS
menu-bar app. It is a [StatusNotifier](https://www.freedesktop.org/wiki/Specifications/StatusNotifierItem/)
tray icon backed by the **same SQLite store**, keystroke capture,
[inertia](inertia.md), and [charts](charts.md) as the Mac. The stats pipeline
(word counting, speed tracking, storage) is shared verbatim across both
platforms.

Tested on Kali/Debian.

!!! warning "X11 only — no Wayland"
    Capture and inertia both talk to an X server. Under a Wayland session there
    is no global `QueryKeymap` and no `xset` repeat control, so typtel cannot
    capture keystrokes. Log in to an **X11 / Xorg** session instead. When
    inertia is disabled (or on exit) typtel restores your original `xset`
    auto-repeat delay and rate.

## How capture works

Capture is **pure Go over X11** (via `github.com/jezek/xgb`, no C event
libraries). typtel polls the X server's global physical key-state bitmap with
`QueryKeymap` at ~125 Hz and reports each key's down-transition.

This means:

- **No root**, no `/dev/input` access, no membership in the `input` group.
- The only requirement is a reachable `$DISPLAY` — the same connection any X
  client makes. There is no equivalent of the macOS Accessibility (TCC) prompt.
- `QueryKeymap` reports physical key state regardless of which window is
  focused, and a held key keeps its bit set, so auto-repeat and inertia's
  synthetic repeats never double-count a press.

## Install a prebuilt binary

The Homebrew cask is **macOS-only**, but each [release](https://github.com/abaj8494/typing-telemetry/releases)
ships a Linux **amd64** tarball (`typtel-<version>-linux-amd64.tar.gz`) with both
binaries — no build needed:

Download the `typtel-<version>-linux-amd64.tar.gz` asset from the
[latest release](https://github.com/abaj8494/typing-telemetry/releases/latest),
then:

```sh
tar -xzf typtel-*-linux-amd64.tar.gz
mkdir -p ~/.local/bin && cp typtel typtel-tray ~/.local/bin/
~/.local/bin/typtel-tray &     # capture + inertia (tray icon if your DE provides one)
```

The tarball includes an `INSTALL.txt` with autostart and headless/CLI notes.

!!! note
    The prebuilt binary targets a recent glibc (Debian 12+/Kali/Ubuntu 22.04+)
    and is **amd64** only — on arm64 (e.g. a Raspberry Pi) or older glibc, build
    from source below. Inertia needs `xset` (`sudo apt install x11-xserver-utils`).

## Build from source

Alternatively, build the two binaries yourself. You need **Go** and a **C
toolchain** — `CGO_ENABLED=1` is required for the SQLite driver.

```sh
git clone https://github.com/abaj8494/typing-telemetry && cd typing-telemetry

# CLI: stats, typing test, charts
CGO_ENABLED=1 go build -o build/typtel      ./cmd/typtel

# tray daemon: capture + inertia
CGO_ENABLED=1 go build -o build/typtel-tray ./cmd/typtel-tray

# run it (tray icon shows today's keystrokes / words / WPM)
./build/typtel-tray &
```

Data lives in `~/.local/share/typtel/typtel.db`, exactly as on macOS — nothing
leaves the machine.

## The tray menu

`typtel-tray` puts a StatusNotifier icon in your panel (XFCE, KDE, or GNOME with
an appindicator extension). The menu offers:

| Item | What it does |
|------|--------------|
| **Keystrokes / Words / Avg WPM / Fastest WPM** | Live, display-only stats for today, refreshed every 2s. |
| **View Charts…** | Generates the rich dashboard (heatmap, key-type breakdown, streaks, peaks) and opens it in your browser via `xdg-open`. Same dashboard as the Mac and as `typtel v`. |
| **Enable Inertia** | Toggles the accelerating key-repeat on/off. |
| **Inertia · Max Speed** | Submenu: top repeat-speed cap (Ultra Fast → Slow). |
| **Inertia · Threshold** | Submenu: delay before acceleration starts (100–350 ms). |
| **Inertia · Acceleration** | Submenu: how quickly the rate ramps up (0.25x–2.0x). |
| **Show Key Types in charts** | Toggles the letters/modifiers/special breakdown in the dashboard. |
| **Quit typtel** | Stops capture, restores `xset` settings, and exits. |

!!! note "Inertia needs `xset`"
    [Inertia](inertia.md) on Linux drives the **X server's own** auto-repeat and
    ramps its rate while a key is held, so repeats stop the instant you release
    — no event injection. The rate/delay are applied with `xset` (package
    `x11-xserver-utils`). If `xset` is missing, inertia silently does nothing.
    Your original repeat settings are saved on enable and **restored when
    disabled** or on exit.

Inertia settings changed out-of-band — e.g. via `typtel inertia toggle` bound to
a window-manager keybind — are picked up by a running tray within ~2s, with its
checkmarks re-synced. No restart needed.

## Autostart on login

Drop a Desktop Entry at `~/.config/autostart/typtel-tray.desktop` so the tray
starts with your X session. Adjust the `Exec` path to wherever you installed the
binary (e.g. `~/.local/bin/`):

```ini
[Desktop Entry]
Type=Application
Name=typtel
Comment=Typing telemetry tray (X11)
Exec=/home/<you>/.local/bin/typtel-tray
Terminal=false
Categories=Utility;
X-GNOME-Autostart-enabled=true
```

## No tray? Drive it from the shell

If you run i3, xmonad, sway, or any setup without a system tray, you can read
stats for your status bar and control inertia from keybindings without the tray
at all. See [Window managers & scripting](scripting.md).

## See also

- [Inertia](inertia.md) — accelerating key-repeat, and how the Linux `xset`
  engine differs from the macOS synthetic-event approach.
- [Charts](charts.md) — the dashboard opened by **View Charts**.
- [Scripting](scripting.md) — `typtel` CLI for bars and WM keybinds.
- [Multi-device feed](multi-device.md) — push this box's totals to another
  machine running typtel.
