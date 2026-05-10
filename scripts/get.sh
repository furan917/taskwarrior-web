#!/usr/bin/env sh
# Download the latest taskwarrior-web release for this OS/arch, extract it,
# and run the install script. Supports macOS (amd64/arm64) and Linux (amd64/arm64).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/furan917/taskwarrior-web/main/scripts/get.sh | sh
#
# Optional: prefix with INSTALL_ALIAS=1 to also add a 'tw' shell alias.

set -eu

REPO="furan917/taskwarrior-web"

# --- detect OS ----------------------------------------------------------------
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
    darwin|linux) ;;
    *) echo "error: unsupported OS '$os'" >&2; exit 1 ;;
esac

# --- detect arch --------------------------------------------------------------
arch=$(uname -m)
case "$arch" in
    x86_64)           arch="amd64" ;;
    aarch64|arm64)    arch="arm64" ;;
    *) echo "error: unsupported architecture '$arch'" >&2; exit 1 ;;
esac

# --- fetch latest tag ---------------------------------------------------------
tag=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep '"tag_name"' | head -1 | cut -d'"' -f4)
if [ -z "$tag" ]; then
    echo "error: could not determine latest release tag" >&2
    exit 1
fi

echo "Installing taskwarrior-web ${tag} (${os}/${arch})..."

# --- download + extract -------------------------------------------------------
archive="taskwarrior-web-${tag}-${os}-${arch}.tar.gz"
url="https://github.com/${REPO}/releases/download/${tag}/${archive}"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

curl -fsSL "$url" | tar -xz -C "$tmp"

# --- run the bundled install script -------------------------------------------
cd "${tmp}/taskwarrior-web-${tag}-${os}-${arch}"
# Release tarballs place the binary at the root; install.sh expects bin/.
[ -f "taskwarrior-web" ] && { mkdir -p bin; mv taskwarrior-web bin/; }
bash scripts/install.sh
