#!/usr/bin/env pwsh
# Build the launcher. -H=windowsgui keeps Windows from attaching a console to a
# double-clicked executable; -s -w drops the symbol table and DWARF data.
$ErrorActionPreference = "Stop"
go build -trimpath -ldflags "-s -w -H=windowsgui" -o bin/launcher.exe ./cmd/launcher
Write-Host "built bin/launcher.exe ($([math]::Round((Get-Item bin/launcher.exe).Length / 1MB, 2)) MB)"
