// Package gui wires the Wails application and exposes Run for the entry point.
package gui

import (
	"context"
	"embed"
	"fmt"
	"os"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// Bound HTTP server Shutdown in simulator.Stop so GUI exit cannot hang
// indefinitely if the listen socket misbehaves.
const simulatorStopTimeout = 5 * time.Second

// Run starts the Wails GUI. Call from cmd/gui/main.go.
//
// On Wails return (window closed by the user) we Stop the simulator so it
// flushes its file logger and tears down its HTTP/RTSP servers. Without
// this, .app bundle exits would leak the open log file's last few records.
func Run() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "ONVIF Simulator",
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.OnStartup,
		Bind:             []any{app},
	})
	if app.sim != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), simulatorStopTimeout)
		stopErr := app.sim.Stop(stopCtx)
		cancel()
		if stopErr != nil {
			fmt.Fprintf(os.Stderr, "simulator stop: %v\n", stopErr)
		}
	}
	if err != nil {
		// stderr is the last-resort path: the simulator's logger may have
		// been closed by Stop above, and the user double-clicked an .app
		// bundle so there's no console — but `wails dev` shows it.
		fmt.Fprintf(os.Stderr, "wails run: %v\n", err)
		os.Exit(1)
	}
}
