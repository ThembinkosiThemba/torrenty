#!/bin/sh
# Installs torrenty for the current user and adds it to the app menu.
set -e

here=$(cd "$(dirname "$0")" && pwd)
bin="$HOME/.local/bin"
data="${XDG_DATA_HOME:-$HOME/.local/share}"
apps="$data/applications"
icons="$data/icons/hicolor/256x256/apps"

mkdir -p "$bin" "$apps" "$icons"
install -m 755 "$here/torrenty" "$bin/torrenty"
install -m 644 "$here/torrenty.png" "$icons/torrenty.png"

# Terminal=true: torrenty is a TUI, so the launcher opens it in a terminal.
cat > "$apps/torrenty.desktop" <<DESKTOP
[Desktop Entry]
Type=Application
Name=torrenty
Comment=Search movies and series and save their .torrent files
Exec=$bin/torrenty
Icon=torrenty
Terminal=true
Categories=Network;FileTransfer;
DESKTOP

command -v update-desktop-database >/dev/null && update-desktop-database "$apps" || true

echo "Installed $bin/torrenty"
echo "Launch it from your app menu, or run: torrenty"
command -v aria2c >/dev/null || echo "Tip: install aria2 to save .torrent files (sudo apt install aria2)"
