package sdk

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL             = "https://mgbsdk.matrix.netease.com"
	defaultAppChannel          = "netease"
	defaultPlatform            = "ad"
	defaultSDKVersion          = "5.16.0"
	defaultAppVersionGV        = "840292836"
	defaultUniSauthUserAgent   = "Dalvik/2.1.0 (Linux; U; Android 12; V2166A Build/b27cb4b.0)"
	defaultCheckEnterUserAgent = "Anti-Addiction/1.11.0 (V2166A;32)"
	defaultCommonSDK           = "ad=2.0.5"
	defaultLoginChannel        = "netease"
	defaultHostID              = 8000
	defaultSignKey             = "3Cz7dGX2EYHORebBUBHwCZ7pltZ_4l-t"
	defaultTimezone            = "+0800"
	defaultTimezoneID          = "Asia/Shanghai"
	defaultCountry             = "CN"
	defaultStep                = "0"
	defaultStep2               = "0"
	defaultDeviceModel         = "V2166A"
	defaultOSName              = "android"
	defaultOSVersion           = "12"
	defaultAreaCode            = "CN"
	defaultOAID                = ""
	defaultMSAOAID             = ""
	defaultOperator            = ""
)

// Client 封装与 x19 sdk 接口交互的 HTTP 客户端。
type Client struct {
	httpClient          *http.Client
	baseURL             string
	signKey             string
	uniSauthUserAgent   string
	checkEnterUserAgent string
	commonSDK           string
}

// NewClient 创建 x19 sdk 客户端。
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		httpClient:          httpClient,
		baseURL:             defaultBaseURL,
		signKey:             defaultSignKey,
		uniSauthUserAgent:   defaultUniSauthUserAgent,
		checkEnterUserAgent: defaultCheckEnterUserAgent,
		commonSDK:           defaultCommonSDK,
	}
}

// CookieData 表示账号 cookie 结构。
type CookieData struct {
	SauthJSON string `json:"sauth_json"`
	MacAddr   string `json:"mac_addr"`
	RAM       string `json:"ram"`
	ROM       string `json:"rom"`
	IsGuest   *bool  `json:"is_guest"`
	Emulator  *int   `json:"emulator"`
}

// SauthData 表示 cookie 中内嵌的 sauth 数据。
type SauthData struct {
	GameID           string `json:"gameid"`
	LoginChannel     string `json:"login_channel"`
	AppChannel       string `json:"app_channel"`
	Platform         string `json:"platform"`
	SDKUID           string `json:"sdkuid"`
	SessionID        string `json:"sessionid"`
	SDKVersion       string `json:"sdk_version"`
	UDID             string `json:"udid"`
	DeviceID         string `json:"deviceid"`
	AimInfo          string `json:"aim_info"`
	ClientLoginSN    string `json:"client_login_sn"`
	GasToken         string `json:"gas_token"`
	SourcePlatform   string `json:"source_platform"`
	SourceAppChannel string `json:"source_app_channel"`
	IP               string `json:"ip"`
	AccessToken      string `json:"access_token"`
}

// AimInfo 表示 uni_sauth 请求中的 aim_info。
type AimInfo struct {
	Aim          string `json:"aim"`
	Country      string `json:"country"`
	TZ           string `json:"tz"`
	TZID         string `json:"tzid"`
	CelluarIP    string `json:"celluar_ip"`
	Operator     string `json:"operator"`
	IsVPNEnabled bool   `json:"is_vpn_enabled"`
}

// UniSauthSDKLog 表示 uni_sauth 请求中的 sdklog。
type UniSauthSDKLog struct {
	DeviceModel string `json:"device_model"`
	OSName      string `json:"os_name"`
	OSVersion   string `json:"os_ver"`
	UDID        string `json:"udid"`
	AppVersion  string `json:"app_ver"`
	IMEI        string `json:"imei"`
	AreaCode    string `json:"area_code"`
	IsEmulator  int    `json:"is_emulator"`
	IsRoot      int    `json:"is_root"`
	OAID        string `json:"oaid"`
	MSAOAID     string `json:"msa_oaid,omitempty"`
}

