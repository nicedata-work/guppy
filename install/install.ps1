<#
.SYNOPSIS
  guppy installer for Windows PowerShell — https://nicedata.work/guppy

.DESCRIPTION
  irm https://nicedata.work/guppy/install.ps1 | iex

  Downloads the checksummed release tarball, verifies it, and installs the
  binary to %USERPROFILE%\.local\bin\guppy.exe (design §10.1). Unattended
  safe: no prompts.

.PARAMETER Version
  Install a specific version instead of latest.txt.

.PARAMETER InstallDir
  Install directory (default %USERPROFILE%\.local\bin).

.PARAMETER NoModifyPath
  Do not add the install directory to the user PATH.

.ENVIRONMENT
  GUPPY_INSTALL_URL    release base URL (default https://github.com/nicedata-work/guppy/releases/latest/download)
  GUPPY_VERSION        pin a version
  GUPPY_NO_MODIFY_PATH set to 1 to skip PATH modification
#>

param(
    [string]$Version = $env:GUPPY_VERSION,
    [string]$InstallDir = $env:GUPPY_INSTALL_DIR,
    [switch]$NoModifyPath
)

$ErrorActionPreference = "Stop"

$BaseUrl = if ($env:GUPPY_INSTALL_URL) { $env:GUPPY_INSTALL_URL } else { "https://github.com/nicedata-work/guppy/releases/latest/download" }
if (-not $InstallDir) { $InstallDir = Join-Path $env:USERPROFILE ".local\bin" }
if ($env:GUPPY_NO_MODIFY_PATH -eq "1") { $NoModifyPath = $true }

function Log($msg) { Write-Host "guppy-installer: $msg" }

# --- platform detection -----------------------------------------------------

$Os = "windows"
switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { $Arch = "amd64" }
    "ARM64" { $Arch = "arm64" }
    default { throw "guppy-installer: unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

# --- download + verify ------------------------------------------------------

$WorkDir = New-TemporaryFile | ForEach-Object { Remove-Item $_; New-Item -ItemType Directory -Path $_ }
try {
    if (-not $Version) {
        $Version = (Invoke-WebRequest -UseBasicParsing "$BaseUrl/latest.txt").Content.Trim()
        if (-not $Version) { throw "guppy-installer: latest.txt at $BaseUrl is empty" }
    }

    $Name = "guppy-$Version-$Os-$Arch.tar.gz"
    $Tarball = Join-Path $WorkDir $Name
    Log "downloading $Name"
    Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/$Name" -OutFile $Tarball
    Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/$Name.sha256" -OutFile "$Tarball.sha256"

    $Expected = ((Get-Content "$Tarball.sha256" -Raw).Trim() -split "\s+")[0].ToLower()
    $Actual = (Get-FileHash -Algorithm SHA256 $Tarball).Hash.ToLower()
    if ($Actual -ne $Expected) { throw "guppy-installer: checksum mismatch for $Name (got $Actual, want $Expected)" }
    Log "checksum verified"

    # Windows 10+ ships bsdtar; it reads .tar.gz natively.
    tar -xzf $Tarball -C $WorkDir
    if ($LASTEXITCODE -ne 0) { throw "guppy-installer: unpack failed" }

    $Source = Join-Path $WorkDir "guppy.exe"
    if (-not (Test-Path $Source)) { throw "guppy-installer: tarball does not contain guppy.exe" }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Copy-Item $Source (Join-Path $InstallDir "guppy.exe") -Force
    Log "guppy $Version installed to $(Join-Path $InstallDir 'guppy.exe')"

    # --- PATH modification --------------------------------------------------

    if (-not $NoModifyPath) {
        $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
        if (-not $UserPath) { $UserPath = "" }
        $Parts = $UserPath -split ";" | Where-Object { $_ }
        if ($Parts -notcontains $InstallDir) {
            [Environment]::SetEnvironmentVariable("Path", ($UserPath.TrimEnd(";") + ";$InstallDir"), "User")
            Log "added $InstallDir to your user PATH (restart your terminal)"
        }
    }

    Log "next steps: guppy --help  ·  guppy new <project>"
}
finally {
    Remove-Item -Recurse -Force $WorkDir -ErrorAction SilentlyContinue
}
