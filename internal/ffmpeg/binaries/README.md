# Embedded ffmpeg binaries

Each subdirectory holds the per-platform statically-linked ffmpeg binary
that `internal/ffmpeg` embeds via `//go:embed` (one directive per
platform in `binary_<goos>_<goarch>.go`). Only the matching tuple is
compiled into a given `onvif-simulator` build, so a linux/amd64 release
artifact carries the ~80 MB linux/amd64 ffmpeg and nothing from the
other platforms.

## Fetch-only pattern (matches `internal/rpicamera/mtxrpicam_*`)

Real binaries are **not** committed. Each subdirectory ships a 1-byte
`placeholder` file (committed) so `//go:embed binaries/<goos>_<goarch>`
finds something at compile time. Run `just ffmpeg-fetch` (or the
per-target `scripts/fetch-ffmpeg.sh <goos> <goarch>`) to populate the
real `ffmpeg` / `ffmpeg.exe` alongside the placeholder; SHA-256 pins in
`scripts/ffmpeg.sha256` gate every download.

`internal/ffmpeg/binary.go` reads the specific binary filename at
runtime; when only the placeholder is present, `ffmpeg.Open` returns
`ErrUnsupported` so the simulator can fail with a clear hint instead of
trying to exec a few-byte file.

## Build flow

```bash
just ffmpeg-fetch       # one-time per (goos, goarch), or whenever
                        # scripts/ffmpeg.sha256 changes
just cli                # go build embeds the real binary for the host
                        # platform
```

CI runs `ffmpeg-fetch` before every release build (see `.goreleaser.yml`).

## Provenance

| GOOS/GOARCH    | Upstream                                  | Version    | License  |
| -------------- | ----------------------------------------- | ---------- | -------- |
| linux/amd64    | johnvansickle.com (release amd64)         | 7.0.2      | GPLv3    |
| linux/arm64    | johnvansickle.com (release arm64)         | 7.0.2      | GPLv3    |
| linux/arm      | johnvansickle.com (release armhf)         | 7.0.2      | GPLv3    |
| darwin/amd64   | evermeet.cx (ffmpeg-7.1.zip)              | 7.1        | GPLv3    |
| darwin/arm64   | osxexperts.net (ffmpeg71arm.zip)          | 7.1        | GPLv3    |
| windows/amd64  | BtbN/FFmpeg-Builds (win64-lgpl static)    | master     | LGPLv2.1 |
| windows/arm64  | BtbN/FFmpeg-Builds (winarm64-lgpl static) | master     | LGPLv2.1 |

See `scripts/ffmpeg-build.md` for the license rationale (fork+exec
boundary keeps GPL blobs from contaminating this MIT codebase).
