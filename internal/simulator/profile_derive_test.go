package simulator

import (
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

func TestIsMJPEGSiblingToken(t *testing.T) {
	t.Parallel()
	cases := []struct {
		token string
		want  bool
	}{
		{"Main", false},
		{"Main_JPEG", true},
		{"profile_1_JPEG", true},
		{"", false},
		{"_JPEG", true},
		{"JPEG", false},
	}
	for _, tc := range cases {
		if got := IsMJPEGSiblingToken(tc.token); got != tc.want {
			t.Errorf("IsMJPEGSiblingToken(%q) = %v, want %v", tc.token, got, tc.want)
		}
	}
}

func TestMJPEGSiblingParentToken(t *testing.T) {
	t.Parallel()
	cases := []struct {
		token string
		want  string
	}{
		{"Main_JPEG", "Main"},
		{"profile_1_JPEG", "profile_1"},
		{"Main", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := MJPEGSiblingParentToken(tc.token); got != tc.want {
			t.Errorf("MJPEGSiblingParentToken(%q) = %q, want %q", tc.token, got, tc.want)
		}
	}
}

func TestDeriveMJPEGSiblings_HasSourceFiltering(t *testing.T) {
	t.Parallel()
	in := []config.ProfileConfig{
		{Name: "WithFile", Token: "wf", Kind: config.ProfileKindFile, MediaFilePath: "/tmp/a.mp4"},
		{Name: "Empty", Token: "empty", Kind: config.ProfileKindFile, MediaFilePath: ""}, // no source
		{Name: "RPI", Token: "rpi", Kind: config.ProfileKindRPICam, RPICam: &config.RPICamConfig{Width: 1920}},
	}
	got := deriveMJPEGSiblings(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 siblings (WithFile, RPI), got %d: %+v", len(got), got)
	}
	wantTokens := map[string]bool{"wf_JPEG": true, "rpi_JPEG": true}
	for i := range got {
		if !wantTokens[got[i].Token] {
			t.Errorf("unexpected sibling token %q", got[i].Token)
		}
	}
}

func TestDeriveMJPEGSiblings_PreservesDimensions(t *testing.T) {
	t.Parallel()
	in := []config.ProfileConfig{{
		Name: "Main", Token: "Main",
		Kind: config.ProfileKindFile, MediaFilePath: "/tmp/x.mp4",
		Width: 1280, Height: 720, FPS: 30, Encoding: "H264",
	}}
	got := deriveMJPEGSiblings(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 sibling, got %d", len(got))
	}
	sib := got[0]
	if sib.Encoding != "MJPEG" {
		t.Errorf("sibling encoding = %q, want MJPEG", sib.Encoding)
	}
	if sib.Width != 1280 || sib.Height != 720 || sib.FPS != 30 {
		t.Errorf("sibling dimensions lost: %+v", sib)
	}
	if sib.MediaFilePath != "/tmp/x.mp4" {
		t.Errorf("sibling media path lost: %q", sib.MediaFilePath)
	}
	if sib.Name != "Main (MJPEG)" {
		t.Errorf("sibling name = %q, want \"Main (MJPEG)\"", sib.Name)
	}
}

func TestDeriveMJPEGSiblings_SkipsAlreadyDerived(t *testing.T) {
	t.Parallel()
	in := []config.ProfileConfig{
		{Name: "Main", Token: "Main", Kind: config.ProfileKindFile, MediaFilePath: "/tmp/x.mp4"},
		{Name: "Main (MJPEG)", Token: "Main_JPEG", Kind: config.ProfileKindFile, MediaFilePath: "/tmp/x.mp4"},
	}
	got := deriveMJPEGSiblings(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 sibling (only Main, not Main_JPEG), got %d: %+v", len(got), got)
	}
	if got[0].Token != "Main_JPEG" {
		t.Errorf("expected token Main_JPEG, got %q", got[0].Token)
	}
}

func TestDeriveMJPEGSiblings_EmptyInput(t *testing.T) {
	t.Parallel()
	got := deriveMJPEGSiblings(nil)
	if len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

func TestDeriveMJPEGSiblings_RPICamSourceCopied(t *testing.T) {
	t.Parallel()
	in := []config.ProfileConfig{{
		Name: "RPI", Token: "rpi", Kind: config.ProfileKindRPICam,
		RPICam: &config.RPICamConfig{CameraID: 0, Width: 1920, Height: 1080, FPS: 30},
		Width:  1920, Height: 1080, FPS: 30,
	}}
	got := deriveMJPEGSiblings(in)
	if got[0].RPICam == nil {
		t.Fatalf("rpicam sibling lost RPICam reference")
	}
	if got[0].RPICam.Width != 1920 {
		t.Errorf("rpicam sibling RPICam.Width = %d, want 1920", got[0].RPICam.Width)
	}
}
