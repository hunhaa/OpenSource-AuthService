package com4399

import (
	"net/http"
	"testing"
)

func TestIsNilRoundTripper(t *testing.T) {
	cases := []struct {
		name string
		rt   http.RoundTripper
		want bool
	}{
		{"nil-iface", nil, true},
		{"typed-nil-ptr", (*http.Transport)(nil), true},
		{"empty-struct", &http.Transport{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := isNilRoundTripper(c.rt)
			if got != c.want {
				t.Fatalf("want %v got %v", c.want, got)
			}
		})
	}
}

func TestSanitizeTransport(t *testing.T) {
	if x := sanitizeTransport(nil); x != nil {
		t.Fatalf("nil -> %v", x)
	}
	if x := sanitizeTransport((*http.Transport)(nil)); x != nil {
		t.Fatalf("typed nil *http.Transport -> %v", x)
	}
	real := &http.Transport{}
	if x := sanitizeTransport(real); x != real {
		t.Fatalf("real transport not passed through")
	}
}

func TestNewDirectHTTPClientNoPanic(t *testing.T) {
	for name, tr := range map[string]http.RoundTripper{
		"nil":          nil,
		"typed-nil":    (*http.Transport)(nil),
		"real-empty":   &http.Transport{},
	} {
		t.Run(name, func(t *testing.T) {
			c := newDirectHTTPClient(tr)
			if c == nil || c.Transport == nil {
				t.Fatalf("client / Transport should not be nil (name=%s)", name)
			}
		})
	}
}
