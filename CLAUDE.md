# CLAUDE.md

This file documents project conventions for AI assistants working in this
repository. It is paired with `SLAVE.md` — see "Collaboration" below.

## What this project is

**typing-telemetry** is a macOS menu-bar app that records keystroke and mouse
metrics locally to SQLite. Two binaries:

- `cmd/typtel-menubar` — the background daemon + systray UI (`fyne.io/systray`).
- `cmd/typtel` — a CLI / Bubble Tea TUI for browsing stats and running typing
  tests.

Data lives in `~/.local/share/typtel/typtel.db`. **Nothing is sent off the
machine.** Accessibility permission is required for the CGEventTap that
captures keystrokes.

## Layout

```
cmd/typtel/                 CLI + TUI
cmd/typtel-menubar/         Menu-bar daemon (CGO + Objective-C for Cocoa)
internal/keylogger/         CGEventTap wrapper (darwin only)
internal/mousetracker/      Mouse position + click tracker
internal/inertia/           Synthetic key-repeat acceleration
internal/wordcounter/       Stateful word-boundary detector (added v1.3.13)
internal/appfilter/         NSWorkspace frontmost-app + per-app allowlist (added v1.3.13)
internal/speedtracker/      Active-time + fastest-pace WPM tracker (added v1.4)
internal/storage/           SQLite store + settings
internal/tui/               Bubble Tea typing test
pkg/stats/                  Pure functions for streaks / averages / peaks
Casks/typtel.rb             Homebrew cask (synced to abaj8494/homebrew-typing-telemetry)
Formula/typing-telemetry.rb Homebrew formula for the CLI
Makefile                    Build, install, app-bundle, release helpers
scripts/                    Info.plist + LaunchAgent plists
```

## Build & test

```sh
make build           # both binaries into build/
make test            # go test ./...
make app             # produce build/Typtel.app
make install-app     # copy to /Applications (replaces existing)
make run-menubar     # build and run menubar from build/
```

`CGO_ENABLED=1` is required for the menubar (Cocoa + CoreGraphics). The CLI is
pure Go. `go test ./...` runs everywhere; the keylogger and appfilter packages
are darwin-only and tested via the consuming code paths.

## Release flow

`abaj8494/typing-telemetry` and `abaj8494/homebrew-typing-telemetry` are the
**same GitHub repo** (the latter is a prior name kept as a redirect). So pushing
to `origin` updates both, the cask at `Casks/typtel.rb` is the one `brew` reads,
and a release/tag exists once under both URLs. Releases are built by CI
(`.github/workflows/release.yml`) on tag push — **do not build/zip/`gh release
create` by hand**; CI rebuilds the artifact and would overwrite a hand-uploaded
zip, leaving any manually-pinned `sha256` stale (the classic `brew upgrade`
"SHA-256 mismatch"). Versions track the digits of √2 — see
[[typtel-version-scheme]].

Standard sequence:

1. Bump `VERSION?=` in `Makefile` (next √2 prefix, e.g. 1.4142).
2. Bump `version` in `Casks/typtel.rb`. Leave `sha256` as-is — CI overwrites it.
3. Commit the feature plus the version bumps in **one** commit.
4. `git tag -a vX -m "…"` with a release-note-style annotation.
5. `git push origin main && git push origin vX`.
6. Wait for the **Release** workflow to finish (~2-3 min). It builds the app,
   publishes the GitHub Release with `Typtel-X.zip`, computes that zip's sha256,
   and commits `ci: pin cask sha256 to vX artifact [skip ci]` to `main`. After
   that commit lands, `brew update && brew upgrade --cask typtel` works.

There is a transient window between the tag push and CI's sha-pin commit where
the cask sha is stale; just wait for the CI commit before upgrading. The CLI
formula (`Formula/typing-telemetry.rb`) is separate — bump its `version`/`tag`
manually when releasing the CLI, and keep it passing `brew style`.

The app is **ad-hoc signed** (`codesign --sign -` via the Go linker default);
no Apple Developer certificate is involved. The cask's `postflight` runs
`xattr -cr` to strip Gatekeeper quarantine.

## Conventions

- macOS keycodes (CGKeyCode physical keys) are referenced as raw integers in a
  few places; `internal/wordcounter/wordcounter.go` defines named constants
  for the printable subset. `internal/storage.ClassifyKeycode` covers the
  letter/modifier/special trichotomy.
- Words are counted via `internal/wordcounter.Counter.Observe(Event)` — a
  boundary detector that mirrors Christian Tietze's WordCounter (the reference
  app, reverse-engineered from its binary): a word is a maximal run of
  non-whitespace characters, committed on the next Space/Return/Tab. It carries
  a single `typingWord` bool (any printable arms it), treats Cmd/Ctrl-held keys
  as shortcuts, and **backspace is a no-op**. Do **not** revive the old
  `isWordBoundary` heuristic, and do **not** reintroduce per-character
  bookkeeping or backspace rollback — the rollback (pre-v1.414) made typtel read
  ~5% below WordCounter because ordinary typo correction erased real words.
- Per-app filtering uses `internal/appfilter.Frontmost()` (NSWorkspace bundle
  ID) gated by an allowlist persisted in settings. Disabled by default.
- Modifier flags travel with keystroke events from the C callback
  (`internal/keylogger/keylogger_darwin.go`) as `KeystrokeEvent{Keycode, Flags}`
  — read them via the helpers `CmdHeld()`, `CtrlHeld()`, `OptHeld()`,
  `ShiftHeld()`.
- Typing speed (v1.4) is WPM measured against *active* typing time: idle gaps
  between keystrokes over 2s are excluded (`internal/speedtracker`, mirrors
  `wordcounter`'s pure/testable design). `daily_summary` carries `active_ms`
  plus three `fastest_*_wpm` columns; period stats come from
  `storage.GetSpeedAggregate` and `stats.AverageWPM`. The menubar batches
  measurements in memory (`cmd/typtel-menubar/speed.go`) and flushes on the
  stats ticker — do **not** write speed per keystroke. `IdleCapMs` is
  duplicated in `internal/storage` (the one-time `BackfillActiveTime`) and must
  stay in sync with `speedtracker.IdleCapMs`.

## Collaboration

This repo is sometimes worked on by more than one AI agent in parallel. To
avoid stomping each other's edits:

- **Claude writes here (`CLAUDE.md`).** Anything I work on or invariants I
  establish go in this file.
- **The other agent writes in `SLAVE.md`.** Their in-flight work, API plans,
  watchdog integration, etc. should land there.
- Before doing large changes, glance at the other file. If a path appears in
  both, mention it in your file so the other agent knows it's shared.

Currently in flight elsewhere: an HTTP/IPC API surface to expose typtel data
to `macOS-watchdog`. Likely import points:
`internal/wordcounter` (for `Counter`), `internal/appfilter` (for allowlist),
and `internal/storage` (for `DailyStats`). Both packages were designed to be
importable from an API layer without dragging in the menubar binary.

Shared since v1.4 (typing speed): `internal/storage` gained the
`daily_summary` speed columns, `SpeedAggregate`, `GetSpeedAggregate`,
`AddActiveTime`, `UpdateFastest`, and `BackfillActiveTime`; `pkg/stats` gained
`AverageWPM`; and `internal/speedtracker` is a new pure package. The watchdog
API can read speed via `GetSpeedAggregate` + `AverageWPM` with no new storage
work. `typtel stats --json` exposes a `speed` block over the same data.
