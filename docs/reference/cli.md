# CLI reference

`typtel` is the command-line / TUI front end. It is pure Go (no CGO) and reads
the same SQLite database as the daemon (`~/.local/share/typtel/typtel.db`), so
it works alongside the macOS menu-bar app or the Linux `typtel-tray`.

Run `typtel help <command>` for built-in help on any command. Commands that
emit JSON pair well with `jq` — see [Scripting](../scripting.md). Device and
push commands are covered in depth under [Multi-device](../multi-device.md);
inertia under [Inertia](../inertia.md).

## Command overview

| Command | Aliases | Purpose |
|---------|---------|---------|
| `typtel` | — | Open the interactive dashboard (TUI) |
| `typtel today` | — | Today's keystroke count |
| `typtel stats` | — | Today + this week + typing speed |
| `typtel test` | — | Interactive typing-speed test |
| `typtel v` | `view`, `charts` | Open charts/heatmap in a browser |
| `typtel version` | `info` | Version information |
| `typtel devices` | — | Manage inbound external-device feeds (host side) |
| `typtel push` | — | Push this machine's stats to a host (device side) |
| `typtel inertia` | — | Inspect and control accelerating key-repeat |

---

### `typtel` (root — no args)

Opens the interactive Bubble Tea dashboard (TUI). From there you can launch the
typing test or the charts view.

```sh
typtel
```

---

### today

Print today's keystroke count. With no flags it emits a single bare integer,
suitable for status-bar scripts.

```text
typtel today [--json] [--device <id>]
```

| Flag | Description |
|------|-------------|
| `--json` | Emit the full breakdown (letters/modifiers/special/words) as JSON instead of a bare integer |
| `--device <id>` | Read an external **device's** today counts instead of this machine's (reads the `device_*` tables) |

```sh
typtel today                 # e.g. 18423
typtel today --json          # JSON document with the full breakdown
typtel today --device rm2    # the device "rm2"'s keystroke count today
```

---

### stats

Show today's and this week's totals plus typing speed (today / all-time average
and the fastest recorded pace). Runs a one-time historical active-time backfill
on first use so speed history is meaningful.

```text
typtel stats [--json] [--device <id>]
```

| Flag | Description |
|------|-------------|
| `--json` | Emit machine-readable JSON (includes the `speed` block) |
| `--device <id>` | Show the per-day table for an external **device** instead of this machine (equivalent to `typtel devices show <id>`) |

```sh
typtel stats                 # human-readable summary + WPM
typtel stats --json | jq .speed
typtel stats --device rm2    # per-day table for device "rm2"
```

---

### test

Start an interactive typing test to measure WPM and accuracy. In-test keys:
`tab` = new words, `esc` = options, `enter` = start, `ctrl+c` = quit.

```text
typtel test [-w|--words <n>] [-f|--file <path>] [-l|--language <variant>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-w`, `--words <n>` | `25` | Number of words in the test |
| `-f`, `--file <path>` | — | Path to a text file with words/passages to type |
| `-l`, `--language <variant>` | — | Spelling variant: `us` or `au`; the chosen value is **saved as the new default** (`typing_test_language`) |

```sh
typtel test                       # default 25-word test
typtel test -w 50                 # 50-word test
typtel test -f words.txt          # use a custom word list
typtel test -f passage.txt -w 100 # 100 words from a custom file
typtel test -l au                 # AU English spelling (persisted)
```

---

### v (aliases: view, charts)

Generate the charts/heatmap HTML and open it in the default browser (uses
`open` on macOS, `xdg-open` on Linux).

```text
typtel v
typtel view
typtel charts
```

```sh
typtel v        # generate and open charts.html
```

---

### version (alias: info)

Print version, homepage, and license.

```text
typtel version
typtel info
```

```sh
typtel version   # Typtel vX.Y …
```

---

### devices

**Host side.** Manage inbound feeds from external devices (e.g. a reMarkable
tablet) that PUT absolute daily aggregates to the opt-in, Tailscale-bound
ingest API. Device stats live in dedicated `device_*` tables and never mix into
this machine's `daily_summary`. With no subcommand, lists registered devices
and today's counts.

```text
typtel devices
typtel devices show <id> [--json]
typtel devices forget <id>
typtel devices enable
typtel devices disable
typtel devices token [--rotate]
```

#### `devices` (no subcommand)

List registered devices with today's KEYS/WORDS/MODS/SPECIAL and last-seen.

```sh
typtel devices
```

#### `devices show <id>`

Show the recent per-day table reported by a device
(DATE / KEYSTROKES / LETTERS / MODIFIERS / SPECIAL / WORDS / ACTIVE_MS).
`<id>` is required.

