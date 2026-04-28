# onvif-simulator

[![CI](https://github.com/GyeongHoKim/onvif-simulator/actions/workflows/ci.yml/badge.svg)](https://github.com/GyeongHoKim/onvif-simulator/actions/workflows/ci.yml)
[![Release](https://github.com/GyeongHoKim/onvif-simulator/actions/workflows/release.yml/badge.svg)](https://github.com/GyeongHoKim/onvif-simulator/actions/workflows/release.yml)
[![codecov](https://codecov.io/gh/GyeongHoKim/onvif-simulator/graph/badge.svg)](https://codecov.io/gh/GyeongHoKim/onvif-simulator)
[![CodeRabbit Pull Request Reviews](https://img.shields.io/coderabbit/prs/github/GyeongHoKim/onvif-simulator?utm_source=oss&utm_medium=github&utm_campaign=GyeongHoKim%2Fonvif-simulator&labelColor=171717&color=FF570A&link=https%3A%2F%2Fcoderabbit.ai&label=CodeRabbit+Reviews)](https://coderabbit.ai)

A cross-platform ONVIF device simulator written in Go. Supports CLI, TUI, and GUI modes, making it easy to test ONVIF clients without real hardware.

## Features

- ONVIF Profile S (camera streaming simulation)
- Multiple interface modes: CLI, TUI, GUI
- Embedded RTSP server — point each profile at a local mp4 and the simulator loops it as the live stream
- Configure multiple streams (main, sub)

## Supported profiles

- **Profile S**

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

## Usage

### CLI Mode

Run a single virtual device directly from the command line.

```bash
# Start a virtual device with default settings(cannot customize in CLI mode)
onvif-simulator start
# List available options
onvif-simulator start --help
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

To start from the bundled example, copy it to whichever location you prefer:

```bash
# Edit the OS-standard location
cp onvif-simulator.example.json "$HOME/.config/onvif-simulator/onvif-simulator.json"

# Or keep it in the working directory for quick experiments
cp onvif-simulator.example.json onvif-simulator.json
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
    "http_port": 8080,
    "rtsp_port": 8554
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

**Optional sections** (all fields shown in `onvif-simulator.example.json`):

| Section | Purpose |
|---------|---------|
| `auth` | Enable HTTP Digest / WS-UsernameToken / JWT authentication and manage users. |
| `runtime` | Persist Device Management runtime state: `discovery_mode`, `hostname`, `dns`, `default_gateway`, `network_protocols`, `system_date_and_time`. Written by ONVIF Set* operations; editing manually sets the initial value. |
| `events` | Configure the Event Service: `max_pull_points`, `subscription_timeout` (Go duration, e.g. `"1h"`), and the `topics` list (name + enabled flag). |

> **Notes:**
> - `network.rtsp_port` defaults to `8554` when omitted and must differ from `http_port`.
> - `media_file_path` must be an absolute path to an mp4 with an H.264 or H.265 video track.
> - `encoding`, `width`, `height`, and `fps` are auto-detected from the mp4 at startup; persisted values are only used as fallback display data when the simulator is stopped.

If you don't have a sample clip handy, generate one with ffmpeg:

```bash
ffmpeg -y -f lavfi -i testsrc=duration=10:size=1280x720:rate=30 \
    -c:v libx264 -pix_fmt yuv420p sample.mp4
```

## Development

### Prerequisites

Install [mise](https://mise.jdx.dev/) and let it provision the required toolchain:

```bash
mise install
```

This installs Go 1.26.2, golangci-lint 2.11.4, and Node.js 24.15.0 (needed for GUI via Wails).

For GUI development, also install the [Wails CLI](https://wails.io/docs/gettingstarted/installation):

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### Setup

```bash
git clone https://github.com/GyeongHoKim/onvif-simulator.git
cd onvif-simulator
go mod tidy
cp onvif-simulator.example.json onvif-simulator.json  # fill in your RTSP URIs
make setup            # install git hooks and commitlint
```

### Common Tasks

| Command | Description |
|---------|-------------|
| `make setup` | Install git hooks and commitlint (run once after cloning) |
| `make cli` | Build the CLI/TUI binary |
| `make gui` | Build the GUI binary (requires Wails) |
| `make format` | Run `go fmt` across all packages |
| `make lint` | Run golangci-lint |
| `make clean` | Remove build artifacts |

### Run

```bash
# CLI / TUI
go run . start
go run .

# GUI (requires Wails)
wails dev
```

## License

MIT
