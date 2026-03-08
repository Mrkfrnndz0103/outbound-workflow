param(
    [string]$OutFile = ".env",
    [string]$Service = "",
    [string]$Environment = "",
    [string]$Prefix = "",
    [switch]$SkipEmpty,
    [switch]$NoBackup,
    [switch]$DryRun,
    [switch]$FailIfUnavailable
)

$ErrorActionPreference = "Stop"

function Convert-RailwayJsonToEntries {
    param(
        [Parameter(Mandatory)]
        $InputObject
    )

    $entries = New-Object System.Collections.Generic.List[object]

    function Add-Entry {
        param(
            [AllowEmptyString()][string]$Key,
            [AllowNull()]$Value
        )

        if ([string]::IsNullOrWhiteSpace($Key)) {
            return
        }
        $entries.Add([PSCustomObject]@{
            Key   = $Key
            Value = if ($null -eq $Value) { "" } else { [string]$Value }
        })
    }

    function Parse-Object {
        param($Obj)

        if ($null -eq $Obj) {
            return
        }

        if ($Obj -is [System.Array]) {
            foreach ($item in $Obj) {
                Parse-Object -Obj $item
            }
            return
        }

        $props = @($Obj.PSObject.Properties.Name)
        if ($props.Count -eq 0) {
            return
        }

        if ($props -contains "variables") {
            Parse-Object -Obj $Obj.variables
            return
        }

        $nameKey = $null
        if ($props -contains "name") { $nameKey = "name" }
        elseif ($props -contains "key") { $nameKey = "key" }
        elseif ($props -contains "Name") { $nameKey = "Name" }
        elseif ($props -contains "Key") { $nameKey = "Key" }

        $valueKey = $null
        if ($props -contains "value") { $valueKey = "value" }
        elseif ($props -contains "Value") { $valueKey = "Value" }

        if ($null -ne $nameKey -and $null -ne $valueKey) {
            Add-Entry -Key ([string]$Obj.$nameKey) -Value $Obj.$valueKey
            return
        }

        foreach ($propName in $props) {
            $propValue = $Obj.$propName
            if ($propValue -is [string] -or $propValue -is [int] -or $propValue -is [long] -or $propValue -is [bool]) {
                Add-Entry -Key $propName -Value $propValue
            }
        }
    }

    Parse-Object -Obj $InputObject

    $map = @{}
    foreach ($entry in $entries) {
        $map[$entry.Key] = $entry.Value
    }
    return @($map.GetEnumerator() | Sort-Object Key | ForEach-Object {
        [PSCustomObject]@{
            Key   = $_.Key
            Value = [string]$_.Value
        }
    })
}

function Format-DotEnvValue {
    param(
        [AllowNull()]
        $Value
    )

    if ($null -eq $Value) {
        return ""
    }

    $text = [string]$Value
    $escaped = $text -replace "\\", "\\\\" -replace '"', '\"' -replace "`r?`n", "\n"
    if ($escaped -match '\s' -or $escaped.Contains("#") -or $escaped.Contains("=") -or $escaped.Contains('"')) {
        return '"' + $escaped + '"'
    }
    return $escaped
}

if (-not (Get-Command railway -ErrorAction SilentlyContinue)) {
    if ($FailIfUnavailable) {
        throw "Railway CLI not found. Install with: npm i -g @railway/cli"
    }
    Write-Host "[railway-env-pull] Railway CLI not found; skipping auto pull."
    exit 0
}

$cmdArgs = @("variables", "--json")
if (-not [string]::IsNullOrWhiteSpace($Service)) {
    $cmdArgs += @("--service", $Service)
}
if (-not [string]::IsNullOrWhiteSpace($Environment)) {
    $cmdArgs += @("--environment", $Environment)
}

Write-Host "[railway-env-pull] Running: railway $($cmdArgs -join ' ')"
$raw = & railway @cmdArgs
if ($LASTEXITCODE -ne 0) {
    if ($FailIfUnavailable) {
        throw "railway variables --json failed with exit code $LASTEXITCODE"
    }
    Write-Host "[railway-env-pull] Railway project not linked or not authenticated; skipping auto pull."
    exit 0
}

$jsonText = ($raw -join "`n").Trim()
if ([string]::IsNullOrWhiteSpace($jsonText)) {
    if ($FailIfUnavailable) {
        throw "railway variables --json returned empty output"
    }
    Write-Host "[railway-env-pull] Railway returned no variables; skipping auto pull."
    exit 0
}

try {
    $parsed = $jsonText | ConvertFrom-Json -Depth 20
} catch {
    if ($FailIfUnavailable) {
        throw "Failed to parse Railway JSON output: $($_.Exception.Message)"
    }
    Write-Host "[railway-env-pull] Unable to parse Railway JSON output; skipping auto pull."
    exit 0
}

$entries = Convert-RailwayJsonToEntries -InputObject $parsed
if (-not [string]::IsNullOrWhiteSpace($Prefix)) {
    $entries = @($entries | Where-Object { $_.Key.StartsWith($Prefix) })
}
if ($SkipEmpty) {
    $entries = @($entries | Where-Object { -not [string]::IsNullOrWhiteSpace($_.Value) })
}

if ($entries.Count -eq 0) {
    if ($FailIfUnavailable) {
        throw "No variables found from Railway (after filters)."
    }
    Write-Host "[railway-env-pull] No variables matched filters; skipping auto pull."
    exit 0
}

$resolvedOutFile = Join-Path (Get-Location) $OutFile
$lines = New-Object System.Collections.Generic.List[string]
$lines.Add("# Auto-generated from Railway CLI on $(Get-Date -Format 'yyyy-MM-ddTHH:mm:ssK')")
$lines.Add("# Source of truth: Railway service variables")
foreach ($entry in $entries) {
    $value = Format-DotEnvValue -Value $entry.Value
    $lines.Add("$($entry.Key)=$value")
}

if ($DryRun) {
    Write-Host "[railway-env-pull] DRY RUN: would write $($entries.Count) variables to $resolvedOutFile"
    $lines | ForEach-Object { Write-Host $_ }
    exit 0
}

if ((Test-Path $resolvedOutFile) -and (-not $NoBackup)) {
    $backupPath = "$resolvedOutFile.bak"
    Copy-Item -Path $resolvedOutFile -Destination $backupPath -Force
    Write-Host "[railway-env-pull] Backup written to $backupPath"
}

Set-Content -Path $resolvedOutFile -Value $lines -Encoding utf8
Write-Host "[railway-env-pull] Wrote $($entries.Count) variables to $resolvedOutFile"
