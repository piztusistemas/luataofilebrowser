#!/usr/bin/env bash
# Compila o instalador de aula para Debian (File Browser Quantum + Tao + Lúa).
#
# Descarga das Releases os binarios de Linux e comproba o SHA-256 de cada un (se non
# coincide, para). Metoos en files/ e compila o instalador con Wails v3.
#
#   ./build-linux.sh
#   TAO_VERSION=26.11.5 LUA_VERSION=26.11.1 ./build-linux.sh
#
# Precisa: go, node/npm, wails3, task, curl, unzip, e as librerías de desenvolvemento
# de Wails: sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
set -euo pipefail

TAO_VERSION="${TAO_VERSION:-26.11.5}"
LUA_VERSION="${LUA_VERSION:-26.11.1}"
FB_VERSION="v1.5.6-stable"
# SHA-256 de linux-amd64-filebrowser da release v1.5.6-stable (fixado).
FB_SHA256="febf1ded3368eac1f13481f413db272c57678b70f09e74f5513d5e25e0bfb0e5"

DIR="$(cd "$(dirname "$0")" && pwd)"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"

for c in go npm wails3 task curl unzip sha256sum; do
  command -v "$c" >/dev/null || { echo "Non se atopou '$c' no PATH" >&2; exit 1; }
done

TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

# descargar URL DESTINO SHA256
descargar() {
  echo "    $1"
  curl -fsSL -o "$2" "$1"
  local got; got="$(sha256sum "$2" | cut -d' ' -f1)"
  [ "$got" = "$3" ] || { echo "SHA-256 incorrecto en $1: esperado $3, obtido $got" >&2; exit 1; }
}
publicado() { curl -fsSL "$1" | awk '{print $1; exit}'; }

echo "==> File Browser Quantum $FB_VERSION"
descargar "https://github.com/gtsteffaniak/filebrowser/releases/download/$FB_VERSION/linux-amd64-filebrowser" "$TMP/filebrowser" "$FB_SHA256"

echo "==> Tao $TAO_VERSION"
TAO_URL="https://github.com/piztusistemas/tao/releases/download/v$TAO_VERSION/tao_$TAO_VERSION.zip"
descargar "$TAO_URL" "$TMP/tao.zip" "$(publicado "$TAO_URL.sha256")"
mkdir "$TMP/tao" && unzip -q "$TMP/tao.zip" -d "$TMP/tao"

echo "==> Lúa $LUA_VERSION"
LUA_URL="https://github.com/piztusistemas/lua/releases/download/$LUA_VERSION/lua_${LUA_VERSION}_linux_amd64.zip"
descargar "$LUA_URL" "$TMP/lua.zip" "$(publicado "$LUA_URL.sha256")"
mkdir "$TMP/lua" && unzip -q "$TMP/lua.zip" -d "$TMP/lua"

echo "==> Enchendo files/"
find "$DIR/files" -mindepth 1 ! -name LEEME.txt -delete
install -m755 "$TMP/filebrowser" "$DIR/files/filebrowser"
install -m755 "$TMP/tao/tao"     "$DIR/files/tao"
install -m644 "$TMP/tao/modulo.json" "$DIR/files/tao-modulo.json"
install -m755 "$TMP/lua/lua"     "$DIR/files/lua"
install -m644 "$TMP/lua/modulo.json" "$DIR/files/lua-modulo.json"
install -m644 "$TMP/lua/logo.png"    "$DIR/files/lua-logo.png"
printf 'File Browser Quantum %s\nTao %s\nLúa %s\n' "$FB_VERSION" "$TAO_VERSION" "$LUA_VERSION" > "$DIR/files/versions.txt"

cd "$DIR"
echo "==> Frontend e bindings"
task common:build:frontend
echo "==> Tests"
go test -tags gtk3 ./...
echo "==> Compilando"
mkdir -p build/bin
go build -tags production,gtk3 -trimpath -buildvcs=false -ldflags="-w -s" -o build/bin/instalador-aula .
( cd build/bin && sha256sum instalador-aula > instalador-aula.sha256 )
echo "Compilado: $DIR/build/bin/instalador-aula"
