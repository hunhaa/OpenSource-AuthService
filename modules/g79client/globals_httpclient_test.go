package g79client

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRefreshG79PackListWithHTTPClient_UsesProvidedClient(t *testing.T) {
	t.Parallel()

	var calls int
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			calls++

			if req.Method != http.MethodGet {
				t.Fatalf("unexpected method: %s", req.Method)
			}
			if req.URL.String() != g79PackListURL {
				t.Fatalf("unexpected url: %s", req.URL.String())
			}
			if ua := req.Header.Get("User-Agent"); ua != "libhttpclient/1.0.0.0" {
				t.Fatalf("unexpected User-Agent: %q", ua)
			}
			if ae := req.Header.Get("Accept-Encoding"); ae != "gzip" {
				t.Fatalf("unexpected Accept-Encoding: %q", ae)
			}
			if ct := req.Header.Get("content-type"); ct != "text/plain" {
				t.Fatalf("unexpected content-type: %q", ct)
			}
			if cc := req.Header.Get("cache-control"); cc != "no-cache" {
				t.Fatalf("unexpected cache-control: %q", cc)
			}

			body := `{"netease":{"url":"u","text":"t","version":"1.0","min_ver":"0"}}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Request:    req,
			}, nil
		}),
	}

	got, err := RefreshG79PackListWithHTTPClient(httpClient)
	if err != nil {
		t.Fatalf("RefreshG79PackListWithHTTPClient error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}

	entry, ok := got["netease"]
	if !ok {
		t.Fatalf("expected key %q to exist", "netease")
	}
	if entry.Version != "1.0" {
		t.Fatalf("unexpected version: %q", entry.Version)
	}
}
