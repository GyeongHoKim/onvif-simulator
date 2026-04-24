package event

import (
	"testing"
	"time"
)

func TestParseISO8601Duration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"PT1H", time.Hour, true},
		{"PT30M", 30 * time.Minute, true},
		{"PT60S", 60 * time.Second, true},
		{"PT1H30M", 90 * time.Minute, true},
		{"PT1H30M15S", time.Hour + 30*time.Minute + 15*time.Second, true},
		{"1h", time.Hour, true},
		{"30m", 30 * time.Minute, true},
		{"", 0, false},
		{"PD1H", 0, false},
		{"PT", 0, false},
		{"PT1X", 0, false},
	}
	for _, tc := range cases {
		d, err := parseISO8601Duration(tc.in)
		if tc.ok {
			if err != nil {
				t.Errorf("parseISO8601Duration(%q): unexpected error: %v", tc.in, err)
			} else if d != tc.want {
				t.Errorf("parseISO8601Duration(%q) = %v, want %v", tc.in, d, tc.want)
			}
		} else {
			if err == nil {
				t.Errorf("parseISO8601Duration(%q): expected error, got %v", tc.in, d)
			}
		}
	}
}

func TestNewSubscriptionID_Unique(t *testing.T) {
	seen := make(map[string]bool, 100)
	for range 100 {
		id := newSubscriptionID()
		if seen[id] {
			t.Fatalf("duplicate subscription ID: %s", id)
		}
		seen[id] = true
	}
}