// UniSauthRequest 表示 /x19/sdk/uni_sauth 请求体。
type UniSauthRequest struct {
	GameID           string         `json:"gameid"`
	LoginChannel     string         `json:"login_channel"`
	AppChannel       string         `json:"app_channel"`
	Platform         string         `json:"platform"`
	SDKUID           string         `json:"sdkuid"`
	UDID             string         `json:"udid"`
	SessionID        string         `json:"sessionid"`
	SDKVersion       string         `json:"sdk_version"`
	IsUnisdkGuest    int            `json:"is_unisdk_guest"`
	IP               string         `json:"ip"`
	AimInfo          AimInfo        `json:"-"`
	SourceAppChannel string         `json:"source_app_channel"`
	SourcePlatform   string         `json:"source_platform"`
	GetAccessToken   string         `json:"get_access_token"`
	DeviceID         string         `json:"deviceid"`
	ClientLoginSN    string         `json:"client_login_sn"`
	Step             string         `json:"step"`
	Step2            string         `json:"step2"`
	HostID           int            `json:"hostid"`
	SDKLog           UniSauthSDKLog `json:"-"`
}

// MarshalJSON 将强类型字段编码为服务端要求的字符串 JSON 字段。
func (r UniSauthRequest) MarshalJSON() ([]byte, error) {
	aimInfoJSON, err := json.Marshal(r.AimInfo)
	if err != nil {
		return nil, err
	}
	sdkLogJSON, err := json.Marshal(r.SDKLog)
	if err != nil {
		return nil, err
	}

	type payload struct {
		GameID           string `json:"gameid"`
		LoginChannel     string `json:"login_channel"`
		AppChannel       string `json:"app_channel"`
		Platform         string `json:"platform"`
		SDKUID           string `json:"sdkuid"`
		UDID             string `json:"udid"`
		SessionID        string `json:"sessionid"`
		SDKVersion       string `json:"sdk_version"`
		IsUnisdkGuest    int    `json:"is_unisdk_guest"`
		IP               string `json:"ip"`
		AimInfo          string `json:"aim_info"`
		SourceAppChannel string `json:"source_app_channel"`
		SourcePlatform   string `json:"source_platform"`
		GetAccessToken   string `json:"get_access_token"`
		DeviceID         string `json:"deviceid"`
		ClientLoginSN    string `json:"client_login_sn"`
		Step             string `json:"step"`
		Step2            string `json:"step2"`
		HostID           int    `json:"hostid"`
		SDKLog           string `json:"sdklog"`
	}

	return json.Marshal(payload{
		GameID:           r.GameID,
		LoginChannel:     r.LoginChannel,
		AppChannel:       r.AppChannel,
		Platform:         r.Platform,
		SDKUID:           r.SDKUID,
		UDID:             r.UDID,
		SessionID:        r.SessionID,
		SDKVersion:       r.SDKVersion,
		IsUnisdkGuest:    r.IsUnisdkGuest,
		IP:               r.IP,
		AimInfo:          string(aimInfoJSON),
		SourceAppChannel: r.SourceAppChannel,
		SourcePlatform:   r.SourcePlatform,
		GetAccessToken:   r.GetAccessToken,
		DeviceID:         r.DeviceID,
		ClientLoginSN:    r.ClientLoginSN,
		Step:             r.Step,
		Step2:            r.Step2,
		HostID:           r.HostID,
		SDKLog:           string(sdkLogJSON),
	})
}

// UniSauthBuildOptions 表示从 cookie 构造 uni_sauth 请求时的扩展参数。
type UniSauthBuildOptions struct {
	AppChannel string
	Platform   string
	SDKVersion string
	IP         string
	HostID     int
	Step       string
	Step2      string
	AimInfo    *AimInfo
	SDKLog     *UniSauthSDKLog
}

// UniSauthResponse 表示 /x19/sdk/uni_sauth 响应。
type UniSauthResponse struct {
	Code               int                     `json:"code"`
	Subcode            int                     `json:"subcode"`
	Status             string                  `json:"status"`
	Msg                string                  `json:"msg"`
	AID                int64                   `json:"aid"`
	SDKUID             string                  `json:"sdkuid"`
	NewTM              int64                   `json:"newtm"`
	UniSDKLoginJSON    string                  `json:"unisdk_login_json"`
	Data               UniSauthResponseData    `json:"data"`
	RealnameMsg        UniSauthRealnameMessage `json:"realname_msg"`
	AASBan             UniSauthAASBan          `json:"aas_ban"`
	FirstLoginPlatform string                  `json:"first_login_platform"`
	UniversalInfo      []json.RawMessage       `json:"universal_info"`
	RawBody            string                  `json:"-"`
}

// UniSauthResponseData 表示 uni_sauth 响应中的 data 字段。
type UniSauthResponseData struct {
	AppChannel string `json:"app_channel"`
	LoginType  int    `json:"login_type"`
}

