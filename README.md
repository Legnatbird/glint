# app-launcher

A Windows-only, keyboard-first launcher for applications — the app-launcher slice
of Wox, rebuilt to stay under 30 MB and respond in milliseconds.

The full product and engineering plan lives in [docs/plan.md](docs/plan.md).

## Status

- **P0** complete: a hidden Win32 window, a global hotkey and an idle message
  loop, measured at **10.8 MB working set**, **2.6 ms boot** and **3.2 ms** from
  hotkey to visible.
- **P1** complete: fuzzy matcher, frecency ranker, portable config and index
  cache in `internal/core`, with no Win32 involved — 3 000 entries ranked in
  **0.11 ms** allocating 2 KB.

## Requirements

- Windows 10 22H2 or Windows 11, x64
- Go 1.27+

## Run the baseline

```powershell
go run ./cmd/spike
```

It shows the window twice, hides it, prints a JSON report, and exits after six
seconds.

## Build

```powershell
go build -trimpath -ldflags "-s -w" -o bin/launcher.exe ./cmd/launcher
```

`cmd/launcher` arrives in P2. Today only the spike exists.

## Test

```powershell
go test ./...
go test -bench . -benchmem ./internal/core/
```

## Layout

```
cmd/spike        P0 measurement harness
internal/win     the only package allowed to call Win32
internal/core    matching, ranking, config, index cache (no Win32)
docs/plan.md     product and engineering plan
```
