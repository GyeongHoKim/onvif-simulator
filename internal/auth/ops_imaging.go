package auth

// ImagingOperationClasses maps ONVIF Imaging Service operation names to their
// access class per ONVIF Core §5.9.4.3. Operations not listed here default
// to ClassWriteSystem (Administrator-only).
var ImagingOperationClasses = map[string]AccessClass{
	opGetServiceCapabilities: ClassPreAuth,

	// Read-only imaging queries.
	"GetImagingSettings": ClassReadSystem,
	"GetOptions":         ClassReadSystem,
	"GetStatus":          ClassReadSystem,
	"GetPresets":         ClassReadSystem,
	"GetCurrentPreset":   ClassReadSystem,

	// Actuate operations (Operator or above).
	"SetImagingSettings": ClassActuate,
	"SetCurrentPreset":   ClassActuate,
}

// ImagingOperationClass returns the AccessClass for the named Imaging Service
// operation. Unknown operations fall back to ClassWriteSystem.
func ImagingOperationClass(op string) AccessClass {
	if c, ok := ImagingOperationClasses[op]; ok {
		return c
	}
	return ClassWriteSystem
}
