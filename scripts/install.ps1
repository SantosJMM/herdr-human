# Paso [[build]] de herdr para Windows: deja el binario de hum en bin\hum.exe.
# Es el gemelo de install.sh.
#
# Camino rapido: descarga el binario ya compilado del release de GitHub cuya
# etiqueta coincide con la version del manifest (herdr-plugin.toml), comprueba su
# SHA-256 contra el SHA256SUMS del mismo release y lo instala.
# Respaldo: si algo falla (sin release, sin red, plataforma sin binario, checksum
# distinto) y hay Go en el PATH, compila desde el codigo. Si tampoco hay Go,
# termina con un mensaje claro. Un binario descargado que no pasa la comprobacion
# nunca se instala.
#
# Compatible con Windows PowerShell 5.1 (el que trae Windows) y solo ASCII: 5.1
# lee un script sin BOM en la pagina de codigos ANSI.
# herdr lanza este paso sin las variables de ejecucion del plugin, asi que la raiz
# se calcula desde la ubicacion del propio script.
#
# Variables para pruebas: HUM_BASE_URL (base de descargas), HUM_GO (ejecutable de
# go), HUM_RETRY_SLEEP (segundos entre reintentos).
$ErrorActionPreference = 'Stop'
# 5.1 dibuja una barra de progreso por trozo descargado y la descarga se hace lenta.
$ProgressPreference = 'SilentlyContinue'
# Un PSModulePath heredado de pwsh 7 carga modulos que 5.1 no puede ejecutar.
$env:PSModulePath = Join-Path $PSHOME 'Modules'
# 5.1 puede negociar TLS 1.0, que GitHub rechaza.
try {
    [Net.ServicePointManager]::SecurityProtocol =
        [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
}
catch { }

$Repo = 'SantosJMM/herdr-human'
$Root = Split-Path -Parent $PSScriptRoot
$BaseUrl = if ($env:HUM_BASE_URL) { $env:HUM_BASE_URL } else { "https://github.com/$Repo/releases/download" }
$GoBin = if ($env:HUM_GO) { $env:HUM_GO } else { 'go' }
$RetrySleep = if ($env:HUM_RETRY_SLEEP) { [int]$env:HUM_RETRY_SLEEP } else { 3 }
$Attempts = 3
$Out = Join-Path (Join-Path $Root 'bin') 'hum.exe'

function Say([string]$Message) {
    [Console]::Error.WriteLine("hum: $Message")
}

# Descarga con reintentos: los activos de un release recien publicado pueden dar
# 404 unos minutos.
function Get-Asset([string]$Url, [string]$Dest) {
    for ($n = 1; $n -le $Attempts; $n++) {
        try {
            Invoke-WebRequest -UseBasicParsing -Uri $Url -OutFile $Dest
            return $true
        }
        catch {
            if ($n -lt $Attempts -and $RetrySleep -gt 0) { Start-Sleep -Seconds $RetrySleep }
        }
    }
    return $false
}

# SHA-256 hexadecimal en minusculas; certutil es el respaldo si Get-FileHash falta.
function Get-Sha256([string]$Path) {
    try {
        return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path -ErrorAction Stop).Hash.ToLowerInvariant()
    }
    catch {
        try {
            $lines = & certutil -hashfile $Path SHA256 2>$null
            $hex = $lines | Where-Object { $_ -match '^[0-9a-fA-F]{64}$' } | Select-Object -First 1
            if ($hex) { return $hex.Trim().ToLowerInvariant() }
        }
        catch { }
        return $null
    }
}

# Instala sin dejar nunca un binario a medias en bin\.
function Place([string]$Source) {
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Out) | Out-Null
    Copy-Item -LiteralPath $Source -Destination "$Out.new" -Force
    Move-Item -LiteralPath "$Out.new" -Destination $Out -Force
}

# Camino rapido. Devuelve $null si instalo el binario, o el motivo si fallo.
function Install-Download {
    $manifest = Join-Path $Root 'herdr-plugin.toml'
    $version = $null
    if (Test-Path -LiteralPath $manifest) {
        foreach ($line in Get-Content -LiteralPath $manifest) {
            if ($line -match '^version\s*=\s*"([^"]+)"') { $version = $Matches[1]; break }
        }
    }
    if (-not $version) { return 'cannot read the version from herdr-plugin.toml' }

    # Un PowerShell de 32 bits en Windows de 64 informa la arquitectura del proceso.
    $cpu = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    switch ($cpu) {
        'AMD64' { $arch = 'amd64' }
        'ARM64' { $arch = 'arm64' }
        default { return "no prebuilt binary for Windows-$cpu" }
    }
    $asset = "hum-windows-$arch.exe"
    $release = "$BaseUrl/v$version"

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ('hum-install-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Say "downloading $asset (v$version)"
        if (-not (Get-Asset "$release/$asset" (Join-Path $tmp $asset))) {
            return "could not download $release/$asset"
        }
        if (-not (Get-Asset "$release/SHA256SUMS" (Join-Path $tmp 'SHA256SUMS'))) {
            return "could not download $release/SHA256SUMS"
        }

        $want = $null
        foreach ($line in Get-Content -LiteralPath (Join-Path $tmp 'SHA256SUMS')) {
            if ($line -match '^([0-9a-fA-F]{64})\s+\*?(.+?)\s*$' -and $Matches[2] -eq $asset) {
                $want = $Matches[1].ToLowerInvariant()
                break
            }
        }
        if (-not $want) { return "SHA256SUMS has no entry for $asset" }

        $got = Get-Sha256 (Join-Path $tmp $asset)
        if (-not $got) { return 'no tool available to verify the download' }
        if ($got -ne $want) { return "checksum mismatch for $asset (expected $want, got $got)" }

        Place (Join-Path $tmp $asset)
        Say "installed $Out"
        return $null
    }
    catch {
        return "unexpected error: $($_.Exception.Message)"
    }
    finally {
        Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }
}

$why = Install-Download
if (-not $why) { exit 0 }

Say "$why - building from source instead."
if (-not (Get-Command $GoBin -ErrorAction SilentlyContinue)) {
    Say 'no prebuilt binary could be installed and Go was not found, so hum cannot be built.'
    Say "Install Go from https://go.dev/dl/ and then run: herdr plugin install $Repo"
    exit 1
}

New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Out) | Out-Null
Push-Location $Root
try {
    & $GoBin build -trimpath -o "$Out.new" ./cmd/hum
    $code = $LASTEXITCODE
}
finally {
    Pop-Location
}
if ($code -ne 0) {
    Remove-Item -LiteralPath "$Out.new" -Force -ErrorAction SilentlyContinue
    Say 'go build failed; see the output above.'
    exit 1
}
Move-Item -LiteralPath "$Out.new" -Destination $Out -Force
Say "built $Out"
exit 0
