$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
$BastInstaller = $true

$Repo = "ellipse-software/bast"
$Channel = if ($env:BAST_INSTALL_CHANNEL -eq "nightly") { "nightly" } else { "stable" }
$InstallerUrl = if ($Channel -eq "nightly") { "https://bast.sh/install-nightly.ps1" } else { "https://bast.sh/install.ps1" }
$ExpectedPublisherSubject = "CN=ellipse Software Group Limited, O=ellipse Software Group Limited, STREET=45 Fitzroy St, L=London, C=GB, PostalCode=W1T 6EB"

function Write-Banner {
    Write-Host ""
    @(
        "██████╗  █████╗ ███████╗████████╗",
        "██╔══██╗██╔══██╗██╔════╝╚══██╔══╝",
        "██████╔╝███████║███████╗   ██║   ",
        "██╔══██╗██╔══██║╚════██║   ██║   ",
        "██████╔╝██║  ██║███████║   ██║   ",
        "╚═════╝ ╚═╝  ╚═╝╚══════╝   ╚═╝   "
    ) | ForEach-Object { Write-Host "  $_" -ForegroundColor Magenta }
    Write-Host ""
}

function Write-Info([string]$Message) {
    Write-Host "  $Message" -ForegroundColor DarkGray
}

function Write-Step([string]$Message) {
    Write-Host "  " -NoNewline
    Write-Host "→" -NoNewline -ForegroundColor Cyan
    Write-Host " $Message"
}

function Write-Success([string]$Message) {
    Write-Host "  " -NoNewline
    Write-Host "✓" -NoNewline -ForegroundColor Green
    Write-Host " $Message"
}

function Write-RunHint {
    Write-Host ""
    Write-Host "  Run " -NoNewline -ForegroundColor DarkGray
    Write-Host "PS> " -NoNewline -ForegroundColor DarkGray
    Write-Host "bast" -NoNewline -ForegroundColor Magenta
    Write-Host " to get started." -ForegroundColor DarkGray
}

function Send-Telemetry([string]$EventName, [string]$Version, [string]$Architecture) {
    if ($env:BAST_NO_TELEMETRY) {
        return
    }
    try {
        $payload = @{
            event = $EventName
            version = $Version
            os = "windows"
            arch = $Architecture
            source = "installer"
        } | ConvertTo-Json -Compress
        Invoke-WebRequest -UseBasicParsing -Method Post -Uri "https://bast.sh/api/telemetry" -ContentType "application/json" -Body $payload -TimeoutSec 5 | Out-Null
    } catch {
        # Installation must not fail when optional telemetry is unavailable.
    }
}