| Flag | Description |
|------|-------------|
| `--json` | Emit the days array as JSON |

```sh
typtel devices show rm2
typtel devices show rm2 --json | jq '.[].words'
```

#### `devices forget <id>`

Delete a device and all of its recorded days (prompts for confirmation).
`<id>` is required.

```sh
typtel devices forget rm2
```

#### `devices enable`

Enable the ingest API and generate a bearer token if none exists. Prints the
token and bind address. Restart the daemon to take effect. Sets
`device_ingest_enabled=true`.

```sh
typtel devices enable
```

#### `devices disable`

Disable the ingest API (`device_ingest_enabled=false`). Restart the daemon to
take effect.

```sh
typtel devices disable
```

#### `devices token`

Print the ingest bearer token (generating one if absent).

| Flag | Description |
|------|-------------|
| `--rotate` | Regenerate the token (invalidates the old one; update each device and restart the daemon) |

```sh
typtel devices token             # print the current token
typtel devices token --rotate    # generate a fresh token
```

---

### push

**Device side.** Outbound counterpart to `devices`: send *this* machine's daily
aggregates to a host typtel's ingest API over Tailscale, so the host can show a
combined cross-device total. OFF by default; a single-device user can ignore
it. With no subcommand, prints the current push status.

```text
typtel push
typtel push enable [--url <u>] [--token <t>] [--id <id>] [--name <n>]
typtel push disable
typtel push status
typtel push now    [--url <u>] [--token <t>] [--id <id>] [--name <n>]
```

The four flags are shared by `enable` and `now`:

| Flag | Description |
|------|-------------|
| `--url <u>` | Host base URL, e.g. `http://100.93.238.15:8889` |
| `--token <t>` | Bearer token from the host (`typtel devices token`) |
| `--id <id>` | This device's id; must match `[a-z0-9-]{1,32}` |
| `--name <n>` | Friendly name shown on the host (optional) |

#### `push` (no subcommand) / `push status`

Show the current push configuration with the token masked.

```sh
typtel push          # same as 'typtel push status'
typtel push status
```

#### `push enable`

Validate and persist the merged config (flags override stored values), then set
`push_enabled=true`. Restart the daemon to begin pushing.

```sh
typtel push enable --url http://100.93.238.15:8889 --token <t> --id laptop --name "Work Laptop"
```

#### `push disable`

Stop pushing (`push_enabled=false`). Stored host/token/id are kept; re-enable
with `typtel push enable`.

```sh
typtel push disable
```

#### `push now`

Push today's stats once immediately. Flags override stored config and this
ignores the enabled state — useful to confirm the host is reachable.

```sh
typtel push now
typtel push now --url http://100.93.238.15:8889 --token <t> --id laptop
```

---

### inertia

Inspect and control accelerating key-repeat from the shell — the scriptable
control surface for window-manager users (i3, xmonad, sway, …). A running
daemon applies changes live within a couple of seconds; otherwise they take
effect on next start. With no subcommand, prints status. See
[Inertia](../inertia.md).

```text
typtel inertia [--json]
typtel inertia status [--json]
typtel inertia on
typtel inertia off
typtel inertia toggle
typtel inertia speed <name>
typtel inertia threshold <ms>
typtel inertia accel <rate>
```

#### `inertia` (no subcommand) / `inertia status`

Show current settings: state, max speed, threshold, accel rate.

| Flag | Description |
|------|-------------|
| `--json` | Emit `{enabled, max_speed, threshold, accel_rate}` as JSON (for polybar/i3blocks/jq) |

```sh
typtel inertia                 # human-readable
typtel inertia status --json | jq .enabled
```

#### `inertia on` / `off` / `toggle`

Enable, disable, or flip inertia. `toggle` is handy bound to a key
(e.g. i3 `bindsym $mod+i exec typtel inertia toggle`).

```sh
typtel inertia on
typtel inertia off
typtel inertia toggle
```

#### `inertia speed <name>`

Set the repeat-rate cap. `<name>` is required and must be one of:
`ultra_fast` (~140 keys/s), `very_fast` (~125), `pretty_fast` (~100),
`fast` (~83), `medium` (~50), `slow` (~20).

```sh
typtel inertia speed fast
```

#### `inertia threshold <ms>`

Set the milliseconds a key is held before acceleration begins. `<ms>` must be a
positive integer.

```sh
typtel inertia threshold 200
```

#### `inertia accel <rate>`

Set the acceleration-rate multiplier. `<rate>` must be a positive number.

```sh
typtel inertia accel 1.0
```
