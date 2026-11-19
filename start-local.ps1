param(
    [int]$Port = 8090,
    [string]$Passcode = ""
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$backendDir = Join-Path $root "backend"
$exe = Join-Path $backendDir "psychgrader.exe"

# Stop existing instance on target port if running
Get-Process -Name "psychgrader" -ErrorAction SilentlyContinue | Stop-Process -Force

$env:PORT = "$Port"
$env:ADMIN_PASSCODE = $Passcode
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
