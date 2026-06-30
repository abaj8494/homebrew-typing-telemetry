# Settings reference

Every typtel preference is a single row in the `settings` table
(`key TEXT PRIMARY KEY, value TEXT`) of the SQLite database at
`~/.local/share/typtel/typtel.db`. Values are always stored as text; booleans
use the canonical strings `"true"`/`"false"` (the reader also accepts `"1"`),
integers and floats are formatted as decimal strings (floats with two decimal
places).

!!! note "How settings are changed"
    You rarely edit these keys by hand. They are written by:

    - the **macOS menu-bar Settings** window (display toggles, mouse tracking,
      distance unit, key-type breakdown, word-count strictness, inertia,
      odometer hotkey, typing-test theme/language);
    - the **Linux `typtel-tray`** menu (the same toggles available on that
      platform);
    - the **CLI**: [`typtel inertia …`](cli.md#inertia), [`typtel devices …`](cli.md#devices),
      and [`typtel push …`](cli.md#push);
    - the **typing-test TUI**, which persists personal-best / average / count
      and theme/language as you play.

    Absent keys fall back to the defaults listed below — booleans default to
    `false` unless a getter says otherwise.

See also: [Inertia](../inertia.md) and [Multi-device](../multi-device.md).

## Display (menu bar / tray)

These toggle what the menu-bar / tray title shows. Read via
`GetMenubarSettings()`; written by `SaveMenubarSettings()`.

| Key | Meaning | Type | Default | Values / notes |
|-----|---------|------|---------|----------------|
| `menubar_show_keystrokes` | Show today's keystroke count in the title | bool | `true` | `true` / `false` |
| `menubar_show_words` | Show today's word count in the title | bool | `true` | `true` / `false` |
| `menubar_show_clicks` | Show today's mouse-click count | bool | `false` | `true` / `false` |
| `menubar_show_distance` | Show today's mouse travel distance | bool | `false` | `true` / `false` |
| `show_key_types` | Show the letter / modifier / special breakdown | bool | `false` | `true` / `false` (`IsShowKeyTypesEnabled`) |

## Mouse

| Key | Meaning | Type | Default | Values / notes |
|-----|---------|------|---------|----------------|
| `mouse_tracking_enabled` | Record mouse movement and clicks | bool | `true` | Any value other than `"false"` is treated as enabled (`IsMouseTrackingEnabled`) |
| `distance_unit` | Unit used when rendering mouse travel distance | enum | `feet` | `feet` (feet/miles, default), `cars` (avg car length ~15 ft), `frisbee` (ultimate frisbee field ~330 ft) |

## Word counting

Per-app word-count filtering. Disabled by default; when off, the normal
keystroke heuristics still run — only the per-app allowlist filter is gated.

| Key | Meaning | Type | Default | Values / notes |
|-----|---------|------|---------|----------------|
| `word_count_strict_mode` | Gate word counting on the per-app allowlist | bool | `false` | `true` / `false` (`IsStrictWordCountEnabled`) |
| `word_count_app_allowlist` | Bundle IDs allowed to count words | list | empty | Newline-separated bundle IDs, returned in arrival order |
| `word_count_apps_seen` | Bundle IDs the daemon has observed (drives the allowlist UI) | list | empty | Newline-separated; appended automatically as apps are seen |

## Inertia

Accelerating synthetic key-repeat. Read as a struct via `GetInertiaSettings()`.
Configurable from the CLI ([`typtel inertia …`](cli.md#inertia)) and the
tray/menu. See [Inertia](../inertia.md) for behaviour.

| Key | Meaning | Type | Default | Values / notes |
|-----|---------|------|---------|----------------|
| `inertia_enabled` | Master on/off for inertia | bool | `false` | `true` / `false` |
| `inertia_max_speed` | Repeat-rate cap | enum | `fast` | `ultra_fast` (~140 keys/s), `very_fast` (~125), `pretty_fast` (~100), `fast` (~83), `medium` (~50), `slow` (~20) |
| `inertia_threshold` | Milliseconds held before acceleration begins | int (ms) | `200` | Positive integer; non-positive / unparseable values are ignored and the default is kept |
| `inertia_accel_rate` | Acceleration-rate multiplier | float | `1.0` | Positive number (stored with 2 decimals); non-positive / unparseable values ignored |

## Typing test

Persisted by the typing-test TUI and by `typtel test`. Personal-best, average
WPM, and test count are tracked both globally and per mode (the per-mode keys
append a `_mode_<words>_<punct|no_punct>` suffix to the three keys below).

| Key | Meaning | Type | Default | Values / notes |
|-----|---------|------|---------|----------------|
| `typing_test_pb` | Best WPM recorded | float | `0` | Highest WPM ever; per-mode variants suffixed with the mode key |
| `typing_test_avg_wpm` | Running (weighted) average WPM | float | `50.0` | Updated after each completed test |
| `typing_test_count` | Number of completed tests | int | `0` | Used to weight the running average |
| `typing_test_theme` | Color theme for the test UI | string | `default` | Theme name |
| `typing_test_language` | Spelling variant for generated words | enum | `us` | `us` (US English), `au` (AU English); `typtel test -l` saves this |
| `typing_test_custom_texts` | Saved custom passages | list | empty | Newline-separated text blocks |

## Device ingest (host side)

Opt-in inbound HTTP API (v1.4142) that accepts absolute daily aggregates pushed
from external devices over a Tailscale-bound loopback listener. Off by default.
Configured with [`typtel devices …`](cli.md#devices). See
[Multi-device](../multi-device.md).

| Key | Meaning | Type | Default | Values / notes |
|-----|---------|------|---------|----------------|
| `device_ingest_enabled` | Run the ingest API in the daemon | bool | `false` | `true` / `false`; `typtel devices enable`/`disable` toggle it (restart the daemon to apply) |
| `device_ingest_token` | Bearer token devices must present | string | empty | 32 hex chars (16 random bytes); auto-generated on enable, printed/rotated via `typtel devices token [--rotate]` |
| `device_ingest_bind_addr` | Listener address | string | `127.0.0.1:8889` | Loopback by default; exposed to the tailnet via `tailscale serve` |
| `device_ingest_peer_allowlist` | Optional peer IP allowlist | list | empty | Behind `tailscale serve` the API sees `RemoteAddr 127.0.0.1`, so keep this empty — the token is the auth boundary |

## Push (device side)

Opt-in **outbound** push (v1.5.0) of this machine's own daily aggregates to a
host typtel's ingest API — the counterpart to the ingest keys above. Off by
default. Configured with [`typtel push …`](cli.md#push). Loaded via
`push.LoadConfig`.

| Key | Meaning | Type | Default | Values / notes |
|-----|---------|------|---------|----------------|
| `push_enabled` | Push this machine's stats to a host | bool | `false` | `true` / `false`; `typtel push enable`/`disable` toggle it (restart the daemon to apply) |
| `push_base_url` | Host ingest base URL | string | empty | e.g. `http://100.93.238.15:8889` |
| `push_token` | Bearer token issued by the host | string | empty | From the host's `typtel devices token` |
| `push_device_id` | This device's id on the host | string | empty | Must match `[a-z0-9-]{1,32}` |
| `push_device_name` | Friendly name shown on the host | string | empty | Optional |

## Odometer

| Key | Meaning | Type | Default | Values / notes |
|-----|---------|------|---------|----------------|
| `odometer_hotkey` | Global hotkey that starts/stops the activity odometer | string | `cmd+ctrl+o` | Hotkey combo string (`GetOdometerHotkey` / `SetOdometerHotkey`) |

## Internal / housekeeping

Not user-facing, but stored in the same table:

| Key | Meaning | Type | Default | Values / notes |
|-----|---------|------|---------|----------------|
| `speed_backfill_done` | Marks the one-time active-time backfill as complete | bool | unset | Set to `"1"` after `BackfillActiveTime` reconstructs historical active typing time on first v1.4 launch |
