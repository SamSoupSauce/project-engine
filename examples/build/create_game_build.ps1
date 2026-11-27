param(
    [string]$GameRoot = "examples/custom-game-root",
    [string]$OutDir = "dist/my-game"
)

$ErrorActionPreference = "Stop"
$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Push-Location $RepoRoot
try {
    $env:CGO_LDFLAGS_ALLOW = "-l:.*\.a"

    if (-not (Test-Path $GameRoot)) {
        throw "Game root '$GameRoot' does not exist."
    }

    if (Test-Path $OutDir) {
        Remove-Item -Recurse -Force $OutDir
    }
    New-Item -ItemType Directory -Path $OutDir | Out-Null

    $binPath = Join-Path $OutDir "project-engine.exe"
    Write-Host "[1/3] Compiling engine binary -> $binPath"
    go build -o $binPath ./cmd/demo

    Write-Host "[2/3] Copying game data root from '$GameRoot'..."
    foreach ($sub in @("assets", "scripts", "prefabs", "config")) {
        $src = Join-Path $GameRoot $sub
        $dst = Join-Path $OutDir $sub
        if (Test-Path $src) {
            Copy-Item -Recurse $src $dst
        } else {
            New-Item -ItemType Directory -Path $dst | Out-Null
        }
    }
    $cfgFile = Join-Path $GameRoot "engine.json"
    if (Test-Path $cfgFile) {
        Copy-Item $cfgFile (Join-Path $OutDir "engine.json")
    }

    Write-Host "[3/3] Writing standalone launcher..."
    $launcher = @'
$ErrorActionPreference = "Stop"
$Here = Split-Path -Parent $MyInvocation.MyCommand.Path
& (Join-Path $Here "project-engine.exe") --root $Here @args
'@
    Set-Content -Path (Join-Path $OutDir "run_game.ps1") -Value $launcher

    Write-Host "Distributable game build created at '$OutDir'."
    Write-Host "Launch with: .\$OutDir\run_game.ps1"
}
finally {
    Pop-Location
}
