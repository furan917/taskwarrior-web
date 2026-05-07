#!/usr/bin/env bash
# Uninstall taskwarrior-web's LaunchAgent and clean up traces.
# Logs in ~/Library/Logs/taskwarrior-web are preserved.

set -euo pipefail

LABEL="local.taskwarrior-web"
PLIST_DST="$HOME/Library/LaunchAgents/${LABEL}.plist"
BIN_DST="$HOME/.local/bin/taskwarrior-web"

# Loose pattern that matches any tw alias the install script might have written
# (covers both macOS `open ...` and Linux `xdg-open ...` flavours, and any
# port the user may have customised). Anchored on the alias keyword + the
# loopback URL so unrelated `tw` aliases users may have set up themselves
# stay untouched.
ALIAS_PATTERN="alias tw=.*127\.0\.0\.1:5050"
ALIAS_COMMENT_PATTERN="# taskwarrior-web (added by install-launchd.sh)"

TARGET="gui/$(id -u)/${LABEL}"

# 1. Stop the service first so the binary isn't held open.
if launchctl print "$TARGET" >/dev/null 2>&1; then
    launchctl bootout "$TARGET" 2>/dev/null || true
    echo "service : stopped"
else
    echo "service : not loaded"
fi

# Wait briefly for it to release.
for _ in 1 2 3; do
    if ! pgrep -f "$BIN_DST" >/dev/null 2>&1; then break; fi
    sleep 1
done

# 2. Remove plist.
if [[ -f "$PLIST_DST" ]]; then
    rm "$PLIST_DST"
    echo "plist   : removed"
fi

# 3. Remove binary.
if [[ -f "$BIN_DST" ]]; then
    rm "$BIN_DST"
    echo "binary  : removed"
fi

# 4. Strip alias from every shell config we know about. Defensive: we don't
#    know which (if any) shell the user installed the alias into, and they
#    may have switched shells since. Each file is checked independently.
#    BSD sed (macOS) needs the empty '' after -i; GNU sed (Linux) accepts
#    -i alone but tolerates -i ''.
strip_alias_from() {
    local cfg="$1"
    [[ -f "$cfg" ]] || return 0
    if grep -qE "$ALIAS_PATTERN" "$cfg" 2>/dev/null; then
        sed -i '' -E "/${ALIAS_PATTERN}/d" "$cfg" 2>/dev/null \
            || sed -i -E "/${ALIAS_PATTERN}/d" "$cfg"
        sed -i '' "/${ALIAS_COMMENT_PATTERN}/d" "$cfg" 2>/dev/null \
            || sed -i "/${ALIAS_COMMENT_PATTERN}/d" "$cfg"
        echo "alias   : removed from $cfg"
    fi
}

strip_alias_from "$HOME/.zshrc"
strip_alias_from "$HOME/.bashrc"
strip_alias_from "$HOME/.bash_profile"
strip_alias_from "$HOME/.config/fish/config.fish"

echo
echo "Uninstalled. Logs at ~/Library/Logs/taskwarrior-web/ kept."
