param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$Args
)

Write-Output "sample workflow executed"
if ($Args.Count -gt 0) {
    Write-Output ("args: " + ($Args -join ", "))
}
