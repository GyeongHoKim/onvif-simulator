package mediasvc

import "encoding/xml"

// Response envelopes for each Media operation. Media responses declare the
// Media WSDL namespace as the default xmlns and alias the ONVIF schema
// namespace as `tt:` so nested types (Bounds, Resolution, H264, Range,
// RateControl, …) can use the tt prefix.

type getServiceCapabilitiesResponse struct {
	XMLName      xml.Name                  `xml:"GetServiceCapabilitiesResponse"`
	XMLNS        string                    `xml:"xmlns,attr"`
	Capabilities mediaCapabilitiesEnvelope `xml:"Capabilities"`
}

type mediaCapabilitiesEnvelope struct {
	SnapshotURI           bool                          `xml:"SnapshotUri,attr,omitempty"`
	Rotation              bool                          `xml:"Rotation,attr,omitempty"`
	VideoSourceMode       bool                          `xml:"VideoSourceMode,attr,omitempty"`
	OSD                   bool                          `xml:"OSD,attr,omitempty"`
	TemporaryOSDText      bool                          `xml:"TemporaryOSDText,attr,omitempty"`
	EXICompression        bool                          `xml:"EXICompression,attr,omitempty"`
	ProfileCapabilities   profileCapabilitiesEnvelope   `xml:"ProfileCapabilities"`
	StreamingCapabilities streamingCapabilitiesEnvelope `xml:"StreamingCapabilities"`
}

type profileCapabilitiesEnvelope struct {
	MaximumNumberOfProfiles int `xml:"MaximumNumberOfProfiles,attr,omitempty"`
}

type streamingCapabilitiesEnvelope struct {
	RTPMulticast        bool `xml:"RTPMulticast,attr,omitempty"`
	RTPTCP              bool `xml:"RTP_TCP,attr,omitempty"`
	RTPRTSPTCP          bool `xml:"RTP_RTSP_TCP,attr,omitempty"`
	NonAggregateControl bool `xml:"NonAggregateControl,attr,omitempty"`
	NoRTSPStreaming     bool `xml:"NoRTSPStreaming,attr,omitempty"`
}

// ---------- Profile envelopes ----------

type getProfilesResponse struct {
	XMLName  xml.Name          `xml:"GetProfilesResponse"`
	XMLNS    string            `xml:"xmlns,attr"`
	XMLNSTT  string            `xml:"xmlns:tt,attr"`
	Profiles []profileEnvelope `xml:"Profiles"`
}

type getProfileResponse struct {
	XMLName xml.Name        `xml:"GetProfileResponse"`
	XMLNS   string          `xml:"xmlns,attr"`
	XMLNSTT string          `xml:"xmlns:tt,attr"`
	Profile profileEnvelope `xml:"Profile"`
}

type createProfileResponse struct {
	XMLName xml.Name        `xml:"CreateProfileResponse"`
	XMLNS   string          `xml:"xmlns,attr"`
	XMLNSTT string          `xml:"xmlns:tt,attr"`
	Profile profileEnvelope `xml:"Profile"`
}

