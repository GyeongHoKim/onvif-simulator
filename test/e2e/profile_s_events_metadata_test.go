//go:build e2e

package e2e

// Profile S e2e coverage for sections that profile_s_test.go intentionally
// left out: §7.7 Event Handling (pull-point + WS-BaseNotification push),
// §7.9 MJPEG advertising (JPEG encoder-instance count), and §7.13 Metadata
// Configuration.
//
// References:
//   - ONVIF Profile S Specification v1.3 — §7.7, §7.9, §7.13.
//   - ONVIF Core Specification — §9 Notification Framework.
//   - WS-BaseNotification 1.3 — Subscribe / SubscriptionManager.

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	onviflib "github.com/use-go/onvif"
	"github.com/use-go/onvif/event"
	"github.com/use-go/onvif/media"
	sdkmedia "github.com/use-go/onvif/sdk/media"
	"github.com/use-go/onvif/xsd"
	onviftypes "github.com/use-go/onvif/xsd/onvif"
)

// --- Profile S v1.3 §7.7 Event Handling ----------------------------------------

func TestONVIF_ProfileS_7_7_Events(t *testing.T) {
	dev := newDevice(t)

	t.Run("GetEventProperties", func(t *testing.T) {
		body := callMethod(t, dev, event.GetEventProperties{})
		if !strings.Contains(body, "GetEventPropertiesResponse") {
			t.Fatalf("missing GetEventPropertiesResponse: %s", body)
		}
		if !strings.Contains(body, "TopicSet") {
			t.Fatalf("GetEventProperties response missing TopicSet: %s", body)
		}
	})

	t.Run("CreatePullPointSubscription", func(t *testing.T) {
		// Empty InitialTerminationTime is valid: the simulator falls back to
		// its configured subscription timeout (see internal/event/broker.go).
		// We avoid the SDK's AbsoluteOrRelativeTimeType because its two
		// anonymously-embedded string aliases produce unstable XML output.
		body := callMethod(t, dev, event.CreatePullPointSubscription{})
		addr := extractSubscriptionReferenceAddress(t, body)
		if addr == "" {
			t.Fatalf("CreatePullPointSubscription: empty SubscriptionReference Address: %s", body)
		}
	})

	t.Run("Subscribe_push", func(t *testing.T) {
		// WS-BaseNotification Subscribe (Profile S §7.7 push). ConsumerReference
		// points at a known-unreachable URL — we are only validating the
		// simulator accepts the subscription and returns SubscriptionReference;
		// no notification delivery is required for this case.
		body := callMethod(t, dev, event.Subscribe{
			ConsumerReference: event.EndpointReferenceType{
				Address: event.AttributedURIType("http://127.0.0.1:9/onvif-simulator-e2e-consumer"),
			},
			InitialTerminationTime: event.TerminationTime("PT5M"),
		})
		addr := extractSubscriptionReferenceAddress(t, body)
		if addr == "" {
			t.Fatalf("Subscribe: empty SubscriptionReference Address: %s", body)
		}
	})
}

// --- Profile S v1.3 §7.9 MJPEG advertising -------------------------------------

func TestONVIF_ProfileS_7_9_MJPEG_advertised(t *testing.T) {
	// Profile S §7.9.1 — device shall declare MJPEG option (JPEG resolutions +
	// encoder instance count). The simulator advertises this even on default
	// (file-backed) deployments per `internal/onvif/mediasvc/jpeg_options*`.
	dev := newDevice(t)
	c := ctx(t)

	prof := mediaFirstProfile(t, c, dev)

	t.Run("GetGuaranteedNumberOfVideoEncoderInstances_includes_JPEG", func(t *testing.T) {
		resp, err := sdkmedia.Call_GetGuaranteedNumberOfVideoEncoderInstances(c, dev,
			media.GetGuaranteedNumberOfVideoEncoderInstances{
				ConfigurationToken: prof.VideoSourceConfiguration.Token,
			})
		if err != nil {
			t.Fatalf("GetGuaranteedNumberOfVideoEncoderInstances: %v", err)
		}
		if resp.JPEG <= 0 {
			t.Fatalf("JPEG instance count must be > 0 for Profile S §7.9 compliance, got %d", resp.JPEG)
		}
	})

	t.Run("GetVideoEncoderConfigurationOptions_advertises_JPEG", func(t *testing.T) {
		resp, err := sdkmedia.Call_GetVideoEncoderConfigurationOptions(c, dev,
			media.GetVideoEncoderConfigurationOptions{
				ProfileToken:       prof.Token,
				ConfigurationToken: prof.VideoEncoderConfiguration.Token,
			})
		if err != nil {
			t.Fatalf("GetVideoEncoderConfigurationOptions: %v", err)
		}
		// use-go/onvif models ResolutionsAvailable as a single struct, so the
		// last <ResolutionsAvailable> entry in the response lands here; any
		// positive Width is sufficient to prove the JPEG branch was advertised.
		if resp.Options.JPEG.ResolutionsAvailable.Width <= 0 ||
			resp.Options.JPEG.ResolutionsAvailable.Height <= 0 {
			t.Fatalf("JPEG.ResolutionsAvailable must be populated for Profile S §7.9: %#v",
				resp.Options.JPEG)
		}
	})
}

// --- Profile S v1.3 §7.13 Metadata Configuration --------------------------------

