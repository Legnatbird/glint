# Icons

The launcher has exactly one mark, and no image editor is involved in keeping it:

```
internal/art/art.go ──┬── internal/win/appicon.go  → HICON for the tray
                      └── tools/icongen            → assets/glint.ico
                                                        │
                                     rsrc ──────────────┘
                                        └── cmd/glint/rsrc.syso → linked into the .exe
```

`internal/art` rasterizes a rounded accent panel with a white chevron, 4×4
supersampled. Every consumer reads that one buffer, so the tray, the settings
row, the window title bar and Explorer cannot drift apart.

## Regenerating

`rsrc.syso` is committed, so a plain `go build` needs no extra tooling. The chain
is only rebuilt when the mark itself changes:

```powershell
# 1. Re-render the .ico from internal/art (no dependencies).
go generate ./tools/icongen

# 2. Re-embed it as a Windows resource.
go install github.com/akavel/rsrc@latest
rsrc -arch amd64 -ico assets/glint.ico -o cmd/glint/rsrc.syso

# 3. Rebuild and check the icon is there.
./build.ps1
powershell -Command "[System.Drawing.Icon]::ExtractAssociatedIcon('bin\glint.exe').ToBitmap().Save('$env:TEMP\icon.png')"
```

## Why the .syso exists

A Windows executable can only carry an icon in its resource section, and the Go
toolchain has no flag for that: the standard answer is a `.syso` object that the
linker picks up from the package directory. For P4 the icon was drawn at runtime
only, precisely to avoid this step. That trade was worth making for a tray icon;
it is not worth making for the executable, because Explorer, Alt+Tab and the
taskbar all read the shell resource, not a running program's HICON.

## Sizes

`assets/glint.ico` holds 16, 20, 24, 32, 40, 48, 64 and 256 pixels. The first
seven are 32bpp bitmaps with an AND mask, which is what the shell uses for
small and medium views; 256 is PNG-compressed because a raw bitmap at that size
would add 265 KB to the binary for a view most people never open.

The executable grows by ~46 KB, from 3.42 MB to 3.47 MB.
