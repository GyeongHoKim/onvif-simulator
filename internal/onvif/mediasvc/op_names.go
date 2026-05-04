package mediasvc

// SOAP operation names and codec tokens repeated across handlers and tests (goconst).
const (
	opGetServiceCapabilities = "GetServiceCapabilities"
	opCreateProfile          = "CreateProfile"
	opDeleteProfile          = "DeleteProfile"

	opGetVideoSourceConfiguration            = "GetVideoSourceConfiguration"
	opSetVideoSourceConfiguration            = "SetVideoSourceConfiguration"
	opAddVideoSourceConfiguration            = "AddVideoSourceConfiguration"
	opRemoveVideoSourceConfiguration         = "RemoveVideoSourceConfiguration"
	opGetCompatibleVideoSourceConfigurations = "GetCompatibleVideoSourceConfigurations"
	opGetVideoSourceConfigurationOptions     = "GetVideoSourceConfigurationOptions"

	opGetVideoEncoderConfiguration            = "GetVideoEncoderConfiguration"
	opSetVideoEncoderConfiguration            = "SetVideoEncoderConfiguration"
	opAddVideoEncoderConfiguration            = "AddVideoEncoderConfiguration"
	opRemoveVideoEncoderConfiguration         = "RemoveVideoEncoderConfiguration"
	opGetCompatibleVideoEncoderConfigurations = "GetCompatibleVideoEncoderConfigurations"
	opGetVideoEncoderConfigurationOptions     = "GetVideoEncoderConfigurationOptions"

	encodingH264 = "H264"
)
