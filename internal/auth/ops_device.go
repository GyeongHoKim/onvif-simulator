package auth

// DeviceOperationClasses maps ONVIF Device Service operation names to their
// access class per ONVIF Core §5.9.4.3. Operations not listed here default
// to ClassWriteSystem (Administrator-only) — the safer choice for a
// simulator receiving an unfamiliar request.
var DeviceOperationClasses = map[string]AccessClass{
	// §5.9.4.3 examples are explicit; the rest follow the same shape.
	"GetEndpointReference":   ClassPreAuth,
	"GetServices":            ClassPreAuth,
	"GetServiceCapabilities": ClassPreAuth,
	"GetCapabilities":        ClassPreAuth,
	"GetWsdlUrl":             ClassPreAuth,
	"GetSystemDateAndTime":   ClassPreAuth,
	"GetHostname":            ClassPreAuth,
	// Read-only configuration queries.
	"GetDeviceInformation":     ClassReadSystem,
	"GetNetworkInterfaces":     ClassReadSystem,
	"GetNetworkProtocols":      ClassReadSystem,
	"GetNetworkDefaultGateway": ClassReadSystem,
	"GetDNS":                   ClassReadSystem,
	"GetNTP":                   ClassReadSystem,
	"GetScopes":                ClassReadSystem,
	"GetDiscoveryMode":         ClassReadSystem,
	"GetGeoLocation":           ClassReadSystem,
	// Sensitive reads.
	"GetUsers":        ClassReadSystemSensitive,
	"GetUserRoles":    ClassReadSystemSensitive,
	"GetAccessPolicy": ClassReadSystemSensitive,
	"GetCertificates": ClassReadSystemSensitive,
	"GetRemoteUser":   ClassReadSystemSensitive,
	// Secret reads.
	"GetSystemLog":                ClassReadSystemSecret,
	"GetSystemBackup":             ClassReadSystemSecret,
	"GetSystemSupportInformation": ClassReadSystemSecret,
	// Write operations (Administrator-only).
	"CreateUsers":              ClassWriteSystem,
	"DeleteUsers":              ClassWriteSystem,
	"SetUser":                  ClassWriteSystem,
	"SetHostname":              ClassWriteSystem,
	"SetNetworkInterfaces":     ClassWriteSystem,
	"SetNetworkProtocols":      ClassWriteSystem,
	"SetNetworkDefaultGateway": ClassWriteSystem,
	"SetDNS":                   ClassWriteSystem,
	"SetNTP":                   ClassWriteSystem,
	"SetScopes":                ClassWriteSystem,
	"AddScopes":                ClassWriteSystem,
	"RemoveScopes":             ClassWriteSystem,
	"SetDiscoveryMode":         ClassWriteSystem,
	"SetGeoLocation":           ClassWriteSystem,
	"SetAccessPolicy":          ClassWriteSystem,
	"SetRemoteUser":            ClassWriteSystem,
	"SetHashingAlgorithm":      ClassWriteSystem,
	// Unrecoverable.
	"SetSystemFactoryDefault": ClassUnrecoverable,
	"UpgradeFirmware":         ClassUnrecoverable,
	"StartFirmwareUpgrade":    ClassUnrecoverable,
	"StartSystemRestore":      ClassUnrecoverable,
	"RestoreSystem":           ClassUnrecoverable,
	"SystemReboot":            ClassUnrecoverable,
	// Actuate.
	"SendAuxiliaryCommand": ClassActuate,
	"SetRelayOutputState":  ClassActuate,
}

// DeviceOperationClass returns the AccessClass for the named Device Service
// operation. Unknown operations fall back to ClassWriteSystem (safer default).
func DeviceOperationClass(op string) AccessClass {
	if c, ok := DeviceOperationClasses[op]; ok {
		return c
	}
	return ClassWriteSystem
}
