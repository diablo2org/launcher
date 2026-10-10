# Signs Windows files with the launcher's code signing certificate, through
# SSL.com's eSigner cloud signing. The key never leaves SSL.com: CodeSignTool
# sends each file's hash to eSigner and writes the signature it returns, with
# a timestamp, so the signature outlives the certificate.
#
# Reads the eSigner account from the environment:
#   ES_USERNAME, ES_PASSWORD  the SSL.com account
#   ES_CREDENTIAL_ID          the certificate's eSigner credential
#   ES_TOTP_SECRET            the eSigner OTP secret, so no code is typed
#
# Every signing counts against the eSigner plan, so CI signs tagged releases
# only. NSIS calls this for the launcher, its uninstaller and the installer
# when LAUNCHER_SIGN is set (see nsis/project.nsi); CI calls it for d2pack.
#
#   pwsh -File build/windows/sign.ps1 bin/launcher.exe [more files]

param(
    [Parameter(Mandatory, ValueFromRemainingArguments)]
    [string[]]$Path
)

$ErrorActionPreference = "Stop"

# CodeSignTool from SSL.com's GitHub release, checked against this hash so a
# changed download is never given the account's password.
$Version = "1.3.2"
$ZipSHA256 = "4afc32e8b7f79bbe1de7e4e7049aaad4e0f754357613b9bbec0e3052f06fd36b"
$ZipURL = "https://github.com/SSLcom/CodeSignTool/releases/download/v$Version/CodeSignTool-v$Version-windows.zip"

foreach ($name in "ES_USERNAME", "ES_PASSWORD", "ES_CREDENTIAL_ID", "ES_TOTP_SECRET") {
    if (-not [Environment]::GetEnvironmentVariable($name)) {
        throw "$name is not set; signing needs the eSigner account (see the top of $PSCommandPath)"
    }
}

$Temp = if ($env:RUNNER_TEMP) { $env:RUNNER_TEMP } else { [IO.Path]::GetTempPath() }

# The download is kept between runs, but checked every time, and unpacked
# into a new folder for this run only: anything an earlier build step left
# in a tool folder is never run with the account's password.
$zip = Join-Path $Temp "CodeSignTool-$Version.zip"
$got = if (Test-Path $zip) { (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower() }
if ($got -ne $ZipSHA256) {
    Invoke-WebRequest $ZipURL -OutFile $zip -UseBasicParsing
    $got = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()
    if ($got -ne $ZipSHA256) {
        Remove-Item $zip
        throw "CodeSignTool download has SHA-256 $got, expected $ZipSHA256"
    }
}

$Tool = Join-Path $Temp ("CodeSignTool-" + [guid]::NewGuid())
Expand-Archive $zip -DestinationPath $Tool
$Jar = Join-Path $Tool "jar\code_sign_tool-$Version.jar"
$Java = Join-Path $Tool "jdk-11.0.2\bin\java.exe"

try {
    foreach ($p in $Path) {
        $file = (Resolve-Path $p).Path

        # CodeSignTool goes by the file's extension, and NSIS hands over the
        # uninstaller as a .tmp file, so anything else is signed as an .exe copy
        # and copied back.
        $work = $file
        if ([IO.Path]::GetExtension($file) -notin ".exe", ".dll") {
            $work = Join-Path $Temp ("sign-" + [guid]::NewGuid() + ".exe")
            Copy-Item $file $work
        }

        # CodeSignTool can report a failure and still exit 0, so the signature on
        # the file is what counts. eSigner may refuse a one-time code used moments
        # before, by the previous file, so a failure is tried once more after the
        # code has changed.
        for ($try = 1; ; $try++) {
            # It reads its settings from conf\ in the working folder.
            Push-Location $Tool
            try {
                # Arguments go to java.exe directly, not through the .bat, so a
                # password with characters cmd treats specially still arrives whole.
                & $Java -jar $Jar sign `
                    "-username=$env:ES_USERNAME" `
                    "-password=$env:ES_PASSWORD" `
                    "-credential_id=$env:ES_CREDENTIAL_ID" `
                    "-totp_secret=$env:ES_TOTP_SECRET" `
                    "-input_file_path=$work" `
                    "-override=true"
                $exit = $LASTEXITCODE
            }
            finally {
                Pop-Location
            }

            $sig = Get-AuthenticodeSignature $work
            if ($exit -eq 0 -and $sig.Status -eq "Valid" -and $sig.TimeStamperCertificate) {
                break
            }
            if ($try -ge 2) {
                throw "signing $file failed: CodeSignTool exited $exit, signature status $($sig.Status)$(if (-not $sig.TimeStamperCertificate) { ', no timestamp' })"
            }
            Write-Warning "signing $file didn't take (exit $exit, status $($sig.Status)); trying again with a new code"
            Start-Sleep -Seconds 31
        }

        if ($work -ne $file) {
            Copy-Item $work $file -Force
            Remove-Item $work
        }

        Write-Host "Signed $file as $($sig.SignerCertificate.Subject), timestamped by $($sig.TimeStamperCertificate.Subject)"
    }
}
finally {
    Remove-Item $Tool -Recurse -Force -ErrorAction SilentlyContinue
}

# NSIS checks the exit code of the command that calls this.
exit 0
