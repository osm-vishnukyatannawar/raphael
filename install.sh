#!/usr/bin/env bash
#
# Raphael installer and updater (Linux).
#
#   wget -qO- https://raw.githubusercontent.com/osm-vishnukyatannawar/raphael/main/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/osm-vishnukyatannawar/raphael/main/install.sh | bash
#
# Installs into $HOME — no sudo, nothing written outside the user's own
# directories. Re-running it is the update path: it compares the installed
# version against the latest release and stops early when they match.
#
# Environment overrides:
#   RAPHAEL_VERSION=v1.1.0   install a specific tag instead of the latest
#   RAPHAEL_FORCE=1          reinstall even when the version already matches
#
set -euo pipefail

REPO="osm-vishnukyatannawar/raphael"
BIN_DIR="$HOME/.local/bin"
ICON_DIR="$HOME/.local/share/icons/hicolor/512x512/apps"
DESKTOP_DIR="$HOME/.local/share/applications"

info() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m warning:\033[0m %s\n' "$*" >&2; }
die() {
  printf '\033[1;31m error:\033[0m %s\n' "$*" >&2
  exit 1
}

# fetch writes a URL to stdout with whichever downloader is present. curl and
# wget are both common and neither is guaranteed, so support both rather than
# making one a hard dependency.
fetch() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- "$1"
  else
    die "neither curl nor wget is installed"
  fi
}

fetch_to_file() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$2" "$1"
  else
    wget -qO "$2" "$1"
  fi
}

[ "$(uname -s)" = "Linux" ] || die "this installer is Linux-only; Windows builds are on the releases page"
case "$(uname -m)" in
x86_64 | amd64) ;;
*) die "no prebuilt binary for $(uname -m); build from source with 'make build'" ;;
esac

# The webview is the one thing that cannot be shipped in the archive. Missing
# libraries surface as a silent failure to open a window, so say so up front —
# but only warn, because distro package names vary and being wrong here should
# not block an install that would have worked.
if command -v ldconfig >/dev/null 2>&1; then
  if ! ldconfig -p | grep -q 'libwebkit2gtk-4\.1'; then
    warn "webkit2gtk-4.1 was not found. Raphael needs it to open a window:"
    warn "  Arch:   sudo pacman -S webkit2gtk-4.1 gtk3"
    warn "  Debian: sudo apt install libwebkit2gtk-4.1-0 libgtk-3-0"
  fi
fi

if [ -n "${RAPHAEL_VERSION:-}" ]; then
  VERSION="$RAPHAEL_VERSION"
else
  info "Looking up the latest release"
  # tag_name from the releases API, without requiring jq to be installed.
  VERSION=$(fetch "https://api.github.com/repos/$REPO/releases/latest" |
    grep -m1 '"tag_name"' | cut -d'"' -f4)
fi
[ -n "$VERSION" ] || die "could not determine the latest version"

INSTALLED=""
if [ -x "$BIN_DIR/raphael" ]; then
  # --version prints and exits before any window is opened, so this is safe to
  # call over SSH or on a machine with no display. Releases before this flag
  # existed print nothing useful, which reads as "unknown" and reinstalls.
  INSTALLED=$("$BIN_DIR/raphael" --version 2>/dev/null | head -n1 || true)
fi

if [ -n "$INSTALLED" ] && [ "$INSTALLED" = "$VERSION" ] && [ -z "${RAPHAEL_FORCE:-}" ]; then
  info "Raphael $VERSION is already installed. Set RAPHAEL_FORCE=1 to reinstall."
  exit 0
fi

STAGE="raphael-$VERSION-linux-amd64"
URL="https://github.com/$REPO/releases/download/$VERSION/$STAGE.tar.gz"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

info "Downloading Raphael $VERSION"
fetch_to_file "$URL" "$TMP/$STAGE.tar.gz" || die "download failed: $URL"

# Verify against the published checksums when they are reachable. A failed
# checksum is fatal; a missing checksums file is not, since an older release may
# predate it.
if fetch_to_file "https://github.com/$REPO/releases/download/$VERSION/SHA256SUMS.txt" "$TMP/SHA256SUMS.txt" 2>/dev/null; then
  info "Verifying checksum"
  (cd "$TMP" && sha256sum -c SHA256SUMS.txt --ignore-missing >/dev/null) ||
    die "checksum mismatch — the download is not what was published"
else
  warn "no SHA256SUMS.txt for $VERSION; skipping verification"
fi

tar -xzf "$TMP/$STAGE.tar.gz" -C "$TMP"

info "Installing into $BIN_DIR"
install -Dm755 "$TMP/$STAGE/raphael" "$BIN_DIR/raphael"
install -Dm644 "$TMP/$STAGE/raphael.png" "$ICON_DIR/raphael.png"
# The desktop entry's filename must stay Raphael.desktop: it has to match the
# GTK program name, which becomes the Wayland app_id. Without the match KDE shows
# a generic window icon and cannot raise the window from a notification.
install -Dm644 "$TMP/$STAGE/Raphael.desktop" "$DESKTOP_DIR/Raphael.desktop"

update-desktop-database "$DESKTOP_DIR" 2>/dev/null || true
gtk-update-icon-cache -f -t "$HOME/.local/share/icons/hicolor" 2>/dev/null || true

if [ -n "$INSTALLED" ]; then
  info "Updated Raphael $INSTALLED → $VERSION"
else
  info "Installed Raphael $VERSION"
fi

case ":$PATH:" in
*":$BIN_DIR:"*) ;;
*) warn "$BIN_DIR is not on your PATH — add it to run 'raphael' from a shell" ;;
esac
