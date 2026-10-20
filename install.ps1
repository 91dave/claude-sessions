$ErrorActionPreference = 'Stop'

$repo = '91dave/claude-sessions'
$installDir = if ($env:CLAUDE_SESSIONS_INSTALL_DIR) { $env:CLAUDE_SESSIONS_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\claude-sessions' }
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$asset = "claude-sessions_windows_$arch.zip"
$url = "https://github.com/$repo/releases/latest/download/$asset"

$work = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid())
New-Item -ItemType Directory -Path $work | Out-Null
try {
    Write-Host "Downloading $url"
    Invoke-WebRequest -Uri $url -OutFile (Join-Path $work $asset) -UseBasicParsing
    Expand-Archive -Path (Join-Path $work $asset) -DestinationPath $work
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
    Copy-Item -Force (Join-Path $work 'claude-sessions.exe') $installDir
}
finally {
    Remove-Item -Recurse -Force $work
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $installDir) {
    [Environment]::SetEnvironmentVariable('Path', (($userPath.TrimEnd(';'), $installDir) -join ';'), 'User')
    Write-Host "Added $installDir to your user PATH. Open a new terminal to use it."
}

Write-Host "Installed claude-sessions to $installDir"
