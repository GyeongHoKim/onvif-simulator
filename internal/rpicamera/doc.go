// Package rpicamera embeds and orchestrates the mtxrpicam helper from the
// mediamtx project (https://github.com/bluenviron/mediamtx) so the simulator
// can serve live H.264 from a Raspberry Pi camera through its embedded RTSP
// server.
//
// The runtime is gated by the rpicam build tag and additionally requires
// linux/arm or linux/arm64. Every other platform/tag combination compiles
// against camera_disabled.go, whose Open returns ErrUnsupported.
//
// On the rpicam-tagged build the package extracts the embedded mtxrpicam
// binary into a per-PID directory under /dev/shm, fork+execs it with two
// OS pipes (one for control messages, one for video access units), and
// surfaces each H.264 access unit through the OnData callback supplied to
// Open. ReloadParams hot-applies new capture parameters; Close stops the
// helper and waits for it to exit.
//
// This package vendors and adapts code from
// github.com/bluenviron/mediamtx/internal/staticsources/rpicamera (MIT
// license). The embedded mtxrpicam binary dynamically links libcamera
// (LGPL-2.1-or-later); see the top-level NOTICE for full attribution.
package rpicamera
