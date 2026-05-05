#!/usr/bin/env sh
# fetch-mtxrpicam.sh — download and verify the mtxrpicam helper from a
# pinned mediamtx release.
#
# Usage:
#   scripts/fetch-mtxrpicam.sh <32|64> <version> <dest_dir>
#
# Example:
#   scripts/fetch-mtxrpicam.sh 64 v1.13.1 internal/rpicamera/mtxrpicam_64
#
# The script is idempotent: when a binary already lives at <dest_dir>/mtxrpicam
# and matches the SHA-256 in scripts/mtxrpicam.sha256, it exits without
# touching the file. This lets `make rpicam-fetch` run cheaply across CI
# invocations.
#
# Distribution: the upstream artifact ships a tarball
# `mediamtx_<version>_linux_<armv7|arm64>.tar.gz` whose embedded
# `mtxrpicam_<32|64>` directory contains the binary plus libcamera shims.

set -eu

if [ "$#" -ne 3 ]; then
    echo "usage: $0 <32|64> <version> <dest_dir>" >&2
    exit 64
fi

WORD_SIZE="$1"
VERSION="$2"
DEST_DIR="$3"

case "$WORD_SIZE" in
    32) ARCH_SUFFIX="armv7" ;;
    64) ARCH_SUFFIX="arm64" ;;
    *) echo "word size must be 32 or 64, got: $WORD_SIZE" >&2; exit 64 ;;
esac

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SHA_FILE="$REPO_ROOT/scripts/mtxrpicam.sha256"
DEST_BIN="$DEST_DIR/mtxrpicam"
SRC_DIR="mtxrpicam_${WORD_SIZE}"

if [ ! -f "$SHA_FILE" ]; then
    echo "missing $SHA_FILE — pin a checksum before fetching" >&2
    exit 1
fi

# Look up the expected hash for this word size. Format: "<sha>  mtxrpicam_<NN>".
EXPECTED_SHA="$(awk -v key="$SRC_DIR" '$2 == key { print $1 }' "$SHA_FILE")"
if [ -z "$EXPECTED_SHA" ]; then
    echo "no SHA-256 entry for $SRC_DIR in $SHA_FILE" >&2
    exit 1
fi

# Fast path: existing blob matches the pin.
if [ -f "$DEST_BIN" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
        ACTUAL="$(sha256sum "$DEST_BIN" | awk '{print $1}')"
    else
        ACTUAL="$(shasum -a 256 "$DEST_BIN" | awk '{print $1}')"
    fi
    if [ "$ACTUAL" = "$EXPECTED_SHA" ]; then
        echo "fetch-mtxrpicam: $DEST_BIN already matches pinned checksum"
        exit 0
    fi
    echo "fetch-mtxrpicam: $DEST_BIN checksum drift, re-downloading" >&2
fi

mkdir -p "$DEST_DIR"

# Drop placeholder files that exist solely to satisfy //go:embed during PR CI.
find "$DEST_DIR" -mindepth 1 -name 'placeholder' -delete

TAR_NAME="mediamtx_${VERSION}_linux_${ARCH_SUFFIX}.tar.gz"
URL="https://github.com/bluenviron/mediamtx/releases/download/${VERSION}/${TAR_NAME}"

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

tar -xzf "$TMP_DIR/$TAR_NAME" -C "$TMP_DIR"

# The released tarball contains a top-level mediamtx binary plus the
# mtxrpicam_<NN>/ directory of shared libraries and the executable. We only
# need the contents of mtxrpicam_<NN>.
EXTRACTED_DIR="$TMP_DIR/$SRC_DIR"
if [ ! -d "$EXTRACTED_DIR" ]; then
    # Older releases nested everything under a versioned dir; locate it.
    CANDIDATE="$(find "$TMP_DIR" -type d -name "$SRC_DIR" -print -quit 2>/dev/null || true)"
    if [ -n "$CANDIDATE" ] && [ -d "$CANDIDATE" ]; then
        EXTRACTED_DIR="$CANDIDATE"
    else
        echo "tarball did not contain $SRC_DIR" >&2
        exit 1
    fi
fi

# Replace dest contents with the extracted directory.
rm -f "$DEST_DIR"/* 2>/dev/null || true
cp -R "$EXTRACTED_DIR/." "$DEST_DIR/"
chmod 0700 "$DEST_BIN"

# Verify checksum on the binary itself.
if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL="$(sha256sum "$DEST_BIN" | awk '{print $1}')"
else
    ACTUAL="$(shasum -a 256 "$DEST_BIN" | awk '{print $1}')"
fi

if [ "$ACTUAL" != "$EXPECTED_SHA" ]; then
    echo "checksum mismatch for $DEST_BIN" >&2
    echo "  expected: $EXPECTED_SHA" >&2
    echo "  got:      $ACTUAL" >&2
    exit 1
fi

echo "fetch-mtxrpicam: $DEST_BIN ready ($ACTUAL)"
