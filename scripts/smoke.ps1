# Prueba de humo de hum contra el herdr REAL de esta máquina. Solo lectura:
# no envía mensajes, no crea, renombra, cierra ni borra nada. Las únicas órdenes
# que lanza están fijadas en cmd\hum\smoke_test.go (si añades una, ese test te
# obliga a decidirlo a la vista).
#
#   pwsh -NoProfile -File scripts\smoke.ps1
#   pwsh -NoProfile -File scripts\smoke.ps1 -Machine odoous    # añade una máquina remota
#
# Requiere herdr en marcha y bin\hum.exe construido (go build -o bin\hum.exe .\cmd\hum).
# Cada paso termina en PASS, FAIL o SKIP. SKIP significa "no se pudo comprobar"
# y NO cuenta como correcto. Código de salida: 1 si algún paso falla, 0 si no.
# No imprime títulos, rutas ni nombres de tus agentes; solo cuentas y resultados.
#Requires -Version 7
[CmdletBinding()]
param(
    [string]$Exe = '',
    # Máquina remota opcional (solo lectura). Una máquina que no responde da SKIP.
    [string]$Machine = '',
    # Límite por orden, en segundos.
    [int]$TimeoutSeconds = 20
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
if (-not $Exe) { $Exe = Join-Path $root 'bin\hum.exe' }
if (-not (Test-Path -LiteralPath $Exe)) {
    Write-Host "hum was not found at $Exe. Build it first: go build -o bin\hum.exe .\cmd\hum"
    exit 2
}

# Lanza hum con un límite de tiempo y devuelve código, salida y errores. Un
# comando que no termina se mata y cuenta como fallo (código -1).
function Invoke-Hum([Parameter(ValueFromRemainingArguments = $true)][string[]]$HumArgs) {
    $psi = [System.Diagnostics.ProcessStartInfo]::new($Exe)
    foreach ($a in $HumArgs) { $psi.ArgumentList.Add($a) }
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.UseShellExecute = $false
    $p = [System.Diagnostics.Process]::Start($psi)
    $out = $p.StandardOutput.ReadToEndAsync()
    $err = $p.StandardError.ReadToEndAsync()
    if (-not $p.WaitForExit($TimeoutSeconds * 1000)) {
        $p.Kill($true)
        return [pscustomobject]@{ Code = -1; Out = ''; Err = "timed out after ${TimeoutSeconds}s" }
    }
    $p.WaitForExit()
    [pscustomobject]@{ Code = $p.ExitCode; Out = $out.Result; Err = $err.Result }
}

$results = [System.Collections.Generic.List[object]]::new()

# El cuerpo lanza una excepción si falla, devuelve 'SKIP: motivo' si no pudo
# comprobar, y cualquier otra cosa cuenta como correcto.
function Step([string]$Name, [scriptblock]$Body) {
    try {
        $r = & $Body
        if ($r -is [string] -and $r.StartsWith('SKIP:')) {
            $status, $detail = 'SKIP', $r.Substring(5).Trim()
        }
        else {
            $status, $detail = 'PASS', ''
        }
    }
    catch {
        $status, $detail = 'FAIL', $_.Exception.Message
    }
    $results.Add([pscustomobject]@{ Status = $status; Name = $Name; Detail = $detail })
    $line = '{0,-5} {1}' -f $status, $Name
    if ($detail) { $line += "  ($detail)" }
    Write-Host $line
}

function Expect([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
}

Write-Host "hum smoke test (read-only) using $Exe"

Step 'help prints the usage' {
    $r = Invoke-Hum help
    Expect ($r.Code -eq 0) "exit $($r.Code)"
    Expect ($r.Out -match 'hum ls') 'the usage does not mention hum ls'
}

$agents = @()
Step 'ls --local --json lists this machine as valid JSON' {
    $r = Invoke-Hum ls --local --json
    Expect ($r.Code -eq 0) "exit $($r.Code): $($r.Err.Trim())"
    $doc = $r.Out | ConvertFrom-Json
    Expect ($null -ne $doc.PSObject.Properties['targets']) 'no "targets" field'
    $script:agents = @($doc.targets)
    Write-Host "      $($script:agents.Count) local agents"
}

Step 'ls --local draws the tree' {
    $r = Invoke-Hum ls --local
    Expect ($r.Code -eq 0) "exit $($r.Code): $($r.Err.Trim())"
    Expect ($r.Out.Trim().Length -gt 0) 'empty output'
}

Step 'attention --local --json is valid JSON with its groups' {
    $r = Invoke-Hum attention --local --json
    Expect ($r.Code -eq 0) "exit $($r.Code): $($r.Err.Trim())"
    $doc = $r.Out | ConvertFrom-Json
    foreach ($key in 'needs_you', 'working') {
        Expect ($null -ne $doc.PSObject.Properties[$key]) "no ""$key"" field"
    }
}

Step 'machines --json is valid JSON' {
    $r = Invoke-Hum machines --json
    Expect ($r.Code -eq 0) "exit $($r.Code): $($r.Err.Trim())"
    $null = $r.Out | ConvertFrom-Json
}

# Mi propio pane si herdr lo dice y está en la lista; si no, el primero.
$pane = ''
if ($agents.Count -gt 0) {
    $mine = @($agents | Where-Object { $_.pane_id -eq $env:HERDR_PANE_ID })
    $pane = if ($mine.Count -gt 0) { $mine[0].pane_id } else { $agents[0].pane_id }
}

Step 'id resolves a pane id to itself' {
    if (-not $pane) { return 'SKIP: there are no local agents' }
    $r = Invoke-Hum id "local/$pane"
    Expect ($r.Code -eq 0) "exit $($r.Code): $($r.Err.Trim())"
    Expect ($r.Out.Trim() -eq $pane) "got ""$($r.Out.Trim())"" for $pane"
}

Step 'look reads the screen of an agent' {
    if (-not $pane) { return 'SKIP: there are no local agents' }
    $r = Invoke-Hum look "local/$pane" --lines 5
    Expect ($r.Code -eq 0) "exit $($r.Code): $($r.Err.Trim())"
}

Step 'completion scripts are printed' {
    $r = Invoke-Hum completion powershell
    Expect ($r.Code -eq 0 -and $r.Out -match 'Register-ArgumentCompleter') 'the PowerShell script is wrong'
    $r = Invoke-Hum completion bash
    Expect ($r.Code -eq 0 -and $r.Out -match 'complete -F') 'the bash script is wrong'
}

Step 'completion suggests subcommands' {
    $r = Invoke-Hum __complete --shell bash --line 'hum a'
    Expect ($r.Code -eq 0) "exit $($r.Code)"
    Expect ($r.Out -match '(?m)^ask$') 'ask is not suggested'
}

Step 'alias ls works on an empty private folder' {
    $dir = Join-Path ([System.IO.Path]::GetTempPath()) ("hum-smoke-" + [guid]::NewGuid().ToString('N'))
    $old = $env:HERDR_PLUGIN_CONFIG_DIR
    try {
        $env:HERDR_PLUGIN_CONFIG_DIR = $dir
        $r = Invoke-Hum alias ls
        Expect ($r.Code -eq 0) "exit $($r.Code): $($r.Err.Trim())"
        Expect ($r.Out.Trim() -eq '') 'a fresh folder should have no aliases'
    }
    finally {
        $env:HERDR_PLUGIN_CONFIG_DIR = $old
        if (Test-Path -LiteralPath $dir) { Remove-Item -LiteralPath $dir -Recurse -Force }
    }
}

if ($Machine) {
    Step "ls --machine $Machine answers (read-only)" {
        # --fresh: que la caché de hum no pueda tapar a una máquina que no responde.
        $r = Invoke-Hum ls --machine $Machine --fresh --server-timeout 5s --json
        if ($r.Code -ne 0) { throw "exit $($r.Code): $($r.Err.Trim())" }
        if ($r.Err -match 'warning:') {
            return "SKIP: $Machine did not answer ($(($r.Err -split "`n")[0].Trim()))"
        }
        $doc = $r.Out | ConvertFrom-Json
        # "machine" se omite en los agentes locales, así que se lee con cuidado.
        $remote = @($doc.targets | Where-Object {
                $p = $_.PSObject.Properties['machine']
                $null -ne $p -and $p.Value -eq $Machine
            })
        Write-Host "      $($remote.Count) agents on $Machine"
    }
}

$fail = @($results | Where-Object Status -eq 'FAIL').Count
$skip = @($results | Where-Object Status -eq 'SKIP').Count
$pass = @($results | Where-Object Status -eq 'PASS').Count
Write-Host ''
Write-Host "passed: $pass, failed: $fail, skipped: $skip (a skipped step was NOT checked)"
if ($fail -gt 0) { exit 1 }
exit 0
