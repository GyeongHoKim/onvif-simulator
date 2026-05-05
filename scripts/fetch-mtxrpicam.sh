#!/usr/bin/env sh
# fetch-mtxrpicam.sh — download and verify the mtxrpicam helper from a
# pinned bluenviron/mediamtx-rpicamera release.
#
# Usage:
#   scripts/fetch-mtxrpicam.sh <32|64> <version> <dest_dir>
#
# Example:
#   scripts/fetch-mtxrpicam.sh 64 v2.4.3 internal/rpicamera/mtxrpicam_64
#
# The script verifies the downloaded tarball against the SHA-256 pinned in
# scripts/mtxrpicam.sha256 — same unit (the .tar.gz blob, not the extracted
# binary) that mediamtx itself uses in
# internal/staticsources/rpicamera/mtxrpicamdownloader/HASH_MTXRPICAM_*_TAR_GZ
# and that mediamtx-rpicamera publishes as checksums.sha256 alongside the
# release. The script is idempotent: when a binary already lives at
# <dest_dir>/mtxrpicam, the script trusts the prior install and exits
# without redownloading.

set -eu

if [ "$#" -ne 3 ]; then
    echo "usage: $0 <32|64> <version> <dest_dir>" >&2
    exit 64
fi

WORD_SIZE="$1"
VERSION="$2"
DEST_DIR="$3"

case "$WORD_SIZE" in
    32|64) ;;
    *) echo "word size must be 32 or 64, got: $WORD_SIZE" >&2; exit 64 ;;
esac

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SHA_FILE="$REPO_ROOT/scripts/mtxrpicam.sha256"
DEST_BIN="$DEST_DIR/mtxrpicam"
TAR_NAME="mtxrpicam_${WORD_SIZE}.tar.gz"
SRC_DIR="mtxrpicam_${WORD_SIZE}"

if [ ! -f "$SHA_FILE" ]; then
    echo "missing $SHA_FILE — pin a checksum before fetching" >&2
    exit 1
fi

# Look up the expected tarball hash. Format: "<sha>  mtxrpicam_<NN>.tar.gz".
EXPECTED_SHA="$(awk -v key="$TAR_NAME" '$2 == key { print $1 }' "$SHA_FILE")"
if [ -z "$EXPECTED_SHA" ]; then
    echo "no SHA-256 entry for $TAR_NAME in $SHA_FILE" >&2
    exit 1
fi

# Fast path: an extracted binary already exists. We trust prior installs
# because we cannot recompute the original tarball's hash from the unpacked
# tree. CI keys its own actions/cache off scripts/mtxrpicam.sha256, so a
# version bump invalidates that cache automatically.
if [ -f "$DEST_BIN" ]; then
    echo "fetch-mtxrpicam: $DEST_BIN already present, trusting prior install"
    exit 0
fi

mkdir -p "$DEST_DIR"

# Drop placeholder files that exist solely to satisfy //go:embed during PR CI.
find "$DEST_DIR" -mindepth 1 -name 'placeholder' -delete

URL="https://github.com/bluenviron/mediamtx-rpicamera/releases/download/${VERSION}/${TAR_NAME}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

echo "fetch-mtxrpicam: downloading $URL"
if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$URL" -o "$TMP_DIR/$TAR_NAME"
elif command -v wget >/dev/null 2>&1; then
    wget -q "$URL" -O "$TMP_DIR/$TAR_NAME"
else
    echo "neither curl nor wget is available" >&2
    exit 1
fi

# Verify the tarball before extracting so a tampered blob never touches
# the working tree.
if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL="$(sha256sum "$TMP_DIR/$TAR_NAME" | awk '{print $1}')"
else
    ACTUAL="$(shasum -a 256 "$TMP_DIR/$TAR_NAME" | awk '{print $1}')"
fi

if [ "$ACTUAL" != "$EXPECTED_SHA" ]; then
    echo "checksum mismatch for $TAR_NAME" >&2
    echo "  expected: $EXPECTED_SHA" >&2
    echo "  got:      $ACTUAL" >&2
    exit 1
fi

# Extract directly into the dest's parent so the tarball's mtxrpicam_<NN>/
# top-level directory lands at $DEST_DIR.
DEST_PARENT="$(dirname "$DEST_DIR")"
DEST_BASENAME="$(basename "$DEST_DIR")"

if [ "$DEST_BASENAME" != "$SRC_DIR" ]; then
    echo "expected dest_dir basename $SRC_DIR, got $DEST_BASENAME" >&2
    exit 1
fi

# Replace any existing dest contents (including stale binaries) with the
# fresh extraction.
rm -rf "$DEST_DIR"
tar -xzf "$TMP_DIR/$TAR_NAME" -C "$DEST_PARENT"
chmod 0700 "$DEST_BIN"

echo "fetch-mtxrpicam: $DEST_BIN ready (tarball $ACTUAL)"
