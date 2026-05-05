#!/usr/bin/env bash
# Installs the latest onvif-simulator CLI binary from GitHub Releases.
# Usage: curl -fsSL …/install.sh | bash
set -euo pipefail

REPO_OWNER="GyeongHoKim"
REPO_NAME="onvif-simulator"
API_LATEST="https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest"

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "install.sh: required command not found: $1" >&2
    exit 1
  }
}

need_cmd curl
need_cmd uname

OS=$(uname -s)
ARCH=$(uname -m)

case "$OS" in
Linux) OS=linux ;;
Darwin) OS=darwin ;;
*)
  echo "install.sh: unsupported OS: $OS (only Linux and macOS)" >&2
  exit 1
  ;;
esac

# Channel selection: default channel (linux/darwin × amd64/arm64) vs rpi channel
# (linux/arm + linux/arm64, embeds mtxrpicam for Pi camera support).
#
# ONVIF_SIMULATOR_CHANNEL overrides auto-detection: "rpi" forces the Pi build,
# "default" forces the generic build, anything else (or unset) auto-detects.
CHANNEL="${ONVIF_SIMULATOR_CHANNEL:-auto}"

is_raspberry_pi() {
  [ "$OS" = linux ] || return 1
  [ -r /proc/device-tree/model ] || return 1
  case "$(tr -d '\0' </proc/device-tree/model)" in
  "Raspberry Pi"*) return 0 ;;
  *) return 1 ;;
  esac
}

if [ "$CHANNEL" = auto ]; then
  if is_raspberry_pi; then
    CHANNEL=rpi
  else
    CHANNEL=default
  fi
fi

case "$CHANNEL" in
default)
  case "$ARCH" in
  x86_64 | amd64) ARCH=amd64 ;;
  aarch64 | arm64) ARCH=arm64 ;;
  *)
    echo "install.sh: unsupported architecture for default channel: $ARCH" >&2
    exit 1
    ;;
  esac
  ARCHIVE_PREFIX="onvif-simulator"
  BIN_NAME="onvif-simulator"
  ;;
rpi)
  if [ "$OS" != linux ]; then
    echo "install.sh: rpi channel requires Linux, got $OS" >&2
    exit 1
  fi
  case "$ARCH" in
  aarch64 | arm64) ARCH=arm64 ;;
  armv7l | armv6l | arm) ARCH=arm ;;
  *)
    echo "install.sh: unsupported architecture for rpi channel: $ARCH" >&2
    exit 1
    ;;
  esac
  ARCHIVE_PREFIX="onvif-simulator-rpi"
  BIN_NAME="onvif-simulator-rpi"
  ;;
*)
  echo "install.sh: unknown channel: $CHANNEL (expected 'rpi', 'default', or 'auto')" >&2
  exit 1
  ;;
esac

if command -v python3 >/dev/null 2>&1; then
  TAG=$(curl -fsSL "$API_LATEST" | python3 -c 'import sys, json; print(json.load(sys.stdin)["tag_name"])')
else
  need_cmd grep
  TAG=$(curl -fsSL "$API_LATEST" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
fi

VERSION="${TAG#v}"
ARCHIVE="${ARCHIVE_PREFIX}_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${TAG}/${ARCHIVE}"

echo "Channel: ${CHANNEL} (${OS}/${ARCH})"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Downloading ${ARCHIVE} ..."
if ! curl -fsSL "$URL" -o "$TMP/archive.tar.gz"; then
  echo "install.sh: failed to download ${URL}" >&2
  echo "install.sh: check that this release includes a build for ${OS}/${ARCH}." >&2
  exit 1
fi

tar -xzf "$TMP/archive.tar.gz" -C "$TMP"

BIN="$TMP/${BIN_NAME}"
if [ ! -f "$BIN" ]; then
  echo "install.sh: could not find ${BIN_NAME} binary inside archive" >&2
  exit 1
fi

chmod +x "$BIN"

# Both channels install as "onvif-simulator" so usage docs (`onvif-simulator …`)
# work identically. A host won't carry both channels at once.
if [ -w "/usr/local/bin" ] 2>/dev/null; then
  DEST="/usr/local/bin/onvif-simulator"
  mv "$BIN" "$DEST"
  echo "Installed: ${DEST}"
else
  DEST_DIR="${HOME}/.local/bin"
  mkdir -p "$DEST_DIR"
  mv "$BIN" "${DEST_DIR}/onvif-simulator"
  echo "Installed: ${DEST_DIR}/onvif-simulator"
  echo "Ensure ${DEST_DIR} is on your PATH (e.g. export PATH=\"\${HOME}/.local/bin:\${PATH}\")."
fi
