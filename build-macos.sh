#!/usr/bin/env bash
# Compila o instalador de aula para macOS (File Browser Quantum + Tao + Lúa).
#
# Descarga das Releases os binarios de macOS e comproba o SHA-256 de cada un (se non
# coincide, para). Metoos en files/ e compila o instalador con Wails v3 coma un
# instalador-aula.app universal (Apple Silicon e Intel), asinado ad hoc.
#
#   ./build-macos.sh
#   TAO_VERSION=26.11.5 LUA_VERSION=26.11.1 ./build-macos.sh
#
# Precisa: go, node/npm, wails3, task, curl e as Command Line Tools de Xcode
# (xcode-select --install: compilador de C para cgo, lipo e codesign).
set -euo pipefail

TAO_VERSION="${TAO_VERSION:-26.11.5}"
LUA_VERSION="${LUA_VERSION:-26.11.1}"
FB_VERSION="v1.5.6-stable"
# SHA-256 de darwin-{arm64,amd64}-filebrowser da release v1.5.6-stable (fixados).
FB_SHA256_ARM64="dff575a3898e845ae2c0ccc6b254cb4819e36eeed5c2ccf16087c570b7fbd081"
FB_SHA256_AMD64="236dd7445e518f8fcfb94ccb45e3e7fa81a284db0994a2105d953c1df2e76448"
# O mesmo mínimo que Lúa (LSMinimumSystemVersion dos Info.plist de prepararApp).
export MACOSX_DEPLOYMENT_TARGET="12.0"

DIR="$(cd "$(dirname "$0")" && pwd)"
VERSION="$(tr -d '[:space:]' < "$DIR/VERSION")"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin:/opt/homebrew/bin"

for c in go npm wails3 task curl unzip shasum lipo codesign ditto; do
  command -v "$c" >/dev/null || { echo "Non se atopou '$c' no PATH" >&2; exit 1; }
done

TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

# descargar URL DESTINO SHA256
descargar() {
  echo "    $1"
  curl -fsSL -o "$2" "$1"
  local got; got="$(shasum -a 256 "$2" | cut -d' ' -f1)"
  [ "$got" = "$3" ] || { echo "SHA-256 incorrecto en $1: esperado $3, obtido $got" >&2; exit 1; }
}
publicado() { curl -fsSL "$1" | awk '{print $1; exit}'; }

echo "==> File Browser Quantum $FB_VERSION (arm64 + amd64 → universal)"
FB_URL="https://github.com/gtsteffaniak/filebrowser/releases/download/$FB_VERSION"
descargar "$FB_URL/darwin-arm64-filebrowser" "$TMP/fb-arm64" "$FB_SHA256_ARM64"
descargar "$FB_URL/darwin-amd64-filebrowser" "$TMP/fb-amd64" "$FB_SHA256_AMD64"
# A release só trae asinado (ad hoc, polo enlazador de Go) o de arm64, e o de x86_64
# vén sen sinatura. Asínase ad hoc o universal xa verificado: macOS non arrinca en Apple
# Silicon un binario sen sinatura, e o firewall de aplicacións identifica o programa por
# ela.
lipo -create "$TMP/fb-arm64" "$TMP/fb-amd64" -output "$TMP/filebrowser"
codesign --force --sign - --identifier gal.omeu.piztu.filebrowser "$TMP/filebrowser"
codesign -v "$TMP/filebrowser"

echo "==> Tao $TAO_VERSION"
TAO_URL="https://github.com/piztusistemas/tao/releases/download/v$TAO_VERSION/Tao-$TAO_VERSION-macos-universal.zip"
descargar "$TAO_URL" "$TMP/tao.zip" "$(publicado "$TAO_URL.sha256")"
mkdir "$TMP/tao" && unzip -q "$TMP/tao.zip" -d "$TMP/tao"
codesign -v "$TMP/tao/tao"

