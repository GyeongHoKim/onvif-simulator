// Package gui wires the Wails application and exposes Run for the entry point.
package gui

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// Run starts the Wails GUI. Call from cmd/gui/main.go.
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
	if err != nil {
		log.Fatalf("wails run: %v", err)
	}
}
