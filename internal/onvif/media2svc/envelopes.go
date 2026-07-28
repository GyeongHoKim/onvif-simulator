package media2svc

import "encoding/xml"

// Response envelopes for each Media2 operation.

type getServiceCapabilitiesResponse struct {
	XMLName      xml.Name                   `xml:"GetServiceCapabilitiesResponse"`
	XMLNS        string                     `xml:"xmlns,attr"`
	Capabilities media2CapabilitiesEnvelope `xml:"tr2:Capabilities"`
}

type media2CapabilitiesEnvelope struct {
	XMLNSTT               string           `xml:"xmlns:tt,attr,omitempty"`
	SnapshotURI           bool             `xml:"SnapshotUri,attr,omitempty"`
	Rotation              bool             `xml:"Rotation,attr,omitempty"`
	VideoSourceMode       bool             `xml:"VideoSourceMode,attr,omitempty"`
	OSD                   bool             `xml:"OSD,attr,omitempty"`
	TemporaryOSDText      bool             `xml:"TemporaryOSDText,attr,omitempty"`
	ProfileCapabilities   profileCapsEnv   `xml:"tt:ProfileCapabilities"`
	StreamingCapabilities streamingCapsEnv `xml:"tt:StreamingCapabilities"`
	H264                  bool             `xml:"H264,attr,omitempty"`
	H265                  bool             `xml:"H265,attr,omitempty"`
	JPEG                  bool             `xml:"JPEG,attr,omitempty"`
}

type profileCapsEnv struct {
	MaximumNumberOfProfiles int `xml:"MaximumNumberOfProfiles"`
}

type streamingCapsEnv struct {
	RTPMulticast        bool `xml:"RTPMulticast"`
	RTPTCP              bool `xml:"RTPTCP"`
	RTPRTSPTCP          bool `xml:"RTPRTSPTCP"`
	NonAggregateControl bool `xml:"NonAggregateControl"`
	NoRTSPStreaming     bool `xml:"NoRTSPStreaming"`
}

// ---------- Profiles ----------

type getProfilesResponse struct {
	XMLName  xml.Name           `xml:"GetProfilesResponse"`
	XMLNS    string             `xml:"xmlns,attr"`
	XMLNSTT  string             `xml:"xmlns:tt,attr"`
	Profiles []media2ProfileEnv `xml:"tr2:Profiles"`
}

type media2ProfileEnv struct {
	Token               string                  `xml:"token,attr"`
	Name                string                  `xml:"tt:Name"`
	VideoEncoderConfigs []media2VideoEncoderEnv `xml:"tt:VideoEncoderConfiguration,omitempty"`
}

type media2VideoEncoderEnv struct {
	Token    string `xml:"token,attr"`
	Encoding string `xml:"tt:Encoding"`
}

// ---------- Video Encoder Configurations ----------

type getVideoEncoderConfigurationsResponse struct {
	XMLName        xml.Name                `xml:"GetVideoEncoderConfigurationsResponse"`
	XMLNS          string                  `xml:"xmlns,attr"`
	Configurations []media2VideoEncoderEnv `xml:"tr2:Configurations"`
}

type getVideoEncoderConfigurationResponse struct {
	XMLName       xml.Name              `xml:"GetVideoEncoderConfigurationResponse"`
	XMLNS         string                `xml:"xmlns,attr"`
	Configuration media2VideoEncoderEnv `xml:"tr2:Configuration"`
}

type getVideoEncoderConfigurationOptionsResponse struct {
	XMLName xml.Name                `xml:"GetVideoEncoderConfigurationOptionsResponse"`
	XMLNS   string                  `xml:"xmlns,attr"`
	Options media2EncoderOptionsEnv `xml:"tr2:Options"`
}

type media2EncoderOptionsEnv struct {
	H264 *h264OptionsEnv `xml:"tt:H264,omitempty"`
	H265 *h265OptionsEnv `xml:"tt:H265,omitempty"`
	JPEG *jpegOptionsEnv `xml:"tt:JPEG,omitempty"`
}

type h264OptionsEnv struct {
	ResolutionsAvailable []resolutionEnv `xml:"tt:ResolutionsAvailable"`
	FrameRateRange       intRangeEnv     `xml:"tt:FrameRateRange"`
	BitrateRange         intRangeEnv     `xml:"tt:BitrateRange"`
}

type h265OptionsEnv struct {
	ResolutionsAvailable []resolutionEnv `xml:"tt:ResolutionsAvailable"`
	FrameRateRange       intRangeEnv     `xml:"tt:FrameRateRange"`
	BitrateRange         intRangeEnv     `xml:"tt:BitrateRange"`
}

type jpegOptionsEnv struct {
	ResolutionsAvailable []resolutionEnv `xml:"tt:ResolutionsAvailable"`
	FrameRateRange       intRangeEnv     `xml:"tt:FrameRateRange"`
}

type resolutionEnv struct {
	Width  int `xml:"tt:Width"`
	Height int `xml:"tt:Height"`
}

type intRangeEnv struct {
	Min int `xml:"tt:Min"`
	Max int `xml:"tt:Max"`
}

type setVideoEncoderConfigurationResponse struct {
	XMLName xml.Name `xml:"SetVideoEncoderConfigurationResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

// ---------- Stream URI ----------

type getStreamURIResponse struct {
	XMLName xml.Name `xml:"GetStreamUriResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
	URI     string   `xml:"tr2:Uri"`
}

// ---------- Snapshot URI ----------

type getSnapshotURIResponse struct {
	XMLName xml.Name `xml:"GetSnapshotUriResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
	URI     string   `xml:"tr2:Uri"`
}
