#!/usr/bin/env bash
# Idempotent install of taskwarrior-web as a user-level service.
#
# macOS: writes a LaunchAgent plist into ~/Library/LaunchAgents/ and bootstraps
# it with `launchctl bootstrap gui/$(id -u)`.
#
# Linux (systemd-based distros - Ubuntu, Debian, Arch, RHEL, Fedora, openSUSE,
# etc.): writes a systemd user unit into ~/.config/systemd/user/ and enables
# it with `systemctl --user enable --now`.
#
# Re-run safely; both code paths bootout/disable any existing service before
# (re-)installing. Non-systemd Linux distros (Alpine, Void, Devuan) are NOT
# supported - the script bails with a helpful error.

set -euo pipefail

# --- shared constants --------------------------------------------------------
LABEL="taskwarrior-web"
PLIST_LABEL="local.taskwarrior-web" # macOS-only: launchd label keeps the
                                    # `local.` prefix as a courtesy domain hint
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_SRC="${REPO_ROOT}/bin/taskwarrior-web"
BIN_DST="$HOME/.local/bin/taskwarrior-web"
URL="http://127.0.0.1:5050"

# OS detection. We use the result both for the `tw` shell alias (open vs
# xdg-open) and to pick the service-install code path.
OS="$(uname -s)"
case "$OS" in
    Darwin) OPEN_CMD="open" ;;
    Linux)  OPEN_CMD="xdg-open" ;;
    *)
        echo "error: unsupported OS '$OS'. taskwarrior-web's install script handles macOS and Linux only." >&2
        exit 1
        ;;
esac
ALIAS_LINE="alias tw='${OPEN_CMD} ${URL}'"
ALIAS_COMMENT="# taskwarrior-web (added by install.sh)"

# --- shared install steps (run on every OS) ---------------------------------

# 1. Verify build artefact present.
if [[ ! -x "$BIN_SRC" ]]; then
    echo "error: $BIN_SRC not found. Run 'make build' first." >&2
    exit 1
fi

# 2. Install binary to a stable absolute path.
mkdir -p "$(dirname "$BIN_DST")"
install -m 0755 "$BIN_SRC" "$BIN_DST"
echo "installed: $BIN_DST"

# 3. Set ~/.task to user-only (defence in depth - Taskwarrior typically does
#    this already, but enforce it). Matches the binary's startup data-dir
#    permission check.
if [[ -d "$HOME/.task" ]]; then
    chmod 700 "$HOME/.task"
fi

# --- macOS service install --------------------------------------------------
install_darwin() {
    local plist_tmpl="${REPO_ROOT}/deploy/${PLIST_LABEL}.plist.tmpl"
    local plist_dst="$HOME/Library/LaunchAgents/${PLIST_LABEL}.plist"
    local log_dir="$HOME/Library/Logs/taskwarrior-web"

    # Log dir mode 700 - logs may carry operational events incl. task data.
    mkdir -p "$log_dir"
    chmod 700 "$log_dir"
    echo "log dir : $log_dir"

    # Render plist template. Atomic write via mktemp + mv so a crash mid-write
    # never leaves a half-rendered plist that launchctl tries to parse. The
    # tempfile lives IN the destination directory (not $TMPDIR) so the mv is
    # always a same-filesystem rename, never a copy+unlink that could expose
    # a partial file to launchd's parser between the writes.
    mkdir -p "$(dirname "$plist_dst")"
    local tmp_plist
    tmp_plist="$(mktemp "$(dirname "$plist_dst")/${PLIST_LABEL}.XXXXXX.plist")"
    sed -e "s|__BIN__|$BIN_DST|g" -e "s|__LOG_DIR__|$log_dir|g" "$plist_tmpl" > "$tmp_plist"
    mv "$tmp_plist" "$plist_dst"
    chmod 644 "$plist_dst"
    echo "plist   : $plist_dst"

    # Bootstrap (re-bootstrap idempotently). bootout returns asynchronously,
    # so spin briefly until the service actually disappears before bootstrap
    # to avoid "Input/output error: 5" when launchd is still mid-unload.
    local target="gui/$(id -u)/${PLIST_LABEL}"
    if launchctl print "$target" >/dev/null 2>&1; then
        launchctl bootout "$target" 2>/dev/null || true
    fi
    for _ in 1 2 3 4 5; do
        launchctl print "$target" >/dev/null 2>&1 || break
        sleep 1
    done
    launchctl bootstrap "gui/$(id -u)" "$plist_dst"
    echo "service : bootstrapped (launchd)"
}

