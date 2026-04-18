# onvif-simulator

A cross-platform ONVIF device simulator written in Go. Supports CLI, TUI, and GUI modes, making it easy to test ONVIF clients without real hardware.

## Features

- ONVIF Profile S (camera streaming simulation)
- Multiple interface modes: CLI, TUI, GUI
- Configure multiple virtual devices
- Lightweight and dependency-free installation

## Installation

### Linux / macOS

```bash
curl -fsSL https://github.com/GyeongHoKim/onvif-simulator/releases/latest/download/install.sh | bash
```

### Windows (PowerShell)

```powershell
iex (irm https://github.com/GyeongHoKim/onvif-simulator/releases/latest/download/install.ps1)
```

After installation, the `onvif-simulator` command will be available in your terminal.

### Manual Installation

Download the binary for your platform from the [Releases](https://github.com/GyeongHoKim/onvif-simulator/releases) page and place it in your `PATH`.

## Usage

### CLI Mode

Run a single virtual device directly from the command line.

```bash
# Start a virtual device with default settings
onvif-simulator start

# Start with custom options (overrides .env values)
onvif-simulator start --port 8080 --name "Camera-01"

# List available options
onvif-simulator start --help
```

### TUI Mode

Interactive terminal UI for managing multiple virtual devices.

```bash
onvif-simulator tui
```

Key bindings:

| Key | Action |
|-----|--------|
| `n` | Add new device |
| `d` | Delete selected device |
| `Enter` | View device details |
| `q` | Quit |

### GUI Mode

Launches a native window with a web-based interface for full graphical management.

```bash
onvif-simulator gui
```

## Configuration

### RTSP Endpoints (.env)

onvif-simulator does not manage RTSP streams itself. You provide pre-existing RTSP endpoints via a `.env` file in the working directory.

Copy `.env.example` and fill in your values:

```bash
cp .env.example .env
```

```env
# Number of virtual devices to simulate
DEVICE_COUNT=2

# Per-device settings (index starts at 1)
DEVICE_1_NAME=Camera-01
DEVICE_1_PORT=8080
DEVICE_1_RTSP_URI=rtsp://localhost:8554/live
DEVICE_1_MANUFACTURER=Acme
DEVICE_1_MODEL=VirtualCam-1000

DEVICE_2_NAME=Camera-02
DEVICE_2_PORT=8081
DEVICE_2_RTSP_URI=rtsp://localhost:8554/live2
DEVICE_2_MANUFACTURER=Acme
DEVICE_2_MODEL=VirtualCam-1000
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
cp .env.example .env  # fill in your RTSP URIs
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
go run . tui

# GUI (requires Wails)
wails dev
```

## License

MIT
