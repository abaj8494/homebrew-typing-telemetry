# Typtel

Keystroke and mouse distance metrics for developers. Tracks every keypress including modifiers, escape sequences, and shortcuts.

![Menu Bar](img/menubar.png)

## Installation

```sh
brew tap abaj8494/typing-telemetry
brew install --cask typtel
```

### Accessibility Permissions

Typtel requires accessibility permissions to capture input events:

1. Open **System Settings** > **Privacy & Security** > **Accessibility**
2. Click **+** and navigate to `/Applications/Typtel.app`
3. Enable the checkbox for Typtel
4. Restart the app from the menu bar or via `open /Applications/Typtel.app`

> **IMPORTANT: After upgrading:** macOS requires you to re-grant permissions when the app binary changes. If Typtel won't launch after an upgrade, remove it from Accessibility and re-add it.

> **Note on uninstalling:** When you delete Typtel.app, macOS may not automatically remove it from the Accessibility list. This is a macOS limitation. To clean up, manually remove the entry from System Settings > Privacy & Security > Accessibility before or after deleting the app.

## CLI

The `typtel` command provides a terminal interface to your typing data.

```sh
typtel              # Interactive TUI dashboard
typtel today        # Today's keystroke count
typtel stats        # Detailed statistics
typtel test         # Typing speed test
typtel test -w 50   # Test with 50 words
typtel test -l au   # Use AU English spelling (saved as default)
```

### Typing Test

| Key      | Action             |
|----------|-------------------|
| `tab`    | Restart with new words |
| `esc`    | Options menu       |
| `enter`  | Start new test     |
| `ctrl+c` | Quit               |

Options include layout emulation, live WPM display, test length, uppercase, punctuation, and pace caret.

## Menu Bar

Click the menu bar icon to view:

- Daily and weekly keystroke/word/click counts
- Mouse distance traveled
- Charts and heatmaps
- Stillness leaderboard (least mouse movement)
- Settings

### Charts

View detailed statistics and activity heatmaps via **View Charts** in the menu bar.

- Change the **Distance Unit** dropdown to update all mouse distance displays (feet, car lengths, or frisbee fields)
- Enable **Key Types** in Settings to see a breakdown of letters (A-Z), modifier keys, and special characters

![Statistics](img/charts-html.png)

![Activity Heatmap](img/hourly-week.png)

## Inertia

Inertia provides accelerating key repeat. When enabled, held keys repeat at increasing speeds based on an acceleration table derived from [accelerated-jk.nvim](https://github.com/rainbowhxch/accelerated-jk.nvim).

Toggle and configure via **Settings** > **Inertia Settings** in the menu bar:

| Setting           | Options                          |
|-------------------|----------------------------------|
| Enable/Disable    | Toggle inertia on or off         |
| Max Speed         | Ultra Fast (140/s), Very Fast (125/s), Fast (83/s), Medium (50/s), Slow (20/s) |
| Threshold         | 100ms - 350ms before acceleration |
| Acceleration Rate | 0.25x - 2.0x multiplier          |

Double-tap Shift to reset acceleration to base speed.

## reMarkable Connection

Typtel can optionally accept keystroke aggregates from an **external device** —
for example a reMarkable Paper Pro with a Type Folio — and store them as a
*separate* feed alongside your Mac's stats. **This is entirely opt-in and
disabled by default.** A fresh install (or a `brew upgrade`) never opens a port
or talks to any device until you explicitly run `typtel devices enable`. If you
don't have a device to feed it, you can ignore this section completely.

The transport is a token-gated HTTP listener bound to **loopback only**
(`127.0.0.1:8889`), reached from the device over your private **Tailscale**
tailnet. Device stats never mix into your Mac totals — they're queryable on
their own (`typtel today --device <id>`) and surface as an optional `"devices"`
block in `typtel stats --json`.

### On the Mac (this app)

1. Enable the ingest API and grab the bearer token:

   ```sh
   typtel devices enable        # generates a token, prints it + the bind addr
   ```

   The token is also printable later with `typtel devices token`
   (`--rotate` to regenerate). Toggling enable/disable requires a **menubar
   restart** to take effect.

2. Publish the loopback listener to your tailnet with `tailscale serve` (raw
   TCP passthrough — keeps the device on plain HTTP, no TLS/MagicDNS needed):

   ```sh
   tailscale serve --bg --tcp 8889 tcp://127.0.0.1:8889
   tailscale serve status       # verify; reset with: tailscale serve --tcp=8889 off
   ```

   Binding loopback + `tailscale serve` is deliberate: on macOS the Tailscale
   client won't deliver inbound tailnet connections to a listener on the utun
   IP, and this keeps the port off your LAN. The bearer token is the auth
   boundary.

### On the device

The device PUTs **absolute** daily totals (not deltas, so a retried PUT over a
flaky link can never double-count) to:

```
PUT http://<your-mac-tailnet-ip>:8889/v1/devices/<id>/days/<YYYY-MM-DD>
Authorization: Bearer <token>
{ "keystrokes": …, "letters": …, "modifiers": …, "special": …,
  "words": …, "active_ms": … }
```

Probe liveness (no auth) with `GET /v1/health`. The device classifies its own
keys and sends pre-aggregated counts — Typtel stores them as opaque totals and
never re-classifies them.

> **reMarkable gotcha:** the tablet's `tailscaled` runs in userspace-networking
> mode (no `/dev/net/tun`), so its own processes can't open a socket directly to
> a `100.x` peer. The device must send its PUTs through tailscaled's netstack
> (an `--outbound-http-proxy-listen` proxy, or `tailscale nc`), not a plain
> `curl`/`requests.put`. That's configured on the device side.

### Inspecting and removing device data

```sh
typtel devices               # list registered devices + last-seen
typtel devices show <id>     # recent days for a device (add --json)
typtel devices forget <id>   # delete a device and all its recorded days
typtel devices disable       # turn the listener off (restart menubar to apply)
```

## Data Storage

All data is stored locally in `~/.local/share/typtel/`:

- `typtel.db` - SQLite database
- `logs/` - Application logs

No data is sent externally.

## Testing

Run the test suite:

```sh
make test
```

### Test Coverage

| Package | Coverage |
|---------|----------|
| pkg/stats | 100% |
| internal/storage | 78.6% |
| internal/tui | 67.5% |
| internal/mousetracker | 21.2% |
| internal/inertia | 11.9% |
| cmd/typtel-menubar | 7.6% |
| cmd/typtel | 6.5% |

Generate an HTML coverage report:

```sh
make test-coverage
```

This creates `coverage.html` with a detailed breakdown by package and function.

## Updating

```sh
brew update && brew upgrade --cask typtel
```

## Uninstalling

```sh
brew uninstall --cask typtel
rm -rf ~/.local/share/typtel  # Optional: remove data
```

## License

MIT
