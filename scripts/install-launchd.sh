#!/usr/bin/env bash
# Idempotent install of taskwarrior-web as a user LaunchAgent.
# Re-run safely; existing service is bootout'd before bootstrap.

set -euo pipefail

LABEL="local.taskwarrior-web"
PLIST_TMPL="$(cd "$(dirname "$0")/.." && pwd)/deploy/${LABEL}.plist.tmpl"
PLIST_DST="$HOME/Library/LaunchAgents/${LABEL}.plist"
BIN_SRC="$(cd "$(dirname "$0")/.." && pwd)/bin/taskwarrior-web"
BIN_DST="$HOME/.local/bin/taskwarrior-web"
LOG_DIR="$HOME/Library/Logs/taskwarrior-web"
URL="http://127.0.0.1:5050"

# OS detection. macOS uses `open` to launch URLs; Linux uses `xdg-open`. The
# alias resolves at install time so users never have to think about it.
case "$(uname -s)" in
    Darwin) OPEN_CMD="open" ;;
    Linux)  OPEN_CMD="xdg-open" ;;
    *)      OPEN_CMD="open" ;;  # best-effort fallback
esac
ALIAS_LINE="alias tw='${OPEN_CMD} ${URL}'"
ALIAS_COMMENT="# taskwarrior-web (added by install-launchd.sh)"

# 1. Verify build artefact present.
if [[ ! -x "$BIN_SRC" ]]; then
    echo "error: $BIN_SRC not found. Run 'make build' first." >&2
    exit 1
fi

# 2. Install binary to a stable absolute path.
mkdir -p "$(dirname "$BIN_DST")"
install -m 0755 "$BIN_SRC" "$BIN_DST"
echo "installed: $BIN_DST"

# 3. Log directory (mode 700; logs may contain operational events).
mkdir -p "$LOG_DIR"
chmod 700 "$LOG_DIR"
echo "log dir : $LOG_DIR"

# 4. Set ~/.task to user-only (defence in depth - Taskwarrior typically does
#    this already, but enforce it).
if [[ -d "$HOME/.task" ]]; then
    chmod 700 "$HOME/.task"
fi

# 5. Render plist template into ~/Library/LaunchAgents (atomic write via mktemp).
mkdir -p "$(dirname "$PLIST_DST")"
TMP_PLIST="$(mktemp "${TMPDIR:-/tmp}/${LABEL}.XXXXXX.plist")"
sed -e "s|__BIN__|$BIN_DST|g" -e "s|__LOG_DIR__|$LOG_DIR|g" "$PLIST_TMPL" > "$TMP_PLIST"
mv "$TMP_PLIST" "$PLIST_DST"
chmod 644 "$PLIST_DST"
echo "plist   : $PLIST_DST"

# 6. Bootstrap (re-bootstrap idempotently). bootout returns asynchronously, so
#    spin briefly until the service actually disappears before bootstrap to
#    avoid "Input/output error: 5" when launchd is still mid-unload.
TARGET="gui/$(id -u)/${LABEL}"
if launchctl print "$TARGET" >/dev/null 2>&1; then
    launchctl bootout "$TARGET" 2>/dev/null || true
fi
for _ in 1 2 3 4 5; do
    launchctl print "$TARGET" >/dev/null 2>&1 || break
    sleep 1
done
launchctl bootstrap "gui/$(id -u)" "$PLIST_DST"
echo "service : bootstrapped"

# 7. Optionally append a `tw` alias. Off by default so a fresh install never
#    silently mutates the user's shell config. Opt in via INSTALL_ALIAS:
#
#      INSTALL_ALIAS=1     auto-detect login shell from $SHELL
#      INSTALL_ALIAS=zsh   write to ~/.zshrc
#      INSTALL_ALIAS=bash  write to ~/.bash_profile (macOS) or ~/.bashrc (Linux)
#      INSTALL_ALIAS=fish  write to ~/.config/fish/config.fish
#      INSTALL_ALIAS=all   write to every shell config above (skipping shells
#                          whose config dir doesn't exist)
#
# Idempotent: re-running won't duplicate the line.

config_for_shell() {
    case "$1" in
        zsh) echo "$HOME/.zshrc" ;;
        bash)
            # macOS login shells source .bash_profile; Linux uses .bashrc.
            # Prefer whichever exists; fall back to the platform default.
            if [[ -f "$HOME/.bash_profile" ]]; then
                echo "$HOME/.bash_profile"
            elif [[ -f "$HOME/.bashrc" ]]; then
                echo "$HOME/.bashrc"
            elif [[ "$(uname -s)" == "Darwin" ]]; then
                echo "$HOME/.bash_profile"
            else
                echo "$HOME/.bashrc"
            fi
            ;;
        fish) echo "$HOME/.config/fish/config.fish" ;;
        *) echo "" ;;
    esac
}

append_alias_to() {
    local cfg="$1"
    [[ -z "$cfg" ]] && return 2
    mkdir -p "$(dirname "$cfg")"
    touch "$cfg"
    if grep -qxF "$ALIAS_LINE" "$cfg" 2>/dev/null; then
        echo "alias   : 'tw' already in $cfg"
        return 0
    fi
    {
        echo ""
        echo "$ALIAS_COMMENT"
        echo "$ALIAS_LINE"
    } >> "$cfg"
    echo "alias   : added 'tw' to $cfg"
    return 0
}

resolve_shells() {
    case "$1" in
        1|auto)
            local s="${SHELL##*/}"
            case "$s" in
                zsh|bash|fish) echo "$s" ;;
                *)
                    echo "alias   : couldn't detect a supported shell from \$SHELL=$SHELL" >&2
                    echo "alias   :   (set INSTALL_ALIAS=zsh|bash|fish|all explicitly)" >&2
                    return 1
                    ;;
            esac
            ;;
        zsh|bash|fish) echo "$1" ;;
        all) echo "zsh bash fish" ;;
        *)
            echo "alias   : INSTALL_ALIAS=$1 not recognised (use 1, zsh, bash, fish, or all)" >&2
            return 1
            ;;
    esac
}

case "${INSTALL_ALIAS:-0}" in
    0|"")
        echo "alias   : skipped (re-run with INSTALL_ALIAS=1|zsh|bash|fish|all to add: $ALIAS_LINE)"
        ;;
    *)
        if shells="$(resolve_shells "$INSTALL_ALIAS")"; then
            for s in $shells; do
                append_alias_to "$(config_for_shell "$s")"
            done
        fi
        ;;
esac

# 8. Smoke test.
sleep 1
if curl -sS -o /dev/null -w '' --max-time 3 "${URL}/healthz"; then
    echo "health  : ok"
else
    echo "health  : FAILED - check $LOG_DIR/err.log" >&2
    exit 1
fi

echo
echo "Installed. Open ${URL} (or run 'tw' in a new terminal once the alias is loaded)."
