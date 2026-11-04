# Comprobaciones de calidad del proyecto. Ejecutar antes de cada commit:
#   pwsh -NoProfile -File scripts\check.ps1
#
# Falla en el primer problema. staticcheck es opcional: se usa solo si está
# instalado Y es compatible con el toolchain de Go del go.mod.
[CmdletBinding()]
param()

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)

function Invoke-Step {
    param([string]$Name, [scriptblock]$Body)
    Write-Host "== $Name"
    & $Body
    if ($LASTEXITCODE -ne 0) {
        throw "$Name failed (exit $LASTEXITCODE)"
    }
}

# Los formateadores listan los archivos que cambiarían; la lista debe estar vacía.
function Assert-NoOutput {
    param([string]$Name, [string[]]$Lines)
    $lines = @($Lines | Where-Object { $_ })
    if ($lines.Count -gt 0) {
        $lines | ForEach-Object { Write-Host "  $_" }
        throw "$Name found files that need formatting (run it with -w)"
    }
}

Write-Host '== gofmt -s'
Assert-NoOutput 'gofmt -s' @(gofmt -s -l .)

if (Get-Command goimports -ErrorAction SilentlyContinue) {
    Write-Host '== goimports'
    Assert-NoOutput 'goimports' @(goimports -l .)
}
else {
    Write-Warning 'goimports not installed: skipped'
}

Invoke-Step 'go vet' { go vet ./... }

if (Get-Command staticcheck -ErrorAction SilentlyContinue) {
    Write-Host '== staticcheck'
    $out = @(staticcheck ./... 2>&1)
    if ($LASTEXITCODE -ne 0) {
        if (($out -join "`n") -match 'export data version|requires newer Go version') {
            Write-Warning 'staticcheck was built with an older Go than go.mod requires: SKIPPED. Rebuild it with the same Go: $env:GOTOOLCHAIN="go1.27.3"; go install honnef.co/go/tools/cmd/staticcheck@latest'
        }
        else {
            $out | ForEach-Object { Write-Host "  $_" }
            throw 'staticcheck reported problems'
        }
    }
}
else {
    Write-Warning 'staticcheck not installed: skipped'
}

Invoke-Step 'go test -race' { go test -race ./... }

Write-Host 'All checks passed'
