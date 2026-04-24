package event

import (
	"context"
	"strings"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/eventsvc"
)

func publishAndPull(t *testing.T, b *Broker, publish func()) eventsvc.NotificationMessage {
	t.Helper()
	id := mustCreateSub(t, b)
	publish()
	result, err := b.PullMessages(context.Background(), id, eventsvc.PullMessagesParams{})
	if err != nil {
		t.Fatalf("PullMessages: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	return result.Messages[0]
}

func TestMotionAlarm_True(t *testing.T) {
	b := New(defaultCfg())
	msg := publishAndPull(t, b, func() { b.MotionAlarm("vs0", true) })

	if msg.Topic != "tns1:VideoSource/MotionAlarm" {
		t.Errorf("topic = %q", msg.Topic)
	}
	if !strings.Contains(msg.Message, `Value="vs0"`) {
		t.Errorf("source token missing: %s", msg.Message)
	}
	if !strings.Contains(msg.Message, `Value="true"`) {
		t.Errorf("state true missing: %s", msg.Message)
	}
	if !strings.Contains(msg.Message, `PropertyType="Changed"`) {
		t.Errorf("PropertyType missing: %s", msg.Message)
	}
}

func TestMotionAlarm_False(t *testing.T) {
	b := New(defaultCfg())
	msg := publishAndPull(t, b, func() { b.MotionAlarm("vs0", false) })
	if !strings.Contains(msg.Message, `Value="false"`) {
		t.Errorf("state false missing: %s", msg.Message)
	}
}

// defaultCfgWithTopic returns a BrokerConfig identical to defaultCfg but with
// the given topic explicitly enabled in addition to the defaults.
func defaultCfgWithTopic(topic string) BrokerConfig {
	cfg := defaultCfg()
	for i := range cfg.Topics {
		if cfg.Topics[i].Name == topic {
			cfg.Topics[i].Enabled = true
			return cfg
		}
	}
	cfg.Topics = append(cfg.Topics, TopicConfig{Name: topic, Enabled: true})
	return cfg
}

func TestImageTooBlurry(t *testing.T) {
	b := New(defaultCfgWithTopic("tns1:VideoSource/ImageTooBlurry"))
	msg := publishAndPull(t, b, func() { b.ImageTooBlurry("vs0", true) })
	if msg.Topic != "tns1:VideoSource/ImageTooBlurry" {
		t.Errorf("topic = %q", msg.Topic)
	}
}

func TestImageTooDark(t *testing.T) {
	b := New(defaultCfgWithTopic("tns1:VideoSource/ImageTooDark"))
	msg := publishAndPull(t, b, func() { b.ImageTooDark("vs0", true) })
	if msg.Topic != "tns1:VideoSource/ImageTooDark" {
		t.Errorf("topic = %q", msg.Topic)
	}
}

func TestImageTooBright(t *testing.T) {
	b := New(defaultCfgWithTopic("tns1:VideoSource/ImageTooBright"))
	msg := publishAndPull(t, b, func() { b.ImageTooBright("vs0", true) })
	if msg.Topic != "tns1:VideoSource/ImageTooBright" {
		t.Errorf("topic = %q", msg.Topic)
	}
}

func TestDigitalInput(t *testing.T) {
	b := New(defaultCfgWithTopic("tns1:Device/Trigger/DigitalInput"))
	msg := publishAndPull(t, b, func() { b.DigitalInput("DI_0", true) })

	if msg.Topic != "tns1:Device/Trigger/DigitalInput" {
		t.Errorf("topic = %q", msg.Topic)
	}
	if !strings.Contains(msg.Message, `Name="InputToken"`) {
		t.Errorf("InputToken source missing: %s", msg.Message)
	}
	if !strings.Contains(msg.Message, `Name="LogicalState"`) {
		t.Errorf("LogicalState data missing: %s", msg.Message)
	}
}

func TestSyncProperty(t *testing.T) {
	b := New(defaultCfg())
	msg := publishAndPull(t, b, func() {
		b.SyncProperty(
			"tns1:VideoSource/MotionAlarm",
			"VideoSourceConfigurationToken", "vs0",
			"State", false,
		)
	})

	if !strings.Contains(msg.Message, `PropertyType="Initialized"`) {
		t.Errorf("PropertyType Initialized missing: %s", msg.Message)
	}
	if !strings.Contains(msg.Message, `Value="false"`) {
		t.Errorf("state false missing: %s", msg.Message)
	}
}

func TestBoolPropertyMessage_XMLStructure(t *testing.T) {
	xml := boolPropertyMessage("vs0", "VideoSourceConfigurationToken", "State", true, PropertyChanged)

	checks := []string{
		`xmlns:tt="http://www.onvif.org/ver10/schema"`,
		`UtcTime=`,
		`<tt:Source>`,
		`<tt:Data>`,
		`<tt:SimpleItem`,
		`</tt:Message>`,
	}
	for _, c := range checks {
		if !strings.Contains(xml, c) {
			t.Errorf("missing %q in: %s", c, xml)
		}
	}
}
