# Install qmeter, a CLI tool to see your AI subscription usage limits.
#
#   irm https://raw.githubusercontent.com/Harrison-Blair/qmeter/main/install.ps1 | iex
#
# Downloads the release archive for this machine from GitHub, verifies its
# SHA-256 against the release's checksums.txt, and unpacks the binary into
# %LOCALAPPDATA%\Programs\qmeter, adding that directory to the user PATH when
# it is not already there. It never needs an elevated prompt.
#
# Environment:
#   QMETER_INSTALL_DIR   where to put the binary (default: %LOCALAPPDATA%\Programs\qmeter)
#   QMETER_VERSION       release tag to install (default: the latest release)
#   QMETER_BASE_URL      repository URL (default: the qmeter repository)
#
# This is the Windows counterpart of install.sh and follows the same steps in
# the same order. Written for Windows PowerShell 5.1 and later.

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
# Invoke-WebRequest in Windows PowerShell crawls while it draws a progress bar.
$ProgressPreference = 'SilentlyContinue'

function Die([string]$Message) {
    # Throw rather than exit: this script is normally run through `iex`, where
    # exit would close the user's shell.
    throw "install.ps1: $Message"
}

function Say([string]$Message) {
    Write-Host $Message
}

$DefaultBaseUrl = 'https://github.com/Harrison-Blair/qmeter'

$BaseUrl = if ($env:QMETER_BASE_URL) { $env:QMETER_BASE_URL } else { $DefaultBaseUrl }
$BaseUrl = $BaseUrl.TrimEnd('/')
$Version = if ($env:QMETER_VERSION) { $env:QMETER_VERSION } else { '' }
$InstallDir = if ($env:QMETER_INSTALL_DIR) {
    $env:QMETER_INSTALL_DIR
} else {
    Join-Path $env:LOCALAPPDATA 'Programs\qmeter'
}

# Windows PowerShell 5.1 still defaults to TLS 1.0, which github.com refuses.
try {
    [Net.ServicePointManager]::SecurityProtocol =
        [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
} catch {
    # Nothing to do: a runtime that has no Tls12 member already defaults higher.
}

# Architecture, the counterpart of `uname -m` in install.sh.
$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { '' }
}
if (-not $Arch) {
    Die "unsupported architecture: $($env:PROCESSOR_ARCHITECTURE) (qmeter ships amd64 and arm64 builds)"
}

function Get-HeaderValue($Headers, [string]$Name) {
    # Headers are a string dictionary in 5.1 and a string[] dictionary in 7+.
    if (-not $Headers) { return $null }
    $value = $null
    try { $value = $Headers[$Name] } catch { $value = $null }
    if ($value -is [array]) { $value = $value[0] }
    return $value
}

function Get-LatestTag([string]$Url) {
    # /releases/latest answers with a redirect to /releases/tag/<tag>; reading
    # the Location header keeps this off the rate-limited REST API. With
    # -MaximumRedirection 0 some PowerShell versions return the 3xx response
    # and others raise it, so read the header out of either one.
    $location = $null
    try {
        $response = Invoke-WebRequest -Uri $Url -MaximumRedirection 0 -UseBasicParsing
        $location = Get-HeaderValue $response.Headers 'Location'
    } catch {
        $response = $null
        try { $response = $_.Exception.Response } catch { $response = $null }
        if ($response) {
            $location = Get-HeaderValue $response.Headers 'Location'
        }
    }
    if (-not $location) { return '' }
    $marker = '/releases/tag/'
    $index = $location.IndexOf($marker)
    if ($index -lt 0) { return '' }
    $tag = $location.Substring($index + $marker.Length)
    return ($tag -split '[/?#]')[0]
}

if (-not $Version) {
    $Version = Get-LatestTag "$BaseUrl/releases/latest"
    if (-not $Version) {
        Die "no qmeter release found at $BaseUrl/releases/latest -- if a release exists, set QMETER_VERSION to its tag, for example v0.1.0"
    }
}

