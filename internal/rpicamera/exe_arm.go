//go:build rpicam && linux && arm

package rpicamera

import "embed"

//go:embed mtxrpicam_32/*
var mtxrpicamFS embed.FS
