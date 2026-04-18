//go:build e2e

package e2e

import (
	"testing"

	"github.com/use-go/onvif/media"
	sdkmedia "github.com/use-go/onvif/sdk/media"
	onviftypes "github.com/use-go/onvif/xsd/onvif"
)

// TestMediaProfileConfiguration verifies that the simulator exposes at least
// one media profile with video source and video encoder configurations.
//
// Derived from ONVIF Media Configuration Device Test Specification,
// test case MEDIA-1-1-1 "Media Profile Configuration".
//
// PASS criteria:
//   - GetProfilesResponse is received
//   - At least one profile is returned
//   - Each profile has a non-empty Token
//   - Each profile contains VideoSourceConfiguration
//   - Each profile contains VideoEncoderConfiguration
func TestMediaProfileConfiguration(t *testing.T) {
	dev := newDevice(t)

	resp, err := sdkmedia.Call_GetProfiles(ctx(t), dev, media.GetProfiles{})
	if err != nil {
		t.Fatalf("GetProfiles failed: %v", err)
	}

	if len(resp.Profiles) == 0 {
		t.Fatal("MEDIA-1-1-1: GetProfilesResponse must contain at least one profile")
	}

	for _, p := range resp.Profiles {
		if string(p.Token) == "" {
			t.Errorf("MEDIA-1-1-1: profile Token must not be empty")
		}

		if string(p.VideoSourceConfiguration.Token) == "" {
			t.Errorf("MEDIA-1-1-1: profile %q missing VideoSourceConfiguration", p.Token)
		}

		if string(p.VideoEncoderConfiguration.Token) == "" {
			t.Errorf("MEDIA-1-1-1: profile %q missing VideoEncoderConfiguration", p.Token)
		}
	}
}

// TestProfilesConsistency verifies that GetProfile and GetProfiles return
// identical information for each profile.
//
// Derived from ONVIF Media Configuration Device Test Specification,
// test case MEDIA-1-1-3 "Profiles Consistency".
//
// PASS criteria:
//   - Each profile token from GetProfiles is unique
//   - For each profile token, GetProfile returns the same Token and Name
func TestProfilesConsistency(t *testing.T) {
	dev := newDevice(t)

	listResp, err := sdkmedia.Call_GetProfiles(ctx(t), dev, media.GetProfiles{})
	if err != nil {
		t.Fatalf("MEDIA-1-1-3: GetProfiles failed: %v", err)
	}

	// verify tokens are unique
	seen := make(map[string]bool)
	for _, p := range listResp.Profiles {
		token := string(p.Token)
		if seen[token] {
			t.Errorf("MEDIA-1-1-3: duplicate profile token %q", token)
		}
		seen[token] = true
	}

	// for each profile, GetProfile must return the same data
	for _, listed := range listResp.Profiles {
		single, err := sdkmedia.Call_GetProfile(ctx(t), dev, media.GetProfile{
			ProfileToken: listed.Token,
		})
		if err != nil {
			t.Errorf("MEDIA-1-1-3: GetProfile(%q) failed: %v", listed.Token, err)
			continue
		}

		if single.Profile.Token != listed.Token {
			t.Errorf("MEDIA-1-1-3: GetProfile token mismatch: want %q, got %q",
				listed.Token, single.Profile.Token)
		}

		if single.Profile.Name != listed.Name {
			t.Errorf("MEDIA-1-1-3: GetProfile name mismatch for token %q: want %q, got %q",
				listed.Token, listed.Name, single.Profile.Name)
		}
	}
}

// TestGetVideoSources verifies that the simulator exposes at least one video
// source, which is required by Profile S.
//
// Derived from ONVIF Media Configuration Device Test Specification,
// "GetVideoSources" requirement.
//
// PASS criteria:
//   - GetVideoSourcesResponse is received
//   - At least one VideoSource is returned with a non-empty Token
func TestGetVideoSources(t *testing.T) {
	dev := newDevice(t)

	resp, err := sdkmedia.Call_GetVideoSources(ctx(t), dev, media.GetVideoSources{})
	if err != nil {
		t.Fatalf("GetVideoSources failed: %v", err)
	}

	// the library maps a single source to a struct, not a slice
	if string(resp.VideoSources.Token) == "" {
		t.Error("GetVideoSourcesResponse must contain at least one VideoSource with a non-empty Token")
	}
}

// TestGetStreamUri verifies that the simulator returns a valid RTSP stream URI
// for each media profile.
//
// Derived from ONVIF Media Configuration Device Test Specification,
// "Real-Time Streaming – GetStreamUri".
//
// PASS criteria:
//   - GetStreamUriResponse is received for each profile
//   - MediaUri.Uri is non-empty
func TestGetStreamUri(t *testing.T) {
	dev := newDevice(t)

	listResp, err := sdkmedia.Call_GetProfiles(ctx(t), dev, media.GetProfiles{})
	if err != nil {
		t.Fatalf("GetProfiles failed: %v", err)
	}

	if len(listResp.Profiles) == 0 {
		t.Fatal("no profiles returned; cannot test GetStreamUri")
	}

	for _, p := range listResp.Profiles {
		uriResp, err := sdkmedia.Call_GetStreamUri(ctx(t), dev, media.GetStreamUri{
			ProfileToken: p.Token,
			StreamSetup: onviftypes.StreamSetup{
				Stream: "RTP-Unicast",
				Transport: onviftypes.Transport{
					Protocol: "RTSP",
				},
			},
		})
		if err != nil {
			t.Errorf("GetStreamUri(%q) failed: %v", p.Token, err)
			continue
		}

		if string(uriResp.MediaUri.Uri) == "" {
			t.Errorf("GetStreamUri(%q): MediaUri.Uri must not be empty", p.Token)
		}
	}
}
