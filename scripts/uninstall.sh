#!/usr/bin/env bash
# Uninstall taskwarrior-web-portal's user-level service and clean up traces.
# Logs (in ~/Library/Logs/taskwarrior-web-portal on macOS, ${XDG_STATE_HOME:-
# ~/.local/state}/taskwarrior-web-portal on Linux) are PRESERVED so users can
# review the historical record after removal.

set -euo pipefail

LABEL="taskwarrior-web-portal"
PLIST_LABEL="local.taskwarrior-web-portal"
BIN_DST="$HOME/.local/bin/taskwarrior-web-portal"

# Loose pattern that matches any tw alias the install script might have
# written (covers both macOS `open ...` and Linux `xdg-open ...` flavours).
# Anchored on the alias keyword + the loopback URL so unrelated `tw`
# aliases the user may have set up themselves stay untouched.
ALIAS_PATTERN="alias tw=.*127\.0\.0\.1:5050"
# Match BOTH the new install.sh comment AND the old install-launchd.sh
# comment from before the script was renamed - ensures upgrades don't
# leave a stranded comment line.
ALIAS_COMMENT_PATTERN_NEW="# taskwarrior-web-portal (added by install.sh)"
ALIAS_COMMENT_PATTERN_OLD="# taskwarrior-web-portal (added by install-launchd.sh)"

OS="$(uname -s)"

uninstall_darwin() {
    local plist_dst="$HOME/Library/LaunchAgents/${PLIST_LABEL}.plist"
    local target="gui/$(id -u)/${PLIST_LABEL}"

    # 1. Stop the service first so the binary isn't held open during rm.
    if launchctl print "$target" >/dev/null 2>&1; then
        launchctl bootout "$target" 2>/dev/null || true
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
    if [[ -f "$plist_dst" ]]; then
        rm "$plist_dst"
        echo "plist   : removed"
    fi
}

uninstall_linux() {
    local unit_dst="$HOME/.config/systemd/user/${LABEL}.service"

    if command -v systemctl >/dev/null 2>&1; then
        # 1. Stop + disable. `is-enabled` / `is-active` checks would race
        #    with the next call; `stop` and `disable` are idempotent and
        #    only return non-zero when systemctl itself fails - which is
        #    why the `|| true` is gated on systemctl-presence above.
        systemctl --user stop "${LABEL}.service" 2>/dev/null || true
        systemctl --user disable "${LABEL}.service" 2>/dev/null || true
        echo "service : stopped + disabled"
    else
        echo "service : systemctl not present (skipped)"
    fi

    # 2. Remove unit file. Wait briefly for the binary to release.
    for _ in 1 2 3; do
        if ! pgrep -f "$BIN_DST" >/dev/null 2>&1; then break; fi
        sleep 1
    done
    if [[ -f "$unit_dst" ]]; then
        rm "$unit_dst"
        echo "unit    : removed"
    fi

    # 3. Reload so systemd forgets the unit. Best-effort.
    if command -v systemctl >/dev/null 2>&1; then
        systemctl --user daemon-reload 2>/dev/null || true
    fi
}

case "$OS" in
    Darwin) uninstall_darwin ;;
    Linux)  uninstall_linux ;;
    *)
        echo "warn: unknown OS '$OS'; skipping service teardown (will still remove binary + alias)" >&2
        ;;
esac

# Shared cleanup: binary + alias.

if [[ -f "$BIN_DST" ]]; then
    rm "$BIN_DST"
    echo "binary  : removed"
fi

# Strip alias from every shell config we know about. Defensive: we don't
# know which (if any) shell the user installed the alias into, and they
# may have switched shells since. Each file is checked independently.
# BSD sed (macOS) needs the empty '' after -i; GNU sed (Linux) accepts
# -i alone but tolerates -i ''.
strip_alias_from() {
    local cfg="$1"
    [[ -f "$cfg" ]] || return 0
    local hit=0
    if grep -qE "$ALIAS_PATTERN" "$cfg" 2>/dev/null; then
        sed -i '' -E "/${ALIAS_PATTERN}/d" "$cfg" 2>/dev/null \
            || sed -i -E "/${ALIAS_PATTERN}/d" "$cfg"
        hit=1
    fi
    for pat in "$ALIAS_COMMENT_PATTERN_NEW" "$ALIAS_COMMENT_PATTERN_OLD"; do
        if grep -qF "$pat" "$cfg" 2>/dev/null; then
            sed -i '' "/${pat}/d" "$cfg" 2>/dev/null \
                || sed -i "/${pat}/d" "$cfg"
            hit=1
        fi
    done
    [[ $hit -eq 1 ]] && echo "alias   : removed from $cfg"
    return 0
}

strip_alias_from "$HOME/.zshrc"
strip_alias_from "$HOME/.bashrc"
strip_alias_from "$HOME/.bash_profile"
strip_alias_from "$HOME/.config/fish/config.fish"

echo
case "$OS" in
    Darwin) echo "Uninstalled. Logs at ~/Library/Logs/taskwarrior-web-portal/ kept." ;;
    Linux)  echo "Uninstalled. Logs at \${XDG_STATE_HOME:-~/.local/state}/taskwarrior-web-portal/ kept." ;;
    *)      echo "Uninstalled." ;;
esac
