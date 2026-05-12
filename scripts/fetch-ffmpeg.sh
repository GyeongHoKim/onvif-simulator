#!/usr/bin/env sh
# fetch-ffmpeg.sh — download and verify a statically-linked LGPL-only
# minimal ffmpeg binary for one target platform.
#
# Usage:
#   scripts/fetch-ffmpeg.sh <goos> <goarch>
#
# Supported (goos, goarch) tuples:
#   linux  amd64   — johnvansickle.com  (GPL static, amd64)
#   linux  arm64   — johnvansickle.com  (GPL static, arm64)
#   linux  arm     — johnvansickle.com  (GPL static, armhf — Pi 2/3 32-bit)
#   darwin amd64   — evermeet.cx        (GPL static, x86_64)
#   darwin arm64   — osxexperts.net     (GPL static, arm64)
#   windows amd64  — BtbN/FFmpeg-Builds (LGPL static, win64)
#   windows arm64  — BtbN/FFmpeg-Builds (LGPL static, winarm64)
# License posture is documented in scripts/ffmpeg-build.md — the fork+exec
# boundary keeps GPL blobs from contaminating the MIT-licensed Go binary.
#
# The destination is fixed at
# internal/ffmpeg/binaries/<goos>_<goarch>/ffmpeg{,.exe} so go:embed picks
# it up unchanged.
#
# The script verifies the downloaded artifact against the SHA-256 pinned in
# scripts/ffmpeg.sha256. CI re-runs the fetch on every build; the 1-byte
# placeholders committed to git keep `go build` working when the real
# binaries have not been fetched yet — internal/ffmpeg/binary.go detects
# the placeholder and returns ErrUnsupported.
#
# Idempotent: when a real (>= 1 MiB) binary already lives at the target,
# the script trusts the prior install and exits.

set -eu

if [ "$#" -ne 2 ]; then
    echo "usage: $0 <goos> <goarch>" >&2
    exit 64
fi

GOOS="$1"
GOARCH="$2"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SHA_FILE="$REPO_ROOT/scripts/ffmpeg.sha256"
DEST_DIR="$REPO_ROOT/internal/ffmpeg/binaries/${GOOS}_${GOARCH}"

if [ "$GOOS" = "windows" ]; then
    DEST_BIN_NAME="ffmpeg.exe"
else
    DEST_BIN_NAME="ffmpeg"
fi
DEST_BIN="$DEST_DIR/$DEST_BIN_NAME"

# Idempotent skip: a >= 1 MiB binary at the target is treated as a real
# install. CI's actions/cache keys off scripts/ffmpeg.sha256 so a version
# bump invalidates that cache automatically.
if [ -f "$DEST_BIN" ]; then
    SIZE="$(wc -c <"$DEST_BIN" | tr -d ' ')"
    if [ "$SIZE" -gt 1048576 ]; then
        echo "fetch-ffmpeg: $DEST_BIN already present (${SIZE} bytes), trusting prior install"
        exit 0
    fi
fi

if [ ! -f "$SHA_FILE" ]; then
    echo "missing $SHA_FILE — pin a checksum before fetching" >&2
    exit 1
fi

