# Windows resources (exe icon/manifest/version): generate .syso for go build to link.
# Requires the wails3 CLI; skipped when absent.
if (Get-Command wails3 -ErrorAction SilentlyContinue) {
  Push-Location build
  wails3 generate syso -arch amd64 -icon windows/icon.ico -manifest windows/wails.exe.manifest -info windows/info.json -out ../wails_windows_amd64.syso
  Pop-Location
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}

# Release build: no console window, production tag (devtools off, prod runtime)
# Output: bin/dsh-shell.exe
go build -tags production -ldflags "-s -w -H windowsgui" -o bin/dsh-shell.exe .
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "built bin/dsh-shell.exe"

# Debug build: no console window (logs go to bin/data/.dsh-shell/dsh-shell.log).
# For live console output use: go build -o bin/dsh-shell-console.exe .
go build -ldflags "-H windowsgui" -o bin/dsh-shell-debug.exe .
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "built bin/dsh-shell-debug.exe"
