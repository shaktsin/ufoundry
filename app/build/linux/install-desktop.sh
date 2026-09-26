#!/usr/bin/env bash
set -euo pipefail
APP_DIR="$(cd "$(dirname "$0")" && pwd)"
DESKTOP_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
ICON_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor/512x512/apps"
mkdir -p "$DESKTOP_DIR" "$ICON_DIR"
install -m 644 "$APP_DIR/ufoundry.png" "$ICON_DIR/ufoundry.png"
launcher="${APP_DIR//\\/\\\\}/ufoundry-launch"
icon="${ICON_DIR//\\/\\\\}/ufoundry.png"
launcher="${launcher//\"/\\\"}"
icon="${icon//\"/\\\"}"
sed "s|@APP_LAUNCHER@|\"$launcher\"|g; s|@ICON_PATH@|$icon|g" \
    "$APP_DIR/ufoundry.desktop" > "$DESKTOP_DIR/ufoundry.desktop"
if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database "$DESKTOP_DIR" >/dev/null 2>&1 || true
fi
echo "UMCode desktop entry installed for this user."