func TestONVIF_ProfileS_7_13_MetadataConfiguration(t *testing.T) {
	// Profile S §7.13.3 — Metadata Configuration Function List for Devices.
	// The simulator may or may not have a metadata configuration on disk;
	// when none exist, the read operations still succeed with an empty list
	// (which is acceptable per the spec) but the configuration-targeted
	// operations are skipped.
	dev := newDevice(t)
	c := ctx(t)

	listResp, err := sdkmedia.Call_GetMetadataConfigurations(c, dev, media.GetMetadataConfigurations{})
	if err != nil {
		t.Fatalf("GetMetadataConfigurations: %v", err)
	}

	prof := mediaFirstProfile(t, c, dev)

	if len(listResp.Configurations.Token) == 0 {
		// No metadata configurations registered — sanity check the
		// configuration-listing path returned successfully, then skip the
		// targeted operations.
		t.Log("no metadata configurations registered; only options/compatible-list paths covered")
	} else {
		t.Run("GetMetadataConfiguration", func(t *testing.T) {
			single, err := sdkmedia.Call_GetMetadataConfiguration(c, dev,
				media.GetMetadataConfiguration{ConfigurationToken: listResp.Configurations.Token})
			if err != nil {
				t.Fatalf("GetMetadataConfiguration: %v", err)
			}
			if single.Configuration.Token != listResp.Configurations.Token {
				t.Fatalf("GetMetadataConfiguration token mismatch: want %q, got %q",
					listResp.Configurations.Token, single.Configuration.Token)
			}
		})

		t.Run("SetMetadataConfiguration_idempotent", func(t *testing.T) {
			// Re-set the existing configuration unchanged; the simulator
			// persists this through config.UpsertMetadataConfig, so the
			// round-trip must succeed without altering identity.
			if _, err := sdkmedia.Call_SetMetadataConfiguration(c, dev, media.SetMetadataConfiguration{
				Configuration:    listResp.Configurations,
				ForcePersistence: xsd.Boolean(true),
			}); err != nil {
				t.Fatalf("SetMetadataConfiguration: %v", err)
			}
		})
	}

	t.Run("GetCompatibleMetadataConfigurations", func(t *testing.T) {
		if _, err := sdkmedia.Call_GetCompatibleMetadataConfigurations(c, dev,
			media.GetCompatibleMetadataConfigurations{ProfileToken: prof.Token}); err != nil {
			t.Fatalf("GetCompatibleMetadataConfigurations: %v", err)
		}
	})

	t.Run("GetMetadataConfigurationOptions", func(t *testing.T) {
		// ConfigurationToken is optional in the spec; omit it when no
		// configuration exists to exercise the unbound options query.
		req := media.GetMetadataConfigurationOptions{ProfileToken: prof.Token}
		if len(listResp.Configurations.Token) > 0 {
			req.ConfigurationToken = listResp.Configurations.Token
		}
		if _, err := sdkmedia.Call_GetMetadataConfigurationOptions(c, dev, req); err != nil {
			t.Fatalf("GetMetadataConfigurationOptions: %v", err)
		}
	})

	t.Run("AddMetadataConfiguration_RemoveMetadataConfiguration", func(t *testing.T) {
		token := onviftypes.ReferenceToken(fmt.Sprintf("e2e_meta_%d", time.Now().UnixNano()))
		// Add does not require the token to pre-exist on the simulator (the
		// provider stub returns nil); both ops are exercised here for their
		// SOAP-wire contract.
		if _, err := sdkmedia.Call_AddMetadataConfiguration(c, dev, media.AddMetadataConfiguration{
			ProfileToken:       prof.Token,
			ConfigurationToken: token,
		}); err != nil {
			t.Fatalf("AddMetadataConfiguration: %v", err)
		}
		if _, err := sdkmedia.Call_RemoveMetadataConfiguration(c, dev, media.RemoveMetadataConfiguration{
			ProfileToken: prof.Token,
		}); err != nil {
			t.Fatalf("RemoveMetadataConfiguration: %v", err)
		}
	})
}

// --- helpers -------------------------------------------------------------------

// callMethod issues a SOAP request through use-go/onvif's dev.CallMethod and
// returns the response body as a string. The Device struct already routes the
// request to the right endpoint based on the request struct's package, and
// attaches WS-UsernameToken when the test credentials are set.
func callMethod(t *testing.T, dev *onviflib.Device, method any) string {
	t.Helper()
	resp, err := dev.CallMethod(method)
	if err != nil {
		t.Fatalf("CallMethod(%T): %v", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CallMethod(%T) status %d: %s", method, resp.StatusCode, body)
	}
	return string(body)
}

// extractSubscriptionReferenceAddress pulls the wsa:Address value out of a
// CreatePullPointSubscriptionResponse or SubscribeResponse SOAP envelope. It
// walks the XML tree by local name so it tolerates any namespace prefix the
// simulator chooses.
func extractSubscriptionReferenceAddress(t *testing.T, body string) string {
	t.Helper()
	decoder := xml.NewDecoder(strings.NewReader(body))
	var inSubRef, inAddr bool
	var addr strings.Builder
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("parse subscribe response: %v", err)
		}
		switch tt := tok.(type) {
		case xml.StartElement:
			switch tt.Name.Local {
			case "SubscriptionReference":
				inSubRef = true
			case "Address":
				if inSubRef {
					inAddr = true
				}
			}
		case xml.CharData:
			if inSubRef && inAddr {
				addr.Write(tt)
			}
		case xml.EndElement:
			switch tt.Name.Local {
			case "Address":
				inAddr = false
			case "SubscriptionReference":
				inSubRef = false
			}
		}
		if inSubRef && !inAddr && addr.Len() > 0 {
			return strings.TrimSpace(addr.String())
		}
	}
	return strings.TrimSpace(addr.String())
}
