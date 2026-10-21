#!/usr/bin/env pwsh
# Build Glint. -H=windowsgui keeps Windows from attaching a console to a
# double-clicked executable; -s -w drops the symbol table and DWARF data.
$ErrorActionPreference = "Stop"
go build -trimpath -ldflags "-s -w -H=windowsgui" -o bin/glint.exe ./cmd/glint
Write-Host "built bin/glint.exe ($([math]::Round((Get-Item bin/glint.exe).Length / 1MB, 2)) MB)"
