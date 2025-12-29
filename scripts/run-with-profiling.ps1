# Launch gocluster with pprof enabled and capture 1m CPU profiles every 15m.
# Usage: run this script from PowerShell; it will start the cluster in a new window
# and keep collecting profiles until the process exits.

$repoRoot        = "C:\src\gocluster"
$exePath         = Join-Path $repoRoot "gocluster.exe"
$configDir       = Join-Path $repoRoot "data\config"
$pprofAddr       = "localhost:6061"
$profileSeconds  = 60      # duration of each CPU profile
$intervalSeconds = 900     # time between captures
$logsDir         = Join-Path $repoRoot "logs"

# Ensure logs directory exists
New-Item -ItemType Directory -Path $logsDir -Force | Out-Null

# Env vars for the cluster
$envVars = @{
    DXC_CONFIG_PATH = $configDir
    DXC_PPROF_ADDR  = $pprofAddr
    # Uncomment to enable periodic heap logging (optional):
    # DXC_HEAP_LOG_INTERVAL = "60s"
}

# Start the cluster in a new window so the console UI stays visible.
$startInfo = New-Object System.Diagnostics.ProcessStartInfo
$startInfo.FileName = $exePath
$startInfo.WorkingDirectory = $repoRoot
$startInfo.UseShellExecute = $false  # required to apply EnvironmentVariables
$startInfo.Arguments = ""
foreach ($k in $envVars.Keys) { $startInfo.EnvironmentVariables[$k] = $envVars[$k] }

$proc = [System.Diagnostics.Process]::Start($startInfo)
if (-not $proc) { Write-Error "Failed to start gocluster"; exit 1 }

Write-Host "gocluster started (PID=$($proc.Id)); pprof at http://$pprofAddr"

# Wait for pprof to come up
$pprofUrl = "http://$pprofAddr/debug/pprof/"
$ready = $false
for ($i=0; $i -lt 15; $i++) {
    try {
        Invoke-WebRequest -Uri $pprofUrl -TimeoutSec 2 -UseBasicParsing | Out-Null
        $ready = $true
        break
    } catch { Start-Sleep -Seconds 2 }
}
if (-not $ready) { Write-Warning "pprof endpoint not reachable yet; proceeding anyway" }

function Get-CPUProfile {
    param($seconds, $destPath, $addr)
    $url = "http://$addr/debug/pprof/profile?seconds=$seconds"
    Invoke-WebRequest -Uri $url -OutFile $destPath -TimeoutSec ($seconds + 10) -UseBasicParsing
}

# Periodic capture loop (stops when the process exits)
while (-not $proc.HasExited) {
    $ts = Get-Date -Format "yyyyMMdd-HHmmss"
    $dest = Join-Path $logsDir ("cpu-$ts.pprof")
    try {
        Get-CPUProfile -seconds $profileSeconds -destPath $dest -addr $pprofAddr
        Write-Host "Captured CPU profile -> $dest"
    } catch {
        Write-Warning "Profile capture failed at ${ts}: $($_)"
    }
    # Sleep, but break early if the process exits
    for ($i=0; $i -lt $intervalSeconds -and -not $proc.HasExited; $i++) {
        Start-Sleep -Seconds 1
    }
}

Write-Host "gocluster exited; stopping capture loop."
