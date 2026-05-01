package snapshot

import "strings"

// PathPrefix is the URL prefix the simulator mounts the snapshot handler at.
// The full URL for a profile is `<PathPrefix><token>.jpg`.
const PathPrefix = "/onvif/snapshot/"

const pathSuffix = ".jpg"

// PathFor returns the URL path the snapshot handler serves a given profile
// token from. Callers concatenate this onto the device's HTTP base URL when
// answering GetSnapshotUri.
func PathFor(token string) string {
	return PathPrefix + token + pathSuffix
}

// TokenFromPath extracts the profile token embedded in an incoming request
// path. Returns "" when the path does not match the expected shape so the
// handler can answer 404 without leaking parser internals.
func TokenFromPath(p string) string {
	if !strings.HasPrefix(p, PathPrefix) {
		return ""
	}
	rest := p[len(PathPrefix):]
	if !strings.HasSuffix(rest, pathSuffix) {
		return ""
	}
	token := rest[:len(rest)-len(pathSuffix)]
	if token == "" || strings.ContainsAny(token, "/?#") {
		return ""
	}
	return token
}
