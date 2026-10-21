# Glint

[![CI](https://github.com/Legnatbird/glint/actions/workflows/ci.yml/badge.svg)](https://github.com/Legnatbird/glint/actions/workflows/ci.yml)

A Windows-only, keyboard-first launcher for applications: the launcher slice
of Wox, rebuilt to stay under 30 MB and answer every keystroke in about a
millisecond.

Plan and living decisions: [docs/plan.md](docs/plan.md). Releases carry the built
executable: [github.com/Legnatbird/glint/releases](https://github.com/Legnatbird/glint/releases).

## Status

- **P0** complete, hidden Win32 shell: 10.8 MB, 2.6 ms boot, 3.2 ms to visible.
- **P1** complete, fuzzy matcher, frecency, portable config and index cache;
  3 000 apps ranked in 0.11 ms.
- **P2** complete, 217 apps discovered (Start Menu, Store/UWP, App Paths, PATH),
  drawn result list, global hotkey, `.lnk` and UWP launching.
- **P3** complete, usage history in `history.json`, frecency drives the ranking, tray menu, autostart toggle.
- **P4** complete, app icons (shell and Store apps), rounded corners, the launcher's own icon embedded in the executable.
- **P5** complete, settings window: editable controls that apply live, no restart.

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

Start `bin/glint.exe`. It registers `Ctrl+Alt+Space`, stays in the background
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

The query is a real text field, so the editing keys are the ones on the rest of the
machine:

| Key | Action |
|---|---|
| `←` `→` `Home` `End` | Move the caret |
| `Shift`+movement | Select |
| `Ctrl+←` `Ctrl+→` | Move by word |
| `Ctrl+A` | Select all, then type to replace, or `Backspace` to clear |
| `Ctrl+Backspace` `Ctrl+Delete` | Delete a word |

The pointer works too: hovering shows which row a click would hit, clicking
launches it, and the wheel moves the selection three rows at a time. Hover is only
feedback, it never moves the keyboard selection, so reaching for the mouse cannot
change what `Enter` will do.

Typing `set` (or `con`, `aju`, `pref`) puts a **Settings** row on top of the list;
so does the tray menu. Both open the settings window.

Starting it again shows the running instance instead of creating a second one.

## Settings window

One column of rows, keyboard first; the pointer hovers and clicks the same rows:

| Key | Action |
|---|---|
| `↑` `↓` `Tab` | Move between rows |
| `←` `→` | Change a value (`max_results`, PATH scope) |
| `Enter` `Space` | Activate: record a hotkey, toggle, pick a folder, clear |
| `Enter` on a manual app | Rename it, `Ctrl+A`, arrows and `Backspace` work while typing |
| `F2` or double click on a manual app | The same rename, for the muscle memory |
| `Enter`, a click outside, or leaving the window | Saves the name; `Esc` discards it |
| `Delete` on a manual app or folder | Remove it (asks twice) |
| `Esc` | Close |

Rows that hold a value also carry `−` and `+` buttons after it, for the mouse.

Every edit is saved to `config.json` and pushed to the running launcher at once:

| Row | What happens immediately |
|---|---|
| Hotkey | The new combination is registered. While recording, the old one is released so the keystroke reaches the window. If Windows refuses the combination, the previous one comes back and the reason shows in the footer. |
| Max results | The launcher list reflows to the new limit. |
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

The window is opaque. A translucent backdrop was tried and removed: the accent
policy Windows 10 uses for it paints the whole window rectangle and is not clipped
by the rounded region, so every corner kept a dark sliver. Opaque plus a proper
rounded clip looks better than translucent with broken corners.

## Inspect what it found

```powershell
./bin/glint.exe -dump | more
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
cmd/glint         the app: window, hotkey, wiring; resource.syso carries the icon and the version block
cmd/spike         P0 measurement harness
internal/art      Glint's mark, rasterized in pure Go
internal/win      the only package allowed to call Win32 (COM, GDI, shell)
internal/sources  Start Menu, Store/UWP, App Paths and PATH scanners
internal/core     matching, ranking, config, cache (no Win32)
internal/ui       launcher state machine and painting
tools/icongen     renders assets/glint.ico from internal/art
docs/plan.md      product and engineering plan
```

## Icon

The mark (a rounded accent panel with a white chevron and a band of light across
its top, which is the glint) is drawn in code, once, in `internal/art`. The tray, the
settings row, the settings title bar and the executable's own icon all come from
that one buffer; see [assets/README.md](assets/README.md) for how the `.ico` is
generated and embedded, and for the version block that makes Windows call the
program Glint rather than `glint.exe`.

Row icons come from the shell (`SHGetFileInfoW` for files, `IShellItemImageFactory`
for packaged apps). Both kinds are decoded once into BGRA at their own size and
drawn into a 32 px slot at 29 px, with one resample that averages area when the
icon shrinks and interpolates when it grows. GDI never scales an icon here:
`DrawIconEx` resamples with the device's raw stretch mode, which leaves hard,
stepped edges, and it was the reason the list looked badly rendered while every
shape in it was correct.

A row is 50 px tall and its text is drawn at 18 px; the window is 700×540 so nine
results still fit. Those three numbers move together: shrinking the row without
the font, or growing the font without the row, is what makes a list look cramped.

The visible box, not the canvas, is what gets fitted. Packaged logos arrive inside
a 32×32 canvas that may hold a 24×18 mark, and file icons carry whatever padding
their `.ico` author chose; fitting the box gives them the same optical weight
without changing their shape. Artwork that is not square (Terminal and PowerShell
ship 4:3 tiles) therefore stays 4:3, which is what its package contains.

## License

Personal use only: the source is published so it can be read, not so it can be
used. You may read it and learn from it; you may not run it for your own purposes,
reuse any part of it, or distribute a build. See [LICENSE](LICENSE) for the exact
terms.

This is not an open source license: it restricts the field of use, which the Open
Source Definition does not allow. The repository is source available.
