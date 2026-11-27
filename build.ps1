param(
    [switch]$Package,
    [switch]$Test,
    [string]$Version = "0.1.0",
    [string]$OutDir = "bin",
    [string]$DistDir = "dist/project-engine"
)

$ErrorActionPreference = "Stop"
$env:CGO_LDFLAGS_ALLOW = "-l:.*\.a"

if (-not (Test-Path $OutDir)) {
    New-Item -ItemType Directory -Path $OutDir | Out-Null
}

$exePath = Join-Path $OutDir "project-engine.exe"
$ldflags = "-X github.com/SamSoupSauce/project-engine/engine.Version=$Version"
Write-Host "Building engine executable -> $exePath (version $Version)"
go build -ldflags $ldflags -o $exePath ./cmd/demo

if ($Test) {
    Write-Host "Running Go test suite..."
    go test -count=1 ./...
}

if ($Package) {
    Write-Host "Creating standalone game build -> $DistDir"
    if (Test-Path $DistDir) {
        Remove-Item -Recurse -Force $DistDir
    }
    New-Item -ItemType Directory -Path $DistDir | Out-Null
    Copy-Item $exePath (Join-Path $DistDir "project-engine.exe")
    Copy-Item "engine.json" (Join-Path $DistDir "engine.json")
    Copy-Item -Recurse "assets" (Join-Path $DistDir "assets")
    Copy-Item -Recurse "scripts" (Join-Path $DistDir "scripts")
    Copy-Item -Recurse "prefabs" (Join-Path $DistDir "prefabs")
    New-Item -ItemType Directory -Path (Join-Path $DistDir "config") -Force | Out-Null
    Write-Host "Standalone build ready at $DistDir"
}
