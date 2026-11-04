#!/bin/sh
# Paso [[build]] de herdr para Linux y macOS: deja el binario de hum en bin/hum.
#
# Camino rápido: descarga el binario ya compilado del release de GitHub cuya
# etiqueta coincide con la versión del manifest (herdr-plugin.toml), comprueba su
# SHA-256 contra el SHA256SUMS del mismo release y lo instala.
# Respaldo: si algo falla (sin release, sin red, plataforma sin binario, checksum
# distinto) y hay Go en el PATH, compila desde el código. Si tampoco hay Go,
# termina con un mensaje claro. Un binario descargado que no pasa la comprobación
# nunca se instala.
#
# herdr lanza este paso sin las variables de ejecución del plugin, así que la raíz
# se calcula desde la ubicación del propio script.
#
# Variables para pruebas: HUM_BASE_URL (base de descargas), HUM_GO (ejecutable de
# go), HUM_RETRY_SLEEP (segundos entre reintentos).
set -u

repo="SantosJMM/herdr-human"
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
root=$(CDPATH= cd -- "$script_dir/.." && pwd)
base_url="${HUM_BASE_URL:-https://github.com/$repo/releases/download}"
go_bin="${HUM_GO:-go}"
retry_sleep="${HUM_RETRY_SLEEP:-3}"
attempts=3
out="$root/bin/hum"

say() { echo "hum: $*" >&2; }
have() { command -v "$1" >/dev/null 2>&1; }

tmpdir=$(mktemp -d 2>/dev/null || mktemp -d -t hum-install)
trap 'rm -rf "$tmpdir"' EXIT HUP INT TERM

# Descarga una URL a un archivo, con reintentos: los activos de un release recién
# publicado pueden dar 404 unos minutos.
download() { # download <url> <destino>
  n=1
  while [ "$n" -le "$attempts" ]; do
    if have curl; then
      curl -fsSL --connect-timeout 15 --max-time 180 -o "$2" "$1" 2>/dev/null && return 0
    elif have wget; then
      wget -q -T 60 -O "$2" "$1" 2>/dev/null && return 0
    else
      return 127
    fi
    [ "$n" -lt "$attempts" ] && sleep "$retry_sleep"
    n=$((n + 1))
  done
  return 1
}

sha256_of() { # imprime el SHA-256 hexadecimal de $1
  if have sha256sum; then
    sha256sum "$1" | awk '{print $1}'
  elif have shasum; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    return 127
  fi
}

# Coloca el binario sin dejar nunca uno a medias en bin/.
place() { # place <origen>
  mkdir -p "$root/bin" || return 1
  cp "$1" "$out.new" && chmod 755 "$out.new" && mv -f "$out.new" "$out"
}

# Intenta el camino rápido. Si falla, deja el motivo en $reason y devuelve 1.
reason=""
try_download() {
  version=$(sed -n 's/^version[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "$root/herdr-plugin.toml" 2>/dev/null | head -n 1)
  if [ -z "$version" ]; then
    reason="cannot read the version from herdr-plugin.toml"
    return 1
  fi

  case "$(uname -s 2>/dev/null)" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) reason="no prebuilt binary for $(uname -s 2>/dev/null || echo this system)"; return 1 ;;
  esac
  case "$(uname -m 2>/dev/null)" in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) reason="no prebuilt binary for $(uname -m 2>/dev/null || echo this CPU)"; return 1 ;;
  esac
  asset="hum-$os-$arch"
  release="$base_url/v$version"

  say "downloading $asset (v$version)"
  if ! download "$release/$asset" "$tmpdir/$asset"; then
    reason="could not download $release/$asset"
    return 1
  fi
  if ! download "$release/SHA256SUMS" "$tmpdir/SHA256SUMS"; then
    reason="could not download $release/SHA256SUMS"
    return 1
  fi

  want=$(awk -v a="$asset" '$2 == a || $2 == "*" a { print $1; exit }' "$tmpdir/SHA256SUMS")
  if [ -z "$want" ]; then
    reason="SHA256SUMS has no entry for $asset"
    return 1
  fi
  got=$(sha256_of "$tmpdir/$asset") || { reason="no sha256sum or shasum to verify the download"; return 1; }
  if [ "$got" != "$want" ]; then
    reason="checksum mismatch for $asset (expected $want, got $got)"
    return 1
  fi

  place "$tmpdir/$asset" || { reason="could not write $out"; return 1; }
  say "installed $out"
  return 0
}

if try_download; then
  exit 0
fi

say "$reason - building from source instead."
if ! have "$go_bin"; then
  say "no prebuilt binary could be installed and Go was not found, so hum cannot be built."
  say "Install Go from https://go.dev/dl/ and then run: herdr plugin install $repo"
  exit 1
fi

cd "$root" || exit 1
mkdir -p bin
if ! "$go_bin" build -trimpath -o "$out.new" ./cmd/hum; then
  rm -f "$out.new"
  say "go build failed; see the output above."
  exit 1
fi
chmod 755 "$out.new" && mv -f "$out.new" "$out" || { say "could not write $out"; exit 1; }
say "built $out"