$Asset = "qmeter_${Version}_windows_${Arch}.zip"
$AssetUrl = "$BaseUrl/releases/download/$Version/$Asset"
$ChecksumsUrl = "$BaseUrl/releases/download/$Version/checksums.txt"

$Temp = Join-Path ([System.IO.Path]::GetTempPath()) ("qmeter-install-" + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $Temp -Force | Out-Null

try {
    Say "Installing qmeter $Version (windows/$Arch)"

    $ArchivePath = Join-Path $Temp $Asset
    $ChecksumsPath = Join-Path $Temp 'checksums.txt'
    try {
        Invoke-WebRequest -Uri $AssetUrl -OutFile $ArchivePath -UseBasicParsing
        Invoke-WebRequest -Uri $ChecksumsUrl -OutFile $ChecksumsPath -UseBasicParsing
    } catch {
        Die "could not download $AssetUrl -- check the tag $Version exists and that you are online"
    }

    # checksums.txt is sha256sum format: "<hex>  <filename>".
    $want = ''
    foreach ($line in (Get-Content -LiteralPath $ChecksumsPath)) {
        $fields = ($line.Trim() -split '\s+')
        if ($fields.Count -ge 2 -and $fields[1].TrimStart('*') -eq $Asset) {
            $want = $fields[0]
            break
        }
    }
    if (-not $want) { Die "checksums.txt for $Version has no entry for $Asset" }

    $got = (Get-FileHash -LiteralPath $ArchivePath -Algorithm SHA256).Hash
    if ($want.ToLowerInvariant() -ne $got.ToLowerInvariant()) {
        Die "checksum mismatch for ${Asset}: expected $want, got $got -- refusing to install"
    }

    $Unpacked = Join-Path $Temp 'unpacked'
    Expand-Archive -LiteralPath $ArchivePath -DestinationPath $Unpacked -Force
    $Binary = Join-Path $Unpacked 'qmeter.exe'
    if (-not (Test-Path -LiteralPath $Binary)) {
        Die "$Asset did not contain qmeter.exe"
    }

    if (-not (Test-Path -LiteralPath $InstallDir)) {
        try {
            New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        } catch {
            Die "could not create $InstallDir -- set QMETER_INSTALL_DIR to a directory you own"
        }
    }
    $probe = Join-Path $InstallDir ('.qmeter-write-test-' + [System.Guid]::NewGuid().ToString('N'))
    try {
        New-Item -ItemType File -Path $probe -Force | Out-Null
        Remove-Item -LiteralPath $probe -Force
    } catch {
        Die "$InstallDir is not writable -- set QMETER_INSTALL_DIR to a directory you own (this script never needs an elevated prompt)"
    }

    $Target = Join-Path $InstallDir 'qmeter.exe'
    Copy-Item -LiteralPath $Binary -Destination $Target -Force
    Say "Installed $Target"

    # Add the install directory to the user PATH only when it is missing.
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not $userPath) { $userPath = '' }
    $onUserPath = $false
    foreach ($entry in ($userPath -split ';')) {
        if ($entry.Trim().TrimEnd('\') -and
            $entry.Trim().TrimEnd('\').ToLowerInvariant() -eq $InstallDir.TrimEnd('\').ToLowerInvariant()) {
            $onUserPath = $true
            break
        }
    }
    if (-not $onUserPath) {
        $newPath = if ($userPath.TrimEnd(';')) { $userPath.TrimEnd(';') + ';' + $InstallDir } else { $InstallDir }
        [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        Say ''
        Say "Added $InstallDir to your user PATH. Open a new terminal for it to take effect."
        Say ''
    }
    # Make qmeter runnable in this session either way.
    $env:Path = $env:Path.TrimEnd(';') + ';' + $InstallDir

    & $Target version
} finally {
    Remove-Item -LiteralPath $Temp -Recurse -Force -ErrorAction SilentlyContinue
}
