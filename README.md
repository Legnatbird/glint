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
- **P3** complete — usage history in `history.json`, frecency drives the ranking, tray menu, autostart toggle.
- **P4** complete — app icons (shell and Store apps), rounded corners, optional acrylic backdrop, the launcher's own icon embedded in the executable.
- **P5** complete — settings window: editable controls that apply live, no restart.

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
| `F5` | Rebuild the index |

Typing `set` (or `con`, `aju`, `pref`) puts a **Settings** row on top of the list;
so does the tray menu. Both open the settings window.

Starting it again toggles the running instance instead of creating a second one.

## Settings window

One column of rows, keyboard first:

| Key | Action |
|---|---|
| `↑` `↓` `Tab` | Move between rows |
| `←` `→` | Change a value (`max_results`, backdrop, PATH scope) |
| `Enter` `Space` | Activate: record a hotkey, toggle, pick a folder, clear |
| `Esc` | Close |

Every edit is saved to `config.json` and pushed to the running launcher at once:

| Row | What happens immediately |
|---|---|
| Hotkey | The new combination is registered. While recording, the old one is released so the keystroke reaches the window. If Windows refuses the combination, the previous one comes back and the reason shows in the footer. |
| Max results | The launcher list reflows to the new limit. |
| Backdrop | The launcher window switches between acrylic and solid in place. |
| Start with Windows | The `Run` key entry is written or removed. |
| PATH scope, folders, manual apps | The index rescans in the background through the same single-flight rebuilder the hotkey uses. |

Destructive rows (a folder, a manual app, the history) ask twice: the first
`Enter` or `Delete` arms the row, the second one acts.

## Configuration

`config.json` sits next to the executable. Missing fields fall back to the
defaults, unknown fields are ignored, and an invalid value never breaks startup.

```json
{
  "hotkey": "ctrl+alt+space",
  "max_results": 8,
  "theme": "dark",
  "path_scope": "gui",
  "backdrop": "solid",
  "autostart": false,
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

`backdrop` is `"solid"` (default) or `"acrylic"`. Acrylic goes through
`SetWindowCompositionAttribute`, an API Microsoft never documented; if the call
fails the launcher silently keeps the solid background.

Solid is the default on purpose. The accent paints the whole window rectangle and
is not clipped by the rounded region, so on Windows 10 acrylic shows as a dark
sliver in the four corners. Windows 11 rounds the window through DWM and does not
have that problem.

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
cmd/launcher      the app: window, hotkey, wiring; rsrc.syso carries the icon
cmd/spike         P0 measurement harness
internal/art      the launcher's mark, rasterized in pure Go
internal/win      the only package allowed to call Win32 (COM, GDI, shell)
internal/sources  Start Menu, Store/UWP, App Paths and PATH scanners
internal/core     matching, ranking, config, cache (no Win32)
internal/ui       launcher state machine and painting
tools/icongen     renders assets/launcher.ico from internal/art
docs/plan.md      product and engineering plan
```

## Icon

The mark (a rounded accent panel with a white chevron) is drawn in code, once, in
`internal/art`. The tray, the settings row, the settings title bar and the
executable's own shell icon all come from that one buffer — see
[assets/README.md](assets/README.md) for how the `.ico` is generated and embedded.
