package config

import (
	"errors"
	"testing"
)

func TestValidateProfileEncoding_Accepts(t *testing.T) {
	t.Parallel()
	for _, encoding := range []string{"", "H264", "H265", "MJPEG"} {
		if err := validateProfileEncoding("p.encoding", encoding); err != nil {
			t.Errorf("expected %q to validate, got %v", encoding, err)
		}
	}
}

func TestValidateProfileEncoding_Rejects(t *testing.T) {
	t.Parallel()
	for _, encoding := range []string{"MPEG4", "VP9", "JPEG", "h264", "h265", "mjpeg", "junk"} {
		err := validateProfileEncoding("p.encoding", encoding)
		if err == nil {
			t.Errorf("expected %q to fail validation, got nil", encoding)
			continue
		}
		if !errors.Is(err, ErrProfileEncodingInvalid) {
			t.Errorf("expected %q to wrap ErrProfileEncodingInvalid, got %v", encoding, err)
		}
	}
}

func TestValidateProfile_IncludesEncodingCheck(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool)
	p := &ProfileConfig{
		Name:          "P",
		Token:         "tok",
		Kind:          ProfileKindFile,
		MediaFilePath: "/tmp/x.mp4",
		Encoding:      "MPEG4", // invalid
	}
	err := validateProfile(0, p, seen)
	if err == nil {
		t.Fatal("expected validation error for bad encoding")
	}
	if !errors.Is(err, ErrProfileEncodingInvalid) {
		t.Fatalf("expected ErrProfileEncodingInvalid, got %v", err)
	}
}
