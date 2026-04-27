package rtsp

import (
	"testing"
)

func TestScale90k(t *testing.T) {
	cases := []struct {
		name      string
		dur       uint32
		timescale uint32
		want      uint32
	}{
		{"identity 90k", 90000, 90000, 90000},
		{"half-second @12800", 6400, 12800, 45000},
		{"zero timescale", 100, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := scale90k(c.dur, c.timescale); got != c.want {
				t.Errorf("scale90k(%d, %d) = %d, want %d", c.dur, c.timescale, got, c.want)
			}
		})
	}
}

func TestSplitAVCCEmpty(t *testing.T) {
	// Empty payload returns empty slice, not an error — looper treats it as
	// a sample to skip.
	nalus, err := splitAVCC(nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(nalus) != 0 {
		t.Errorf("expected 0 nalus, got %d", len(nalus))
	}
}

func TestSplitAVCCRoundTrip(t *testing.T) {
	// Two NAL units of length 3 and 5 in AVCC framing.
	buf := []byte{
		0x00, 0x00, 0x00, 0x03, 0x65, 0x88, 0x84,
		0x00, 0x00, 0x00, 0x05, 0x68, 0xee, 0x3c, 0x80, 0x00,
	}
	nalus, err := splitAVCC(buf)
	if err != nil {
		t.Fatalf("splitAVCC: %v", err)
	}
	if len(nalus) != 2 {
		t.Fatalf("got %d nalus, want 2", len(nalus))
	}
	if len(nalus[0]) != 3 || len(nalus[1]) != 5 {
		t.Errorf("nalu sizes = [%d, %d], want [3, 5]", len(nalus[0]), len(nalus[1]))
	}
}

func TestSplitAVCCGarbage(t *testing.T) {
	_, err := splitAVCC([]byte{0xff, 0xff, 0xff, 0xff, 0x01})
	if err == nil {
		t.Fatal("expected parse error for invalid avcc payload")
	}
}

func TestRandomUint32(t *testing.T) {
	a, err := randomUint32()
	if err != nil {
		t.Fatalf("randomUint32: %v", err)
	}
	b, err := randomUint32()
	if err != nil {
		t.Fatalf("randomUint32 (2nd): %v", err)
	}
	// Two consecutive samples should virtually always differ; if they
	// match we are likely seeing a broken RNG.
	if a == b {
		t.Errorf("expected randomness; both calls returned %#x", a)
	}
}

func TestPathToken(t *testing.T) {
	cases := []struct{ in, want string }{
		{"main", "main"},
		{"/main", "main"},
		{"//main", "main"},
		{"/main/trackID=0", "main"},
		{"", ""},
	}
	for _, c := range cases {
		if got := pathToken(c.in); got != c.want {
			t.Errorf("pathToken(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
