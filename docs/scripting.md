# Window managers & scripting

typtel was built so that the tray menu is *optional*. Everything the tray does —
read today's keystrokes, toggle inertia, change the max-speed cap — is also a
plain shell command. If you run i3, sway, xmonad, or bspwm with polybar /
i3blocks / xmobar / waybar and have no system tray (or don't want to fight
StatusNotifier quirks), this is the recommended way to integrate typtel:

- **read** stats with `typtel today` / `typtel stats --json` for your bar, and
- **control** inertia with `typtel inertia …` bound to keys.

## The idea: a scriptable control surface

The `typtel-tray` daemon does the actual capturing and runs inertia. But it
also **watches its own settings** on a 2-second ticker and applies any change it
finds — including changes made out-of-band by the `typtel inertia …` CLI from a
keybinding. So you never restart the daemon: flip a setting from the shell and
the running daemon picks it up **live within ~2 seconds**.

```
keybind ──► typtel inertia toggle ──► writes settings (SQLite)
                                          │
            typtel-tray (2s ticker) ◄─────┘  applies live, no restart
```

That makes the whole CLI safe to drive from keybindings and bar polling loops.
You do not need the tray icon visible — you just need the daemon process
running.

!!! note
    If the daemon isn't running, `typtel inertia …` still persists the change;
    it simply takes effect the next time the daemon starts. Writes are durable
    either way.

## Reading stats

### `typtel today` — a bare number

`typtel today` prints **only** today's keystroke count and a newline. That's
deliberate: it drops straight into a status bar with no parsing.

```sh
$ typtel today
18423
```

### `typtel stats --json` — the full picture

```sh
$ typtel stats --json
```

```json
{
  "today": {
    "date": "2026-07-01",
    "keystrokes": 18423,
    "words": 2911,
    "letters": 14002,
    "modifiers": 1840,
    "special": 2581,
    "mouse_clicks": 412,
    "mouse_distance_px": 184022.5,
    "mouse_distance_m": 46.74,
    "active_hours": 6,
    "avg_wpm": 71.4
  },
  "week": [
    { "date": "2026-06-25", "keystrokes": 12001, "words": 1900 }
  ],
  "week_totals":   { "date": "", "keystrokes": 98230, "words": 15110 },
  "week_averages": { "keystrokes": 14032.8, "words": 2158.5 },
  "speed": {
    "avg_wpm": { "day": 71.4, "week": 68.0, "month": 66.2, "year": 64.9, "all": 63.1 },
    "fastest": { "burst_wpm": 142.0, "window_wpm": 121.3, "minute_wpm": 98.7 }
  }
}
```

`typtel today --json` returns just the `today` object above (same keys).

Extract individual fields with [`jq`](https://stedolan.github.io/jq/):

```sh
typtel stats --json | jq -r '.today.words'              # 2911
typtel stats --json | jq -r '.speed.avg_wpm.day'        # 71.4
typtel stats --json | jq -r '.speed.fastest.burst_wpm'  # 142
```

### `typtel inertia status --json` — inertia state

```sh
$ typtel inertia status --json
```

```json
{
  "enabled": true,
  "max_speed": "very_fast",
  "threshold": 200,
  "accel_rate": 1
}
```

```sh
typtel inertia status --json | jq -r .max_speed   # very_fast
typtel inertia status --json | jq -r .enabled     # true
```

`typtel inertia` (or `typtel inertia status`) with no `--json` prints a
human-readable block instead.

## Controlling inertia

[Inertia](inertia.md) is accelerating key-repeat — explained in full on its own
page. Here are the knobs you can drive from the shell:

```sh
typtel inertia status         # show current settings (human-readable)
typtel inertia status --json  # ...as JSON
typtel inertia on             # enable
typtel inertia off            # disable
typtel inertia toggle         # flip on/off — ideal for a single keybind

# Max-speed cap (one of these six names):
typtel inertia speed ultra_fast    # ~140 keys/s
typtel inertia speed very_fast     # ~125 keys/s
typtel inertia speed pretty_fast   # ~100 keys/s
typtel inertia speed fast          # ~83 keys/s
typtel inertia speed medium        # ~50 keys/s
typtel inertia speed slow          # ~20 keys/s

typtel inertia threshold 200       # ms held before acceleration kicks in
typtel inertia accel 1.0           # acceleration-rate multiplier
```

`speed` rejects anything outside the six names above; `threshold` wants a
positive integer (ms); `accel` wants a positive number. Each command prints the
resulting settings on success. See [inertia](inertia.md) for what the values
actually do to repeat behaviour, and the [CLI reference](reference/cli.md) for
the complete command list.

## Worked examples

### i3 (`~/.config/i3/config`)

```ini
# Toggle inertia on/off
bindsym $mod+i exec --no-startup-id typtel inertia toggle

# Jump straight to a fast cap
bindsym $mod+Shift+i exec --no-startup-id typtel inertia speed very_fast
```

### sway (`~/.config/sway/config`)

Same `bindsym` syntax as i3:

```ini
bindsym $mod+i exec typtel inertia toggle
bindsym $mod+Shift+i exec typtel inertia speed very_fast
```

### xmonad (`xmonad.hs`)

Using `XMonad.Util.EZConfig`'s `additionalKeysP`:

```haskell
import XMonad.Util.EZConfig (additionalKeysP)

main :: IO ()
main = xmonad $ def `additionalKeysP`
  [ ("M-i",   spawn "typtel inertia toggle")
  , ("M-S-i", spawn "typtel inertia speed very_fast")
  ]
```

### polybar (`~/.config/polybar/config.ini`)

A keystroke-count module and an inertia-state module:

```ini
[module/typing]
type = custom/script
exec = typtel today
interval = 5
label = ⌨ %output%

[module/inertia]
type = custom/script
exec = typtel inertia status --json | jq -r .max_speed
interval = 5
label = » %output%
; click to toggle inertia
click-left = typtel inertia toggle
```

Then add `typing` and `inertia` to a bar's `modules-right`.

### i3blocks (`~/.config/i3blocks/config`)

```ini
[typing]
command=typtel today
interval=5
label=⌨ 

[inertia]
command=typtel inertia status --json | jq -r .max_speed
interval=5
label=» 
```

### xmobar (`~/.xmobarrc`)

```haskell
Config
  { commands =
      [ Run Com "typtel" ["today"] "typtel" 50   -- refresh every 5s (tenths)
      ]
  , template = "%typtel% keys | %StdinReader%"
  }
```

`Run Com "typtel" ["today"] "typtel" 50` runs `typtel today`, aliases the
output as `%typtel%`, and refreshes every 50 tenths-of-a-second (5s).

### rofi / dmenu — pick a max speed

Pipe the six speed names into a menu and apply the choice:

```sh
#!/bin/sh
choice=$(printf '%s\n' ultra_fast very_fast pretty_fast fast medium slow \
  | rofi -dmenu -p "inertia speed")
[ -n "$choice" ] && typtel inertia speed "$choice"
```

Swap `rofi -dmenu` for `dmenu` if that's your launcher. Bind the script to a
key and the new cap takes effect within ~2 seconds.

## Why this path on minimal WMs

On bare i3 / sway / bspwm there's often no system tray at all, and the
StatusNotifier/DBusMenu hosts that *do* exist behave inconsistently across
panels (nested submenus that won't open, icons that won't render, click events
that don't fire). Rather than fight that per-panel, drive typtel from the shell:
your bar polls `typtel today` / `typtel … --json`, your keybindings call
`typtel inertia …`, and the running daemon applies changes live. It's the
most reliable integration on a minimal window manager.
