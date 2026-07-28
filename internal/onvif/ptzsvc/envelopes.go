package ptzsvc

import "encoding/xml"

// Response envelopes for each PTZ operation.

type getServiceCapabilitiesResponse struct {
	XMLName      xml.Name                  `xml:"GetServiceCapabilitiesResponse"`
	XMLNS        string                    `xml:"xmlns,attr"`
	Capabilities ptzCapabilitiesEnvelope   `xml:"tptz:Capabilities"`
}

type ptzCapabilitiesEnvelope struct {
	EFlip                       bool `xml:"EFlip,attr,omitempty"`
	Reverse                     bool `xml:"Reverse,attr,omitempty"`
	GetCompatibleConfigurations bool `xml:"GetCompatibleConfigurations,attr,omitempty"`
	MoveStatus                  bool `xml:"MoveStatus,attr,omitempty"`
	StatusPosition              bool `xml:"StatusPosition,attr,omitempty"`
}

// ---------- Node envelopes ----------

type getNodesResponse struct {
	XMLName xml.Name           `xml:"GetNodesResponse"`
	XMLNS   string             `xml:"xmlns,attr"`
	XMLNSTT string             `xml:"xmlns:tt,attr"`
	Nodes   []ptzNodeEnvelope  `xml:"tptz:PTZNode"`
}

type getNodeResponse struct {
	XMLName xml.Name          `xml:"GetNodeResponse"`
	XMLNS   string            `xml:"xmlns,attr"`
	XMLNSTT string            `xml:"xmlns:tt,attr"`
	Node    ptzNodeEnvelope   `xml:"tptz:PTZNode"`
}

type ptzNodeEnvelope struct {
	Token string `xml:"token,attr"`
	Name  string `xml:"tt:Name"`
}

// ---------- Configuration envelopes ----------

type getConfigurationsResponse struct {
	XMLName        xml.Name                   `xml:"GetConfigurationsResponse"`
	XMLNS          string                     `xml:"xmlns,attr"`
	XMLNSTT        string                     `xml:"xmlns:tt,attr"`
	Configurations []ptzConfigurationEnvelope `xml:"tptz:PTZConfiguration"`
}

type getConfigurationResponse struct {
	XMLName       xml.Name                  `xml:"GetConfigurationResponse"`
	XMLNS         string                    `xml:"xmlns,attr"`
	XMLNSTT       string                    `xml:"xmlns:tt,attr"`
	Configuration ptzConfigurationEnvelope  `xml:"tptz:PTZConfiguration"`
}

type ptzConfigurationEnvelope struct {
	Token      string `xml:"token,attr"`
	Name       string `xml:"tt:Name"`
	UseCount   int    `xml:"tt:UseCount"`
	NodeToken  string `xml:"tt:NodeToken"`
}

type getConfigurationOptionsResponse struct {
	XMLName xml.Name                    `xml:"GetConfigurationOptionsResponse"`
	XMLNS   string                      `xml:"xmlns,attr"`
	XMLNSTT string                      `xml:"xmlns:tt,attr"`
	Options ptzConfigurationOptionsEnv  `xml:"tptz:PTZConfigurationOptions"`
}

type ptzConfigurationOptionsEnv struct {
	PanTiltPositionSpaceRange []spaceRangeEnv `xml:"tt:PanTiltPositionSpaceRange,omitempty"`
	ZoomPositionSpaceRange    []spaceRangeEnv `xml:"tt:ZoomPositionSpaceRange,omitempty"`
	PTZTimeout                intRangeEnv     `xml:"tt:PTZTimeout"`
}

type spaceRangeEnv struct {
	URI   string     `xml:"tt:URI"`
	XRange intRangeEnv `xml:"tt:XRange"`
	YRange intRangeEnv `xml:"tt:YRange"`
}

type intRangeEnv struct {
	Min float64 `xml:"tt:Min"`
	Max float64 `xml:"tt:Max"`
}

// ---------- Status envelopes ----------

type getStatusResponse struct {
	XMLName xml.Name        `xml:"GetStatusResponse"`
	XMLNS   string          `xml:"xmlns,attr"`
	XMLNSTT string          `xml:"xmlns:tt,attr"`
	Status  statusEnvelope  `xml:"tptz:PTZStatus"`
}

type statusEnvelope struct {
	Position   *vectorEnvelope  `xml:"tt:Position"`
	MoveStatus moveStatusEnv    `xml:"tt:MoveStatus"`
}

type moveStatusEnv struct {
	PanTilt string `xml:"tt:PanTilt"`
	Zoom    string `xml:"tt:Zoom"`
}

// ---------- Vector / Speed envelopes ----------

type vectorEnvelope struct {
	PanTilt *panTiltEnv `xml:"tt:PanTilt,omitempty"`
	Zoom    *zoomEnv    `xml:"tt:Zoom,omitempty"`
}

type panTiltEnv struct {
	XMLName xml.Name `xml:"tt:PanTilt"`
	X       float64  `xml:"x,attr"`
	Y       float64  `xml:"y,attr"`
}

type zoomEnv struct {
	XMLName xml.Name `xml:"tt:Zoom"`
	X       float64  `xml:"x,attr"`
}

// ---------- Preset envelopes ----------

type getPresetsResponse struct {
	XMLName  xml.Name           `xml:"GetPresetsResponse"`
	XMLNS    string             `xml:"xmlns,attr"`
	XMLNSTT  string             `xml:"xmlns:tt,attr"`
	Presets  []presetEnvelope   `xml:"tptz:Preset"`
}

type presetEnvelope struct {
	Token    string          `xml:"token,attr"`
	Name     string          `xml:"tt:Name"`
	Position *vectorEnvelope `xml:"tt:Position,omitempty"`
}

type setPresetResponse struct {
	XMLName      xml.Name `xml:"SetPresetResponse"`
	XMLNS        string   `xml:"xmlns,attr"`
	PresetToken  string   `xml:"tptz:PresetToken"`
}

type removePresetResponse struct {
	XMLName xml.Name `xml:"RemovePresetResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

// ---------- Move responses ----------

type continuousMoveResponse struct {
	XMLName xml.Name `xml:"ContinuousMoveResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

type absoluteMoveResponse struct {
	XMLName xml.Name `xml:"AbsoluteMoveResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

type relativeMoveResponse struct {
	XMLName xml.Name `xml:"RelativeMoveResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

type stopResponse struct {
	XMLName xml.Name `xml:"StopResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

type gotoPresetResponse struct {
	XMLName xml.Name `xml:"GotoPresetResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

type gotoHomePositionResponse struct {
	XMLName xml.Name `xml:"GotoHomePositionResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

type setHomePositionResponse struct {
	XMLName xml.Name `xml:"SetHomePositionResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}
