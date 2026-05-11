# ffmpeg binaries embedded by `internal/ffmpeg`

The simulator runs ffmpeg as a fork+exec subprocess to convert an
H.264/H.265 video track into a continuous MJPEG byte stream. Each
supported (goos, goarch) tuple ships a single self-contained ffmpeg
binary that go:embed bakes into the matching `onvif-simulator` build.

## Where the upstream binaries come from

`scripts/fetch-ffmpeg.sh` resolves the per-platform URL:

| GOOS/GOARCH      | Source                                       | Build flavor             | Size       |
| ---------------- | -------------------------------------------- | ------------------------ | ---------- |
| linux/amd64      | johnvansickle.com (release amd64)            | GPL static (7.0.2)       | ~80 MB     |
| linux/arm64      | johnvansickle.com (release arm64)            | GPL static (7.0.2)       | ~50 MB     |
| linux/arm        | johnvansickle.com (release armhf)            | GPL static (7.0.2)       | ~32 MB     |
| darwin/amd64     | evermeet.cx (ffmpeg-7.1.zip)                 | GPL static (7.1)         | ~80 MB     |
| darwin/arm64     | osxexperts.net (ffmpeg71arm.zip)             | GPL static (7.1)         | ~50 MB     |
| windows/amd64    | BtbN/FFmpeg-Builds (win64-lgpl static)       | LGPL static (latest)     | ~170 MB    |
| windows/arm64    | BtbN/FFmpeg-Builds (winarm64-lgpl static)    | LGPL static (latest)     | ~120 MB    |

SHA-256 pins for each archive live in `scripts/ffmpeg.sha256`; the fetch
script refuses to extract a blob whose hash does not match.

## License posture

- Source repo (this codebase): MIT (see `LICENSE`).
- johnvansickle / evermeet / osxexperts: GPLv3 builds — they enable
  `--enable-gpl` and link libx264 / libx265 / libxvid. The simulator
  does not use those encoders (only the LGPL mjpeg encoder and the
  LGPL h264/hevc decoders), but they are present in the binary.
- BtbN "lgpl" build: LGPL-only (no `--enable-gpl`).
- Distribution mode: **fork+exec**. The simulator launches the embedded
  ffmpeg as a separate OS process and communicates over pipes. FFmpeg's
  own [`legal.html`](https://www.ffmpeg.org/legal.html) treats an
  independent process as a non-derivative-work boundary, which the
  ifrOSS commentary on GPL fork-exec confirms is the consensus reading.
  Result: even the GPL builds do not contaminate this MIT-licensed Go
  binary because the ffmpeg blob ships as a separate executable
  artifact, not statically linked into Go code.

The simulator should reference the embedded ffmpeg's license in its
NOTICE file alongside the existing mtxrpicam attribution; ship the
ffmpeg COPYING.GPLv3 (or COPYING.LGPLv2.1 for the Windows build)
alongside the release archive when distributing precompiled binaries.

## Why we ship full builds, not a minimal one

A custom `--disable-everything` minimal build can shrink ffmpeg into the
8-15 MB range, but that requires per-platform cross-compilation
infrastructure (autoconf, nasm/yasm, target sysroot) — a real CI
investment. The fetched binaries are a few hundred MB across platforms
but each individual build channel only embeds its own target's blob
because `internal/ffmpeg/binary_<goos>_<goarch>.go` uses build-tag-gated
`//go:embed`. A linux/amd64 release archive carries just the ~80 MB
linux/amd64 ffmpeg, not the whole matrix.

If binary size becomes a release-blocker, the minimal-build flag set is:

```sh
./configure \
  --disable-everything \
  --disable-doc \
  --disable-debug \
  --disable-network \
  --enable-decoder=h264,hevc \
  --enable-encoder=mjpeg \
  --enable-muxer=mpjpeg,image2pipe \
  --enable-demuxer=mov,matroska,h264,hevc \
  --enable-protocol=file,pipe \
  --enable-bsf=h264_mp4toannexb,hevc_mp4toannexb \
  --enable-parser=h264,hevc,mjpeg \
  --enable-filter=fps,format,scale \
  --enable-small
strip ffmpeg
```

Key points:

- `--disable-everything` zeroes the default codec/muxer/protocol set so we
  pick exactly what the simulator's MJPEG path uses.
- No `--enable-gpl` — keeps the binary LGPL-only and free of x264/x265
  encoders we do not need.
- `--enable-bsf=h264_mp4toannexb,hevc_mp4toannexb` is required so the
  MP4 input demuxer's AVCC NAL framing converts to Annex-B before the
  decoder consumes it.
- `mpjpeg` muxer is preferred over `image2pipe` because it includes
  Content-Length per frame, which makes `internal/ffmpeg/mpjpeg.go`'s
  parser robust against JPEG-internal `0xFFD8`/`0xFFD9` byte
  collisions.

## Verifying placeholders vs real binaries

`internal/ffmpeg/binary.go:placeholderThreshold` (1 MiB) splits the two:
anything smaller is treated as a placeholder and `ffmpeg.Available()`
returns `ErrUnsupported`. A real build always exceeds this threshold,
so the runtime gate distinguishes safely without parsing the
ELF/Mach-O/PE header.
