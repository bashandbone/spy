# PowerShell sample
[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [string]$Path = 'input.txt'
)

class Counter {
    [int]$Value = 0

    [int]Increment([int]$n) {
        $this.Value += $n
        return $this.Value
    }
}

function Get-LineByteCount {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$Path
    )
    if (-not (Test-Path -LiteralPath $Path)) {
        Write-Error -Message "input file not found: $Path"
        return $null
    }
    $counter = [Counter]::new()
    Get-Content -LiteralPath $Path | ForEach-Object {
        if ($_ -ne '') {
            [void]$counter.Increment($_.Length)
        }
    }
    return $counter.Value
}

$total = Get-LineByteCount -Path $Path
Write-Output ("total bytes: {0}" -f $total)
