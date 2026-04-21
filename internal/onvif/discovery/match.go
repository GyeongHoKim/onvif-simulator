package discovery

import (
	"net/url"
	"strings"
)

// ProbeMatches reports whether a Target Service satisfies a Probe's Types and Scopes
// (WS-Discovery §5.1). Empty probe Types means any type; empty probe Scopes means any scope.
// deviceTypes and deviceScopes come from the simulated device (Hello / config).
func ProbeMatches(probe *IncomingProbe, deviceTypes, deviceScopes []string) bool {
	if probe == nil {
		return false
	}
	if !typesMatch(probe.Types, deviceTypes) {
		return false
	}
	return scopesMatch(probe.Scopes, deviceScopes, probe.MatchBy)
}

// MatchResolve reports whether the Resolve request targets this device's endpoint address
// (trimmed string equality; extend with WS-Addressing comparison if needed).
func MatchResolve(resolve *IncomingResolve, deviceEndpointAddress string) bool {
	if resolve == nil {
		return false
	}
	a := strings.TrimSpace(resolve.EndpointAddress)
	b := strings.TrimSpace(deviceEndpointAddress)
	return a != "" && strings.EqualFold(a, b)
}

func typesMatch(probeTypes, deviceTypes []string) bool {
	if len(probeTypes) == 0 {
		return true
	}
	for _, pt := range probeTypes {
		pt = strings.TrimSpace(pt)
		if pt == "" {
			continue
		}
		if !containsQName(deviceTypes, pt) {
			return false
		}
	}
	return true
}

func containsQName(haystack []string, needle string) bool {
	for _, h := range haystack {
		if qnamesEqual(strings.TrimSpace(h), strings.TrimSpace(needle)) {
			return true
		}
	}
	return false
}

func qnamesEqual(a, b string) bool {
	if a == b {
		return true
	}
	ea, la, okA := expandQName(a)
	eb, lb, okB := expandQName(b)
	if okA && okB {
		return ea == eb && la == lb
	}
	return false
}

func expandQName(q string) (ns, local string, ok bool) {
	q = strings.TrimSpace(q)
	i := strings.IndexByte(q, ':')
	if i <= 0 || i == len(q)-1 {
		return "", "", false
	}
	prefix := q[:i]
	local = q[i+1:]
	ns, ok = knownQNamePrefixes[prefix]
	if !ok {
		return "", "", false
	}
	return ns, local, true
}

var knownQNamePrefixes = map[string]string{
	"tds": NSDeviceService,
}

func scopesMatch(probeScopes, deviceScopes []string, matchBy string) bool {
	if len(probeScopes) == 0 {
		return true
	}
	mb := strings.TrimSpace(matchBy)
	if mb == "" {
		mb = MatchByRFC2396
	}
	for _, ps := range probeScopes {
		ps = strings.TrimSpace(ps)
		if ps == "" {
			continue
		}
		if !anyScopeMatches(ps, deviceScopes, mb) {
			return false
		}
	}
	return true
}

func anyScopeMatches(probeScope string, deviceScopes []string, matchBy string) bool {
	for _, ds := range deviceScopes {
		if scopePairMatches(probeScope, strings.TrimSpace(ds), matchBy) {
			return true
		}
	}
	return false
}

func scopePairMatches(s1, s2, matchBy string) bool {
	switch matchBy {
	case MatchByRFC3986, MatchByRFC2396:
		return scopePathPrefixMatch(s1, s2)
	case MatchByStrcmp0:
		return s1 == s2
	default:
		// Unknown rule: conservative — only support rules above; fail to match.
		return false
	}
}

// scopePathPrefixMatch implements WS-Discovery §5.1 rfc2396-style segment prefix
// (ONVIF uses rfc3986 URI form; same segment-wise prefix behavior for http(s) and onvif:).
func scopePathPrefixMatch(s1, s2 string) bool {
	u1, err1 := url.Parse(s1)
	u2, err2 := url.Parse(s2)
	if err1 != nil || err2 != nil {
		return false
	}
	if !strings.EqualFold(u1.Scheme, u2.Scheme) {
		return false
	}
	host1 := strings.ToLower(strings.TrimSpace(u1.Host))
	host2 := strings.ToLower(strings.TrimSpace(u2.Host))
	if host1 != host2 {
		return false
	}
	segs1 := pathSegments(u1)
	segs2 := pathSegments(u2)
	return prefixSegments(segs1, segs2)
}

func pathSegments(u *url.URL) []string {
	p := strings.Trim(u.Path, "/")
	if p == "" {
		return nil
	}
	parts := strings.Split(p, "/")
	out := parts[:0]
	for _, p := range parts {
		if p == "." || p == ".." {
			return nil
		}
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func prefixSegments(short, long []string) bool {
	if len(short) > len(long) {
		return false
	}
	for i := range short {
		if short[i] != long[i] {
			return false
		}
	}
	return true
}
