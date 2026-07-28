package imgsvc

import "encoding/xml"

// Response envelopes for each Imaging operation.

type getServiceCapabilitiesResponse struct {
	XMLName      xml.Name                `xml:"GetServiceCapabilitiesResponse"`
	XMLNS        string                  `xml:"xmlns,attr"`
	Capabilities imgCapabilitiesEnvelope `xml:"timg:Capabilities"`
}

type imgCapabilitiesEnvelope struct {
	ImageStabilization bool `xml:"ImageStabilization,attr,omitempty"`
}

// ---------- Imaging Settings envelopes ----------

type getImagingSettingsResponse struct {
	XMLName         xml.Name                `xml:"GetImagingSettingsResponse"`
	XMLNS           string                  `xml:"xmlns,attr"`
	XMLNSTT         string                  `xml:"xmlns:tt,attr"`
	ImagingSettings imagingSettingsEnvelope `xml:"timg:ImagingSettings"`
}

type imagingSettingsEnvelope struct {
	Brightness            *brightnessVal       `xml:"tt:Brightness,omitempty"`
	Contrast              *contrastVal         `xml:"tt:Contrast,omitempty"`
	Sharpness             *sharpnessVal        `xml:"tt:Sharpness,omitempty"`
	Exposure              *exposureEnv         `xml:"tt:Exposure,omitempty"`
	WhiteBalance          *whiteBalanceEnv     `xml:"tt:WhiteBalance,omitempty"`
	IrCutFilter           *irCutFilterVal      `xml:"tt:IrCutFilter,omitempty"`
	BacklightCompensation *backlightCompEnv    `xml:"tt:BacklightCompensation,omitempty"`
	WideDynamicRange      *wideDynamicRangeEnv `xml:"tt:WideDynamicRange,omitempty"`
	Focus                 *focusSettingsEnv    `xml:"tt:Focus,omitempty"`
}

type brightnessVal struct {
	XMLName xml.Name `xml:"tt:Brightness"`
	Value   float64  `xml:",chardata"`
}

type contrastVal struct {
	XMLName xml.Name `xml:"tt:Contrast"`
	Value   float64  `xml:",chardata"`
}

type sharpnessVal struct {
	XMLName xml.Name `xml:"tt:Sharpness"`
	Value   float64  `xml:",chardata"`
}

type irCutFilterVal struct {
	XMLName xml.Name `xml:"tt:IrCutFilter"`
	Value   string   `xml:",chardata"`
}

type exposureEnv struct {
	Mode     string `xml:"tt:Mode"`
	Priority string `xml:"tt:Priority,omitempty"`
}

type whiteBalanceEnv struct {
	Mode string `xml:"tt:Mode"`
}

type backlightCompEnv struct {
	Mode  string  `xml:"tt:Mode"`
	Level float64 `xml:"tt:Level,omitempty"`
}

type wideDynamicRangeEnv struct {
	Mode  string  `xml:"tt:Mode"`
	Level float64 `xml:"tt:Level,omitempty"`
}

type focusSettingsEnv struct {
	FocusMode string `xml:"tt:FocusMode"`
}

type setImagingSettingsResponse struct {
	XMLName xml.Name `xml:"SetImagingSettingsResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

// ---------- Imaging Options envelopes ----------

type getOptionsResponse struct {
	XMLName        xml.Name               `xml:"GetOptionsResponse"`
	XMLNS          string                 `xml:"xmlns,attr"`
	XMLNSTT        string                 `xml:"xmlns:tt,attr"`
	ImagingOptions imagingOptionsEnvelope `xml:"timg:ImagingOptions"`
}

type imagingOptionsEnvelope struct {
	Brightness            floatRangeEnv `xml:"tt:Brightness"`
	Contrast              floatRangeEnv `xml:"tt:Contrast"`
	Sharpness             floatRangeEnv `xml:"tt:Sharpness"`
	ExposureModes         stringListEnv `xml:"tt:ExposureMode"`
	ExposurePriorities    stringListEnv `xml:"tt:ExposurePriority"`
	WhiteBalanceModes     stringListEnv `xml:"tt:WhiteBalanceMode"`
	IrCutFilterModes      stringListEnv `xml:"tt:IrCutFilterModes"`
	BacklightCompModes    stringListEnv `xml:"tt:BacklightCompensationMode"`
	WideDynamicRangeModes stringListEnv `xml:"tt:WideDynamicRangeMode"`
	FocusModes            stringListEnv `xml:"tt:FocusModes"`
}

type floatRangeEnv struct {
	Min float64 `xml:"tt:Min"`
	Max float64 `xml:"tt:Max"`
}

type stringListEnv struct {
	Items []string `xml:"tt:Items"`
}

// ---------- Imaging Status envelopes ----------

type getStatusResponse struct {
	XMLName xml.Name              `xml:"GetStatusResponse"`
	XMLNS   string                `xml:"xmlns,attr"`
	XMLNSTT string                `xml:"xmlns:tt,attr"`
	Status  imagingStatusEnvelope `xml:"timg:Status"`
}

type imagingStatusEnvelope struct {
	FocusStatus *focusStatusEnv `xml:"tt:FocusStatus,omitempty"`
}

type focusStatusEnv struct {
	Position string `xml:"tt:Position"`
	Error    string `xml:"tt:Error,omitempty"`
}

// ---------- Imaging Preset envelopes ----------

type getPresetsResponse struct {
	XMLName xml.Name           `xml:"GetPresetsResponse"`
	XMLNS   string             `xml:"xmlns,attr"`
	Presets []imagingPresetEnv `xml:"timg:Preset"`
}

type imagingPresetEnv struct {
	Token string `xml:"token,attr"`
	Name  string `xml:"timg:Name"`
	Type  string `xml:"type,attr"`
}

type getCurrentPresetResponse struct {
	XMLName xml.Name          `xml:"GetCurrentPresetResponse"`
	XMLNS   string            `xml:"xmlns,attr"`
	Preset  *imagingPresetEnv `xml:"timg:Preset,omitempty"`
}

type setCurrentPresetResponse struct {
	XMLName xml.Name `xml:"SetCurrentPresetResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}
