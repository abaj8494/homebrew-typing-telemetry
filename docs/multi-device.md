# Multi-device feed

typtel can act as a **host** that receives daily typing aggregates from other
**devices** and shows a single combined total. A Mac running the menu-bar app
collects its own keystrokes locally; on top of that it can ingest the daily
totals that your other machines (a Linux laptop, a second Mac, even a
reMarkable tablet) push to it, and display the sum.

This entire feature is **opt-in and off by default**. A fresh install — or a
`brew upgrade` — opens no port and pushes nowhere until you explicitly run
`typtel devices enable` (to receive) or `typtel push enable` (to send). Device
stats land in dedicated `device_*` tables and are **never mixed into the host's
own daily totals**: the host's local capture path (`daily_summary`) is
untouched, and device feeds are always queryable as a separate source.

!!! note "Single device? Skip this."
    If you only run typtel on one machine, none of this applies to you. Leave
    everything at its default and ignore this page entirely.

## The two roles

| Role | Command family | What it does |
| --- | --- | --- |
| **Host** | `typtel devices …` | Runs the ingest listener; receives and stores device feeds; shows the combined total. |
| **Device** | `typtel push …` | Sends *this* machine's daily totals to a host. |

A machine can be both, but the commands are distinct: `devices` is host-side,
`push` is device-side.

## Set up the host

The host is typically the Mac running the [menu-bar app](macos.md).

### 1. Enable the ingest API

```sh
typtel devices enable
```

This flips the ingest setting on and, if one does not already exist, generates
a random bearer token (16 random bytes, 32 hex characters). It prints the token
and the bind address.

```sh
typtel devices token            # print the current token again
typtel devices token --rotate   # regenerate it (then update every device)
```

### 2. Expose the listener over Tailscale

The ingest listener binds **loopback only** — `127.0.0.1:8889`. It is not
reachable from your LAN or the internet. Publish it to your private Tailscale
tailnet with a raw-TCP `serve` (no TLS or MagicDNS needed; the device stays on
plain HTTP):

```sh
tailscale serve --bg --tcp 8889 tcp://127.0.0.1:8889
tailscale serve status                 # verify
tailscale serve --tcp=8889 off         # tear it down later
```

Binding loopback plus `tailscale serve` is deliberate: on macOS the Tailscale
client will not deliver inbound tailnet connections to a listener on the utun
IP, and this keeps the port off your LAN.

!!! warning "The bearer token is the auth boundary"
    Behind `tailscale serve`, every request arrives with a **loopback source
    IP** (`127.0.0.1`) — typtel cannot distinguish tailnet peers by address.
    The bearer token is therefore the *only* thing gating ingest. Keep it
    secret, and rotate it with `typtel devices token --rotate` if it leaks.

### 3. Restart the menu-bar app

The enable/disable setting is read at startup, so **restart the menu-bar app**
for the listener to actually come up (or go down).

## Set up a device

On any machine that runs typtel (Linux or Mac), point its built-in push client
at the host — no custom client needed.

```sh
typtel push enable \
  --url http://<host-tailnet-ip>:8889 \
  --token <token> \
  --id <id> \
  --name "<Name>"
```

- `--url` — the host's tailnet base URL (the API paths are appended internally).
- `--token` — the bearer token from `typtel devices token` on the host.
- `--id` — this device's id; must match `[a-z0-9-]{1,32}`.
- `--name` — optional friendly name shown on the host (sent as `?name=`).

Send one push immediately to confirm the host is reachable (this runs a
`GET /v1/health` probe first, then a PUT; it ignores the enabled flag and lets
flags override stored config):

```sh
typtel push now
```

Then **restart the daemon** (`typtel-tray` on Linux, the menu-bar app on macOS)
so the background push loop starts.

### How pushing works

The push loop PUTs **absolute** daily totals — never deltas — roughly every
**45 seconds**. Because the host stores them INSERT-OR-REPLACE, a retried push
**never double-counts** and a missed push is corrected by the next one. When
the local date rolls over, the loop flushes the day that just ended one last
time before moving to the new day, so final totals are not stranded.

Manage the device side with:

```sh
typtel push status    # show config (token masked)
typtel push disable   # stop pushing; stored host/token/id are kept
```

## Viewing combined stats on the host

Once devices report, the host surfaces them in two places:

- **Menu bar.** Devices appear under the **📱 Devices** entry in the macOS
  [menu bar](macos.md). Clicking the menu-bar icon shows the **orange combined
  SUM** across all devices.
- **CLI.** Inspect feeds without the GUI:

```sh
typtel devices              # list registered devices + today's counts
typtel devices show <id>    # recent days for one device (table)
typtel devices show <id> --json
typtel devices forget <id>  # delete a device and all its recorded days
```

See the [CLI reference](reference/cli.md) for the full command set.

## The raw HTTP API

For a device that does **not** run typtel — a reMarkable tablet, a script, an
embedded gadget — push aggregates directly to the ingest API. All counts are
absolute daily totals and must be non-negative integers.

### Upload a day

```
PUT /v1/devices/{id}/days/{YYYY-MM-DD}
Authorization: Bearer <token>
Content-Type: application/json

{
  "keystrokes": 0,
  "letters":    0,
  "modifiers":  0,
  "special":    0,
  "words":      0,
  "active_ms":  0
}
```

- `{id}` must match `[a-z0-9-]{1,32}`; `{YYYY-MM-DD}` is the device-local date.
- An optional `?name=<friendly name>` query sets the display name (≤ 64 chars,
  no control characters). Omitting it never clears a previously-set name.
- The device classifies its own keys and sends pre-aggregated counts; typtel
  stores them as opaque totals and never re-classifies them.
- A successful upload returns **`204 No Content`**. First contact from an
  unknown id self-registers the device.

### Liveness probe

```
GET /v1/health
```

Unauthenticated — intentionally, so a device can confirm reachability before it
holds a token. Returns `200` with `{"ok": true, "version": "…"}`.

### Other endpoints

All of these require the bearer token:

| Method & path | Purpose |
| --- | --- |
| `GET /v1/devices` | List registered devices. |
| `GET /v1/devices/{id}/days` | A device's days (optional `?since=YYYY-MM-DD`). |
| `GET /v1/devices/{id}/days/{date}` | One device-day's counts. |
| `DELETE /v1/devices/{id}/days/{date}` | Erase one device-day. |
| `DELETE /v1/devices/{id}` | Forget a device and all its days. |
| `GET /v1/self/days` | The host's *own* daily aggregates, so a device can pull them back. |

!!! warning "reMarkable gotcha"
    On a reMarkable tablet, `tailscaled` runs in **userspace-networking mode**
    (no `/dev/net/tun`), so the tablet's own processes cannot open a socket
    directly to a `100.x` tailnet peer. PUTs must go **through tailscaled's
    proxy** — an `--outbound-http-proxy-listen` proxy or `tailscale nc` — and
    **not** a plain `curl` / `requests.put`. That routing is configured on the
    device side.
