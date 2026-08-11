package sdk

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSignUniSauthBodyMatchesHAR(t *testing.T) {
	body := "{\"gameid\":\"x19\",\"login_channel\":\"netease\",\"app_channel\":\"netease\",\"platform\":\"ad\",\"sdkuid\":\"aibgt475voncthif\",\"udid\":\"fefc6d34106fa22f\",\"sessionid\":\"1-eyJzaSI6ICJmM2UxNDRhMzQ2NzA0ZWQzZDVkZGM2ZjJiNGQ4MGUwNThiNmIxMTlmIiwgIm9kaSI6ICJhbWF3dGxpYWF4bW5mMnh1LWQiLCAicyI6ICJuY3diMGwxbndhM2FkcmxwbmZjODl1NHFtNnZlNmFqeSIsICJ1IjogImFpYmd0NDc1dm9uY3RoaWYiLCAidCI6IDIsICJwcnMiOiAxMjgsICJnX2kiOiAiYWVjZnJ4b2R5cWFhYWFqcCIsICJzYSI6IC02fSAg\",\"sdk_version\":\"5.16.0\",\"is_unisdk_guest\":0,\"ip\":\"180.141.225.142\",\"aim_info\":\"{\\\"aim\\\":\\\"180.141.225.142\\\",\\\"country\\\":\\\"CN\\\",\\\"tz\\\":\\\"+0800\\\",\\\"tzid\\\":\\\"Asia\\\\\\/Shanghai\\\",\\\"celluar_ip\\\":\\\"\\\",\\\"operator\\\":\\\"\\\",\\\"is_vpn_enabled\\\":false}\",\"source_app_channel\":\"netease\",\"source_platform\":\"ad\",\"get_access_token\":\"1\",\"deviceid\":\"amawtliaaxmnf2xu-d\",\"client_login_sn\":\"31fed417fad50b5d13ef839f55ff05fb\",\"step\":\"0\",\"step2\":\"0\",\"hostid\":8000,\"sdklog\":\"{\\\"device_model\\\":\\\"PJF110\\\",\\\"os_name\\\":\\\"android\\\",\\\"os_ver\\\":\\\"15\\\",\\\"udid\\\":\\\"fefc6d34106fa22f\\\",\\\"app_ver\\\":\\\"840292836\\\",\\\"imei\\\":\\\"\\\",\\\"area_code\\\":\\\"CN\\\",\\\"is_emulator\\\":0,\\\"is_root\\\":0,\\\"oaid\\\":\\\"138CEDD06BE9455A9B11946E316502318bd1371d153bcf749b9809a72f07f827\\\",\\\"msa_oaid\\\":\\\"138CEDD06BE9455A9B11946E316502318bd1371d153bcf749b9809a72f07f827\\\"}\"}"
	got := signUniSauthBody(defaultSignKey, "/x19/sdk/uni_sauth", body, "1777597869", "207728f4c34a4f73924e11e18c0fd873")
	want := "002a1f152b09382e35263b746e5e016354f505a11c1af9c390cee61620fa2a57"
	if got != want {
		t.Fatalf("unexpected uni_sauth sign: got %s want %s", got, want)
	}
}

func TestSignCheckEnterBodyMatchesHAR(t *testing.T) {
	body := "{\"platform\":\"ad\",\"udid\":\"fefc6d34106fa22f\",\"aid\":938168464,\"sdkuid\":\"aibgt475voncthif\",\"login_channel\":\"netease\",\"hostid\":8000,\"roleid\":\"2719712857\",\"client_login_sn\":\"31fed417fad50b5d13ef839f55ff05fb\"}"
	got := signCheckEnterBody(defaultSignKey, "/x19/sdk/check_enter", body)
	want := "b2f2d46604d900199656696d6245e10988985ed71beb390533ac2fd2aabb7f68"
	if got != want {
		t.Fatalf("unexpected check_enter sign: got %s want %s", got, want)
	}
}

func TestCheckEnterWithCookieStopsAfterUniSauthFailure(t *testing.T) {
	client := NewClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/x19/sdk/uni_sauth" {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}
			return jsonResponse(`{"code":403,"subcode":10,"status":"account blocked","msg":"账号风控拦截"}`), nil
		}),
	})

	result, err := client.CheckEnterWithCookie(testCookie, &CookieCheckEnterOptions{
		RoleID: "2719712857",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.UniSauth == nil {
		t.Fatalf("expected uni_sauth result")
	}
	if result.UniSauth.Code != 403 {
		t.Fatalf("unexpected uni_sauth code: %d", result.UniSauth.Code)
	}
	if result.CheckEnter != nil {
		t.Fatalf("check_enter should not be called when uni_sauth fails")
	}
}

func TestCheckEnterWithCookieStopsWithoutRoleID(t *testing.T) {
	uniPayload := "eyJ1c2VybmFtZSI6InRlc3QiLCJyZWFsbmFtZV9tc2ciOnsicmVhbG5hbWVfc3RhdHVzIjoiMSIsImlzX2FkdWx0IjoiMSJ9fQ=="

	client := NewClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/x19/sdk/uni_sauth" {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}
			return jsonResponse(`{"code":200,"subcode":0,"status":"ok","aid":938168464,"sdkuid":"aibgrag426imjd65","unisdk_login_json":"` + uniPayload + `"}`), nil
		}),
	})

	result, err := client.CheckEnterWithCookie(testCookie, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.UniSauth == nil {
		t.Fatalf("expected uni_sauth result")
	}
	if result.DecodedUniSDKLoginPayload == nil {
		t.Fatalf("expected decoded payload")
	}
	if result.CheckEnterRequest != nil || result.CheckEnter != nil {
		t.Fatalf("check_enter should not run without roleID")
	}
}

