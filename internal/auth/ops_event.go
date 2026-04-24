package auth

// EventOperationClasses maps ONVIF Event Service and WS-BaseNotification
// SubscriptionManager operation names to their access class per
// ONVIF Core §5.9.4.3.
//
// Operations from both handlers share one map because the same
// auth.MapOperationClass call wires both handlers.
var EventOperationClasses = map[string]AccessClass{
	"GetServiceCapabilities": ClassPreAuth,

	// GetEventProperties reads the static topic set description.
	"GetEventProperties": ClassReadSystem,

	// The remaining operations create or manage runtime subscriptions.
	"CreatePullPointSubscription": ClassActuate,
	"PullMessages":                ClassActuate,
	"SetSynchronizationPoint":     ClassActuate,
	"Renew":                       ClassActuate,
	"Unsubscribe":                 ClassActuate,
}

// EventOperationClass returns the AccessClass for the named Event Service
// or SubscriptionManager operation. Unknown operations fall back to
// ClassWriteSystem (safer default).
func EventOperationClass(op string) AccessClass {
	if c, ok := EventOperationClasses[op]; ok {
		return c
	}
	return ClassWriteSystem
}
