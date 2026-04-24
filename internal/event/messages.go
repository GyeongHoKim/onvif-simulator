package event

import (
	"fmt"
	"html"
	"time"
)

// PropertyType describes how an event message should be interpreted by the
// receiver. ONVIF Core §9.6.
type PropertyType string

const (
	// PropertyChanged signals that the monitored property changed its value.
	PropertyChanged PropertyType = "Changed"
	// PropertyInitialized is sent on SetSynchronizationPoint to report the
	// current property value without implying a change.
	PropertyInitialized PropertyType = "Initialized"
	// PropertyDeleted signals that the monitored property no longer exists.
	PropertyDeleted PropertyType = "Deleted"
)

// MotionAlarm publishes a tns1:VideoSource/MotionAlarm event.
//
// sourceToken identifies the VideoSourceConfiguration (e.g. "vs0").
// state is true when motion is detected, false when it clears.
//
// This is the most common Profile S event; it maps to the ONVIF
// RuleEngine/FieldDetector topic family as well as the simpler
// VideoSource/MotionAlarm property topic.
func (b *Broker) MotionAlarm(sourceToken string, state bool) {
	b.Publish(
		"tns1:VideoSource/MotionAlarm",
		boolPropertyMessage(sourceToken, "VideoSourceConfigurationToken", "State", state, PropertyChanged),
	)
}

// ImageTooBlurry publishes a tns1:VideoSource/ImageTooBlurry event.
//
// sourceToken identifies the VideoSourceConfiguration.
// state is true when the image quality alert is active.
func (b *Broker) ImageTooBlurry(sourceToken string, state bool) {
	b.Publish(
		"tns1:VideoSource/ImageTooBlurry",
		boolPropertyMessage(sourceToken, "VideoSourceConfigurationToken", "State", state, PropertyChanged),
	)
}

// ImageTooDark publishes a tns1:VideoSource/ImageTooDark event.
func (b *Broker) ImageTooDark(sourceToken string, state bool) {
	b.Publish(
		"tns1:VideoSource/ImageTooDark",
		boolPropertyMessage(sourceToken, "VideoSourceConfigurationToken", "State", state, PropertyChanged),
	)
}

// ImageTooBright publishes a tns1:VideoSource/ImageTooBright event.
func (b *Broker) ImageTooBright(sourceToken string, state bool) {
	b.Publish(
		"tns1:VideoSource/ImageTooBright",
		boolPropertyMessage(sourceToken, "VideoSourceConfigurationToken", "State", state, PropertyChanged),
	)
}

// DigitalInput publishes a tns1:Device/Trigger/DigitalInput event.
//
// inputToken identifies the digital input port (e.g. "DI_0").
// logicalState is true when the input is active.
func (b *Broker) DigitalInput(inputToken string, logicalState bool) {
	b.Publish(
		"tns1:Device/Trigger/DigitalInput",
		boolPropertyMessage(inputToken, "InputToken", "LogicalState", logicalState, PropertyChanged),
	)
}

// SyncProperty re-publishes the current value of an arbitrary topic with
// PropertyType="Initialized". This is called by the SubscriptionManager when
// SetSynchronizationPoint is received, but can also be called from a GUI/TUI
// to push the current state to newly connected clients.
//
// topic must be one of the tns1: topic strings the broker advertises.
// sourceToken and state are the current property values.
func (b *Broker) SyncProperty(topic, sourceItemName, sourceToken, dataItemName string, state bool) {
	b.Publish(topic, boolPropertyMessage(sourceToken, sourceItemName, dataItemName, state, PropertyInitialized))
}

// ---------- internal helpers ----------------------------------------------------

// boolPropertyMessage builds the tt:Message XML fragment for a single
// boolean property event. The structure follows ONVIF Core §9.6 and the
// Profile S topic definitions.
//
//	<tt:Message xmlns:tt="http://www.onvif.org/ver10/schema"
//	            UtcTime="..." PropertyType="Changed">
//	  <tt:Source>
//	    <tt:SimpleItem Name="VideoSourceConfigurationToken" Value="vs0"/>
//	  </tt:Source>
//	  <tt:Data>
//	    <tt:SimpleItem Name="State" Value="true"/>
//	  </tt:Data>
//	</tt:Message>
func boolPropertyMessage(
	sourceValue, sourceItemName, dataItemName string,
	state bool,
	pt PropertyType,
) string {
	return fmt.Sprintf(
		`<tt:Message xmlns:tt="http://www.onvif.org/ver10/schema" UtcTime="%s" PropertyType="%s">`+
			`<tt:Source><tt:SimpleItem Name="%s" Value="%s"/></tt:Source>`+
			`<tt:Data><tt:SimpleItem Name="%s" Value="%s"/></tt:Data>`+
			`</tt:Message>`,
		time.Now().UTC().Format(time.RFC3339),
		string(pt),
		html.EscapeString(sourceItemName), html.EscapeString(sourceValue),
		html.EscapeString(dataItemName), formatBool(state),
	)
}

func formatBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