func TestCheckEnterWithCookieRunsFullFlow(t *testing.T) {
	uniPayload := "eyJ1c2VybmFtZSI6InRlc3QiLCJyZWFsbmFtZV9tc2ciOnsicmVhbG5hbWVfc3RhdHVzIjoiMSIsImlzX2FkdWx0IjoiMSJ9fQ=="
	calledCheckEnter := false

	client := NewClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}

			switch req.URL.Path {
			case "/x19/sdk/uni_sauth":
				return jsonResponse(`{"code":200,"subcode":0,"status":"ok","aid":938168464,"sdkuid":"aibgrag426imjd65","unisdk_login_json":"` + uniPayload + `"}`), nil
			case "/x19/sdk/check_enter":
				calledCheckEnter = true
				bodyStr := string(body)
				if !strings.Contains(bodyStr, `"roleid":"2719712857"`) {
					t.Fatalf("unexpected check_enter body: %s", bodyStr)
				}
				return jsonResponse(`{"code":405,"subcode":11,"status":"unrealname online (account) time limit","msg":"限制提示","rm":"00","online_msg":null}`), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	})

	result, err := client.CheckEnterWithCookie(testCookie, &CookieCheckEnterOptions{
		RoleID: "2719712857",
		HostID: 8000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !calledCheckEnter {
		t.Fatalf("expected check_enter to be called")
	}
	if result.CheckEnterRequest == nil || result.CheckEnter == nil {
		t.Fatalf("expected full result")
	}
	if result.CheckEnter.Code != 405 {
		t.Fatalf("unexpected check_enter code: %d", result.CheckEnter.Code)
	}
}

func TestBuildUniSauthRequestFromAndroidCookie(t *testing.T) {
	request, err := BuildUniSauthRequestFromCookieString(testCookie, nil)
	if err != nil {
		t.Fatalf("BuildUniSauthRequestFromCookieString failed: %v", err)
	}
	if request.Platform != "ad" || request.SourcePlatform != "ad" {
		t.Fatalf("unexpected platform: %+v", request)
	}
	if request.SDKVersion != "5.16.0" {
		t.Fatalf("unexpected sdk version: %s", request.SDKVersion)
	}
	if request.SDKLog.DeviceModel != "V2166A" || request.SDKLog.OSVersion != "12" {
		t.Fatalf("unexpected sdklog: %+v", request.SDKLog)
	}
	if request.IsUnisdkGuest != 1 {
		t.Fatalf("unexpected guest flag: %d", request.IsUnisdkGuest)
	}

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request failed: %v", err)
	}
	bodyStr := string(body)
	if !strings.Contains(bodyStr, `"platform":"ad"`) || !strings.Contains(bodyStr, `"sdk_version":"5.16.0"`) {
		t.Fatalf("unexpected request body: %s", bodyStr)
	}
}

const testCookie = `{"sauth_json":"{\"gameid\":\"x19\",\"login_channel\":\"netease\",\"app_channel\":\"netease\",\"platform\":\"ad\",\"sdkuid\":\"aibgrag426imjd65\",\"sessionid\":\"1-test-session\",\"sdk_version\":\"5.16.0\",\"udid\":\"8d3ffe9277e569de\",\"deviceid\":\"amawraaaawsa5odz-d\",\"aim_info\":\"{\\\"aim\\\":\\\"127.0.0.1\\\",\\\"country\\\":\\\"CN\\\",\\\"tz\\\":\\\"+0800\\\",\\\"tzid\\\":\\\"Asia/Shanghai\\\",\\\"celluar_ip\\\":\\\"\\\",\\\"operator\\\":\\\"\\\",\\\"is_vpn_enabled\\\":false}\",\"client_login_sn\":\"F3F65AF678B99B323B95D0D7DEFEDD14\",\"gas_token\":\"\",\"source_app_channel\":\"netease\",\"source_platform\":\"ad\",\"ip\":\"127.0.0.1\"}","mac_addr":"bb5363e74b776bca12c76e1028859b71","ram":"1035337728","rom":"134208294912","is_guest":true,"emulator":1}`

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}