# --- Linux service install --------------------------------------------------
install_linux() {
    if ! command -v systemctl >/dev/null 2>&1; then
        cat >&2 <<EOF
error: systemctl not found. taskwarrior-web's Linux install requires
       systemd-based init (Ubuntu, Debian, Arch, RHEL, Fedora, openSUSE,
       Manjaro, etc.). Non-systemd distros (Alpine, Void, Devuan, Slackware)
       can run the binary directly via:

         $BIN_DST &

       and supervise it with whatever process manager they prefer (s6,
       runit, OpenRC, supervisord, ...).
EOF
        exit 1
    fi

    local unit_tmpl="${REPO_ROOT}/deploy/${LABEL}.service.tmpl"
    local unit_dst="$HOME/.config/systemd/user/${LABEL}.service"
    # Resolve XDG_STATE_HOME to its concrete value at install time. The
    # template carries an Environment= line pinning this so the systemd
    # --user manager (which inherits a curated env, NOT the user's shell
    # rc-file vars) and install.sh always agree on the log dir, even if
    # the user has a custom XDG_STATE_HOME in ~/.zshrc.
    local xdg_state_home="${XDG_STATE_HOME:-$HOME/.local/state}"
    local log_dir="${xdg_state_home}/taskwarrior-web"

    # Log dir mode 700 - same rationale as macOS path. Binary creates this
    # itself on startup if missing, but we mkdir here too so the path is
    # visible in the install summary.
    mkdir -p "$log_dir"
    chmod 700 "$log_dir"
    echo "log dir : $log_dir"

    # Render systemd unit template. Atomic write via mktemp + mv. Tempfile
    # lives in the destination directory so the mv is always a same-fs
    # rename, never a copy+unlink that could expose a partial unit to
    # `systemctl daemon-reload` between writes.
    mkdir -p "$(dirname "$unit_dst")"
    local tmp_unit
    tmp_unit="$(mktemp "$(dirname "$unit_dst")/${LABEL}.XXXXXX.service")"
    sed -e "s|__BIN__|$BIN_DST|g" -e "s|__XDG_STATE_HOME__|$xdg_state_home|g" "$unit_tmpl" > "$tmp_unit"
    mv "$tmp_unit" "$unit_dst"
    chmod 644 "$unit_dst"
    echo "unit    : $unit_dst"

    # Reload + enable + start. systemctl is idempotent for these operations:
    # daemon-reload picks up the new unit, enable creates the WantedBy symlink
    # (no-op if already present), and `restart` covers both first-install
    # (start) and re-install (replace running instance).
    systemctl --user daemon-reload
    systemctl --user enable "${LABEL}.service" >/dev/null
    systemctl --user restart "${LABEL}.service"
    echo "service : enabled + started (systemd --user)"

    # Linger guidance. Without it the service stops at logout and only
    # restarts on next login; for a desktop user this is usually fine. If
    # the user wants it persistent across login sessions they need to opt
    # in explicitly with sudo.
    if ! loginctl show-user "$USER" 2>/dev/null | grep -q '^Linger=yes$'; then
        echo "linger  : off (service stops at logout)"
        echo "          run 'sudo loginctl enable-linger $USER' to keep it running across sessions"
    else
        echo "linger  : on  (service persists across logout)"
    fi
}

case "$OS" in
    Darwin) install_darwin ;;
    Linux)  install_linux ;;
esac

# --- shared post-service: optional shell alias + smoke test -----------------

# Optionally append a `tw` alias. Off by default so a fresh install never
# silently mutates the user's shell config. Opt in via INSTALL_ALIAS:
#
#   INSTALL_ALIAS=1     auto-detect login shell from $SHELL
#   INSTALL_ALIAS=zsh   write to ~/.zshrc
#   INSTALL_ALIAS=bash  write to ~/.bash_profile (macOS) or ~/.bashrc (Linux)
#   INSTALL_ALIAS=fish  write to ~/.config/fish/config.fish
#   INSTALL_ALIAS=all   write to every shell config above (skipping shells
#                       whose config dir doesn't exist)
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
            elif [[ "$OS" == "Darwin" ]]; then
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

# Smoke test - poll up to 5s for the service to answer on loopback. The
# original 1s sleep was tight on cold-cache Linux (especially Pi-class or
# RHEL VM first run with SQLite open + Go runtime init); a small retry loop
# is harmless on fast machines and saves us a "FAILED" false alarm on slow
# ones. If still not answering after 5s, surface a per-OS log path hint.
health_ok=0
for _ in 1 2 3 4 5 6 7 8 9 10; do
    if curl -sS -o /dev/null -w '' --max-time 1 "${URL}/healthz"; then
        health_ok=1
        break
    fi
    sleep 0.5
done
if [[ $health_ok -eq 1 ]]; then
    echo "health  : ok"
else
    echo "health  : FAILED" >&2
    case "$OS" in
        Darwin) echo "          check ~/Library/Logs/taskwarrior-web/{out,err,app}.log" >&2 ;;
        Linux)  echo "          check 'journalctl --user -u ${LABEL} -n 50' and \${XDG_STATE_HOME:-~/.local/state}/taskwarrior-web/app.log" >&2 ;;
    esac
    exit 1
fi

echo
echo "Installed. Open ${URL} (or run 'tw' in a new terminal once the alias is loaded)."