function Add-UserPath([string]$Directory) {
    $current = [Environment]::GetEnvironmentVariable("Path", "User")
    $parts = @($current -split ";" | Where-Object { $_ })
    if (-not ($parts | Where-Object { $_.TrimEnd("\") -ieq $Directory.TrimEnd("\") })) {
        $next = (@($parts) + $Directory) -join ";"
        [Environment]::SetEnvironmentVariable("Path", $next, "User")
    }
    if (-not (($env:Path -split ";") | Where-Object { $_.TrimEnd("\") -ieq $Directory.TrimEnd("\") })) {
        $env:Path = "$Directory;$env:Path"
    }
}

function Install-StagedBinary(
    [string]$Source,
    [string]$Destination,
    [string]$Receipt,
    [string]$Version,
    [string]$TemporaryDirectory
) {
    $backup = "$Destination.previous"
    Remove-Item $backup -Force -ErrorAction SilentlyContinue
    if (Test-Path $Destination) {
        Move-Item $Destination $backup -Force
    }
    try {
        Move-Item $Source $Destination -Force
        $installed = (& $Destination --version 2>$null) -join "`n"
        if ($installed -notmatch [regex]::Escape($Version)) {
            throw "installed binary did not report version $Version"
        }
        Set-Content -Path $Receipt -Value $InstallerUrl -NoNewline -Encoding ASCII
        Remove-Item $backup -Force -ErrorAction SilentlyContinue
    } catch {
        Remove-Item $Destination -Force -ErrorAction SilentlyContinue
        if (Test-Path $backup) {
            Move-Item $backup $Destination -Force
        }
        throw
    } finally {
        Remove-Item $TemporaryDirectory -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Write-Banner
Write-Info "Bast collects basic anonymous telemetry to help improve the tool."
Write-Info "Opt out with BAST_NO_TELEMETRY=1."
Write-Host ""

if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
    throw "This installer supports Windows 11. Use https://bast.sh/install on macOS or Linux."
}
$windows = Get-CimInstance -ClassName Win32_OperatingSystem
if ([int]$windows.ProductType -ne 1 -or [int]$windows.BuildNumber -lt 22000) {
    throw "Bast supports Windows 11. Windows 10 and Windows Server are not supported."
}

$osArchitecture = [string]$windows.OSArchitecture
$goArchitecture = if ($osArchitecture -match "ARM.*64") {
    "arm64"
} elseif ($osArchitecture -match "64") {
    "amd64"
} else {
    throw "Unsupported Windows architecture: $osArchitecture"
}

$installDirectory = if ($env:BAST_INSTALL_DIR) { $env:BAST_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "Programs\Bast" }
if ($installDirectory.Contains('"')) {
    throw "BAST_INSTALL_DIR cannot contain a quote"
}
$destination = Join-Path $installDirectory "bast.exe"
$receipt = "$destination.install-receipt"
$wasInstalled = Test-Path $destination

Write-Info "Platform: windows/$goArchitecture"
Write-Info "Install location: $destination"
Write-Host ""
Write-Step "Checking for the latest release..."

$headers = @{ Accept = "application/vnd.github+json"; "User-Agent" = "bast-installer" }
if ($Channel -eq "nightly") {
    if ($env:BAST_NIGHTLY_VERSION) {
        $version = $env:BAST_NIGHTLY_VERSION
    } else {
        $releases = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases?per_page=50" -Headers $headers -TimeoutSec 30
        $release = $releases | Where-Object { $_.prerelease -and -not $_.draft -and $_.tag_name -match '^nightly\.[0-9]{8}\.[0-9a-f]{7}$' } | Sort-Object published_at -Descending | Select-Object -First 1
        $version = [string]$release.tag_name
    }
    if ($version -notmatch '^nightly\.[0-9]{8}\.[0-9a-f]{7}$') {
        throw "No supported nightly release was found"
    }
} else {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers $headers -TimeoutSec 30
    $version = [string]$release.tag_name
    if ($version -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+$') {
        throw "The latest release has an unsupported version: $version"
    }
}
Write-Success "Latest release: $version"

$cleanVersion = $version.TrimStart("v")
$bundle = "bast_${cleanVersion}_windows_${goArchitecture}"
$archive = "$bundle.zip"
$assetBase = "https://github.com/$Repo/releases/download/$version"
if ($Channel -eq "stable") {
    $assetNames = @($release.assets | ForEach-Object { [string]$_.name })
    if ($assetNames -notcontains $archive) {
        throw "Bast $version does not provide a Windows $goArchitecture build. Windows builds begin with Bast v0.9.0."
    }
}

New-Item -ItemType Directory -Force $installDirectory | Out-Null

if ((Test-Path $destination) -and (Test-Path $receipt)) {
    $installedReceipt = (Get-Content $receipt -Raw -ErrorAction SilentlyContinue).Trim()
    $installedVersion = (& $destination --version 2>$null) -join "`n"
    if ($installedReceipt -eq $InstallerUrl -and $installedVersion -match [regex]::Escape($version)) {
        Add-UserPath $installDirectory
        Write-Host ""
        Write-Success "Bast $version is already up to date."
        Write-RunHint
        Send-Telemetry "up_to_date" $version $goArchitecture
        Write-Host ""
        return
    }
}

$temporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) ("bast-install-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory $temporaryDirectory | Out-Null
try {
    Write-Host ""
    if ($wasInstalled) {
        Write-Step "Updating Bast to $version..."
    } else {
        Write-Step "Installing Bast $version..."
    }
    Write-Step "Downloading $archive..."
    $archivePath = Join-Path $temporaryDirectory $archive
    $checksumPath = "$archivePath.sha256"
    Invoke-WebRequest -UseBasicParsing -Uri "$assetBase/$archive" -OutFile $archivePath -TimeoutSec 120
    Invoke-WebRequest -UseBasicParsing -Uri "$assetBase/$archive.sha256" -OutFile $checksumPath -TimeoutSec 30
    Write-Success "Download complete"

    Write-Step "Verifying SHA-256 checksum..."
    $checksumLine = (Get-Content $checksumPath -Raw).Trim()
    $checksumMatch = [regex]::Match($checksumLine, '^([0-9a-fA-F]{64})\s+(.+)$')
    if (-not $checksumMatch.Success -or $checksumMatch.Groups[2].Value.TrimStart('*') -ne $archive) {
        throw "Invalid checksum file"
    }
    $actualChecksum = (Get-FileHash $archivePath -Algorithm SHA256).Hash
    if ($actualChecksum -ine $checksumMatch.Groups[1].Value) {
        throw "Checksum verification failed"
    }
    Write-Success "Checksum verified"

    Expand-Archive -Path $archivePath -DestinationPath $temporaryDirectory
    $staged = Join-Path $temporaryDirectory "$bundle\bast.exe"
    Write-Step "Verifying Authenticode signature..."
    $signature = Get-AuthenticodeSignature $staged
    if ($signature.Status -ne "Valid" -or $signature.SignerCertificate.Subject -cne $ExpectedPublisherSubject) {
        throw "Authenticode verification failed"
    }
    Write-Success "Authenticode signature verified"

    if ($env:BAST_UPDATE_PARENT_PID) {
        $parentId = 0
        if (-not [int]::TryParse($env:BAST_UPDATE_PARENT_PID, [ref]$parentId) -or $parentId -le 0) {
            throw "Invalid update parent process"
        }
        $helperPath = Join-Path $temporaryDirectory "finish-update.ps1"
        $helperConfigPath = Join-Path $temporaryDirectory "finish-update.json"
        @{
            ParentId = $parentId
            Source = $staged
            Destination = $destination
            Receipt = $receipt
            InstallerUrl = $InstallerUrl
            Version = $version
            TemporaryDirectory = $temporaryDirectory
        } | ConvertTo-Json | Set-Content $helperConfigPath -Encoding Unicode
        @'
param($ConfigPath)
$ErrorActionPreference = "Stop"
$config = Get-Content $ConfigPath -Raw | ConvertFrom-Json
Wait-Process -Id $config.ParentId -ErrorAction SilentlyContinue
$backup = "$($config.Destination).previous"
Remove-Item $backup -Force -ErrorAction SilentlyContinue
if (Test-Path $config.Destination) { Move-Item $config.Destination $backup -Force }
try {
    Move-Item $config.Source $config.Destination -Force
    $installed = (& $config.Destination --version 2>$null) -join "`n"
    if ($installed -notmatch [regex]::Escape($config.Version)) { throw "installed binary did not report version $($config.Version)" }
    Set-Content -Path $config.Receipt -Value $config.InstallerUrl -NoNewline -Encoding ASCII
    Remove-Item $backup -Force -ErrorAction SilentlyContinue
} catch {
    Remove-Item $config.Destination -Force -ErrorAction SilentlyContinue
    if (Test-Path $backup) { Move-Item $backup $config.Destination -Force }
    $_ | Out-String | Set-Content (Join-Path (Split-Path $config.Destination) "bast-update-error.log")
    exit 1
} finally {
    Remove-Item $config.TemporaryDirectory -Recurse -Force -ErrorAction SilentlyContinue
}
'@ | Set-Content $helperPath -Encoding Unicode
        $arguments = "-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File `"$helperPath`" `"$helperConfigPath`""
        Start-Process -FilePath "powershell.exe" -ArgumentList $arguments -WindowStyle Hidden
        Add-UserPath $installDirectory
        Write-Success "Bast $version will finish installing after this process exits."
        Write-RunHint
        $temporaryDirectory = $null
    } else {
        Write-Step "Installing to $destination..."
        Install-StagedBinary $staged $destination $receipt $version $temporaryDirectory
        $temporaryDirectory = $null
        Add-UserPath $installDirectory
        if ($wasInstalled) {
            Write-Success "Updated Bast to $version."
            Send-Telemetry "update" $version $goArchitecture
        } else {
            Write-Success "Installed Bast $version."
            Send-Telemetry "install" $version $goArchitecture
        }
        Write-RunHint
        Write-Host ""
    }
} finally {
    if ($temporaryDirectory) {
        Remove-Item $temporaryDirectory -Recurse -Force -ErrorAction SilentlyContinue
    }
}
