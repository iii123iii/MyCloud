Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectDir = Resolve-Path (Join-Path $ScriptDir "..")

if ([string]::IsNullOrWhiteSpace($env:MYCLOUD_VERSION)) {
  Push-Location $ProjectDir
  try {
    $null = cmd /c "git rev-parse --is-inside-work-tree 2>nul"
    if ($LASTEXITCODE -eq 0) {
      $resolvedVersion = cmd /c "git describe --tags --exact-match 2>nul"
      if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($resolvedVersion)) {
        $resolvedVersion = cmd /c "git describe --tags --abbrev=0 2>nul"
      }
      if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($resolvedVersion)) {
        $shortSha = cmd /c "git rev-parse --short HEAD 2>nul"
        if ($LASTEXITCODE -eq 0 -and -not [string]::IsNullOrWhiteSpace($shortSha)) {
          $resolvedVersion = "dev-$shortSha"
        }
      }
      if (-not [string]::IsNullOrWhiteSpace($resolvedVersion)) {
        $env:MYCLOUD_VERSION = $resolvedVersion.Trim()
      }
    }
  } finally {
    Pop-Location
  }
}

if ([string]::IsNullOrWhiteSpace($env:MYCLOUD_VERSION)) {
  $env:MYCLOUD_VERSION = "dev"
}

Write-Host "Using MYCLOUD_VERSION=$env:MYCLOUD_VERSION"

Push-Location $ProjectDir
try {
  & docker compose @args
  exit $LASTEXITCODE
} finally {
  Pop-Location
}
