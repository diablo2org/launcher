# Local end-to-end environment. Builds three test servers from a
# working SlashDiablo install, serves them over HTTPS on 127.0.0.1:8667, and
# starts the launcher pointed at them with its own data folder and a sandbox
# Diablo II folder made of hard links to your real archives.
#
#   .\testenv\dev.ps1 -Source "C:\Games\Diablo II"
#   .\testenv\dev.ps1 -Source "C:\Games\Diablo II" -Art ..\slashdiablo-launcher\qml\assets
#   .\testenv\dev.ps1 -Source ... -NoRun          build and serve only
#
# The three servers are SlashDiablo, Resurgence and Diablo 09. The last two are
# dummy profiles: real names, links and art, but made-up gateways, and all
# three install SlashDiablo's files.
#
# -Art takes the old launcher's qml\assets folder, for SlashDiablo's key art.
# Resurgence's and Diablo 09's art is downloaded from their public sites into
# testenv\.dev\art on the first run; it is theirs, so it stays out of the
# repository. Everything generated lives in testenv\.dev (gitignored). Your
# Diablo II folder is only read. Pressing Play does write the Battle.net
# registry values, as a real launch would.

param(
    [Parameter(Mandatory = $true)][string]$Source,
    [string]$Art = "",
    [switch]$NoRun
)

$ErrorActionPreference = "Stop"

# Use a Go from %USERPROFILE%\sdk if none is on PATH, and make sure a GOROOT
# left over from an older Go setup doesn't get in the way.
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    $sdk = Get-ChildItem "$env:USERPROFILE\sdk\go*\bin\go.exe" -ErrorAction SilentlyContinue | Sort-Object FullName | Select-Object -Last 1
    if ($sdk) { $env:PATH = "$($sdk.DirectoryName);$env:PATH" }
}
$env:GOROOT = ""
$env:GO111MODULE = "on"
$env:PATH = "$env:USERPROFILE\go\bin;$env:PATH"

$Root = Split-Path -Parent $PSScriptRoot
$Dev = Join-Path $PSScriptRoot ".dev"
$Port = 8667

Push-Location $Root
try {
    New-Item -ItemType Directory -Force $Dev | Out-Null

    go build -o "$Dev\devsite.exe" .\cmd\devsite
    go build -o "$Dev\devserve.exe" .\cmd\devserve
    if ($LASTEXITCODE -ne 0) { throw "build failed" }

    # Other servers' published art, fetched once. Failing to fetch just means
    # those servers show without art.
    $ArtDir = Join-Path $Dev "art"
    New-Item -ItemType Directory -Force $ArtDir | Out-Null
    $ThirdPartyArt = @{
        "resurgence-logo.png" = "https://raw.githubusercontent.com/DoctorWoot420/resurgence-launcher/develop/qml/assets/resurgence-logo.png"
        "resurgence-bg.png"   = "https://raw.githubusercontent.com/DoctorWoot420/resurgence-launcher/develop/qml/assets/resurgence-bg3.png"
        "diablo09-og.jpg"     = "https://diablo09.com/images/og-image.jpg"
    }
    foreach ($name in $ThirdPartyArt.Keys) {
        $file = Join-Path $ArtDir $name
        if (-not (Test-Path $file)) {
            try { Invoke-WebRequest $ThirdPartyArt[$name] -OutFile $file -UseBasicParsing }
            catch { Write-Warning "Couldn't fetch $name; that server will show without art." }
        }
    }

    & "$Dev\devsite.exe" -source $Source -art $Art -extra-art $ArtDir -out $Dev -url "https://127.0.0.1:$Port"
    if ($LASTEXITCODE -ne 0) { throw "devsite failed" }

    Get-Process devserve -ErrorAction SilentlyContinue | Stop-Process -ErrorAction SilentlyContinue
    $Server = Start-Process -FilePath "$Dev\devserve.exe" `
        -ArgumentList "-dir `"$Dev\site`" -cert `"$Dev\devserve.pem`" -addr 127.0.0.1:$Port" `
        -PassThru -WindowStyle Minimized
    Start-Sleep 1
    Write-Host "Serving $Dev\site at https://127.0.0.1:$Port (pid $($Server.Id))"

    if ($NoRun) { return }

    wails3 build
    if ($LASTEXITCODE -ne 0) { throw "launcher build failed" }

    $env:LAUNCHER_LISTING_DIR = "$Dev\listing"
    $env:LAUNCHER_DEV_CA = "$Dev\devserve.pem"
    $env:LAUNCHER_DATA_DIR = "$Dev\data"
    & "$Root\bin\launcher.exe"
}
finally {
    Pop-Location
}