// UniSauthRealnameMessage 表示实名认证信息。
type UniSauthRealnameMessage struct {
	Oversea      bool   `json:"oversea"`
	IDHash       string `json:"id_hash"`
	VerifyStatus int    `json:"verify_status"`
	Birthday     string `json:"birthday"`
	AgeRange     int    `json:"age_range"`
	UpdateGuide  int    `json:"update_guide"`
	Age          int    `json:"age"`
	AgeRangeV2   int    `json:"age_range_v2"`
}

// UniSauthAASBan 表示防沉迷状态。
type UniSauthAASBan struct {
	Ban    int    `json:"ban"`
	BanMsg string `json:"ban_msg"`
}

// DecodedUniSDKLoginPayload 表示解码后的 unisdk_login_json。
type DecodedUniSDKLoginPayload struct {
	AID                      string                        `json:"aid"`
	SDKUID                   string                        `json:"sdkuid"`
	Username                 string                        `json:"username"`
	ServerTime               int64                         `json:"server_time"`
	AccessToken              string                        `json:"access_token"`
	ExpiresIn                string                        `json:"expires_in"`
	RefreshToken             string                        `json:"refresh_token"`
	RefreshExpiresIn         string                        `json:"refresh_expires_in"`
	RealnameMsg              DecodedUniSDKLoginRealnameMsg `json:"realname_msg"`
	ChlSDKJSON               DecodedUniSDKLoginChlSDKJSON  `json:"chl_sdk_json"`
	AASVersion               string                        `json:"aas_version"`
	ConfigEnableGuardian     bool                          `json:"config_enable_guardian"`
	Extra                    DecodedUniSDKLoginExtra       `json:"extra"`
	OAuth2                   DecodedUniSDKLoginOAuth2      `json:"oauth2"`
	Region                   string                        `json:"region"`
	IsSelectedForGrayRelease bool                          `json:"is_selected_for_gray_release"`
}

// DecodedUniSDKLoginRealnameMsg 表示解码后的实名认证状态。
type DecodedUniSDKLoginRealnameMsg struct {
	RealnameStatus string `json:"realname_status"`
	IsAdult        string `json:"is_adult"`
}

// DecodedUniSDKLoginChlSDKJSON 表示解码后的渠道登录信息。
type DecodedUniSDKLoginChlSDKJSON struct {
	GameID       string `json:"gameid"`
	LoginChannel string `json:"login_channel"`
	SDKUID       string `json:"sdkuid"`
	AID          string `json:"aid"`
	UDID         string `json:"udid"`
	Timestamp    string `json:"timestamp"`
	Sign         string `json:"sign"`
}

// DecodedUniSDKLoginExtra 表示解码后的 extra 信息。
type DecodedUniSDKLoginExtra struct {
	SDKOpenID              string `json:"sdk_open_id"`
	ExternalUsername       string `json:"external_username"`
	ExternalName           string `json:"external_name"`
	LoginType              string `json:"login_type"`
	NeedQueryRealnameGuide int    `json:"need_query_realname_guide"`
	OriginLoginChannel     string `json:"origin_login_channel"`
	OriginSDKUID           string `json:"origin_sdkuid"`
}

// DecodedUniSDKLoginOAuth2 表示解码后的 oauth2 信息。
type DecodedUniSDKLoginOAuth2 struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// DecodeUniSDKLoginJSON 解码 base64 的 unisdk_login_json。
func (r *UniSauthResponse) DecodeUniSDKLoginJSON() (*DecodedUniSDKLoginPayload, error) {
	if r == nil {
		return nil, fmt.Errorf("uni_sauth 响应为空")
	}
	if strings.TrimSpace(r.UniSDKLoginJSON) == "" {
		return nil, fmt.Errorf("uni_sauth 响应缺少 unisdk_login_json")
	}

	raw, err := base64.StdEncoding.DecodeString(r.UniSDKLoginJSON)
	if err != nil {
		return nil, fmt.Errorf("解码 unisdk_login_json 失败: %w", err)
	}

	var result DecodedUniSDKLoginPayload
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("解析 unisdk_login_json 失败: %w", err)
	}
	return &result, nil
}

// CheckEnterRequest 表示 /x19/sdk/check_enter 请求体。
type CheckEnterRequest struct {
	Platform      string `json:"platform"`
	UDID          string `json:"udid"`
	AID           int64  `json:"aid"`
	SDKUID        string `json:"sdkuid"`
	LoginChannel  string `json:"login_channel"`
	HostID        int    `json:"hostid"`
	RoleID        string `json:"roleid"`
	ClientLoginSN string `json:"client_login_sn"`
}

