package g79client

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGetCurrencyOnline(t *testing.T) {
	client := testVitalityClient(t, func(req *http.Request, body string) string {
		if req.URL.Path != vitalityGetCurrencyOnlinePath {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		if body != "{}" {
			t.Fatalf("unexpected body: %s", body)
		}
		assertVitalityHeaders(t, req, vitalityGetCurrencyOnlinePath, body)
		return `{"code":0,"message":"正常返回","details":"","entity":{"rest_currency_time":3547,"date":"2026-05-07 09:00:19"}}`
	})

	resp, err := client.GetCurrencyOnline()
	if err != nil {
		t.Fatalf("GetCurrencyOnline failed: %v", err)
	}
	if resp.Entity.RestCurrencyTime != 3547 || resp.Entity.Date != "2026-05-07 09:00:19" {
		t.Fatalf("unexpected response: %+v", resp.Entity)
	}
}

func TestGetDailyGrowth(t *testing.T) {
	client := testVitalityClient(t, func(req *http.Request, body string) string {
		if req.URL.Path != vitalityDailyGrowthPath {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		assertVitalityHeaders(t, req, vitalityDailyGrowthPath, body)
		return `{"code":0,"message":"正常返回","details":"","entity":{"1":12,"2":3}}`
	})

	resp, err := client.GetDailyGrowth()
	if err != nil {
		t.Fatalf("GetDailyGrowth failed: %v", err)
	}
	if resp.Entity.XPFromOnline != 12 || resp.Entity.XPFromRecharge != 3 {
		t.Fatalf("unexpected response: %+v", resp.Entity)
	}
}

func testVitalityClient(t *testing.T, handle func(*http.Request, string) string) *Client {
	t.Helper()
	return &Client{
		UserID:    "3054162156",
		UserToken: "test-token",
		ReleaseJSON: G79ReleaseJSON{
			ApiGatewayUrl: "https://g79apigatewayobt.minecraft.cn",
		},
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				bodyBytes, err := io.ReadAll(req.Body)
				if err != nil {
					return nil, err
				}
				body := string(bodyBytes)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(handle(req, body))),
				}, nil
			}),
		},
	}
}

func assertVitalityHeaders(t *testing.T, req *http.Request, api, body string) {
	t.Helper()
	if req.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", req.Method)
	}
	if req.Header.Get("User-Agent") != "libhttpclient/1.0.0.0" {
		t.Fatalf("unexpected user-agent: %s", req.Header.Get("User-Agent"))
	}
	if req.Header.Get("user-id") != "3054162156" {
		t.Fatalf("unexpected user-id: %s", req.Header.Get("user-id"))
	}
	wantToken := CalculateDynamicToken(api, body, "test-token")
	if req.Header.Get("user-token") != wantToken {
		t.Fatalf("unexpected user-token: %s want %s", req.Header.Get("user-token"), wantToken)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
