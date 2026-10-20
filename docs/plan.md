# app-launcher — product and engineering plan

Status: **P3 complete** · Updated: 2026-10-20

A Windows-only, keyboard-first launcher that finds and starts applications. It is
the app-launcher slice of [Wox](https://github.com/Wox-launcher/Wox) rebuilt around
one constraint: it must cost almost nothing while idle.

Wox v2 ships a Go core, a Flutter UI and plugin/cloud/AI/telemetry subsystems, and
declares ~150 MB resident. This project keeps the one verb people repeat all day —
*hotkey → type → Enter* — and drops everything else.

## Quick path

1. `go run ./cmd/spike` — prints the measured baseline (P0 evidence, ~6 s).
2. `go test ./...` — matching, ranking and config rules.
3. `go build -trimpath -ldflags "-s -w" -o bin/launcher.exe ./cmd/launcher` — P2.

## Decisions

| Area | Decision | Why |
|---|---|---|
| Platform | Windows 10 22H2+ and Windows 11, x64 | Win32 launcher; no cross-platform tax |
| Stack | Go 1.27 + direct Win32 (`internal/win`) | Already installed; 11 MB proven; single .exe, no runtime to ship |
| UI | Native GDI-drawn borderless window | WebView2 would add 4–6 processes and 100+ MB; WPF needs a 250 MB SDK |
| Process model | One resident process with a hidden, pre-built window | 3 ms hotkey→visible; no cold start per invocation |
| Scope | Applications only | Files, web, clipboard, plugins are separate products |
| Config | JSON file next to the executable (portable), `%APPDATA%` fallback | No registry writes, no installer |
| Distribution | Portable folder, no installer, no updater, no network | Personal use |
| Dependencies | `golang.org/x/sys` only | Every dependency is memory and attack surface |

## Non-goals (v1)

File search, content search, web search, calculator, clipboard history, plugins,
theme store, cloud sync, AI, telemetry, auto-update, multi-user profiles,
non-Windows platforms.

## Resource budget vs measured baseline

| Metric | Budget | Measured |
|---|---|---|
| Idle working set, full launcher with 96 apps | ≤ 30 MB | **24.5 MB** hidden · **27.5 MB** with icons on screen |
| Idle working set, bare shell (P0 spike) | — | **7.7 MB** before first show · **10.8 MB** after two show/hide cycles |
| Private bytes (spike) | ≤ 30 MB | **12.6 MB** |
| Boot: `main` → hotkey registered | ≤ 120 ms | **2.6 ms** |
| `Show()` (ShowWindow + SetForegroundWindow) | ≤ 50 ms | **3.2 ms** cold · **< 0.1 ms** warm |
| First `WM_PAINT` after show | ≤ 50 ms | **2.2 ms** |
| Full scan of every source | ≤ 300 ms | **74 ms** for 96 apps (163 ms with a cold file cache) |
| Search over 3 000 entries | ≤ 1 ms | **0.11 ms**, 2 KB allocated per query |
| CPU, whole 6 s spike run incl. startup and 2 paints | — | **~16 ms total** (0.0 % steady state) |
| OS threads | ≤ 16 | **13** |
| Kernel handles | ≤ 500 | **375** idle · **452** after the first icon extraction |
| GDI objects | ≤ 60 | **8** (before icons) |
| Icon extraction (first time only) | — | **+2.4 MB private, +76 handles**, then flat |
| Binary size | ≤ 15 MB | **3.0 MB** |
| Processes | 1 | **1** |
| Network connections | 0 | **0** |

Reproduce with `go run ./cmd/spike`. The report is JSON on stdout.

**Headroom.** Of the 30 MB budget, ~19 MB remain for the app index, the icon cache
and the result list. A 3 000-entry index at ~100 B per entry is ~300 KB; 64 cached
HICONs are a few hundred KB. The budget is not tight — it is a guardrail against
the usual launcher bloat (frameworks, webviews, idle pollers).

## What "an app" means

| Source | How it is read | How it is launched |
|---|---|---|
| Start Menu shortcuts | Recursive scan of `%ProgramData%\Microsoft\Windows\Start Menu\Programs` and `%APPDATA%\Microsoft\Windows\Start Menu\Programs`; parse `.lnk` with `IShellLinkW` for target, args, workdir, icon | `ShellExecuteExW` on the `.lnk` |
| Store / UWP apps | Enumerate `shell:AppsFolder` (`SHGetKnownFolderItem` + `IShellFolder`); keep the AppUserModelID | `IApplicationActivationManager::ActivateApplication` |
| Custom folders (config) | Recursive scan for `.exe` / `.lnk` with include/exclude globs | `ShellExecuteExW` on the file |
| `App Paths` | `HKLM/HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths` | `ShellExecuteExW` with the resolved path |
| `PATH` (filtered) | `%PATH%`, minus Windows system folders, minus `WindowsApps` aliases; only PE subsystem `WINDOWS_GUI` unless `path_scope` says otherwise | `ShellExecuteExW` with the resolved path |

**Deduplication** is by canonical identity: resolved target path (case-folded) for
Win32 entries, AppUserModelID for UWP. When two sources produce the same app,
priority is Start Menu → custom folder → App Paths → PATH. The Start Menu entry
wins because it carries the display name and icon users recognize.

Advertised (MSI) shortcuts are a known trap: their `.lnk` points at an installer
stub, not at the executable. Resolving them with `IShellLinkW` can yield a
meaningless target, so launch always goes through `ShellExecuteExW` on the
shortcut, never through the parsed path.

## Index lifecycle

1. **Cold start**: load `index-cache.json` if present and its schema version
   matches. The first hotkey never waits for a scan.
2. **Background refresh**: enumerate all sources on a low-priority goroutine;
   compare against the cache per root using directory mtimes; persist only what
   changed.
3. **Watchers**: `ReadDirectoryChangesW` on the Start Menu roots and custom
   folders, debounced 2 s and coalesced into one rebuild.
4. **Manual refresh**: `F5` inside the launcher re-scans everything.
5. **Failure policy**: a broken `.lnk` or a locked registry key is skipped and
   logged; it never aborts the build.

Registry `App Paths` is re-read on every full rebuild, never per keystroke.

## Matching and ranking

- **Normalization**: lowercase, strip diacritics, then build two forms — the
  display form (for highlighting) and a compact form without spaces, dots and
  dashes (`visual studio code` and `vscode` must both hit).
- **Match**: ordered subsequence with a score. Bonuses for consecutive characters,
  word starts and prefix matches; penalty for gaps. No fuzzy-edit distance in v1 —
  it costs time per key and users rarely mistype the first two letters.
- **Rank**: `score * 0.6 + frecency * 0.4`, where frecency is a decaying use count
  (7-day half-life, Laplace-smoothed so a single use does not dominate). Ties break
  on last-used descending, then name.
- **Empty query**: top 8 by frecency, so the first `Enter` after the hotkey is
  usually the right app.
- **Cost**: a linear scan of 3 000 entries is well under 1 ms; no index structure,
  no SQLite, no FTS.

## Launching

| Case | Call |
|---|---|
| Normal | `ShellExecuteExW(SEE_MASK_INVOKEIDLIST)` |
| Run as admin (`Shift+Enter`) | `ShellExecuteExW` with `runas` verb |
| Open containing folder (`Ctrl+Enter`) | `explorer.exe /select,<path>` |
| UWP | `IApplicationActivationManager::ActivateApplication` |

Failures surface as a non-blocking toast in the window footer, never as a modal.

## UI and keys

Window: 640×420, horizontally centered, ~28 % from the top of the monitor under
the cursor, borderless, topmost, `WS_EX_TOOLWINDOW` (no taskbar button), dark
palette, rounded corners where the OS supports it.

| Key | Action |
|---|---|
| `Ctrl+Alt+Space` (configurable) | Show / focus the launcher |
| Type | Filter |
| `↑` `↓` | Move selection, wrapping |
| `Enter` | Launch |
| `Esc` | Clear query, or hide if empty |
| `Shift+Enter` | Launch as administrator |
| `Ctrl+Enter` | Open containing folder |
| `F5` | Rebuild index |

Focus stealing is the one behavior Windows actively fights: `SetForegroundWindow`
can be refused when another app owns the foreground. The fallback is the
`AttachThreadInput` dance, plus `SetWindowPos(HWND_TOPMOST)` refresh. This is a
P2 acceptance criterion, not a nice-to-have.

## Architecture

```
cmd/launcher        composition root; flags, single-instance, tray
cmd/spike           P0 measurement harness (kept: it is the regression baseline)
internal/win        the only package allowed to call Win32
internal/core       models, matcher, ranker, config, index cache   ← no Win32, 100% testable
internal/sources    Start Menu, AppsFolder/UWP, custom folders, App Paths
internal/ui         window, input box, virtualized result list, painting
```

`internal/core` must not import `internal/win` or `golang.org/x/sys`. That rule is
what keeps matching and ranking testable without a desktop session.

## Phases

| Phase | Deliverable | Exit criteria |
|---|---|---|
| **P0** ✅ | Spike: hidden window, global hotkey, self-measurement | Measured budget met with 3× headroom (see table) |
| **P1** ✅ | `core`: matcher, ranker, config, cache format | 31 test functions green; 3 000 entries in **0.11 ms** |
| **P2** ✅ | Sources + window + launch | 96 apps found (73 Start Menu, 11 Store, 15 App Paths, 12 PATH after the GUI filter); `.lnk` and UWP both launch; hotkey steals the foreground from another app |
| **P3** ✅ | Frecency wired to the ranker, `history.json`, tray menu, autostart | Launched apps outrank text matches after a restart (`history.json` round-trips); tray adds, toggles autostart and quits; empty query shows only used apps |
| **P4** 🚧 | Polish: icons ✅, acrylic ✅, DPI, packaging | Acrylic needs a layered window and a 91% tint to stay readable; solid remains the fallback |

## Risks

| Risk | Impact | Mitigation |
|---|---|---|
| `SetForegroundWindow` refused | Launcher opens behind the active app | `AttachThreadInput` fallback; P2 acceptance test with a fullscreen game |
| MSI advertised shortcuts resolve to a stub | App "opens" nothing | Launch the `.lnk` via ShellExecute; never the parsed target |
| Some UWP entries lack a launchable AUMID | Dead result rows | Validate at index time; drop entries that fail activation |
| Icon extraction cost | Slow first paint, handle growth | Lazy per-visible-row icons, LRU cap 64, GDI handle budget asserted in tests |
| Windows 10 has no Mica | Square, flat window vs Win11 | Solid dark background with DWM rounded corners on 11; documented, not a blocker |
| Antivirus/SmartScreen on unsigned exe | Distribution friction | Out of scope: portable, personal use |

## Open decisions (defaults already chosen; say the word to change any)

| Topic | Default taken |
|---|---|
| Product name | Working name `app-launcher`, binary `launcher.exe` |
| Default hotkey | `Ctrl+Alt+Space` |
| UI language | English |
| Empty-query behavior | Top 8 by frecency |
| Icon style | Native app icons at 32 px, no badge or overlay |
| Theme | Dark, follows nothing (`system` mode deferred to P4) |
