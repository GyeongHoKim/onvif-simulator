package event

import "testing"

func TestSubscription_ZeroValueIsPullKind(t *testing.T) {
	var s subscription
	if s.kind != subKindPull {
		t.Errorf("zero-value subscription kind = %v, want subKindPull", s.kind)
	}
	if s.notifyFailures != 0 {
		t.Errorf("zero-value notifyFailures = %d, want 0", s.notifyFailures)
	}
	if s.consumer.address != "" || s.consumer.referenceParams != "" {
		t.Errorf("zero-value consumer = %+v, want empty", s.consumer)
	}
}

func TestSubscription_PushKindStoresConsumerEPR(t *testing.T) {
	s := subscription{
		kind: subKindPush,
		consumer: consumerEPR{
			address:         "http://consumer/sink",
			referenceParams: "<wsa:MyId>abc</wsa:MyId>",
		},
	}
	if s.kind != subKindPush {
		t.Errorf("kind = %v, want subKindPush", s.kind)
	}
	if s.consumer.address != "http://consumer/sink" {
		t.Errorf("consumer.address = %q", s.consumer.address)
	}
	if s.consumer.referenceParams != "<wsa:MyId>abc</wsa:MyId>" {
		t.Errorf("consumer.referenceParams = %q", s.consumer.referenceParams)
	}
}
