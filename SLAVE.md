# SLAVE Log

A record of changes made by Claude Code sub-agents acting on behalf of the
human operator. Newest entries on top.

---

## 2026-05-21 — Machine-readable JSON surface for `today` / `stats`

### What

Added a `--json` flag to `typtel today` and `typtel stats` so other tools
on the same machine can consume typing-telemetry data without parsing the
TUI/text output. Implementation lives in a new file:

- `cmd/typtel/json_output.go` — schema definitions, builders, JSON runners.
- `cmd/typtel/main.go` — wires the `--json` boolean flag onto the two
  existing cobra subcommands; no new subcommands were introduced.

No storage schema changes. The new code reads through the existing
`internal/storage.Store` API (`GetTodayStats`, `GetTodayMouseStats`,
`GetHourlyStats`, `GetWeekStats`).

### Why

Sister project [`macos-watchdog`](https://github.com/abaj8494/macos-watchdog)
optionally surfaces today's typing volume in its CLI `summary` and in its
local dashboard. It shells out to `typtel today --json` when the binary
is present on PATH and silently no-ops otherwise. Watchdog does NOT
persist typtel data — every read is on-demand — so typing-telemetry
remains the single source of truth and retains full control of retention.

### JSON schemas

`typtel today --json`:

```json
{
  "date": "2026-05-21",
  "keystrokes": 12345,
  "words": 1500,
  "letters": 9000,
  "modifiers": 1000,
  "special": 2345,
  "mouse_clicks": 800,
  "mouse_distance_px": 178997.98,
  "mouse_distance_m": 45.47,
  "active_hours": 8
}
```

Notes:

- `date` is the local YYYY-MM-DD.
- `mouse_distance_px` is the raw lossless figure from storage; the metres
  conversion uses `DefaultPPI = 100` and `metersPerInch = 0.0254`.
- `active_hours` counts distinct hours today with at least one keystroke
  (a coarse activity proxy; finer resolution would need a new storage
  query).
- `letters` / `modifiers` / `special` come from the existing keycode
  classification (see `storage.ClassifyKeycode`).

`typtel stats --json`:

```json
{
  "today": { /* same shape as `today --json` */ },
  "week": [
    {"date": "2026-05-15", "keystrokes": 6264, "words": 997},
    ...
  ],
  "week_totals": {"date": "", "keystrokes": 63101, "words": 9962},
  "week_averages": {"keystrokes": 9014.4, "words": 1423.1}
}
```

`week` is chronological (oldest first) and always exactly 7 entries —
matching `Store.GetWeekStats()`.

### Stability contract

Field names use snake_case. The schemas are **additive only**: fields
may be added in future releases, but never renamed or removed without a
matched change on the watchdog side.

### Commits / branches in this repo

- Branch: `main`
- New commit: `feat: add --json output to typtel today and typtel stats`
  (no other modified files staged into this commit; unrelated working-tree
  changes were left alone for the human to commit separately).

### Files changed

- `cmd/typtel/main.go` — added `jsonOutput` flag var, flag registration,
  and `--json` branch in the two RunE closures.
- `cmd/typtel/json_output.go` — new file containing all schema and
  marshalling logic.
