param(
    [int]$Port = 8090,
    [string]$Passcode = "",
    [string]$BindHost = "127.0.0.1"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$backendDir = Join-Path $root "backend"
$exe = Join-Path $backendDir "psychgrader.exe"

# Load simple KEY=VALUE pairs from .env (comments and blank lines ignored).
$envFile = Join-Path $root ".env"
if (Test-Path $envFile) {
    Get-Content $envFile | ForEach-Object {
        $line = $_.Trim()
        if ($line -eq "" -or $line.StartsWith("#")) { return }
        if ($line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)\s*$') {
            $name = $matches[1]
            $value = $matches[2].Trim().Trim('"').Trim("'")
            if (-not [Environment]::GetEnvironmentVariable($name, "Process")) {
                [Environment]::SetEnvironmentVariable($name, $value, "Process")
            }
        }
    }
}

# Stop existing instance on target port if running
Get-Process -Name "psychgrader" -ErrorAction SilentlyContinue | Stop-Process -Force

$env:HOST = $BindHost
$env:PORT = "$Port"
if ($Passcode) { $env:ADMIN_PASSCODE = $Passcode }
if (-not $env:APP_SECRET -or $env:APP_SECRET -eq "replace-with-a-random-secret" -or $env:APP_SECRET.Length -lt 32) {
    # Local runs get a process-local random secret; tokens expire on restart.
    $env:APP_SECRET = ([guid]::NewGuid().ToString("N") + [guid]::NewGuid().ToString("N"))
}
if (-not $env:ADMIN_PASSCODE) {
    if ($BindHost -notin @("127.0.0.1", "localhost", "::1")) {
        # Fail-closed policy allows non-loopback only with auth enabled.
        $env:ADMIN_PASSCODE = "pg-" + [guid]::NewGuid().ToString("N").Substring(0, 20)
        Write-Host "Generated ADMIN_PASSCODE: $($env:ADMIN_PASSCODE)"
    } else {
        Write-Warning "ADMIN_PASSCODE is empty. The server binds 127.0.0.1 only; set it before exposing the app."
    }
}
$env:DB_PATH = Join-Path $backendDir "data\psychgrader.db"
$env:STATIC_DIR = Join-Path $root "frontend\dist"

# Start-Process inherits the current process environment (HOST, PORT,
# ADMIN_PASSCODE, SECRET, DB_PATH, STATIC_DIR); WMI Win32_Process does not.
$proc = Start-Process -FilePath $exe -WorkingDirectory $backendDir -WindowStyle Hidden -PassThru
if (-not $proc) {
    throw "Failed to launch psychgrader.exe"
}

Start-Sleep -Milliseconds 800
Write-Host "PsychGrader started (PID=$($proc.Id), HOST=$BindHost) -> http://127.0.0.1:$Port"
if ($BindHost -eq "0.0.0.0") {
    $lan = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        Where-Object { $_.IPAddress -notlike "127.*" -and $_.IPAddress -notlike "169.254.*" } |
        Select-Object -First 1 -ExpandProperty IPAddress
    if ($lan) { Write-Host "LAN URL: http://${lan}:$Port" }
}
