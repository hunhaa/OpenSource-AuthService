package g79client

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

func TestRefreshG79CPPVersionMapWithHTTPClient_UsesProvidedClient(t *testing.T) {
	g79ReleaseMu.Lock()
	oldReleaseJSON := globalG79ReleaseJSON
	globalG79ReleaseJSON = &G79ReleaseJSON{
		TransferServerNewHttpUrl: "https://g79mcltransfer.minecraft.cn",
	}
	g79ReleaseMu.Unlock()
	defer func() {
		g79ReleaseMu.Lock()
		globalG79ReleaseJSON = oldReleaseJSON
		g79ReleaseMu.Unlock()
	}()

	g79CPPVersionMapMu.Lock()
	oldVersionMap := globalG79CPPVersionMap
	globalG79CPPVersionMap = nil
	g79CPPVersionMapMu.Unlock()
	defer func() {
		g79CPPVersionMapMu.Lock()
		globalG79CPPVersionMap = oldVersionMap
		g79CPPVersionMapMu.Unlock()
	}()

	var calls int
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			calls++

			if req.Method != http.MethodGet {
				t.Fatalf("unexpected method: %s", req.Method)
			}
			if req.URL.String() != "https://g79mcltransfer.minecraft.cn/cpp-version-map" {
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

			body := `{"pc":40,"pe":[{"version":"3.7","value":40},{"version":"3.8","value":41}]}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Request:    req,
			}, nil
		}),
	}

	got, err := RefreshG79CPPVersionMapWithHTTPClient(httpClient)
	if err != nil {
		t.Fatalf("RefreshG79CPPVersionMapWithHTTPClient error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
	if got.PC != 40 {
		t.Fatalf("unexpected pc value: %d", got.PC)
	}
	if len(got.PE) != 2 {
		t.Fatalf("unexpected pe length: %d", len(got.PE))
	}
	if got.PE[0].Version != "3.7" || got.PE[0].Value != 40 {
		t.Fatalf("unexpected first pe entry: %+v", got.PE[0])
	}
}
