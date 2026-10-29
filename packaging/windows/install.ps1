# Builds torrenty and installs it for the current user: the exe in
# %LOCALAPPDATA%\Programs\torrenty, that folder on the user PATH, and a
# Start Menu shortcut with the torrenty icon.
#
# Skips the build when the installed exe is newer than every source file,
# unless -Force. -Hook prints a one-line JSON systemMessage for Claude Code.
param([switch]$Force, [switch]$Hook)

$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$dir = Join-Path $env:LOCALAPPDATA 'Programs\torrenty'
$exe = Join-Path $dir 'torrenty.exe'
$ico = Join-Path $dir 'torrenty.ico'

function Say($msg) {
    if ($Hook) { @{ systemMessage = $msg } | ConvertTo-Json -Compress } else { $msg }
}

$sources = Get-ChildItem $repo -Recurse -File -Include *.go, go.mod, go.sum, *.syso, *.ico |
    Where-Object { $_.FullName -notlike '*\.git\*' }
$newest = ($sources | Measure-Object LastWriteTime -Maximum).Maximum
if (-not $Force -and (Test-Path $exe) -and (Get-Item $exe).LastWriteTime -ge $newest) {
    exit 0
}

if (Get-Process torrenty -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $exe }) {
    Say 'torrenty not reinstalled: it is running. Close it and run packaging\windows\install.ps1.'
    exit 0
}

New-Item -ItemType Directory -Force $dir | Out-Null
$env:GOTOOLCHAIN = 'local'
$env:CGO_ENABLED = '0'
Push-Location $repo
try {
    $out = & go build -trimpath -ldflags '-s -w' -o $exe ./cmd/torrenty 2>&1
    if ($LASTEXITCODE -ne 0) {
        Say "torrenty build failed: $($out -join ' ')"
        exit 0
    }
} finally { Pop-Location }

Copy-Item (Join-Path $repo 'packaging\windows\torrenty.ico') $ico -Force

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $dir) {
    [Environment]::SetEnvironmentVariable('Path', ($userPath.TrimEnd(';') + ';' + $dir), 'User')
}

$lnk = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\torrenty.lnk'
$s = (New-Object -ComObject WScript.Shell).CreateShortcut($lnk)
$s.TargetPath = $exe
$s.WorkingDirectory = $env:USERPROFILE
$s.IconLocation = "$ico,0"
$s.Description = 'Search movies and series and save their .torrent files'
$s.Save()

Say "torrenty rebuilt and installed to $exe"
