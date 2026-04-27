package tui

// Key name constants — used across screens so goconst/nolint pressure stays
// low and typos in key bindings fail at compile time.
const (
	keyEsc      = "esc"
	keyEnter    = "enter"
	keyTab      = "tab"
	keyShiftTab = "shift+tab"
	keyUp       = "up"
	keyDown     = "down"
	keyLeft     = "left"
	keyRight    = "right"
	keyCtrlS    = "ctrl+s"

	prefixSel   = "> "
	prefixUnsel = "  "
)