# URL + archive key per platform. Keys look like "<goos>_<goarch>" so the
# sha256 file can use a single namespace shared with this dispatch table.
case "${GOOS}_${GOARCH}" in
    linux_amd64)
        URL="https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz"
        ARCHIVE="ffmpeg-release-amd64-static.tar.xz"
        EXTRACT_PATTERN="ffmpeg-*-amd64-static/ffmpeg"
        ;;
    linux_arm64)
        URL="https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-arm64-static.tar.xz"
        ARCHIVE="ffmpeg-release-arm64-static.tar.xz"
        EXTRACT_PATTERN="ffmpeg-*-arm64-static/ffmpeg"
        ;;
    linux_arm)
        URL="https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-armhf-static.tar.xz"
        ARCHIVE="ffmpeg-release-armhf-static.tar.xz"
        EXTRACT_PATTERN="ffmpeg-*-armhf-static/ffmpeg"
        ;;
    darwin_amd64)
        URL="https://evermeet.cx/ffmpeg/ffmpeg-7.1.zip"
        ARCHIVE="ffmpeg-darwin-amd64.zip"
        EXTRACT_PATTERN="ffmpeg"
        ;;
    darwin_arm64)
        URL="https://www.osxexperts.net/ffmpeg71arm.zip"
        ARCHIVE="ffmpeg-darwin-arm64.zip"
        EXTRACT_PATTERN="ffmpeg"
        ;;
    windows_amd64)
        # Use the static (non-shared) LGPL build so the single ffmpeg.exe
        # is self-contained — go:embed cannot ship a directory of DLLs
        # without extra extraction logic.
        URL="https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-lgpl.zip"
        ARCHIVE="ffmpeg-windows-amd64.zip"
        EXTRACT_PATTERN="*/bin/ffmpeg.exe"
        ;;
    windows_arm64)
        URL="https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-winarm64-lgpl.zip"
        ARCHIVE="ffmpeg-windows-arm64.zip"
        EXTRACT_PATTERN="*/bin/ffmpeg.exe"
        ;;
    *)
        echo "unsupported (goos, goarch): ${GOOS}_${GOARCH}" >&2
        exit 64
        ;;
esac

EXPECTED_SHA="$(awk -v key="${GOOS}_${GOARCH}" '$1 !~ /^#/ && $2 == key { print $1 }' "$SHA_FILE")"
if [ -z "$EXPECTED_SHA" ]; then
    echo "no SHA-256 entry for ${GOOS}_${GOARCH} in $SHA_FILE" >&2
    echo "  pin one before re-running:" >&2
    echo "  curl -fsSL '$URL' | sha256sum" >&2
    exit 1
fi

mkdir -p "$DEST_DIR"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

echo "fetch-ffmpeg: downloading $URL"
if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$URL" -o "$TMP_DIR/$ARCHIVE"
elif command -v wget >/dev/null 2>&1; then
    wget -q "$URL" -O "$TMP_DIR/$ARCHIVE"
else
    echo "neither curl nor wget is available" >&2
    exit 1
fi

# Verify the archive before extracting so a tampered blob never touches
# the working tree.
if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL="$(sha256sum "$TMP_DIR/$ARCHIVE" | awk '{print $1}')"
else
    ACTUAL="$(shasum -a 256 "$TMP_DIR/$ARCHIVE" | awk '{print $1}')"
fi

if [ "$ACTUAL" != "$EXPECTED_SHA" ]; then
    echo "checksum mismatch for ${GOOS}_${GOARCH}" >&2
    echo "  expected: $EXPECTED_SHA" >&2
    echo "  got:      $ACTUAL" >&2
    exit 1
fi

# Extract the binary using whichever tool fits the archive type.
case "$ARCHIVE" in
    *.tar.xz)
        tar -C "$TMP_DIR" -xJf "$TMP_DIR/$ARCHIVE"
        ;;
    *.tar.gz|*.tgz)
        tar -C "$TMP_DIR" -xzf "$TMP_DIR/$ARCHIVE"
        ;;
    *.zip)
        unzip -q "$TMP_DIR/$ARCHIVE" -d "$TMP_DIR"
        ;;
    *)
        echo "unhandled archive format: $ARCHIVE" >&2
        exit 1
        ;;
esac

# Locate the extracted binary using the per-platform glob, then move it.
# shellcheck disable=SC2086 # word-splitting the pattern is intentional
SRC="$(find "$TMP_DIR" -path "*/${EXTRACT_PATTERN}" -type f | head -n1)"
if [ -z "$SRC" ] || [ ! -f "$SRC" ]; then
    # Some archives place ffmpeg at top-level without a subdir prefix.
    SRC="$(find "$TMP_DIR" -name "$DEST_BIN_NAME" -type f | head -n1)"
fi
if [ -z "$SRC" ] || [ ! -f "$SRC" ]; then
    echo "could not locate $DEST_BIN_NAME inside $ARCHIVE" >&2
    exit 1
fi

mv "$SRC" "$DEST_BIN"
chmod 0755 "$DEST_BIN"

echo "fetch-ffmpeg: $DEST_BIN ready (archive $ACTUAL)"
