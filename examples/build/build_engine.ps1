param(
    [string]$Output = "bin/project-engine.exe"
)

$ErrorActionPreference = "Stop"
$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Push-Location $RepoRoot
try {
    # Required by Go CGO to link bundled static archives (libjoltc.a / libluajit.a)
    $env:CGO_LDFLAGS_ALLOW = "-l:.*\.a"

    $outDir = Split-Path -Parent $Output
    if ($outDir -and -not (Test-Path $outDir)) {
        New-Item -ItemType Directory -Path $outDir | Out-Null
    }

    Write-Host "[1/2] Verifying Go toolchain..."
    go version

    Write-Host "[2/2] Building Project Engine executable -> $Output"
    go build -o $Output ./cmd/demo

    Write-Host "Build succeeded: $Output"
    Write-Host "Run locally:  .\$Output"
    Write-Host "Custom root:  .\$Output --root examples\custom-game-root"
}
finally {
    Pop-Location
}
