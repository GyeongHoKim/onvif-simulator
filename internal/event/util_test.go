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
		// past RFC3339 absolute timestamps must be rejected
		{"2000-01-01T00:00:00Z", 0, false},
		// non-RFC3339 garbage
		{"not-a-duration", 0, false},
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

	// Future RFC3339 absolute timestamp: duration must be positive and close to 1 hour.
	future := time.Now().Add(time.Hour).Format(time.RFC3339)
	d, err := parseISO8601Duration(future)
	if err != nil {
		t.Errorf("parseISO8601Duration(future RFC3339): unexpected error: %v", err)
	} else if d <= 0 || d > time.Hour+2*time.Second {
		t.Errorf("parseISO8601Duration(future RFC3339) = %v, want (0, ~1h]", d)
	}
}

func TestNewSubscriptionID_Unique(t *testing.T) {
	seen := make(map[string]bool, 100)
	for range 100 {
		id, err := newSubscriptionID()
		if err != nil {
			t.Fatalf("newSubscriptionID: unexpected error: %v", err)
		}
		if seen[id] {
			t.Fatalf("duplicate subscription ID: %s", id)
		}
		seen[id] = true
	}
}
