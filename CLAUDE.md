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

The cask is hosted at `abaj8494/homebrew-typing-telemetry`. The binary release
(the `.zip`) is published as a GitHub Release on **that** repo (not on
`typing-telemetry`).

Standard sequence:

1. Bump `VERSION?=` in `Makefile`.
2. Bump `version` in `Casks/typtel.rb`. Leave the `sha256` line as the old
   value temporarily — it will be replaced after the build.
3. Commit the feature plus the version bumps in **one** commit.
4. `git tag -a vX.Y.Z` with release-note-style annotation.
5. `make app VERSION=X.Y.Z` to produce `build/Typtel.app`.
6. `(cd build && zip -r ../Typtel-X.Y.Z.zip Typtel.app)`.
7. `shasum -a 256 Typtel-X.Y.Z.zip > Typtel-X.Y.Z.zip.sha256`.
8. Update `Casks/typtel.rb` `sha256` line; second commit:
   `fix: update cask SHA256 to match GitHub release for vX.Y.Z`.
9. `git push origin main && git push origin vX.Y.Z`.
10. `gh release create vX.Y.Z --repo abaj8494/homebrew-typing-telemetry
    Typtel-X.Y.Z.zip Typtel-X.Y.Z.zip.sha256 --title "vX.Y.Z" --notes "…"`.

The app is **ad-hoc signed** (`codesign --sign -` via the Go linker default);
no Apple Developer certificate is involved. The cask's `postflight` runs
`xattr -cr` to strip Gatekeeper quarantine.

## Conventions

- macOS keycodes (CGKeyCode physical keys) are referenced as raw integers in a
  few places; `internal/wordcounter/wordcounter.go` defines named constants
  for the printable subset. `internal/storage.ClassifyKeycode` covers the
  letter/modifier/special trichotomy.
- Words are counted via `internal/wordcounter.Counter.Observe(Event)` — a
  stateful boundary detector that requires content before a space/return/tab
  fires, treats Cmd/Ctrl-held keys as shortcuts, and rolls back on backspace.
  Do **not** revive the old `isWordBoundary` heuristic.
- Per-app filtering uses `internal/appfilter.Frontmost()` (NSWorkspace bundle
  ID) gated by an allowlist persisted in settings. Disabled by default.
- Modifier flags travel with keystroke events from the C callback
  (`internal/keylogger/keylogger_darwin.go`) as `KeystrokeEvent{Keycode, Flags}`
  — read them via the helpers `CmdHeld()`, `CtrlHeld()`, `OptHeld()`,
  `ShiftHeld()`.

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