type deleteProfileResponse struct {
	XMLName xml.Name `xml:"DeleteProfileResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

type profileEnvelope struct {
	Token                     string                             `xml:"token,attr"`
	Fixed                     string                             `xml:"fixed,attr,omitempty"`
	Name                      string                             `xml:"tt:Name"`
	VideoSourceConfiguration  *videoSourceConfigurationEnvelope  `xml:"tt:VideoSourceConfiguration,omitempty"`
	VideoEncoderConfiguration *videoEncoderConfigurationEnvelope `xml:"tt:VideoEncoderConfiguration,omitempty"`
}

// ---------- VideoSource envelopes ----------

type getVideoSourcesResponse struct {
	XMLName      xml.Name              `xml:"GetVideoSourcesResponse"`
	XMLNS        string                `xml:"xmlns,attr"`
	XMLNSTT      string                `xml:"xmlns:tt,attr"`
	VideoSources []videoSourceEnvelope `xml:"VideoSources"`
}

type videoSourceEnvelope struct {
	Token      string             `xml:"token,attr"`
	Framerate  int                `xml:"tt:Framerate"`
	Resolution resolutionEnvelope `xml:"tt:Resolution"`
}

type getVideoSourceConfigurationsResponse struct {
	XMLName        xml.Name                           `xml:"GetVideoSourceConfigurationsResponse"`
	XMLNS          string                             `xml:"xmlns,attr"`
	XMLNSTT        string                             `xml:"xmlns:tt,attr"`
	Configurations []videoSourceConfigurationEnvelope `xml:"Configurations"`
}

type getVideoSourceConfigurationResponse struct {
	XMLName       xml.Name                         `xml:"GetVideoSourceConfigurationResponse"`
	XMLNS         string                           `xml:"xmlns,attr"`
	XMLNSTT       string                           `xml:"xmlns:tt,attr"`
	Configuration videoSourceConfigurationEnvelope `xml:"Configuration"`
}

type setVideoSourceConfigurationResponse struct {
	XMLName xml.Name `xml:"SetVideoSourceConfigurationResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

type addVideoSourceConfigurationResponse struct {
	XMLName xml.Name `xml:"AddVideoSourceConfigurationResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

type removeVideoSourceConfigurationResponse struct {
	XMLName xml.Name `xml:"RemoveVideoSourceConfigurationResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

type getCompatibleVideoSourceConfigurationsResponse struct {
	XMLName        xml.Name                           `xml:"GetCompatibleVideoSourceConfigurationsResponse"`
	XMLNS          string                             `xml:"xmlns,attr"`
	XMLNSTT        string                             `xml:"xmlns:tt,attr"`
	Configurations []videoSourceConfigurationEnvelope `xml:"Configurations"`
}

type getVideoSourceConfigurationOptionsResponse struct {
	XMLName xml.Name                                `xml:"GetVideoSourceConfigurationOptionsResponse"`
	XMLNS   string                                  `xml:"xmlns,attr"`
	XMLNSTT string                                  `xml:"xmlns:tt,attr"`
	Options videoSourceConfigurationOptionsEnvelope `xml:"Options"`
}

type videoSourceConfigurationEnvelope struct {
	Token       string         `xml:"token,attr"`
	Name        string         `xml:"tt:Name"`
	UseCount    int            `xml:"tt:UseCount"`
	SourceToken string         `xml:"tt:SourceToken"`
	Bounds      boundsEnvelope `xml:"tt:Bounds"`
}

type boundsEnvelope struct {
	X      int `xml:"x,attr"`
	Y      int `xml:"y,attr"`
	Width  int `xml:"width,attr"`
	Height int `xml:"height,attr"`
}

type resolutionEnvelope struct {
	Width  int `xml:"tt:Width"`
	Height int `xml:"tt:Height"`
}

type videoSourceConfigurationOptionsEnvelope struct {
	MaximumNumberOfProfiles    int                 `xml:"MaximumNumberOfProfiles,attr,omitempty"`
	BoundsRange                boundsRangeEnvelope `xml:"tt:BoundsRange"`
	VideoSourceTokensAvailable []string            `xml:"tt:VideoSourceTokensAvailable"`
}

type boundsRangeEnvelope struct {
	XRange      intRangeEnvelope `xml:"tt:XRange"`
	YRange      intRangeEnvelope `xml:"tt:YRange"`
	WidthRange  intRangeEnvelope `xml:"tt:WidthRange"`
	HeightRange intRangeEnvelope `xml:"tt:HeightRange"`
}

type intRangeEnvelope struct {
	Min int `xml:"tt:Min"`
	Max int `xml:"tt:Max"`
}

// ---------- VideoEncoder envelopes ----------

type getVideoEncoderConfigurationsResponse struct {
	XMLName        xml.Name                            `xml:"GetVideoEncoderConfigurationsResponse"`
	XMLNS          string                              `xml:"xmlns,attr"`
	XMLNSTT        string                              `xml:"xmlns:tt,attr"`
	Configurations []videoEncoderConfigurationEnvelope `xml:"Configurations"`
}

type getVideoEncoderConfigurationResponse struct {
	XMLName       xml.Name                          `xml:"GetVideoEncoderConfigurationResponse"`
	XMLNS         string                            `xml:"xmlns,attr"`
	XMLNSTT       string                            `xml:"xmlns:tt,attr"`
	Configuration videoEncoderConfigurationEnvelope `xml:"Configuration"`
}

type setVideoEncoderConfigurationResponse struct {
	XMLName xml.Name `xml:"SetVideoEncoderConfigurationResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

type addVideoEncoderConfigurationResponse struct {
	XMLName xml.Name `xml:"AddVideoEncoderConfigurationResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

type removeVideoEncoderConfigurationResponse struct {
	XMLName xml.Name `xml:"RemoveVideoEncoderConfigurationResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

type getCompatibleVideoEncoderConfigurationsResponse struct {
	XMLName        xml.Name                            `xml:"GetCompatibleVideoEncoderConfigurationsResponse"`
	XMLNS          string                              `xml:"xmlns,attr"`
	XMLNSTT        string                              `xml:"xmlns:tt,attr"`
	Configurations []videoEncoderConfigurationEnvelope `xml:"Configurations"`
}

type getVideoEncoderConfigurationOptionsResponse struct {
	XMLName xml.Name                                 `xml:"GetVideoEncoderConfigurationOptionsResponse"`
	XMLNS   string                                   `xml:"xmlns,attr"`
	XMLNSTT string                                   `xml:"xmlns:tt,attr"`
	Options videoEncoderConfigurationOptionsEnvelope `xml:"Options"`
}

type videoEncoderConfigurationEnvelope struct {
	Token          string                     `xml:"token,attr"`
	Name           string                     `xml:"tt:Name"`
	UseCount       int                        `xml:"tt:UseCount"`
	Encoding       string                     `xml:"tt:Encoding"`
	Resolution     resolutionEnvelope         `xml:"tt:Resolution"`
	Quality        float64                    `xml:"tt:Quality"`
	RateControl    *rateControlEnvelope       `xml:"tt:RateControl,omitempty"`
	H264           *h264ConfigurationEnvelope `xml:"tt:H264,omitempty"`
	SessionTimeout string                     `xml:"tt:SessionTimeout,omitempty"`
}

type rateControlEnvelope struct {
	FrameRateLimit   int `xml:"tt:FrameRateLimit,omitempty"`
	EncodingInterval int `xml:"tt:EncodingInterval,omitempty"`
	BitrateLimit     int `xml:"tt:BitrateLimit,omitempty"`
}

type h264ConfigurationEnvelope struct {
	GovLength   int    `xml:"tt:GovLength,omitempty"`
	H264Profile string `xml:"tt:H264Profile,omitempty"`
}

type videoEncoderConfigurationOptionsEnvelope struct {
	QualityRange intRangeEnvelope    `xml:"tt:QualityRange"`
	H264         h264OptionsEnvelope `xml:"tt:H264"`
}

type h264OptionsEnvelope struct {
	ResolutionsAvailable  []resolutionEnvelope `xml:"tt:ResolutionsAvailable"`
	GovLengthRange        intRangeEnvelope     `xml:"tt:GovLengthRange"`
	FrameRateRange        intRangeEnvelope     `xml:"tt:FrameRateRange"`
	EncodingIntervalRange intRangeEnvelope     `xml:"tt:EncodingIntervalRange"`
	H264ProfilesSupported []string             `xml:"tt:H264ProfilesSupported"`
}

// ---------- Stream / Snapshot URI envelopes ----------

type getStreamURIResponse struct {
	XMLName  xml.Name         `xml:"GetStreamUriResponse"`
	XMLNS    string           `xml:"xmlns,attr"`
	XMLNSTT  string           `xml:"xmlns:tt,attr"`
	MediaURI mediaURIEnvelope `xml:"MediaUri"`
}

type getSnapshotURIResponse struct {
	XMLName  xml.Name         `xml:"GetSnapshotUriResponse"`
	XMLNS    string           `xml:"xmlns,attr"`
	XMLNSTT  string           `xml:"xmlns:tt,attr"`
	MediaURI mediaURIEnvelope `xml:"MediaUri"`
}

type mediaURIEnvelope struct {
	URI                 string `xml:"tt:Uri"`
	InvalidAfterConnect bool   `xml:"tt:InvalidAfterConnect"`
	InvalidAfterReboot  bool   `xml:"tt:InvalidAfterReboot"`
	Timeout             string `xml:"tt:Timeout"`
}