// CheckEnterResponse 表示 /x19/sdk/check_enter 响应。
type CheckEnterResponse struct {
	Code      int             `json:"code"`
	Subcode   int             `json:"subcode"`
	Status    string          `json:"status"`
	Msg       string          `json:"msg"`
	RM        string          `json:"rm"`
	OnlineMsg json.RawMessage `json:"online_msg"`
	RawBody   string          `json:"-"`
}

// CookieCheckEnterOptions 表示基于 cookie 检测 check_enter 的参数。
type CookieCheckEnterOptions struct {
	UniSauth *UniSauthBuildOptions
	RoleID   string
	HostID   int
}

// CookieCheckEnterResult 表示基于 cookie 的完整检测结果。
type CookieCheckEnterResult struct {
	UniSauth                  *UniSauthResponse
	DecodedUniSDKLoginPayload *DecodedUniSDKLoginPayload
	CheckEnterRequest         *CheckEnterRequest
	CheckEnter                *CheckEnterResponse
}

// ParseCookieString 解析 cookie 字符串。
func ParseCookieString(cookieStr string) (*CookieData, *SauthData, error) {
	var cookie CookieData
	if err := json.Unmarshal([]byte(cookieStr), &cookie); err != nil {
		return nil, nil, fmt.Errorf("解析 cookie 失败: %w", err)
	}

	var sauth SauthData
	if err := json.Unmarshal([]byte(cookie.SauthJSON), &sauth); err != nil {
		return nil, nil, fmt.Errorf("解析 sauth_json 失败: %w", err)
	}

	return &cookie, &sauth, nil
}

// BuildUniSauthRequestFromCookieString 从 cookie 构造 uni_sauth 请求。
func BuildUniSauthRequestFromCookieString(cookieStr string, opts *UniSauthBuildOptions) (*UniSauthRequest, error) {
	cookie, sauth, err := ParseCookieString(cookieStr)
	if err != nil {
		return nil, err
	}
	return BuildUniSauthRequestFromCookieData(cookie, sauth, opts)
}

// BuildUniSauthRequestFromCookieData 从解析后的 cookie 和 sauth 构造 uni_sauth 请求。
func BuildUniSauthRequestFromCookieData(cookie *CookieData, sauth *SauthData, opts *UniSauthBuildOptions) (*UniSauthRequest, error) {
	if sauth == nil {
		return nil, fmt.Errorf("sauth 数据不能为空")
	}

	appChannel := defaultAppChannel
	if opts != nil && strings.TrimSpace(opts.AppChannel) != "" {
		appChannel = opts.AppChannel
	} else if strings.TrimSpace(sauth.AppChannel) != "" {
		appChannel = sauth.AppChannel
	}

	platform := defaultPlatform
	if opts != nil && strings.TrimSpace(opts.Platform) != "" {
		platform = opts.Platform
	} else if strings.TrimSpace(sauth.Platform) != "" {
		platform = sauth.Platform
	}

	sdkVersion := defaultSDKVersion
	if opts != nil && strings.TrimSpace(opts.SDKVersion) != "" {
		sdkVersion = opts.SDKVersion
	} else if strings.TrimSpace(sauth.SDKVersion) != "" {
		sdkVersion = sauth.SDKVersion
	}

	ip := strings.TrimSpace(sauth.IP)
	if opts != nil && strings.TrimSpace(opts.IP) != "" {
		ip = opts.IP
	}
	if ip == "" {
		ip = "127.0.0.1"
	}

	hostID := defaultHostID
	if opts != nil && opts.HostID > 0 {
		hostID = opts.HostID
	}

	step := defaultStep
	if opts != nil && opts.Step != "" {
		step = opts.Step
	}
	step2 := defaultStep2
	if opts != nil && opts.Step2 != "" {
		step2 = opts.Step2
	}

	aimInfo := defaultAimInfo(ip)
	if parsedAim, err := parseAimInfo(sauth.AimInfo); err == nil && parsedAim != nil {
		aimInfo = *parsedAim
		if aimInfo.Aim == "" {
			aimInfo.Aim = ip
		}
	}
	if opts != nil && opts.AimInfo != nil {
		aimInfo = *opts.AimInfo
	}

	sdkLog := defaultSDKLog(sauth.UDID, cookie)
	if opts != nil && opts.SDKLog != nil {
		sdkLog = *opts.SDKLog
	}

	request := &UniSauthRequest{
		GameID:           firstNonEmpty(sauth.GameID, "x19"),
		LoginChannel:     firstNonEmpty(sauth.LoginChannel, defaultLoginChannel),
		AppChannel:       appChannel,
		Platform:         platform,
		SDKUID:           sauth.SDKUID,
		UDID:             sauth.UDID,
		SessionID:        sauth.SessionID,
		SDKVersion:       sdkVersion,
		IsUnisdkGuest:    guestFlag(cookie),
		IP:               ip,
		AimInfo:          aimInfo,
		SourceAppChannel: firstNonEmpty(sauth.SourceAppChannel, appChannel),
		SourcePlatform:   firstNonEmpty(sauth.SourcePlatform, platform),
		GetAccessToken:   "1",
		DeviceID:         sauth.DeviceID,
		ClientLoginSN:    sauth.ClientLoginSN,
		Step:             step,
		Step2:            step2,
		HostID:           hostID,
		SDKLog:           sdkLog,
	}

	if strings.TrimSpace(request.SDKUID) == "" || strings.TrimSpace(request.SessionID) == "" ||
		strings.TrimSpace(request.UDID) == "" || strings.TrimSpace(request.DeviceID) == "" {
		return nil, fmt.Errorf("cookie 中缺少 uni_sauth 必填字段")
	}

	return request, nil
}

