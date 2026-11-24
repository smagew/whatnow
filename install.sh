#!/bin/sh
# Installs the skill into Claude Code's skills folder and builds the whatnow program next to it
# (~/.claude/skills/whatnow/whatnow: the hook, the commands, the server and the desktop window in one file).
# Run it after you change anything in ./skill or ./app. Needs Go (https://go.dev/dl/).
set -e
HERE="$(cd "$(dirname "$0")" && pwd)"
DEST="$HOME/.claude/skills/whatnow"
mkdir -p "$DEST/examples"
find "$HERE/skill" -maxdepth 1 -type f -exec cp {} "$DEST"/ \;
rm -f "$DEST"/*.py; rm -rf "$DEST/__pycache__"   # the earlier Python version
find "$HERE/examples" -maxdepth 1 -type f -exec cp {} "$DEST/examples/" \;

if ! command -v go >/dev/null 2>&1; then
  echo "Go is not installed: the whatnow program was not built. Install Go and run ./install.sh again." >&2
  exit 1
fi
cd "$HERE"
EXE=whatnow
case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) EXE=whatnow.exe ;; esac
TMP="$DEST/.new-$EXE"
# With the window (Wails, system WebView). Where the WebView libraries are missing, build the commands only.
case "$(uname -s)" in
  Darwin)
    CGO_LDFLAGS="-framework UniformTypeIdentifiers -mmacosx-version-min=10.13" \
      go build -tags desktop,production -ldflags "-s -w" -o "$TMP" ./app 2>/dev/null \
      || go build -ldflags "-s -w" -o "$TMP" ./app ;;
  Linux)   # WebKitGTK 4.1 (Debian 12, Ubuntu 22.04 and newer), then 4.0, then no window
    go build -tags desktop,production,webkit2_41 -ldflags "-s -w" -o "$TMP" ./app 2>/dev/null \
      || go build -tags desktop,production -ldflags "-s -w" -o "$TMP" ./app 2>/dev/null \
      || { echo "WebKitGTK not found (libwebkit2gtk-4.1-dev): building without the window" >&2;
           go build -ldflags "-s -w" -o "$TMP" ./app; } ;;
  *)       # Windows (Git Bash): WebView2 needs no C compiler
    go build -tags desktop,production -ldflags "-s -w -H windowsgui" -o "$TMP" ./app ;;
esac
mv -f "$TMP" "$DEST/$EXE"   # a rename: hooks running right now keep the old file

# The autostarted server runs this program: restart it so it serves the new pages.
if [ "$(uname)" = "Darwin" ] && [ -f "$HOME/Library/LaunchAgents/whatnow.server.plist" ] \
   && grep -q "$DEST/whatnow" "$HOME/Library/LaunchAgents/whatnow.server.plist"; then
  launchctl kickstart -k "gui/$(id -u)/whatnow.server" >/dev/null 2>&1 || true
elif command -v systemctl >/dev/null 2>&1 && [ -f "$HOME/.config/systemd/user/whatnow.service" ]; then
  systemctl --user restart whatnow.service >/dev/null 2>&1 || true
fi
echo "Installed to $DEST"
