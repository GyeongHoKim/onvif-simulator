package auth

// MediaOperationClasses maps ONVIF Media Service operation names to their
// access class per ONVIF Core §5.9.4.3. Operations not listed here default
// to ClassWriteSystem (Administrator-only) — the safer choice for a
// simulator receiving an unfamiliar request.
//
// Only operations that the mediasvc handler actually dispatches are listed.
// Operations that require direct control of the RTP pipeline
// (StartMulticastStreaming, StopMulticastStreaming, SetSynchronizationPoint)
// are intentionally omitted because the simulator does not run a media
// pipeline — RTP endpoints are external processes configured via
// ProfileConfig.RTSP.
var MediaOperationClasses = map[string]AccessClass{
	"GetServiceCapabilities": ClassPreAuth,

	// Read-only media queries.
	"GetProfiles":                             ClassReadMedia,
	"GetProfile":                              ClassReadMedia,
	"GetVideoSources":                         ClassReadMedia,
	"GetVideoSourceConfigurations":            ClassReadMedia,
	"GetVideoSourceConfiguration":             ClassReadMedia,
	"GetVideoEncoderConfigurations":           ClassReadMedia,
	"GetVideoEncoderConfiguration":            ClassReadMedia,
	"GetCompatibleVideoSourceConfigurations":  ClassReadMedia,
	"GetCompatibleVideoEncoderConfigurations": ClassReadMedia,
	"GetVideoSourceConfigurationOptions":      ClassReadMedia,
	"GetVideoEncoderConfigurationOptions":     ClassReadMedia,
	"GetStreamUri":                            ClassReadMedia,
	"GetSnapshotUri":                          ClassReadMedia,

	// Read-only media queries (continued).
	"GetGuaranteedNumberOfVideoEncoderInstances": ClassReadMedia,
	"GetMetadataConfigurations":                  ClassReadMedia,
	"GetMetadataConfiguration":                   ClassReadMedia,
	"GetCompatibleMetadataConfigurations":        ClassReadMedia,
	"GetMetadataConfigurationOptions":            ClassReadMedia,

	// Runtime-affecting writes.
	"CreateProfile":                   ClassActuate,
	"DeleteProfile":                   ClassActuate,
	"AddVideoSourceConfiguration":     ClassActuate,
	"RemoveVideoSourceConfiguration":  ClassActuate,
	"AddVideoEncoderConfiguration":    ClassActuate,
	"RemoveVideoEncoderConfiguration": ClassActuate,
	"SetVideoSourceConfiguration":     ClassActuate,
	"SetVideoEncoderConfiguration":    ClassActuate,
	"AddMetadataConfiguration":        ClassActuate,
	"RemoveMetadataConfiguration":     ClassActuate,
	"SetMetadataConfiguration":        ClassActuate,
}

// MediaOperationClass returns the AccessClass for the named Media Service
// operation. Unknown operations fall back to ClassWriteSystem (safer default).
func MediaOperationClass(op string) AccessClass {
	if c, ok := MediaOperationClasses[op]; ok {
		return c
	}
	return ClassWriteSystem
}
