# Changelog

All notable changes to this project are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0] — Profile S compliance

### Highlights

- **ONVIF Profile S v1.3 conformance.** Every device-side mandatory feature
  in [Profile S Specification v1.3](https://www.onvif.org/profiles/profile-s/)
  §7.1–§7.13 is implemented. The README headline and CLI/TUI/GUI surfaces
  now claim Profile S compliance.

### Added

- **§7.7 Event Handling — WS-BaseNotification push subscriptions** (`GYE-77`).
  The event service now accepts `wsnt:Subscribe` requests, validates
  `ConsumerReference`, fans `Notify` POSTs out to the registered consumer
  asynchronously, and force-expires consumers that exceed the configured
  consecutive-failure threshold. Pull-point and push subscriptions share
  one keyed registry but count against separate capacity caps.
- **§7.9 MJPEG over RTSP** (`GYE-76`). The embedded RTSP server gains an
  `MJPEGSource`; `kind=file` profiles transparently transcode through a
  bundled ffmpeg helper, and the Raspberry Pi build channel exposes the Pi
  ISP secondary stream as MJPEG. `GetVideoEncoderConfigurationOptions`
  advertises the `JPEG` option block and `GetGuaranteedNumberOfVideoEncoder
  Instances` reports a non-zero `JPEG` instance count.
- **Profile S conformance validation harness.** `just e2e` exercises every
  Profile S mandatory operation, including the new event push and metadata
  configuration paths, against a running simulator. The DTT (ONVIF Device
  Test Tool) workflow is documented under "Profile S conformance
  validation" in the README.
- **CHANGELOG.md.** First release of this file; v0.4.0 is the baseline
  entry.

### Changed

- Task runner migrated from `make` to [`just`](https://github.com/casey/just)
  (`GYE-79`). The legacy `Makefile` remains as a deprecation wrapper that
  forwards every target to the matching `just` recipe.
- CLI `--help` and `version` now show the Profile S compliance claim. The
  TUI status bar and GUI footer both surface "ONVIF Profile S v1.3
  compliant".

### Fixed

- RTSP live source no longer flips `ready` before the first IDR, so DESCRIBE
  responses never expose a pre-keyframe SDP.
- SPS/PPS are repeated in-band on every outgoing IDR for clients that miss
  the SDP-borne copy.
- `rpicamera` Contrast/Saturation/Sharpness defaults are aligned with the
  upstream `mtxrpicam` semantics (1.0 = neutral).

## [0.3.1]

See git history for pre-v0.4.0 changes — entries before this changelog was
introduced are tracked in commit messages and PR descriptions.

[Unreleased]: https://github.com/GyeongHoKim/onvif-simulator/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/GyeongHoKim/onvif-simulator/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/GyeongHoKim/onvif-simulator/releases/tag/v0.3.1