// BuildCheckEnterRequestFromCookieString 从 cookie 构造 check_enter 请求。
func BuildCheckEnterRequestFromCookieString(cookieStr string, aid int64, roleID string, hostID int) (*CheckEnterRequest, error) {
	_, sauth, err := ParseCookieString(cookieStr)
	if err != nil {
		return nil, err
	}

	if aid <= 0 {
		return nil, fmt.Errorf("aid 必须大于 0")
	}
	if strings.TrimSpace(roleID) == "" {
		return nil, fmt.Errorf("roleID 不能为空")
	}
	if hostID <= 0 {
		hostID = defaultHostID
	}

	return &CheckEnterRequest{
		Platform:      firstNonEmpty(sauth.Platform, defaultPlatform),
		UDID:          sauth.UDID,
		AID:           aid,
		SDKUID:        sauth.SDKUID,
		LoginChannel:  firstNonEmpty(sauth.LoginChannel, defaultLoginChannel),
		HostID:        hostID,
		RoleID:        roleID,
		ClientLoginSN: sauth.ClientLoginSN,
	}, nil
}

// CheckEnterWithCookie 使用 cookie 执行 uni_sauth，并在满足条件时继续 check_enter。
func (c *Client) CheckEnterWithCookie(cookieStr string, opts *CookieCheckEnterOptions) (*CookieCheckEnterResult, error) {
	uniReq, err := BuildUniSauthRequestFromCookieString(cookieStr, optionsUniSauth(opts))
	if err != nil {
		return nil, err
	}

	uniResp, err := c.UniSauth(uniReq)
	if err != nil {
		return nil, err
	}

	result := &CookieCheckEnterResult{
		UniSauth: uniResp,
	}

	if uniResp.Code != 200 || uniResp.Subcode != 0 {
		return result, nil
	}

	if strings.TrimSpace(uniResp.UniSDKLoginJSON) != "" {
		decoded, err := uniResp.DecodeUniSDKLoginJSON()
		if err != nil {
			return nil, err
		}
		result.DecodedUniSDKLoginPayload = decoded
	}

	roleID := ""
	hostID := 0
	if opts != nil {
		roleID = opts.RoleID
		hostID = opts.HostID
	}
	if strings.TrimSpace(roleID) == "" {
		return result, nil
	}

	checkReq, err := BuildCheckEnterRequestFromCookieString(cookieStr, uniResp.AID, roleID, hostID)
	if err != nil {
		return nil, err
	}
	result.CheckEnterRequest = checkReq

	checkResp, err := c.CheckEnter(checkReq)
	if err != nil {
		return nil, err
	}
	result.CheckEnter = checkResp
	return result, nil
}

