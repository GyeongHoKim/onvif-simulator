# onvif-simulator

[![CI](https://github.com/GyeongHoKim/onvif-simulator/actions/workflows/ci.yml/badge.svg)](https://github.com/GyeongHoKim/onvif-simulator/actions/workflows/ci.yml)
[![Release](https://github.com/GyeongHoKim/onvif-simulator/actions/workflows/release.yml/badge.svg)](https://github.com/GyeongHoKim/onvif-simulator/actions/workflows/release.yml)
[![codecov](https://codecov.io/gh/GyeongHoKim/onvif-simulator/graph/badge.svg)](https://codecov.io/gh/GyeongHoKim/onvif-simulator)
[![CodeRabbit Pull Request Reviews](https://img.shields.io/coderabbit/prs/github/GyeongHoKim/onvif-simulator?utm_source=oss&utm_medium=github&utm_campaign=GyeongHoKim%2Fonvif-simulator&labelColor=171717&color=FF570A&link=https%3A%2F%2Fcoderabbit.ai&label=CodeRabbit+Reviews)](https://coderabbit.ai)

A cross-platform ONVIF device simulator written in Go. Supports CLI, TUI, and GUI modes, making it easy to test ONVIF clients without real hardware.

## Features

- ONVIF Profile S (camera streaming simulation)
- Multiple interface modes: CLI, TUI, GUI
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

onvif-simulator does not manage RTSP streams itself. You should provide pre-existing RTSP endpoints in a JSON config file in the working directory.

Copy the example file and edit the URIs:

```bash
cp onvif-simulator.example.json onvif-simulator.json
```

```json
{
  "version": 1,
  "main_rtsp_uri": "rtsp://localhost:8554/live",
  "sub_rtsp_uri": "rtsp://localhost:8554/live2"
}
```

The RTSP URIs must be reachable at runtime — onvif-simulator forwards them as-is to ONVIF clients.

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
