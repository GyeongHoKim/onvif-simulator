package event

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	errEmptyDuration    = errors.New("event: empty duration string")
	errMalformedISO8601 = errors.New("event: malformed ISO 8601 duration")
	errUnsupportedFmt   = errors.New("event: unsupported duration format")
	errUnknownUnit      = errors.New("event: unknown ISO 8601 duration unit")
)

const subscriptionIDBytes = 8

// newSubscriptionID returns a random opaque subscription token.
func newSubscriptionID() string {
	b := make([]byte, subscriptionIDBytes)
	_, _ = rand.Read(b)
	return "sub-" + hex.EncodeToString(b)
}

// parseISO8601Duration parses a Go duration string (e.g. "1h", "30m") or a
// simple ISO 8601 duration subset (PTnH, PTnM, PTnS, PTnHnM, PTnHnMnS).
// Returns an error if the string is empty or cannot be parsed.
//
//nolint:cyclop // parsing a compact grammar with multiple branches; splitting adds no clarity
func parseISO8601Duration(s string) (time.Duration, error) {
	if s == "" {
		return 0, errEmptyDuration
	}

	// Try Go duration first (e.g. "1h30m").
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// ISO 8601 subset: PT[nH][nM][nS]
	upper := strings.ToUpper(s)
	if !strings.HasPrefix(upper, "PT") {
		return 0, fmt.Errorf("%w: %q", errUnsupportedFmt, s)
	}
	rest := upper[2:] // strip "PT"
	if rest == "" {
		return 0, fmt.Errorf("%w: %q", errMalformedISO8601, s)
	}

	var total time.Duration
	for rest != "" {
		// find first non-digit
		i := 0
		for i < len(rest) && (rest[i] >= '0' && rest[i] <= '9') {
			i++
		}
		if i == 0 || i >= len(rest) {
			return 0, fmt.Errorf("%w: %q", errMalformedISO8601, s)
		}
		n, err := strconv.Atoi(rest[:i])
		if err != nil {
			return 0, fmt.Errorf("%w: %q: %w", errMalformedISO8601, s, err)
		}
		unit := rest[i]
		rest = rest[i+1:]
		switch unit {
		case 'H':
			total += time.Duration(n) * time.Hour
		case 'M':
			total += time.Duration(n) * time.Minute
		case 'S':
			total += time.Duration(n) * time.Second
		default:
			return 0, fmt.Errorf("%w: %q in %q", errUnknownUnit, string(unit), s)
		}
	}
	return total, nil
}