// UniSauth 调用 /x19/sdk/uni_sauth。
func (c *Client) UniSauth(request *UniSauthRequest) (*UniSauthResponse, error) {
	if request == nil {
		return nil, fmt.Errorf("uni_sauth 请求不能为空")
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("序列化 uni_sauth 请求失败: %w", err)
	}

	gasTimestamp := strconv.FormatInt(time.Now().Unix(), 10)
	gasNonce, err := randomHexString(16)
	if err != nil {
		return nil, fmt.Errorf("生成 X-Gas-Nonce 失败: %w", err)
	}

	path := "/x19/sdk/uni_sauth"
	signature := signUniSauthBody(c.signKey, path, string(body), gasTimestamp, gasNonce)
	taskID := buildTaskID(request.UDID)

	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(c.baseURL, "/")+path, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-Sign", signature)
	req.Header.Set("X-Common-SDK", c.commonSDK)
	req.Header.Set("X-Gas-Nonce", gasNonce)
	req.Header.Set("X-Gas-Timestamp", gasTimestamp)
	req.Header.Set("X-TASK-ID", taskID)
	req.Header.Set("User-Agent", c.uniSauthUserAgent)
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result UniSauthResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析 uni_sauth 响应失败: %w, 响应内容: %s", err, string(respBody))
	}
	result.RawBody = string(respBody)
	return &result, nil
}

// CheckEnter 调用 /x19/sdk/check_enter。
func (c *Client) CheckEnter(request *CheckEnterRequest) (*CheckEnterResponse, error) {
	if request == nil {
		return nil, fmt.Errorf("check_enter 请求不能为空")
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("序列化 check_enter 请求失败: %w", err)
	}

	path := "/x19/sdk/check_enter"
	signature := signCheckEnterBody(c.signKey, path, string(body))
	taskID := buildTaskID(request.UDID)

	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(c.baseURL, "/")+path, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept-Charset", "UTF-8")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-TASK-ID", taskID)
	req.Header.Set("User-Agent", c.checkEnterUserAgent)
	req.Header.Set("X-Client-Sign", signature)
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result CheckEnterResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析 check_enter 响应失败: %w, 响应内容: %s", err, string(respBody))
	}
	result.RawBody = string(respBody)
	return &result, nil
}

func signUniSauthBody(key, path, body, gasTimestamp, gasNonce string) string {
	message := http.MethodPost + path + body + "\n" + gasTimestamp + "\n" + gasNonce
	return signHMACSHA256Hex(key, message)
}

func signCheckEnterBody(key, path, body string) string {
	message := http.MethodPost + path + body
	return signHMACSHA256Hex(key, message)
}

func signHMACSHA256Hex(key, message string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(message))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func buildTaskID(udid string) string {
	return fmt.Sprintf("transid=%s,uni_transaction_id=%s", newTransID(udid), newTransID(udid))
}

func newTransID(udid string) string {
	return fmt.Sprintf("%s_%d_%s", udid, time.Now().UnixMilli(), randomDigits(9))
}

func optionsUniSauth(opts *CookieCheckEnterOptions) *UniSauthBuildOptions {
	if opts == nil {
		return nil
	}
	return opts.UniSauth
}

func randomDigits(length int) string {
	if length <= 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(length)
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		for i := 0; i < length; i++ {
			b.WriteByte('0')
		}
		return b.String()
	}
	for _, v := range buf {
		b.WriteByte('0' + (v % 10))
	}
	return b.String()
}

func randomHexString(byteLen int) (string, error) {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", buf), nil
}

func defaultAimInfo(ip string) AimInfo {
	return AimInfo{
		Aim:          firstNonEmpty(ip, "127.0.0.1"),
		Country:      defaultCountry,
		TZ:           defaultTimezone,
		TZID:         defaultTimezoneID,
		CelluarIP:    "",
		Operator:     defaultOperator,
		IsVPNEnabled: false,
	}
}

func defaultSDKLog(udid string, cookie *CookieData) UniSauthSDKLog {
	return UniSauthSDKLog{
		DeviceModel: defaultDeviceModel,
		OSName:      defaultOSName,
		OSVersion:   defaultOSVersion,
		UDID:        udid,
		AppVersion:  defaultAppVersionGV,
		IMEI:        "",
		AreaCode:    defaultAreaCode,
		IsEmulator:  emulatorFlag(cookie),
		IsRoot:      0,
		OAID:        defaultOAID,
		MSAOAID:     defaultMSAOAID,
	}
}

func parseAimInfo(raw string) (*AimInfo, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("empty aim_info")
	}
	var result AimInfo
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func guestFlag(cookie *CookieData) int {
	if cookie != nil && cookie.IsGuest != nil && *cookie.IsGuest {
		return 1
	}
	return 0
}

func emulatorFlag(cookie *CookieData) int {
	if cookie != nil && cookie.Emulator != nil {
		return *cookie.Emulator
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
