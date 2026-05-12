package auth

import "testing"

func TestEventOperationClass_SubscribeIsActuate(t *testing.T) {
	if got := EventOperationClass("Subscribe"); got != ClassActuate {
		t.Errorf("EventOperationClass(\"Subscribe\") = %v, want ClassActuate", got)
	}
}

func TestEventOperationClass_UnknownDefaultsToWriteSystem(t *testing.T) {
	if got := EventOperationClass("MysteryOp"); got != ClassWriteSystem {
		t.Errorf("EventOperationClass(\"MysteryOp\") = %v, want ClassWriteSystem", got)
	}
}
