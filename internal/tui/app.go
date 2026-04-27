// Package tui renders the terminal user interface for the ONVIF simulator.
//
// The TUI is a Bubble Tea program with six screens (Dashboard, Events, Media,
// Auth, Device, Log) that share a single *simulator.Simulator through the
// SimulatorAPI contract. Screens are keyboard-navigable; top-level nav is
// tab/shift+tab or direct jumps 1-6.
//
// Embed the TUI into a CLI subcommand via Run. To surface simulator events
// and mutations in the Log screen, construct a CallbackBridge before
// simulator.New and pass its OnEvent / OnMutation methods into Options.
package tui
