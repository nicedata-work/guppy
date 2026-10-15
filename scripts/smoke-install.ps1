# Installer smoke test for Windows — the Phase 6 cross-platform matrix cell.
# Proves the full flow against a local tarball server:
#
#   release artifacts → install.ps1 (checksum-verified) → guppy.exe --help
#   → update replaces the binary → corrupt release is refused (exit 3,
#     binary untouched) → uninstall removes binary + data + cache
#
# Mirrors scripts/smoke-install.sh (the POSIX cell). Usage:
#   pwsh scripts/smoke-install.ps1          (CI: windows-latest + setup-go)

$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
$ServeDir = Join-Path ([System.IO.Path]::GetTempPath()) ("guppy-smoke-" + [System.Guid]::NewGuid().ToString("N").Substring(0, 8))
$InstallDir = Join-Path $ServeDir "install-bin"
$Server = $null

function Cleanup {
    if ($Server -and -not $Server.HasExited) { Stop-Process -Id $Server.Id -Force -ErrorAction SilentlyContinue }
    Remove-Item -Recurse -Force $ServeDir -ErrorAction SilentlyContinue
}
trap { Cleanup; throw }
Cleanup # remove any leftover from a previous run

New-Item -ItemType Directory -Force -Path $ServeDir | Out-Null

# --- 1. build release artifacts (A = pinned install, B = update target, ----
#        C = corrupt-checksum release used to prove the rollback path) ------

function Build-Release([string]$Version, [string]$Dist) {
    New-Item -ItemType Directory -Force -Path $Dist | Out-Null
    $env:CGO_ENABLED = "0"; $env:GOOS = "windows"; $env:GOARCH = "amd64"
    go build -trimpath -ldflags "-s -w -X github.com/nicedata-work/guppy/internal/version.Version=$Version" `
        -o (Join-Path $Dist "guppy.exe") (Join-Path $Root "cmd\guppy")
    if ($LASTEXITCODE -ne 0) { throw "build failed for $Version" }
    tar -czf (Join-Path $Dist "guppy-$Version-windows-amd64.tar.gz") -C $Dist guppy.exe
    if ($LASTEXITCODE -ne 0) { throw "pack failed for $Version" }
    Remove-Item (Join-Path $Dist "guppy.exe")
    $Hash = (Get-FileHash -Algorithm SHA256 (Join-Path $Dist "guppy-$Version-windows-amd64.tar.gz")).Hash.ToLower()
    "$Hash  guppy-$Version-windows-amd64.tar.gz" | Set-Content -NoNewline (Join-Path $Dist "guppy-$Version-windows-amd64.tar.gz.sha256")
}

Write-Host "smoke: building release artifacts"
Build-Release "0.1.0" (Join-Path $ServeDir "build-a")
Build-Release "0.1.1" (Join-Path $ServeDir "build-b")
Build-Release "0.1.2" (Join-Path $ServeDir "build-c")
foreach ($sub in "build-a", "build-b", "build-c") {
    Get-ChildItem (Join-Path $ServeDir $sub) | Copy-Item -Destination $ServeDir
}
# latest.txt points at the newest *uncorrupt* version.
"0.1.1" | Set-Content -NoNewline (Join-Path $ServeDir "latest.txt")
# Corrupt C's checksum: update --version 0.1.2 must be refused.
"0000000000000000000000000000000000000000000000000000000000000000  guppy-0.1.2-windows-amd64.tar.gz" |
    Set-Content -NoNewline (Join-Path $ServeDir "guppy-0.1.2-windows-amd64.tar.gz.sha256")

# --- 2. local tarball server ----------------------------------------------

$Port = Get-Random -Minimum 20000 -Maximum 40000
$Server = Start-Process python -ArgumentList "-m", "http.server", "$Port", "--directory", "$ServeDir", "--bind", "127.0.0.1" -PassThru -WindowStyle Hidden
$Base = "http://127.0.0.1:$Port"
Start-Sleep -Seconds 2

# --- 3. the assertions ------------------------------------------------------

$env:GUPPY_INSTALL_URL = $Base
$env:GUPPY_RELEASE_BASE = $Base
$env:GUPPY_INSTALL_DIR = $InstallDir
$env:GUPPY_NO_MODIFY_PATH = "1"
$Guppy = Join-Path $InstallDir "guppy.exe"

Write-Host "--- install pinned 0.1.0 (install.ps1) ---"
& (Join-Path $Root "install\install.ps1") -Version "0.1.0"
if (-not (Test-Path $Guppy)) { throw "FAIL: install.ps1 did not produce $Guppy" }
$V = & $Guppy --version
if ("$V" -notmatch "0\.1\.0") { throw "FAIL: version = $V, want 0.1.0" }
Write-Host "guppy --version: $V"

Write-Host "--- update -> 0.1.1 ---"
& $Guppy update
if ($LASTEXITCODE -ne 0) { throw "FAIL: update exited $LASTEXITCODE" }
$V = & $Guppy --version
if ("$V" -notmatch "0\.1\.1") { throw "FAIL: update did not replace binary (got $V)" }

Write-Host "--- update rollback: corrupt checksum for 0.1.2 ---"
& $Guppy update --version "0.1.2"
if ($LASTEXITCODE -ne 3) { throw "FAIL: corrupt update exited $LASTEXITCODE, want 3 (network/registry)" }
$V = & $Guppy --version
if ("$V" -notmatch "0\.1\.1") { throw "FAIL: binary changed despite checksum failure (got $V)" }

Write-Host "--- uninstall + cleanup verification ---"
& $Guppy uninstall --yes --purge
if ($LASTEXITCODE -ne 0) { throw "FAIL: uninstall exited $LASTEXITCODE" }
if (Test-Path $Guppy) { throw "FAIL: binary still present" }
$DataDir = if ($env:LOCALAPPDATA) { Join-Path $env:LOCALAPPDATA "guppy" } else { $null }
if ($DataDir -and (Test-Path $DataDir)) { throw "FAIL: data dir still present: $DataDir" }

Write-Host "SMOKE OK"
Cleanup