echo "==> Lúa $LUA_VERSION"
# O zip "darwin_arm64" leva o executable universal (o mesmo ca o "darwin_amd64").
LUA_URL="https://github.com/piztusistemas/lua/releases/download/$LUA_VERSION/lua_${LUA_VERSION}_darwin_arm64.zip"
descargar "$LUA_URL" "$TMP/lua.zip" "$(publicado "$LUA_URL.sha256")"
mkdir "$TMP/lua" && unzip -q "$TMP/lua.zip" -d "$TMP/lua"
LUA_BIN="$(find "$TMP/lua" -type f -name lua | head -n1)"
[ -n "$LUA_BIN" ] || { echo "o zip de Lúa non leva o executable lua" >&2; exit 1; }
# Só se comproba que vén asinado: a súa sinatura liga o Info.plist do Lua.app orixinal,
# e solto codesign -v dá "invalid Info.plist" aínda que o executable estea ben.
codesign -d "$LUA_BIN" 2>/dev/null || { echo "o executable de Lúa non vén asinado" >&2; exit 1; }

echo "==> Enchendo files/"
find "$DIR/files" -mindepth 1 ! -name LEEME.txt -delete
install -m755 "$TMP/filebrowser" "$DIR/files/filebrowser"
install -m755 "$TMP/tao/tao"     "$DIR/files/tao"
install -m755 "$LUA_BIN"         "$DIR/files/lua"
install -m644 "$DIR/build/darwin/tao.icns" "$DIR/files/tao.icns"
install -m644 "$DIR/build/darwin/lua.icns" "$DIR/files/lua.icns"
printf 'File Browser Quantum %s\nTao %s\nLúa %s\n' "$FB_VERSION" "$TAO_VERSION" "$LUA_VERSION" > "$DIR/files/versions.txt"
lipo -info "$DIR/files/filebrowser" "$DIR/files/tao" "$DIR/files/lua"

cd "$DIR"
echo "==> Frontend e bindings"
task common:build:frontend
echo "==> Tests"
CGO_CFLAGS="-mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET" CGO_LDFLAGS="-mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET" \
  go test ./...

echo "==> Compilando (arm64 + amd64)"
mkdir -p build/bin
for ARQ in arm64 amd64; do
  GOOS=darwin GOARCH=$ARQ CGO_ENABLED=1 \
  CGO_CFLAGS="-mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET" CGO_LDFLAGS="-mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET" \
    go build -tags production -trimpath -buildvcs=false -ldflags="-w -s" -o "$TMP/instalador-$ARQ" .
done

echo "==> instalador-aula.app"
APP="build/bin/instalador-aula.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
lipo -create "$TMP/instalador-arm64" "$TMP/instalador-amd64" -output "$APP/Contents/MacOS/instalador-aula"
cp build/darwin/icons.icns "$APP/Contents/Resources/icons.icns"
printf 'APPL????' > "$APP/Contents/PkgInfo"
cat > "$APP/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleName</key>
	<string>Instalador de aula</string>
	<key>CFBundleDisplayName</key>
	<string>Instalador de aula</string>
	<key>CFBundleExecutable</key>
	<string>instalador-aula</string>
	<key>CFBundleIdentifier</key>
	<string>gal.omeu.piztu.instalador-aula</string>
	<key>CFBundleVersion</key>
	<string>$VERSION</string>
	<key>CFBundleShortVersionString</key>
	<string>$VERSION</string>
	<key>CFBundleIconFile</key>
	<string>icons</string>
	<key>LSMinimumSystemVersion</key>
	<string>$MACOSX_DEPLOYMENT_TARGET</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>NSHumanReadableCopyright</key>
	<string>Copyright © Piztu</string>
</dict>
</plist>
EOF
codesign --force --deep --sign - "$APP"
codesign -v "$APP"
lipo -info "$APP/Contents/MacOS/instalador-aula"

# ditto (non zip): conserva permisos, sinatura e estrutura do .app.
ZIP="build/bin/instalador-aula-macos_$VERSION.zip"
rm -f "$ZIP"
ditto -c -k --keepParent --norsrc --noextattr --noqtn "$APP" "$ZIP"
( cd build/bin && shasum -a 256 "$(basename "$ZIP")" > "$(basename "$ZIP").sha256" )
echo "Compilado: $DIR/$APP"
echo "       Zip: $DIR/$ZIP"
cat "$ZIP.sha256"
