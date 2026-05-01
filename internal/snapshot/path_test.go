package snapshot_test

import (
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/snapshot"
)

func TestPathFor(t *testing.T) {
	t.Parallel()

	if got, want := snapshot.PathFor("main"), "/onvif/snapshot/main.jpg"; got != want {
		t.Fatalf("PathFor(main) = %q, want %q", got, want)
	}
}

func TestTokenFromPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{"/onvif/snapshot/main.jpg", "main"},
		{"/onvif/snapshot/profile_0.jpg", "profile_0"},

		{"", ""},
		{"/", ""},
		{"/onvif/snapshot/", ""},
		{"/onvif/snapshot/main", ""},
		{"/onvif/snapshot/main.png", ""},
		{"/onvif/snapshot/.jpg", ""},
		{"/onvif/snapshot/sub/dir.jpg", ""},
		{"/onvif/snapshot/with?query.jpg", ""},
		{"/different/snapshot/main.jpg", ""},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := snapshot.TokenFromPath(tc.in); got != tc.want {
				t.Fatalf("TokenFromPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
