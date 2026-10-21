# Icons and version information

Glint has exactly one mark, and no image editor is involved in keeping it:

```
internal/art/art.go ──┬── internal/win/appicon.go  → HICON for the tray
                      └── tools/icongen            → assets/glint.ico
                                                          │
                            goversioninfo ────────────────┘
                               └── cmd/glint/resource.syso → linked into the .exe
                                   (icon + version info)
```

`internal/art` rasterizes a rounded accent panel with a white chevron, 4×4
supersampled, with a band of light across its top — the glint the name is about.
Every consumer reads that one buffer, so the tray, the settings row, the window
title bar and Explorer cannot drift apart.

## Regenerating

`resource.syso` is committed, so a plain `go build` needs no extra tooling. The
chain is only rebuilt when the mark, or the version strings, change:

```powershell
# 1. Re-render the .ico from internal/art (no dependencies).
go generate ./tools/icongen

# 2. Re-embed the icon and the version info as Windows resources.
go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
goversioninfo -64 -icon assets/glint.ico -o cmd/glint/resource.syso cmd/glint/versioninfo.json

# 3. Rebuild and check both survived.
./build.ps1
powershell -Command "[System.Diagnostics.FileVersionInfo]::GetVersionInfo('bin\glint.exe') | fl ProductName,FileDescription,FileVersion"
powershell -Command "[System.Drawing.Icon]::ExtractAssociatedIcon('bin\glint.exe').ToBitmap().Save('$env:TEMP\icon.png')"
```

Icon and version live in **one** `.syso` on purpose: the Go linker merges a single
`.rsrc` section per package, so two objects that both carry one fail the build with
`too many .rsrc sections`.

## Why the .syso exists

A Windows executable can only carry an icon and a version block in its resource
section, and the Go toolchain has no flag for either: the standard answer is a
`.syso` object that the linker picks up from the package directory. For P4 the icon
was drawn at runtime only, precisely to avoid this step. That trade was worth
making for a tray icon; it is not worth making for the executable, because
Explorer, Alt+Tab and the taskbar all read the shell resource, not a running
program's HICON.

The version block is what makes the startup list say **Glint** instead of
`glint.exe`: Windows shows `FileDescription` when it exists and falls back to the
file name when it does not. `versioninfo.json` is therefore part of the identity,
not decoration — changing `ProductName` there changes what the operating system
calls the program.

The fields are not interchangeable, and Windows labels them in the user's language:

| Field | Spanish Explorer | Value |
|---|---|---|
| `ProductName` | Nombre del producto | Glint |
| `FileDescription` | Descripción | Glint — what the startup list shows |
| `CompanyName` | **Editor** | Legnatbird — the person, not the program |
| `LegalCopyright` | Copyright | © Legnatbird |
| `OriginalFilename` | Nombre original | glint.exe |

## Sizes

`assets/glint.ico` holds 16, 20, 24, 32, 40, 48, 64 and 256 pixels. The first seven
are 32bpp bitmaps with an AND mask, which is what the shell uses for small and
medium views; 256 is PNG-compressed because a raw bitmap at that size would add
265 KB to the binary for a view most people never open.

The executable grows by ~100 KB with the resources in place, most of it the icon's
eight frames.
