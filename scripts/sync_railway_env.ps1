param(
    [string]$EnvFile = ".env.example",
    [string]$Service = "",
    [string]$Environment = "",
    [string]$Prefix = "",
    [int]$ChunkSize = 40,
    [switch]$IncludeEmpty,
    [switch]$DryRun,
    [switch]$NoSkipDeploys
)

$ErrorActionPreference = "Stop"

function Parse-DotEnvLine {
    param(
        [AllowEmptyString()]
        [string]$Line
    )

    $trimmed = $Line.Trim()
    if ([string]::IsNullOrWhiteSpace($trimmed)) {
        return $null
    }
    if ($trimmed.StartsWith("#")) {
        return $null
    }
    if ($trimmed.StartsWith("export ")) {
        $trimmed = $trimmed.Substring(7).TrimStart()
    }

    $eqIndex = $trimmed.IndexOf("=")
    if ($eqIndex -lt 1) {
        return $null
    }

    $key = $trimmed.Substring(0, $eqIndex).Trim()
    $value = $trimmed.Substring($eqIndex + 1)
    if ($key -notmatch "^[A-Za-z_][A-Za-z0-9_]*$") {
        return $null
    }

    if ($value.Length -ge 2) {
        if (($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'"))) {
            $value = $value.Substring(1, $value.Length - 2)
        }
    }

    [PSCustomObject]@{
        Key = $key
        Value = $value
    }
}

if (-not (Get-Command railway -ErrorAction SilentlyContinue)) {
    if ($DryRun) {
        Write-Host "[railway-env-sync] Railway CLI not found; continuing in dry-run mode."
    } else {
        throw "Railway CLI not found. Install with: npm i -g @railway/cli"
    }
}

$resolvedEnvFile = Resolve-Path -Path $EnvFile -ErrorAction Stop
$rawLines = Get-Content -Path $resolvedEnvFile

$parsedMap = @{}
foreach ($line in $rawLines) {
    $entry = Parse-DotEnvLine -Line $line
    if ($null -eq $entry) {
        continue
    }
    $parsedMap[$entry.Key] = $entry.Value
}

$keys = @($parsedMap.Keys | Sort-Object)
if (-not [string]::IsNullOrWhiteSpace($Prefix)) {
    $keys = @($keys | Where-Object { $_.StartsWith($Prefix) })
}

$pairs = New-Object System.Collections.Generic.List[string]
$skippedEmpty = New-Object System.Collections.Generic.List[string]
foreach ($key in $keys) {
    $value = [string]$parsedMap[$key]
    if (-not $IncludeEmpty -and [string]::IsNullOrWhiteSpace($value)) {
        $skippedEmpty.Add($key)
        continue
    }
    $pairs.Add("$key=$value")
}

if ($pairs.Count -eq 0) {
    Write-Host "[railway-env-sync] No variables to sync from $resolvedEnvFile."
    if ($skippedEmpty.Count -gt 0) {
        Write-Host "[railway-env-sync] Skipped empty values ($($skippedEmpty.Count)). Use -IncludeEmpty to sync them."
    }
    exit 0
}

if ($ChunkSize -lt 1) {
    $ChunkSize = 1
}

$total = $pairs.Count
Write-Host "[railway-env-sync] File: $resolvedEnvFile"
Write-Host "[railway-env-sync] Variables selected: $total"
if ($skippedEmpty.Count -gt 0) {
    Write-Host "[railway-env-sync] Skipped empty values: $($skippedEmpty.Count) (use -IncludeEmpty to include)"
}

$baseArgs = @("variables", "set")
if (-not $NoSkipDeploys) {
    $baseArgs += "--skip-deploys"
}
if (-not [string]::IsNullOrWhiteSpace($Service)) {
    $baseArgs += @("--service", $Service)
}
if (-not [string]::IsNullOrWhiteSpace($Environment)) {
    $baseArgs += @("--environment", $Environment)
}

for ($start = 0; $start -lt $total; $start += $ChunkSize) {
    $end = [Math]::Min($start + $ChunkSize - 1, $total - 1)
    $chunk = @($pairs[$start..$end])
    $cmdArgs = @($baseArgs + $chunk)
    $chunkNumber = [int]($start / $ChunkSize) + 1
    $chunkCount = [int][Math]::Ceiling($total / [double]$ChunkSize)

    if ($DryRun) {
        Write-Host "[railway-env-sync] DRY RUN chunk $chunkNumber/$chunkCount -> railway $($cmdArgs -join ' ')"
        continue
    }

    Write-Host "[railway-env-sync] Applying chunk $chunkNumber/$chunkCount ($($chunk.Count) vars)..."
    & railway @cmdArgs
    if ($LASTEXITCODE -ne 0) {
        throw "railway variables set failed in chunk $chunkNumber/$chunkCount with exit code $LASTEXITCODE"
    }
}

if ($DryRun) {
    Write-Host "[railway-env-sync] Dry run complete."
} else {
    Write-Host "[railway-env-sync] Sync complete."
    if (-not $NoSkipDeploys) {
        Write-Host "[railway-env-sync] Changes were applied with --skip-deploys. Run 'railway up' when ready."
    }
}
