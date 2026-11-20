#!/bin/sh
# Copies the skill from this project into Claude Code's skills folder.
# Run it after you change anything in ./skill.
set -e
HERE="$(cd "$(dirname "$0")" && pwd)"
DEST="$HOME/.claude/skills/whatnow"
mkdir -p "$DEST/examples"
cp "$HERE"/skill/* "$DEST"/
cp "$HERE"/examples/* "$DEST/examples/"
echo "Installed to $DEST"
