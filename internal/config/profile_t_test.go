package config

import "testing"

func TestDefaultScopesIncludeProfileT(t *testing.T) {
	cfg := Default()
	found := false
	for _, scope := range cfg.Device.Scopes {
		if scope == "onvif://www.onvif.org/Profile/T" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("default scopes must include Profile T scope, got: %v", cfg.Device.Scopes)
	}
}

func TestDefaultScopesIncludeProfileS(t *testing.T) {
	cfg := Default()
	found := false
	for _, scope := range cfg.Device.Scopes {
		if scope == "onvif://www.onvif.org/Profile/Streaming" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("default scopes must include Profile S scope, got: %v", cfg.Device.Scopes)
	}
}
