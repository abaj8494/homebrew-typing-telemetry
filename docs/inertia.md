# Inertia

**Inertia** is typtel's accelerating key-repeat. Hold a key down and, instead of
the operating system's flat repeat rate, the repeat rate *ramps up* the longer
you hold — slow at first, then faster and faster along an acceleration curve, up
to a configurable ceiling. It is inspired by
[accelerated-jk.nvim](https://github.com/rainbowhxch/accelerated-jk.nvim): the
same idea of "the longer you move, the faster you go," applied globally to every
key rather than just `j`/`k` inside an editor.

It is most useful for repeated motion — scrolling through a buffer with the
arrow keys, holding Backspace to clear a line, walking the cursor across a long
path — where a fixed repeat rate is either frustratingly slow or overshoots.

!!! info "Off by default"
    Inertia does nothing until you turn it on. typtel ships with it disabled,
    and every control below is a no-op until you enable it (see
    [How to control it](#how-to-control-it)).

## The acceleration curve

While a key is held, typtel counts how long it has been repeating and maps that
count onto a curve of speed *steps*. The curve is identical on both platforms:

- A **base repeat interval** of `35 ms` (the slow starting rate).
- A set of key-count **step thresholds** — `{7, 12, 17, 21, 24, 26, 28, 30}` —
  that the running count is compared against. Each threshold crossed advances
  the speed step, and the repeat interval is `35 ms ÷ step`, so the interval
  shrinks (the rate climbs) as you hold.
- A **Max Speed cap** that floors the interval: once the curve reaches the cap,
  it stops getting faster and the key just repeats at that top rate.

The **Acceleration Rate** multiplier scales the running count *before* it is
compared against the thresholds, so a higher rate reaches each step — and the
cap — with fewer repeats, while a lower rate makes you hold longer to climb.

## Parameters

| Parameter | Values | Default | Effect |
|-----------|--------|---------|--------|
| **Max Speed** | `ultra_fast` / `very_fast` / `pretty_fast` / `fast` / `medium` / `slow` | `fast` | Top-speed cap — the fastest the repeat can get (see table below). |
| **Threshold** | ms held before acceleration starts (e.g. `100` / `150` / `200` / `250` / `350`) | `200` | How long you must hold a key before inertia kicks in at all. |
| **Acceleration Rate** | `0.25x` – `2.0x` | `1.0x` | How quickly you climb the curve toward the cap. Higher = reach top speed sooner. |

### Max Speed caps

Each cap is a floor on the repeat interval; the approximate top rate is its
reciprocal. The caps are deliberately bounded by what terminals and editors can
realistically keep up with.

| Max Speed | Interval floor | Approx. rate |
|-----------|---------------|--------------|
| `ultra_fast` | 7 ms | ~140 keys/s |
| `very_fast` | 8 ms | ~125 keys/s |
| `pretty_fast` | 10 ms | ~100 keys/s |
| `fast` *(default)* | 12 ms | ~83 keys/s |
| `medium` | 20 ms | ~50 keys/s |
| `slow` | 50 ms | ~20 keys/s |

## How it differs by platform

The *behaviour* is the same curve, caps, and parameters on both platforms — but
the *mechanism* is fundamentally different, because the two operating systems
give typtel very different control over the key-repeat stream.

!!! abstract "macOS — suppress and synthesize"
    On macOS a **CGEventTap** intercepts keyboard events. typtel **suppresses the
    OS auto-repeat** for the held key and **posts its own accelerating synthetic
    key-down events** at the intervals dictated by the curve. Because typtel owns
    the repeat loop, it controls each individual keystroke. Double-tapping
    **Shift** resets the acceleration back to the base rate, so you can start a
    fresh slow ramp without releasing.

!!! abstract "Linux / X11 — drive the server's own auto-repeat"
    On X11 typtel **cannot suppress events** (a passive monitor can't swallow
    them), and an injected `KeyPress` on an already-held key is **de-duplicated
    by the X server**, so the macOS "synthesize key-downs" approach delivers
    nothing. Instead typtel drives the **X server's *own* auto-repeat** and ramps
    *its* rate along the same curve, applying each new rate with
    `xset r rate` (XKB controls). A `QueryKeymap` poller watches for the physical
    press and release: because the server owns the key state, the ramp
    **self-terminates the instant you release** — no injection, no release
    detection, no corrupted held-key state. typtel saves your original `xset`
    delay/rate on enable and **restores them when inertia is disabled or on
    exit**.

    This requires **`xset`** (the `x11-xserver-utils` package). If `xset` is
    missing, inertia silently does nothing — matching the macOS "fail quietly"
    behaviour. Wayland is unsupported; see [Linux](linux.md).

## How to control it

Inertia state lives in the shared SQLite store, so all three control surfaces
read and write the same [settings](reference/settings.md).

=== "macOS"
    Open the typtel menu bar (see [macOS](macos.md)) and use **⚙️ Settings →
    Inertia** to toggle it on and pick Max Speed, Threshold, and Acceleration.

=== "Linux"
    Use the [tray](linux.md) **Inertia** submenus — **Enable Inertia**,
    **Inertia · Max Speed**, **Inertia · Threshold**, and **Inertia ·
    Acceleration** — or the CLI below.

=== "CLI (cross-platform)"
    The `typtel inertia` subcommands work on both platforms and are scriptable —
    ideal for window-manager keybinds and status bars (see
    [scripting](scripting.md)):

    ```sh
    typtel inertia on            # enable
    typtel inertia off           # disable
    typtel inertia toggle        # flip on/off (bind to a key)
    typtel inertia speed fast    # ultra_fast|very_fast|pretty_fast|fast|medium|slow
    typtel inertia threshold 200 # ms before acceleration starts
    typtel inertia accel 1.0     # acceleration-rate multiplier (0.25–2.0)
    typtel inertia status        # human-readable settings
    typtel inertia status --json # machine-readable (for polybar/i3blocks/jq)
    ```

!!! tip "Live updates"
    A running typtel daemon applies CLI changes **live within ~2s** on Linux
    (the tray re-syncs its checkmarks); otherwise changes take effect the next
    time the daemon starts.

## See also

- [macOS](macos.md) — the menu-bar app and its Inertia settings.
- [Linux](linux.md) — the tray, X11 capture, and the `xset` requirement.
- [Scripting](scripting.md) — driving inertia from the shell and WM keybinds.
- [Settings reference](reference/settings.md) — the underlying stored values.
