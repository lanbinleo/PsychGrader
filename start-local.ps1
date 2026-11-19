param(
    [int]$Port = 8090,
    [string]$Passcode = ""
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

$env:HOST = "127.0.0.1"
$env:PORT = "$Port"
if ($Passcode) { $env:ADMIN_PASSCODE = $Passcode }
if (-not $env:ADMIN_PASSCODE) {
    Write-Warning "ADMIN_PASSCODE is empty. The server binds 127.0.0.1 only; set it before exposing the app."
}
$env:DB_PATH = Join-Path $backendDir "data\psychgrader.db"
$env:STATIC_DIR = Join-Path $root "frontend\dist"

$cmdLine = "`"$exe`""
$res = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{
    CommandLine = $cmdLine
    CurrentDirectory = $backendDir
}

if ($res.ReturnValue -ne 0) {
    throw "Failed to launch psychgrader.exe (ReturnValue=$($res.ReturnValue))"
}

Start-Sleep -Milliseconds 800
Write-Host "PsychGrader started (PID=$($res.ProcessId)) -> http://127.0.0.1:$Port"
