package auth

// PTZOperationClasses maps ONVIF PTZ Service operation names to their
// access class per ONVIF Core §5.9.4.3. Operations not listed here default
// to ClassWriteSystem (Administrator-only).
var PTZOperationClasses = map[string]AccessClass{
	opGetServiceCapabilities: ClassPreAuth,

	// Read-only PTZ queries.
	"GetNodes":                    ClassReadSystem,
	"GetNode":                     ClassReadSystem,
	"GetConfigurations":           ClassReadSystem,
	"GetConfiguration":            ClassReadSystem,
	"GetConfigurationOptions":     ClassReadSystem,
	"GetCompatibleConfigurations": ClassReadSystem,

	// Status queries (Operator or above).
	"GetStatus":  ClassActuate,
	"GetPresets": ClassActuate,

	// Actuate operations (Operator or above).
	"ContinuousMove":       ClassActuate,
	"AbsoluteMove":         ClassActuate,
	"RelativeMove":         ClassActuate,
	"Stop":                 ClassActuate,
	"SetPreset":            ClassActuate,
	"RemovePreset":         ClassActuate,
	"GotoPreset":           ClassActuate,
	"GotoHomePosition":     ClassActuate,
	"SendAuxiliaryCommand": ClassActuate,

	// Write operations (Administrator-only).
	"SetHomePosition":  ClassWriteSystem,
	"SetConfiguration": ClassWriteSystem,
}

// PTZOperationClass returns the AccessClass for the named PTZ Service
// operation. Unknown operations fall back to ClassWriteSystem.
func PTZOperationClass(op string) AccessClass {
	if c, ok := PTZOperationClasses[op]; ok {
		return c
	}
	return ClassWriteSystem
}
