# onvif-simulator

[![CI](https://github.com/GyeongHoKim/onvif-simulator/actions/workflows/ci.yml/badge.svg)](https://github.com/GyeongHoKim/onvif-simulator/actions/workflows/ci.yml)
[![Release](https://github.com/GyeongHoKim/onvif-simulator/actions/workflows/release.yml/badge.svg)](https://github.com/GyeongHoKim/onvif-simulator/actions/workflows/release.yml)
[![codecov](https://codecov.io/gh/GyeongHoKim/onvif-simulator/graph/badge.svg)](https://codecov.io/gh/GyeongHoKim/onvif-simulator)
[![CodeRabbit Pull Request Reviews](https://img.shields.io/coderabbit/prs/github/GyeongHoKim/onvif-simulator?utm_source=oss&utm_medium=github&utm_campaign=GyeongHoKim%2Fonvif-simulator&labelColor=171717&color=FF570A&link=https%3A%2F%2Fcoderabbit.ai&label=CodeRabbit+Reviews)](https://coderabbit.ai)
[![ONVIF Profile S compliant](https://img.shields.io/badge/ONVIF-Profile%20S%20compliant-0aa)](https://www.onvif.org/profiles/profile-s/)

A cross-platform ONVIF device simulator written in Go. Supports CLI, TUI, and GUI modes, making it easy to test ONVIF clients without real hardware.

**v0.4.0** is the first release to implement every **ONVIF Profile S v1.3** device-side mandatory feature — discovery, capabilities, system + network configuration, user handling, media profile/encoder/source/metadata configuration, H.264/H.265/MJPEG over RTSP, and WS-BaseNotification pull-point + push event handling.

## Features

- ONVIF Profile S (camera streaming simulation)
- Multiple interface modes: CLI, TUI, GUI
- Embedded RTSP server — point each profile at a local mp4 and the simulator loops it as the live stream
- Configure multiple streams (main, sub)

## Supported profiles

- **Profile S** (v1.3) — every device-side mandatory feature implemented as of v0.4.0; see [`doc/`](doc/) for the bundled Profile S Specification PDF, and [Profile S conformance validation](#profile-s-conformance-validation) below for the validation harness.

## Installation

### CLI / TUI

**Linux / macOS**

```bash
curl -fsSL https://github.com/GyeongHoKim/onvif-simulator/releases/latest/download/install.sh | bash
```

**Windows (PowerShell)**

```powershell
iex (irm https://github.com/GyeongHoKim/onvif-simulator/releases/latest/download/install.ps1)
```

After installation, the `onvif-simulator` command will be available in your terminal.

### GUI

Download the installer for your platform from the [Releases](https://github.com/GyeongHoKim/onvif-simulator/releases) page:

| Platform | File |
|----------|------|
| Windows  | `onvif-simulator-gui-windows-amd64.exe` |
| macOS    | `onvif-simulator-gui-darwin-amd64.dmg` |
| Linux    | `onvif-simulator-gui-linux-amd64.AppImage` |

Run the installer and follow the on-screen instructions.

### Raspberry Pi (live camera)

The Raspberry Pi build channel embeds the `mtxrpicam` capture helper from
[bluenviron/mediamtx](https://github.com/bluenviron/mediamtx) so a Pi Camera
Module can drive an ONVIF device on Raspberry Pi OS without any extra setup.
The build is CLI/TUI only — there is no Pi GUI binary.

| Pi model            | OS arch    | Release archive                                          |
|---------------------|------------|----------------------------------------------------------|
| Pi Zero / 2 / 3     | 32-bit     | `onvif-simulator-rpi_<version>_linux_arm.tar.gz`         |
| Pi 3 / 4 / 5 (64)   | 64-bit     | `onvif-simulator-rpi_<version>_linux_arm64.tar.gz`       |

Each archive contains a single `onvif-simulator-rpi` binary.

The `install.sh` one-liner above auto-detects Raspberry Pi via
`/proc/device-tree/model` and pulls the matching rpi archive — running it on a
Pi installs the rpi build as `onvif-simulator` with no extra flags. Set
`ONVIF_SIMULATOR_CHANNEL=default` (or `=rpi`) to override detection.

To install manually, download the matching archive from the
[Releases](https://github.com/GyeongHoKim/onvif-simulator/releases) page and
configure a profile with `"kind": "rpicam"`:

```jsonc
{
  "name": "main",
  "token": "profile_main",
  "kind": "rpicam",
  "rpicam": {
    "camera_id": 0,
    "width": 1920,
    "height": 1080,
    "fps": 30,
    "bitrate": 4000000,
    "idr_period": 60
  }
}
```

See `onvif-simulator.example.rpi.json` for a worked example. The default build
channel (Linux/macOS/Windows × amd64/arm64) does not carry `mtxrpicam` and
rejects `kind=rpicam` profiles at startup with a clear error message.

Third-party software notice for the Raspberry Pi channel: see
[`NOTICE`](NOTICE).

#### Run as a systemd service

After `install.sh` has placed the binary at `/usr/local/bin/onvif-simulator`
and you have copied the rpi example config into place (see
[Configuration](#onvif-simulatorjson)), register the simulator as a system
service so it starts at boot and restarts on failure.

Create `/etc/systemd/system/onvif-simulator.service`:

```ini
[Unit]
Description=ONVIF Simulator
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=pi
ExecStart=/usr/local/bin/onvif-simulator serve
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
```

Then enable and start it:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now onvif-simulator.service
systemctl status onvif-simulator.service
journalctl -u onvif-simulator.service -f   # tail logs
```

Notes:

- Replace `User=pi` with the account that owns the config file. The user
  must be in the `video` group for `kind=rpicam` profiles to access the
  camera (the default `pi` account on Raspberry Pi OS already is).
- The simulator reads `onvif-simulator.json` from that user's XDG config
  directory (`~/.config/onvif-simulator/onvif-simulator.json` on Linux).
  To pin a different path, change `ExecStart` to
  `/usr/local/bin/onvif-simulator serve -config /etc/onvif-simulator.json`.
- To apply config changes, edit the JSON and run
  `sudo systemctl restart onvif-simulator.service`.

## Usage

### CLI Mode

Run a single virtual device directly from the command line.

```bash
# Start a virtual device with default settings(cannot customize in CLI mode)
onvif-simulator serve
# List available options
onvif-simulator serve --help
```

### TUI Mode

Interactive terminal UI for managing:

- Device Service
  - change device information
- Media Service
  - change stream uri
- Event Service
  - trigger motion detection

```bash
onvif-simulator
```

### GUI Mode

Download and run the installer for your platform from the [Releases](https://github.com/GyeongHoKim/onvif-simulator/releases) page. The GUI provides a native window with a web-based interface for full graphical management of virtual devices.

The features are the same as the TUI mode.

## Configuration

### `onvif-simulator.json`

onvif-simulator embeds its own RTSP server. For each profile, point `media_file_path` at a local H.264/H.265 mp4 file; the simulator loops it and serves an RTSP stream at `rtsp://<host>:<rtsp_port>/<profile token>`, which is what `GetStreamUri` returns.

#### File location

The config file is named `onvif-simulator.json` and is auto-created on first run at the OS-standard user config directory:

| OS      | Path                                                                                                          |
|---------|---------------------------------------------------------------------------------------------------------------|
| macOS   | `~/Library/Application Support/onvif-simulator/onvif-simulator.json`                                          |
| Linux   | `$XDG_CONFIG_HOME/onvif-simulator/onvif-simulator.json` &nbsp;(falls back to `~/.config/onvif-simulator/...`) |
| Windows | `%AppData%\onvif-simulator\onvif-simulator.json` &nbsp;(typically `C:\Users\<you>\AppData\Roaming\...`)       |

To override the path for a single run, pass `-config /path/to/onvif-simulator.json` to the CLI. As a fallback for ad-hoc use and tests, `Load` also accepts `./onvif-simulator.json` in the working directory when no path has been set.

Two example files ship in every release:

- `onvif-simulator.example.json` — default channel (file-backed profiles).
- `onvif-simulator.example.rpi.json` — Raspberry Pi channel (rpicam profile). Use this on a Pi.

To start from the bundled example, create the user config directory if it does not exist, then copy the example file to the path for your OS (see the table above) or into the working directory. **Both examples must be renamed to `onvif-simulator.json` at the destination** — that is the only filename the simulator reads.

```bash
# Linux: respects XDG_CONFIG_HOME when set, otherwise ~/.config
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/onvif-simulator"
mkdir -p "$CONFIG_DIR"
cp onvif-simulator.example.json "$CONFIG_DIR/onvif-simulator.json"

# macOS
mkdir -p "$HOME/Library/Application Support/onvif-simulator"
cp onvif-simulator.example.json "$HOME/Library/Application Support/onvif-simulator/onvif-simulator.json"

# Or keep it in the working directory for quick experiments (no extra directory needed)
cp onvif-simulator.example.json onvif-simulator.json
```

**Raspberry Pi** — recommended on the rpi build channel. Substitute the rpi example file:

```bash
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/onvif-simulator"
mkdir -p "$CONFIG_DIR"
cp onvif-simulator.example.rpi.json "$CONFIG_DIR/onvif-simulator.json"
```

**Windows (PowerShell)** — `%AppData%` expands to your roaming profile directory (see table above):

```powershell
New-Item -ItemType Directory -Force -Path "$env:APPDATA\onvif-simulator" | Out-Null
Copy-Item onvif-simulator.example.json "$env:APPDATA\onvif-simulator\onvif-simulator.json"
# Or working directory:
Copy-Item onvif-simulator.example.json onvif-simulator.json
```

**Minimal required fields:**

```json
{
  "version": 1,
  "device": {
    "uuid": "urn:uuid:00000000-0000-4000-8000-000000000001",
    "manufacturer": "ONVIF Simulator",
    "model": "SimCam-100",
    "serial": "SN-0001"
  },
  "network": {
    "http_port": 8080
  },
  "media": {
    "profiles": [
      {
        "name": "main",
        "token": "profile_main",
        "media_file_path": "/absolute/path/to/main.mp4"
      }
    ]
  }
}
```

(`network.rtsp_port` is optional; omit it to use the default `8554`.)

**Optional sections** (all fields shown in `onvif-simulator.example.json`):

| Section | Purpose |
|---------|---------|
| `auth` | Enable HTTP Digest / WS-UsernameToken / JWT authentication and manage users. |
| `runtime` | Persist Device Management runtime state: `discovery_mode`, `hostname`, `dns`, `default_gateway`, `network_protocols`, `system_date_and_time`. Written by ONVIF Set* operations; editing manually sets the initial value. |
| `events` | Configure the Event Service: `max_pull_points`, `subscription_timeout` (Go duration, e.g. `"1h"`), and the `topics` list (name + enabled flag). |

> **Notes:**
> - `network.rtsp_port` is optional and defaults to `8554` when omitted or set to `0`; it must differ from `http_port`.
> - `media_file_path` must be an absolute path to an mp4 with an H.264 or H.265 video track.
> - When `media_file_path` is set, `encoding`, `width`, `height`, and `fps` are read from the file at startup (via probe) and replace the in-memory profile values for the running simulator; `bitrate` and `gop_length` are not derived from the file and still come from the config for ONVIF. Persisted JSON values for the probe-derived fields are only used as fallback display data when the simulator is stopped.

If you don't have a sample clip handy, generate one with ffmpeg:

```bash
ffmpeg -y -f lavfi -i testsrc=duration=10:size=1280x720:rate=30 \
    -c:v libx264 -pix_fmt yuv420p sample.mp4
```

## Profile S conformance validation

v0.4.0 advertises ONVIF Profile S v1.3 device-side mandatory features. The compliance posture is verified at two levels:

### `just e2e` — SOAP-level regression suite

The `test/e2e` package drives a running simulator with [`use-go/onvif`](https://github.com/use-go/onvif) and exercises every Profile S mandatory operation (§7.1 auth — §7.13 metadata), the WS-BaseNotification pull-point + push event handlers, MJPEG advertising (`JPEG` instance count + resolutions), and the embedded RTSP playback path.

```bash
# Terminal 1 — start the simulator pointed at an H.264/H.265 mp4 (see Configuration)
onvif-simulator serve

# Terminal 2 — run the SOAP-level suite. ONVIF_HOST/USERNAME/PASSWORD override defaults.
just e2e
```

The suite passes against the bundled `onvif-simulator.example.json` once `media_file_path` is set to an absolute path. CI runs it for every PR.

### ONVIF Device Test Tool (DTT) — vendor-level conformance

The [ONVIF Device Test Tool](https://www.onvif.org/profiles/conformance/device-test/) is the canonical conformance suite. It is Windows-only and is **not run automatically** — execute it locally before tagging a Profile S–claiming release:

1. Start the simulator on the test machine (or a reachable host) with `onvif-simulator serve`. Make sure the configured `media_file_path` points at an H.264 (or H.265) mp4 — the JPEG output is synthesised on the fly from the same source.
2. In DTT, **Add Device** by IP, run **Discover** if WS-Discovery is reachable, and select the **Profile S** test pool.
3. Capture the DTT log into `doc/conformance/dtt-v0.4.0.log` (or attach to the release notes). Profile S–mandatory test cases must all return `Passed`; investigate any `Failed`/`Skipped` row before a release.

Test machine specifics, expected runtime, and known limitations live in [`doc/conformance/README.md`](doc/conformance/README.md) (created on first run).

## Development

### Prerequisites

Install [mise](https://mise.jdx.dev/) and let it provision the required toolchain:

```bash
mise install
```

This installs Go 1.26.2, golangci-lint 2.11.4, and Node.js 24.15.0 (needed for GUI via Wails).

On **Windows**, `just rpicam-fetch`, `just ffmpeg-fetch`, and the `cli-rpi-*` recipes call `bash` to run scripts under `scripts/`. Install [Git for Windows](https://git-scm.com/download/win) (Git Bash), [WSL](https://learn.microsoft.com/en-us/windows/wsl/install), [MSYS2](https://www.msys2.org/), or another distribution that puts `bash` on your `PATH`, then confirm with `bash --version`.

For GUI development, also install the [Wails CLI](https://wails.io/docs/gettingstarted/installation):

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### Setup

The repo uses [just](https://github.com/casey/just) as its task runner. Install it once via any of:

```bash
mise install                  # picks up the version pinned in mise.toml
brew install just             # macOS / Linuxbrew
scoop install just            # Windows (scoop)
cargo install just            # any platform with Rust
```

Then:

```bash
git clone https://github.com/GyeongHoKim/onvif-simulator.git
cd onvif-simulator
go mod tidy
cp onvif-simulator.example.json onvif-simulator.json  # fill in your RTSP URIs
just setup            # install git hooks and commitlint
```

The legacy `Makefile` is a thin deprecation wrapper that simply forwards every target to the matching `just` recipe; new work should call `just <recipe>` directly. Run `just --list` to see everything available.

### Common Tasks

| Command | Description |
|---------|-------------|
| `just setup` | Install git hooks and commitlint (run once after cloning) |
| `just cli` | Build the CLI/TUI binary |
| `just gui` | Build the GUI binary (requires Wails) |
| `just format` | Run `go fmt` across all packages |
| `just lint` | Run golangci-lint |
| `just clean` | Remove build artifacts |

### Run

```bash
# CLI / TUI
go run . serve
go run .

# GUI (requires Wails)
wails dev
```

## License

MIT
