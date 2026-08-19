$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
$BastInstaller = $true
$previousChannel = $env:BAST_INSTALL_CHANNEL
$env:BAST_INSTALL_CHANNEL = "nightly"
try {
    $installer = (Invoke-WebRequest -UseBasicParsing -Uri "https://bast.sh/install.ps1" -TimeoutSec 30).Content
    & ([ScriptBlock]::Create($installer))
} finally {
    $env:BAST_INSTALL_CHANNEL = $previousChannel
}
