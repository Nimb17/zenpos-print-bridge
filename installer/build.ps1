# =============================================================================
#  Mivy Print Bridge — installer build script
#
#  Usage:
#      .\installer\build.ps1
#      .\installer\build.ps1 -Version 1.2.0
#      .\installer\build.ps1 -SignCert "C:\certs\mycert.pfx" -SignPassword "…"
#
#  What it does:
#      1. Compiles mivy-bridge.exe with ldflags -s -w (strip symbols)
#      2. (Optional) signs the binary with signtool + your code-signing cert
#      3. Runs Inno Setup (iscc.exe) to build the installer
#      4. (Optional) signs the installer too
#      5. Drops the result in .\build\
#
#  Requirements:
#      - Go 1.22+  (https://go.dev)
#      - Inno Setup 6.2+  (https://jrsoftware.org/isdl.php)
#          Default install path is auto-detected.
#      - (Optional) Windows SDK signtool.exe for code signing
# =============================================================================

[CmdletBinding()]
param(
    [string]$Version        = "1.0.0",
    [string]$ISCC           = "",
    [string]$SignCert       = "",
    [string]$SignPassword   = "",
    [string]$TimestampUrl   = "http://timestamp.digicert.com",
    [switch]$SkipGoBuild
)

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path "$PSScriptRoot\..").Path
Push-Location $repoRoot
try {
    Write-Host "==> Mivy Print Bridge v$Version" -ForegroundColor Cyan

    # --- 1. Build Go binary ---------------------------------------------------
    if (-not $SkipGoBuild) {
        Write-Host "--> Compiling mivy-bridge.exe" -ForegroundColor Yellow
        $env:CGO_ENABLED = "0"
        $env:GOOS = "windows"
        $env:GOARCH = "amd64"
        $ldflags = "-s -w -X main.version=$Version"
        & go build -trimpath -ldflags="$ldflags" -o mivy-bridge.exe .
        if ($LASTEXITCODE -ne 0) { throw "go build failed" }
    } else {
        Write-Host "--> Skipping Go build (using existing mivy-bridge.exe)" -ForegroundColor DarkGray
    }

    if (-not (Test-Path ".\mivy-bridge.exe")) {
        throw "mivy-bridge.exe not found in $repoRoot"
    }

    # --- 2. Sign the binary (optional) ---------------------------------------
    if ($SignCert) {
        Write-Host "--> Signing mivy-bridge.exe" -ForegroundColor Yellow
        $signtool = (Get-Command signtool.exe -ErrorAction SilentlyContinue)?.Source
        if (-not $signtool) {
            $candidates = Get-ChildItem "C:\Program Files (x86)\Windows Kits\10\bin" -Recurse -Filter signtool.exe -ErrorAction SilentlyContinue |
                          Where-Object { $_.FullName -match 'x64\\signtool\.exe$' } |
                          Sort-Object LastWriteTime -Descending
            if ($candidates) { $signtool = $candidates[0].FullName }
        }
        if (-not $signtool) { throw "signtool.exe not found. Install the Windows 10/11 SDK." }
        & $signtool sign /f $SignCert /p $SignPassword /fd SHA256 /tr $TimestampUrl /td SHA256 .\mivy-bridge.exe
        if ($LASTEXITCODE -ne 0) { throw "signtool failed on mivy-bridge.exe" }
    }

    # --- 3. Locate Inno Setup compiler ---------------------------------------
    if (-not $ISCC) {
        $candidates = @(
            "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
            "${env:ProgramFiles}\Inno Setup 6\ISCC.exe"
        )
        $ISCC = $candidates | Where-Object { Test-Path $_ } | Select-Object -First 1
    }
    if (-not $ISCC -or -not (Test-Path $ISCC)) {
        throw "ISCC.exe not found. Install Inno Setup 6 from https://jrsoftware.org/isdl.php or pass -ISCC 'C:\path\to\ISCC.exe'"
    }
    Write-Host "--> Using Inno Setup: $ISCC" -ForegroundColor DarkGray

    # --- 4. Build installer ---------------------------------------------------
    Write-Host "--> Building installer" -ForegroundColor Yellow
    New-Item -ItemType Directory -Force -Path .\build | Out-Null
    & $ISCC "/DAppVersion=$Version" ".\installer\setup.iss"
    if ($LASTEXITCODE -ne 0) { throw "Inno Setup compiler failed" }

    $installer = Get-ChildItem ".\build\Mivy-PrintBridge-Setup-$Version.exe" -ErrorAction SilentlyContinue
    if (-not $installer) { throw "Installer not produced in .\build" }

    # --- 5. Sign installer (optional) ----------------------------------------
    if ($SignCert) {
        Write-Host "--> Signing installer" -ForegroundColor Yellow
        & $signtool sign /f $SignCert /p $SignPassword /fd SHA256 /tr $TimestampUrl /td SHA256 $installer.FullName
        if ($LASTEXITCODE -ne 0) { throw "signtool failed on installer" }
    }

    Write-Host ""
    Write-Host "==> Done." -ForegroundColor Green
    Write-Host "    $($installer.FullName)"
    Write-Host "    Size: $([Math]::Round($installer.Length / 1MB, 2)) MB"
}
finally {
    Pop-Location
}
