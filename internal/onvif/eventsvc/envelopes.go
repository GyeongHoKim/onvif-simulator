package eventsvc

import (
	"encoding/xml"
	"time"
)

// formatXSDDateTime formats t as an xs:dateTime string in UTC.
func formatXSDDateTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// ---------- shared SOAP structs --------------------------------------------------

type soapEnvelope struct {
	XMLName  xml.Name `xml:"env:Envelope"`
	XMLNSEnv string   `xml:"xmlns:env,attr"`
	Body     soapBody `xml:"env:Body"`
}

type soapBody struct {
	InnerXML string `xml:",innerxml"`
}

// ---------- WS-Addressing --------------------------------------------------------

// endpointReferenceEnvelope is a WS-Addressing EndpointReference (EPR).
type endpointReferenceEnvelope struct {
	Address string `xml:"wsa:Address"`
}

// ---------- EventService responses -----------------------------------------------

type getServiceCapabilitiesResponse struct {
	XMLName      xml.Name                         `xml:"GetServiceCapabilitiesResponse"`
	XMLNS        string                           `xml:"xmlns,attr"`
	Capabilities eventServiceCapabilitiesEnvelope `xml:"Capabilities"`
}

type eventServiceCapabilitiesEnvelope struct {
	WSSubscriptionPolicySupport                   bool `xml:"WSSubscriptionPolicySupport,attr,omitempty"`
	WSPullPointSupport                            bool `xml:"WSPullPointSupport,attr,omitempty"`
	WSPausableSubscriptionManagerInterfaceSupport bool `xml:"WSPausableSubscriptionManagerInterfaceSupport,attr,omitempty"`
	MaxNotificationProducers                      int  `xml:"MaxNotificationProducers,attr,omitempty"`
	MaxPullPoints                                 int  `xml:"MaxPullPoints,attr,omitempty"`
	PersistentNotificationStorage                 bool `xml:"PersistentNotificationStorage,attr,omitempty"`
}

// createPullPointSubscriptionResponse is the CreatePullPointSubscription
// response. The SubscriptionReference carries the WS-Addressing EPR that the
// client uses to call PullMessages, Renew, SetSynchronizationPoint, and
// Unsubscribe.
type createPullPointSubscriptionResponse struct {
	XMLName               xml.Name                  `xml:"CreatePullPointSubscriptionResponse"`
	XMLNS                 string                    `xml:"xmlns,attr"`
	XMLNSWsa              string                    `xml:"xmlns:wsa,attr"`
	SubscriptionReference endpointReferenceEnvelope `xml:"SubscriptionReference"`
	CurrentTime           string                    `xml:"CurrentTime"`
	TerminationTime       string                    `xml:"TerminationTime"`
}

type getEventPropertiesResponse struct {
	XMLName                xml.Name         `xml:"GetEventPropertiesResponse"`
	XMLNS                  string           `xml:"xmlns,attr"`
	TopicNamespaceLocation []string         `xml:"TopicNamespaceLocation,omitempty"`
	FixedTopicSet          bool             `xml:"FixedTopicSet"`
	TopicSet               topicSetEnvelope `xml:"TopicSet"`
}

// topicSetEnvelope wraps the raw topic XML fragment so the caller can inject
// a tns1: subtree verbatim.
type topicSetEnvelope struct {
	InnerXML string `xml:",innerxml"`
}

// ---------- SubscriptionManager responses ----------------------------------------

type pullMessagesResponse struct {
	XMLName             xml.Name                      `xml:"PullMessagesResponse"`
	XMLNS               string                        `xml:"xmlns,attr"`
	XMLNSWsa            string                        `xml:"xmlns:wsa,attr"`
	XMLNSTt             string                        `xml:"xmlns:tt,attr"`
	CurrentTime         string                        `xml:"CurrentTime"`
	TerminationTime     string                        `xml:"TerminationTime"`
	NotificationMessage []notificationMessageEnvelope `xml:"NotificationMessage,omitempty"`
}

type notificationMessageEnvelope struct {
	SubscriptionReference endpointReferenceEnvelope  `xml:"SubscriptionReference"`
	Topic                 topicExpressionEnvelope    `xml:"Topic"`
	ProducerReference     *endpointReferenceEnvelope `xml:"ProducerReference,omitempty"`
	Message               notificationMessageBody    `xml:"Message"`
}

type topicExpressionEnvelope struct {
	Dialect string `xml:"Dialect,attr"`
	Value   string `xml:",chardata"`
}

// notificationMessageBody wraps a raw tt:Message XML fragment.
type notificationMessageBody struct {
	InnerXML string `xml:",innerxml"`
}

type setSynchronizationPointResponse struct {
	XMLName xml.Name `xml:"SetSynchronizationPointResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}

// renewResponse uses WSNBaseNotificationNS as its default namespace because
// Renew is a WS-BaseNotification operation.
type renewResponse struct {
	XMLName         xml.Name `xml:"RenewResponse"`
	XMLNS           string   `xml:"xmlns,attr"`
	TerminationTime string   `xml:"TerminationTime"`
	CurrentTime     string   `xml:"CurrentTime,omitempty"`
}

// unsubscribeResponse uses WSNBaseNotificationNS as its default namespace.
type unsubscribeResponse struct {
	XMLName xml.Name `xml:"UnsubscribeResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
}
