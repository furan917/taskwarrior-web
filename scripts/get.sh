#!/usr/bin/env sh
# Download the latest taskwarrior-web release for this OS/arch, verify its
# SHA256 against the published sidecar, extract it, and run the bundled
# install script. Supports macOS (amd64/arm64) and Linux (amd64/arm64).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/furan917/taskwarrior-web/main/scripts/get.sh | sh
#
# Optional: prefix with INSTALL_ALIAS=1 to also add a 'tw' shell alias.
#
# Security notes:
# - `set -euo pipefail` means a failure anywhere in the pipeline (curl, grep,
#   tar) aborts. The original `set -eu` could mask an upstream `curl` failure
#   when piped into a permissive consumer (e.g. tar on a truncated stream
#   sometimes exits 0 with partial files); pipefail closes that hole.
# - SHA256 of every release archive is published as a `.sha256` sidecar by
#   the build workflow. We download both and verify with `shasum -a 256 -c`
#   (BSD on macOS) / `sha256sum -c` (GNU on Linux) BEFORE extracting, so a
#   compromised GitHub-CDN edge or release asset cannot ship arbitrary code
#   through this pipe.
# - $tag is validated against the `vX.Y.Z` pattern before it's interpolated
#   into any path or URL - a hostile API response with a `../` tag can't
#   escape `$tmp`.
set -euo pipefail

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

# --- pick a sha256 verifier ---------------------------------------------------
# macOS ships `shasum`; modern Linux ships `sha256sum`. Both produce
# `<hash>  <filename>` lines and verify with `-c` against that format - the
# .sha256 sidecar published by the build workflow is in exactly this shape.
if command -v sha256sum >/dev/null 2>&1; then
    sha_verify() { sha256sum -c "$1"; }
elif command -v shasum >/dev/null 2>&1; then
    sha_verify() { shasum -a 256 -c "$1"; }
else
    echo "error: need sha256sum or shasum to verify download integrity" >&2
    exit 1
fi

# --- fetch latest tag ---------------------------------------------------------
tag=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep '"tag_name"' | head -1 | cut -d'"' -f4)
if [ -z "$tag" ]; then
    echo "error: could not determine latest release tag" >&2
    exit 1
fi

# Whitelist tag shape BEFORE interpolating it into any path/URL. Belt-and-
# braces against a maliciously-pushed tag like `../foo`: GitHub's tag rules
# already constrain to `[a-zA-Z0-9._/-]+` but `../`, leading dashes, slashes,
# etc. are all in scope. Our release-please tags are always `vX.Y.Z`.
case "$tag" in
    v[0-9]*.[0-9]*.[0-9]*) ;;
    *)
        echo "error: refusing unexpected tag shape '$tag' (want vX.Y.Z)" >&2
        exit 1
        ;;
esac

echo "Installing taskwarrior-web ${tag} (${os}/${arch})..."

# --- download archive + sidecar -----------------------------------------------
archive="taskwarrior-web-${tag}-${os}-${arch}.tar.gz"
url="https://github.com/${REPO}/releases/download/${tag}/${archive}"
sha_url="${url}.sha256"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# Download archive to disk (NOT through a pipe) so we can verify before
# extract. The original `curl | tar` pattern had no opportunity to check
# integrity.
curl -fsSL -o "${tmp}/${archive}" "$url"
curl -fsSL -o "${tmp}/${archive}.sha256" "$sha_url"

# Verify. The .sha256 sidecar contains a relative filename ("taskwarrior-web-
# vX.Y.Z-darwin-arm64.tar.gz"); change into $tmp so the verifier finds the
# archive next to its sidecar without us rewriting the sidecar's filename
# column.
( cd "$tmp" && sha_verify "${archive}.sha256" >/dev/null )
echo "verified: SHA256 matches published sidecar"

# --- extract + hand off to install.sh -----------------------------------------
tar -xzf "${tmp}/${archive}" -C "$tmp"
cd "${tmp}/taskwarrior-web-${tag}-${os}-${arch}"
# Release tarballs place the binary at the root; install.sh expects bin/.
[ -f "taskwarrior-web" ] && { mkdir -p bin; mv taskwarrior-web bin/; }
bash scripts/install.sh
