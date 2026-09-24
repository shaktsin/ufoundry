#!/usr/bin/env bash
# Build UFoundry.app: the Wails shell, the engine binary and the LaunchAgent
# that runs the engine in the background. Run this on macOS.
#
#   app/build/macos/bundle.sh [--universal] [--sign "Developer ID Application: …"]
#
# Without --sign the bundle is ad-hoc signed, which is enough to run it
# locally; notarization needs a real Developer ID certificate.
set -euo pipefail

cd "$(dirname "$0")/../../.."   # repo root
REPO="$PWD"
APP_DIR="$REPO/app"
OUT="$REPO/bin/UFoundry.app"
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
UNIVERSAL=0
SIGN_ID=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --universal) UNIVERSAL=1; shift ;;
        --sign) SIGN_ID="$2"; shift 2 ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
done

if [[ "$(uname -s)" != "Darwin" ]]; then
    echo "This builds a macOS app bundle; run it on a Mac." >&2
    exit 1
fi

echo "==> Frontend"
(cd "$APP_DIR/frontend" && npm install --silent && npm run build --silent)

ENGINE_LDFLAGS="-s -w -X github.com/shaktsin/ufoundry/internal/version.Version=$VERSION -X github.com/shaktsin/ufoundry/internal/version.Commit=$COMMIT"
APP_LDFLAGS="-s -w -X main.Version=$VERSION"

build_engine() { # $1=arch $2=out
    GOOS=darwin GOARCH="$1" CGO_ENABLED=1 go build -trimpath -ldflags "$ENGINE_LDFLAGS" -o "$2" ./cmd/ufoundry
}
build_app() { # $1=arch $2=out
    (cd "$APP_DIR" && GOOS=darwin GOARCH="$1" CGO_ENABLED=1 go build -trimpath -tags production \
        -ldflags "$APP_LDFLAGS" -o "$2" .)
}

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "==> Engine and app binaries ($( ((UNIVERSAL)) && echo "arm64 + amd64" || echo "$(uname -m)" ))"
if ((UNIVERSAL)); then
    build_engine arm64 "$TMP/engine-arm64"
    build_engine amd64 "$TMP/engine-amd64"
    lipo -create -output "$TMP/engine-bin" "$TMP/engine-arm64" "$TMP/engine-amd64"
    build_app arm64 "$TMP/app-arm64"
    build_app amd64 "$TMP/app-amd64"
    lipo -create -output "$TMP/app-bin" "$TMP/app-arm64" "$TMP/app-amd64"
else
    ARCH="$(uname -m)"; [[ "$ARCH" == "x86_64" ]] && ARCH=amd64 || ARCH=arm64
    # Never use names that differ only by case here. The default macOS file
    # system is case-insensitive, so `ufoundry` and `UFoundry` are one file.
    build_engine "$ARCH" "$TMP/engine-bin"
    build_app "$ARCH" "$TMP/app-bin"
fi

echo "==> Bundle"
rm -rf "$OUT"
mkdir -p "$OUT/Contents/MacOS" "$OUT/Contents/Resources" "$OUT/Contents/Library/LaunchAgents"
cp "$TMP/app-bin" "$OUT/Contents/MacOS/UFoundry"
# The engine goes in Resources, not next to the app binary: the Mac's file
# system ignores case, so Contents/MacOS/ufoundry and .../UFoundry would be the
# same file and the engine would overwrite the app.
cp "$TMP/engine-bin" "$OUT/Contents/Resources/ufoundry"
sed "s/__VERSION__/${VERSION#v}/g" "$APP_DIR/build/macos/Info.plist" > "$OUT/Contents/Info.plist"
cp "$APP_DIR/build/macos/com.ufoundry.engine.plist" "$OUT/Contents/Library/LaunchAgents/"
printf 'APPL????' > "$OUT/Contents/PkgInfo"

# Icon: build an .icns from the PNG when the tools are there.
if command -v iconutil >/dev/null && command -v sips >/dev/null; then
    ICONSET="$TMP/appicon.iconset"; mkdir -p "$ICONSET"
    for size in 16 32 64 128 256 512; do
        sips -z $size $size "$APP_DIR/icons/appicon.png" --out "$ICONSET/icon_${size}x${size}.png" >/dev/null
        sips -z $((size * 2)) $((size * 2)) "$APP_DIR/icons/appicon.png" --out "$ICONSET/icon_${size}x${size}@2x.png" >/dev/null
    done
    iconutil -c icns "$ICONSET" -o "$OUT/Contents/Resources/appicon.icns"
fi

# Guard against the case-insensitivity trap coming back.
if ! cmp -s "$TMP/app-bin" "$OUT/Contents/MacOS/UFoundry"; then
    echo "The app binary in the bundle is not the app; check where the engine was copied." >&2
    exit 1
fi
if cmp -s "$OUT/Contents/MacOS/UFoundry" "$OUT/Contents/Resources/ufoundry"; then
    echo "The app and engine binaries are identical; the bundle is invalid." >&2
    exit 1
fi
if ! "$OUT/Contents/Resources/ufoundry" version | grep -q '^ufoundry '; then
    echo "The bundled engine does not identify itself as the ufoundry CLI." >&2
    exit 1
fi

echo "==> Signing"
if [[ -n "$SIGN_ID" ]]; then
    codesign --force --options runtime --timestamp --sign "$SIGN_ID" "$OUT/Contents/Resources/ufoundry"
    codesign --force --options runtime --timestamp --sign "$SIGN_ID" "$OUT"
    echo "Signed with: $SIGN_ID"
    echo "Notarize with: xcrun notarytool submit --keychain-profile <profile> --wait <zip> && xcrun stapler staple '$OUT'"
else
    codesign --force --deep --sign - "$OUT"
    echo "Ad-hoc signed. SMAppService login items need a Developer ID signature to register."
fi

echo "==> Done: $OUT"
