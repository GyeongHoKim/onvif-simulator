//go:build rpicam && linux && arm64

package rpicamera

import "embed"

//go:embed mtxrpicam_64/*
var mtxrpicamFS embed.FS
