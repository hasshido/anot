# PowerShell Release Script for anot
# Usage: .\release.ps1 -Version "1.0.0" [-DryRun]
param(
    [Parameter(Mandatory=$true)]
    [string]$Version,
    
    [Parameter(Mandatory=$false)]
    [switch]$DryRun = $false
)

$ErrorActionPreference = "Stop"

# Configuration
$USER = "hasshido"
$REPO = "anot"
$BINARY = "anot"
$TAG = "v$Version"

# Check prerequisites
if (-not $DryRun) {
    if (-not $env:GITHUB_TOKEN) {
        Write-Error "GITHUB_TOKEN environment variable is not set. Please set it with your GitHub personal access token."
        exit 2
    }
    
    # Check if gh CLI is available
    try {
        gh --version | Out-Null
    } catch {
        Write-Error "GitHub CLI (gh) is not installed or not in PATH. Please install it from https://cli.github.com/"
        exit 5
    }
}

Write-Host "Building anot version $Version..." -ForegroundColor Green
Write-Host "Project directory: $(Get-Location)" -ForegroundColor Yellow
if ($DryRun) {
    Write-Host "DRY RUN MODE - No GitHub release will be created" -ForegroundColor Yellow
}

# Run tests
Write-Host "Running tests..." -ForegroundColor Green
go test
if ($LASTEXITCODE -ne 0) {
    Write-Error "Tests failed. Aborting."
    exit 3
}

Write-Host "Tests passed. Building binaries..." -ForegroundColor Green

$platforms = @(
    @{ OS = "windows"; ARCH = "amd64" },
    @{ OS = "windows"; ARCH = "386" },
    @{ OS = "linux"; ARCH = "amd64" },
    @{ OS = "linux"; ARCH = "386" },
    @{ OS = "darwin"; ARCH = "amd64" },
    @{ OS = "freebsd"; ARCH = "amd64" },
    @{ OS = "freebsd"; ARCH = "386" }
)

$archives = @()

foreach ($platform in $platforms) {
    $os = $platform.OS
    $arch = $platform.ARCH
    
    $binfile = "anot"
    if ($os -eq "windows") {
        $binfile += ".exe"
    }
    
    Write-Host "Building $os/$arch..." -ForegroundColor Cyan
    
    $env:GOOS = $os
    $env:GOARCH = $arch
    
    go build -o $binfile .
    
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Build failed for $os/$arch. Aborting."
        exit 4
    }
    
    $archive = "anot-$os-$arch-$Version"
    if ($os -eq "windows") {
        $archive += ".zip"
        Compress-Archive -Path $binfile -DestinationPath $archive -Force
    } else {
        $archive += ".tgz"
        # Use tar (available in Windows 10+) to create proper tgz archives
        if (Get-Command tar -ErrorAction SilentlyContinue) {
            tar -czf $archive $binfile
        } else {
            Write-Warning "tar command not found. Creating zip instead of tgz for $os/$arch"
            $archive = $archive -replace "\.tgz$", ".zip"
            Compress-Archive -Path $binfile -DestinationPath $archive -Force
        }
    }
    
    Remove-Item $binfile -Force
    $archives += $archive
}

Write-Host "Build completed successfully!" -ForegroundColor Green
Write-Host "Generated archives:" -ForegroundColor Yellow
$archives | ForEach-Object { 
    if (Test-Path $_) {
        $size = [math]::Round((Get-Item $_).Length / 1KB, 1)
        Write-Host "  $_ ($size KB)" -ForegroundColor Gray
    } else {
        Write-Host "  $_ (not created)" -ForegroundColor Red
    }
}

if (-not $DryRun) {
    Write-Host "`nCreating GitHub release $TAG..." -ForegroundColor Green
    
    $releaseNotes = @"
# anot $Version

Complementary tool to [anew](https://github.com/tomnomnom/anew) for removing lines from files.

## Features
- Remove exact string matches from files
- Wildcard domain support (*.example.com)
- IP address and CIDR range filtering
- Cross-platform support (Windows, Linux, macOS, FreeBSD)
- High-performance optimizations

## Installation
Download the appropriate binary for your platform from the assets below.

## Usage
```bash
# Remove lines from stdin that match patterns in file
cat patterns.txt | anot target.txt

# Dry run (don't modify file)
cat patterns.txt | anot -d target.txt

# Quiet mode (no stdout output)
cat patterns.txt | anot -q target.txt
```
"@
    
    try {
        # Create the release
        $archiveFiles = $archives | Where-Object { Test-Path $_ }
        gh release create $TAG $archiveFiles --title "anot $Version" --notes $releaseNotes
        
        Write-Host "GitHub release $TAG created successfully!" -ForegroundColor Green
        Write-Host "Release URL: https://github.com/$USER/$REPO/releases/tag/$TAG" -ForegroundColor Cyan
    } catch {
        Write-Error "Failed to create GitHub release: $_"
        Write-Host "Archives are ready for manual upload:" -ForegroundColor Yellow
        $archives | Where-Object { Test-Path $_ } | ForEach-Object { Write-Host "  $_" -ForegroundColor Gray }
        exit 6
    }
} else {
    Write-Host "`nDRY RUN: Would create GitHub release $TAG with:" -ForegroundColor Yellow
    $archives | Where-Object { Test-Path $_ } | ForEach-Object { Write-Host "  $_" -ForegroundColor Gray }
}

# Clean up archives (unless dry run)
if (-not $DryRun) {
    Write-Host "`nCleaning up archives..." -ForegroundColor Green
    $archives | Where-Object { Test-Path $_ } | Remove-Item -Force
    Write-Host "Release $Version completed successfully!" -ForegroundColor Green
} else {
    Write-Host "`nDRY RUN completed. Archives preserved for inspection." -ForegroundColor Yellow
}
