# app-launcher

A Windows-only, keyboard-first launcher for applications — the app-launcher slice
of Wox, rebuilt to stay under 30 MB and answer every keystroke in about a
millisecond.

Plan and living decisions: [docs/plan.md](docs/plan.md).

## Status

- **P0** complete — hidden Win32 shell: 10.8 MB, 2.6 ms boot, 3.2 ms to visible.
- **P1** complete — fuzzy matcher, frecency, portable config and index cache;
  3 000 apps ranked in 0.11 ms.
- **P2** complete — 217 apps discovered (Start Menu, Store/UWP, App Paths, PATH),
  drawn result list, global hotkey, `.lnk` and UWP launching.
- P3 next: usage tracking wired to the ranker, tray icon, autostart.

## Requirements

- Windows 10 22H2 or Windows 11, x64
- Go 1.27+ to build

## Build

```powershell
./build.ps1
```

`-H=windowsgui` keeps Windows from attaching a console to a double-clicked
executable. `-s -w` drops the symbol table: the binary is ~3 MB.

## Run

Start `bin/launcher.exe`. It registers `Ctrl+Alt+Space`, stays in the background
and shows the launcher on demand.

| Key | Action |
|---|---|
| `Ctrl+Alt+Space` | Show / hide |
| `↑` `↓` | Move selection |
| `Enter` | Launch |
| `Shift+Enter` | Launch as administrator |
| `Ctrl+Enter` | Open the containing folder |
| `Esc` | Clear the query, then hide |
| `F5` | (planned) rebuild the index |

Starting it again toggles the running instance instead of creating a second one.

## Configuration

`config.json` sits next to the executable. Missing fields fall back to the
defaults, unknown fields are ignored, and an invalid value never breaks startup.

```json
{
  "hotkey": "ctrl+alt+space",
  "max_results": 8,
  "theme": "dark",
  "path_scope": "gui",
  "custom_folders": ["D:\\Portables"],
  "exclude_folders": [],
  "include_app_paths": true,
  "history": { "enabled": true, "max_entries": 2000 }
}
```

`path_scope` decides what PATH contributes:

| Value | Effect |
|---|---|
| `"gui"` (default) | Only executables whose PE subsystem is `WINDOWS_GUI`. On this machine: 135 PATH entries become 12. |
| `"all"` | Every executable on PATH, console tools included. |
| `"off"` | Ignore PATH entirely. |

Windows system folders and the `WindowsApps` alias stubs are always skipped.

## Inspect what it found

```powershell
./bin/launcher.exe -dump | more
```

`-demo <query> -seconds <n>` opens the launcher with a query already typed, which
is how the screenshots are produced.

## Test

```powershell
go test ./...
go test -bench . -benchmem ./internal/core/
go run ./cmd/spike
```

## Layout

```
cmd/launcher      the app: window, hotkey, wiring
cmd/spike         P0 measurement harness
internal/win      the only package allowed to call Win32 (COM, GDI, shell)
internal/sources  Start Menu, Store/UWP, App Paths and PATH scanners
internal/core     matching, ranking, config, cache (no Win32)
internal/ui       launcher state machine and painting
docs/plan.md      product and engineering plan
```
