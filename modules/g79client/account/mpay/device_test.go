package mpay

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestGenerateDeviceUsesAndroidFormAndUploads(t *testing.T) {
	var sawCreate bool
	var sawUpload bool

	client := NewClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			form, err := url.ParseQuery(string(body))
			if err != nil {
				return nil, err
			}

			switch req.URL.Path {
			case deviceEndpoint:
				sawCreate = true
				assertFormValue(t, form, "gv", defaultGV)
				assertFormValue(t, form, "gvn", defaultGVN)
				assertFormValue(t, form, "cv", defaultCV)
				assertFormValue(t, form, "brand", defaultDeviceBrand)
				assertFormValue(t, form, "device_model", defaultDeviceModel)
				assertFormValue(t, form, "system_version", defaultSystemVersion)
				assertFormValue(t, form, "jf_game_id", defaultJFGameID)
				assertFormValue(t, form, "pkg_channel", defaultPkgChannel)
				assertFormValue(t, form, "sc", defaultSC)
				if len(form.Get("udid")) != 16 {
					t.Fatalf("unexpected udid length: %s", form.Get("udid"))
				}
				return jsonResponse(http.StatusCreated, `{"device":{"id":"amawtestdevice-d","key":"396e667a76686264327037616579746d"}}`), nil
			case deviceUploadEndpoint:
				sawUpload = true
				assertFormValue(t, form, "device_id", "amawtestdevice-d")
				assertFormValue(t, form, "gv", defaultGV)
				assertFormValue(t, form, "jf_game_id", defaultJFGameID)
				return jsonResponse(http.StatusOK, `{"upload_time":1778110563}`), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	})

	device, err := client.GenerateDevice(context.Background())
	if err != nil {
		t.Fatalf("GenerateDevice failed: %v", err)
	}
	if !sawCreate || !sawUpload {
		t.Fatalf("expected create and upload requests, create=%v upload=%v", sawCreate, sawUpload)
	}
	if device.ID != "amawtestdevice-d" {
		t.Fatalf("unexpected device id: %s", device.ID)
	}
}

func TestGuestUsesAndroidForm(t *testing.T) {
	device := testDevice(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != deviceEndpoint+"/amawtestdevice-d/users/by_guest" {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			form, err := url.ParseQuery(string(body))
			if err != nil {
				return nil, err
			}
			assertFormValue(t, form, "opt_fields", defaultOptFields)
			assertFormValue(t, form, "gv", defaultGV)
			assertFormValue(t, form, "cv", defaultCV)
			assertFormValue(t, form, "jf_game_id", defaultJFGameID)
			assertFormValue(t, form, "pkg_channel", defaultPkgChannel)
			assertFormValue(t, form, "sc", defaultSC)
			params := form.Get("params")
			if params == "" {
				t.Fatalf("params should not be empty")
			}
			if _, err := hex.DecodeString(params); err != nil {
				t.Fatalf("params should be hex: %v", err)
			}
			return jsonResponse(http.StatusCreated, `{"user":{"id":"aibgtestuser","token":"1-test-token","udid":"8d3ffe9277e569de"}}`), nil
		}),
	})

	user, err := device.Guest(context.Background())
	if err != nil {
		t.Fatalf("Guest failed: %v", err)
	}
	if !user.IsGuest || user.Sauth.Platform != "ad" || user.Sauth.SDKVersion != "5.16.0" {
		t.Fatalf("unexpected user sauth: %+v", user.Sauth)
	}
}

func TestGuestNeedVerifyError(t *testing.T) {
	device := testDevice(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusBadRequest, `{"reason":"需要安全验证","code":1351,"verify_url":"https://service.mkey.163.com/verify"}`), nil
		}),
	})

	_, err := device.Guest(context.Background())
	var verifyErr *NeedVerifyError
	if !errors.As(err, &verifyErr) {
		t.Fatalf("expected NeedVerifyError, got %v", err)
	}
	if verifyErr.Code != 1351 || !strings.Contains(verifyErr.VerifyURL, "/verify") {
		t.Fatalf("unexpected verify error: %+v", verifyErr)
	}
}

func TestCookieStringIncludesAndroidSauthAndDeviceInfo(t *testing.T) {
	user := newUser("amawtestdevice-d", "aibgtestuser", "1-test-token", "8d3ffe9277e569de", "bb5363e74b776bca12c76e1028859b71", true, defaultRAMBytes, defaultROMBytes, "", "")

	cookie, err := user.CookieString()
	if err != nil {
		t.Fatalf("CookieString failed: %v", err)
	}

	var payload struct {
		SauthJSON string `json:"sauth_json"`
		MacAddr   string `json:"mac_addr"`
		RAM       string `json:"ram"`
		ROM       string `json:"rom"`
		IsGuest   bool   `json:"is_guest"`
		Emulator  int    `json:"emulator"`
	}
	if err := json.Unmarshal([]byte(cookie), &payload); err != nil {
		t.Fatalf("parse cookie failed: %v", err)
	}
	var sauth Sauth
	if err := json.Unmarshal([]byte(payload.SauthJSON), &sauth); err != nil {
		t.Fatalf("parse sauth failed: %v", err)
	}
	if sauth.Platform != "ad" || sauth.SourcePlatform != "ad" || sauth.SDKVersion != "5.16.0" {
		t.Fatalf("unexpected sauth: %+v", sauth)
	}
	if payload.MacAddr == "" || payload.RAM == "" || payload.ROM == "" || !payload.IsGuest || payload.Emulator != 1 {
		t.Fatalf("unexpected cookie payload: %+v", payload)
	}
}

func testDevice(httpClient *http.Client) *Device {
	return &Device{
		Key:                 "396e667a76686264327037616579746d",
		ID:                  "amawtestdevice-d",
		MCountTransactionID: "8d3ffe9277e569de_1778092263945_305440709",
		TransID:             "8d3ffe9277e569de_1778091941849_592919137",
		UniqueID:            "18688206-8c91-41ee-b990-f9b63793fe9e1778092184109",
		UDID:                "8d3ffe9277e569de",
		Mac:                 "bb5363e74b776bca12c76e1028859b71",
		Ram:                 defaultRAMBytes,
		Rom:                 defaultROMBytes,
		ExtCI:               "ad5df4ce5819c66a4235f66e446d9c8d0aa994f8ce87c64cecb481685621535a",
		client:              NewClient(httpClient),
	}
}

func assertFormValue(t *testing.T, form url.Values, key, want string) {
	t.Helper()
	if got := form.Get(key); got != want {
		t.Fatalf("form %s: got %q want %q", key, got, want)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
