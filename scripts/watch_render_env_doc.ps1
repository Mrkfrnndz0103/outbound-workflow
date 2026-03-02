param(
    [int]$PollSeconds = 2,
    [switch]$RunOnce
)

$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot

$watchFiles = @(
    ".env",
    ".env.example",
    "render.yaml"
)

function Get-FileStamp {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    if (-not (Test-Path $Path)) {
        return "<missing>"
    }

    return (Get-FileHash -Path $Path -Algorithm SHA256).Hash
}

function Update-RenderEnvDoc {
    Write-Host "[render-env] regenerating docs/render-env.md ..."
    go run ./scripts/generate_render_env_doc.go
    if ($LASTEXITCODE -ne 0) {
        throw "[render-env] generator failed with exit code $LASTEXITCODE"
    }
    Write-Host "[render-env] updated docs/render-env.md"
}

$stamps = @{}
foreach ($file in $watchFiles) {
    $stamps[$file] = Get-FileStamp -Path $file
}

Update-RenderEnvDoc

if ($RunOnce) {
    return
}

if ($PollSeconds -lt 1) {
    $PollSeconds = 1
}

Write-Host "[render-env] watching: $($watchFiles -join ', ') (poll every $PollSeconds s)"
while ($true) {
    Start-Sleep -Seconds $PollSeconds

    $changed = @()
    foreach ($file in $watchFiles) {
        $current = Get-FileStamp -Path $file
        if ($stamps[$file] -ne $current) {
            $stamps[$file] = $current
            $changed += $file
        }
    }

    if ($changed.Count -gt 0) {
        Write-Host ("[render-env] change detected in: " + ($changed -join ", "))
        Update-RenderEnvDoc
    }
}
