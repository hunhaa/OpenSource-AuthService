package g79client

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Yeah114/g79client/utils"
	"github.com/google/uuid"
)

// 认证相关结构体
type LoginEntity struct {
	EntityID string `json:"entity_id"`
	Token    string `json:"token"`
	Seed     string `json:"seed"`
}

type LoginResponse struct {
	Response
	Entity LoginEntity `json:"entity"`
}

// {'HostNum': 200, 'ServerHostNum': 8000, 'TempServerStop': 0, 'CdnUrl': 'https://g79.gdl.netease.com/', 'H5VersionUrl': 'https://g79.update.netease.com/cdnversion/obt_h5version.json', 'SeadraUrl': 'https://pub-api.seadra.netease.com', 'HomeServerUrl': 'https://g79mclobthome.minecraft.cn', 'HomeServerGrayUrl': 'https://g79mclobthomegray.nie.netease.com:9443', 'WebServerUrl': 'https://g79mclobt.minecraft.cn', 'WebServerGrayUrl': 'https://g79mclobtgray.nie.netease.com:9443', 'CoreServerUrl': 'https://g79obtapigtcoregray.minecraft.cn', 'CoreServerGrayUrl': 'https://g79obtapigtcoregray.minecraft.cn', 'TransferServerUrl': 'https://g79.update.netease.com/transferserver_obt_new.list', 'TransferServerHttpUrl': 'https://g79transfernew.nie.netease.com', 'TransferServerNewHttpUrl': 'https://g79mcltransfer.minecraft.cn', 'MomentUrl': 'https://x19-pyq.webcgi.163.com/', 'ForumUrl': 'https://mcpel-web.16163.com', 'AuthServerUrl': 'https://g79authobt.minecraft.cn', 'ChatServerUrl': 'https://x19.update.netease.com/chatserver.list', 'PathNUrl': 'https://impression.update.netease.com/lighten/atlas_x19_hangzhou-{isp}.txt', 'PePathNUrl': 'https://impression.update.netease.com/lighten/atlas_g79_hangzhou-{isp}.txt', 'PathNIpv6Url': 'https://impression.update.netease.com/lighten/x19/cnv6.txt', 'PePathNIpv6Url': 'https://impression.update.netease.com/lighten/g79/cnv6.txt', 'LinkServerUrl': 'https://g79.update.netease.com/linkserver_obt.list', 'ApiGatewayUrl': 'https://g79apigatewayobt.minecraft.cn', 'ApiGatewayWeiXinUrl': 'https://g79apigatewayobtweixin.minecraft.cn', 'ApiGatewayGrayUrl': 'https://g79apigatewaygrayobt.nie.netease.com', 'communityHost': 'https://news-api.16163.com/app/g79/api', 'WelfareUrl': 'https://mc.163.com/pe/client/', 'DCWebUrl': 'https://x19apigatewayobt.nie.netease.com', 'RentalTransferUrl': 'https://mcrealms.update.netease.com/isp_map_production.json', 'MgbSdkUrl': 'https://mgbsdk.matrix.netease.com'}
type G79ReleaseJSON struct {
	CoreServerURL              string `json:"CoreServerUrl"`
	CoreServerGrayURL          string `json:"CoreServerGrayUrl"`
	AuthServerURL              string `json:"AuthServerUrl"`
	AuthServerNewHttpURL       string `json:"AuthServerNewHttpUrl"`
	WebServerUrl               string `json:"WebServerUrl"`
	WebServerGrayURL           string `json:"WebServerGrayUrl"`
	WebServerNewHttpURL        string `json:"WebServerNewHttpUrl"`
	ApiGatewayUrl              string `json:"ApiGatewayUrl"`
	ApiGatewayGrayUrl          string `json:"ApiGatewayGrayUrl"`
	TransferServerUrl          string `json:"TransferServerUrl"`
	TransferServerHttpUrl      string `json:"TransferServerHttpUrl"`
	TransferServerNewHttpUrl   string `json:"TransferServerNewHttpUrl"`
	ChatServerURL              string `json:"ChatServerUrl"`
	LinkServerURL              string `json:"LinkServerUrl"`
	RentalTransferURL          string `json:"RentalTransferUrl"`
	DCWebURL                   string `json:"DCWebUrl"`
	HomeServerURL              string `json:"HomeServerUrl"`
	HomeServerGrayURL          string `json:"HomeServerGrayUrl"`
	MgbSdkUrl                  string `json:"MgbSdkUrl"`
	ApiGatewayWeiXinUrl        string `json:"ApiGatewayWeiXinUrl"`
	CommunityHost              string `json:"communityHost"`
	WelfareUrl                 string `json:"WelfareUrl"`
	SeadraURL                  string `json:"SeadraUrl"`
}

type X19ReleaseJSON struct {
	CoreServerURL              string `json:"CoreServerUrl"`
	WebServerURL               string `json:"WebServerUrl"`
	WebServerGrayURL           string `json:"WebServerGrayUrl"`
	TransferServerURL          string `json:"TransferServerUrl"`
	TransferServerHTTPURL      string `json:"TransferServerHttpUrl"`
	PeTransferServerURL        string `json:"PeTransferServerUrl"`
	PeTransferServerHTTPURL    string `json:"PeTransferServerHttpUrl"`
	PeTransferServerNewHTTPURL string `json:"PeTransferServerNewHttpUrl"`
	AuthServerURL              string `json:"AuthServerUrl"`
	AuthServerCppURL           string `json:"AuthServerCppUrl"`
	ChatServerURL              string `json:"ChatServerUrl"`
	ApiGatewayURL              string `json:"ApiGatewayUrl"`
	ApiGatewayGrayURL          string `json:"ApiGatewayGrayUrl"`
	DCWebURL                   string `json:"DCWebUrl"`
	RentalTransferURL          string `json:"RentalTransferUrl"`
}

// Cookie数据结构
type CookieData struct {
	SauthJSON string `json:"sauth_json"`
	MacAddr   string `json:"mac_addr"`
	RAM       string `json:"ram"`
	ROM       string `json:"rom"`
	IsGuest   *bool  `json:"is_guest"`
	Emulator  *int   `json:"emulator"`
}

func (c *CookieData) macAddress() string {
	mac := strings.ToUpper(strings.TrimSpace(c.MacAddr))
	if mac == "" {
		return "02:00:00:00:00:00"
	}
	mac = strings.ReplaceAll(mac, "-", ":")
	return mac
}

func (c *CookieData) ramValue() string {
	if strings.TrimSpace(c.RAM) == "" {
		return "4294967296"
	}
	return c.RAM
}

func (c *CookieData) romValue() string {
	return c.ROM
}

func (c *CookieData) isGuestFlag() int {
	if c.IsGuest != nil && *c.IsGuest {
		return 1
	}
	return 0
}

func (c *CookieData) emulatorFlag() int {
	if c.Emulator != nil {
		return *c.Emulator
	}
	return 0
}

type SauthData struct {
	GameID         string `json:"gameid"`
	LoginChannel   string `json:"login_channel"`
	AppChannel     string `json:"app_channel"`
	Platform       string `json:"platform"`
	SDKUID         string `json:"sdkuid"`
	SessionID      string `json:"sessionid"`
	SDKVersion     string `json:"sdk_version"`
	UDID           string `json:"udid"`
	DeviceID       string `json:"deviceid"`
	AimInfo        string `json:"aim_info"`
	ClientLoginSN  string `json:"client_login_sn"`
	GasToken       string `json:"gas_token"`
	SourcePlatform string `json:"source_platform"`
	IP             string `json:"ip"`
}

type PeAuthData struct {
	EngineVersion string         `json:"engine_version"`
	ExtraParam    string         `json:"extra_param"`
	Message       string         `json:"message"`
	PatchVersion  string         `json:"patch_version"`
	PayChannel    string         `json:"pay_channel"`
	SaData        string         `json:"sa_data"`
	SauthJSON     map[string]any `json:"sauth_json"`
	Seed          string         `json:"seed"`
	Sign          string         `json:"sign"`
}

// 使用Cookie进行PE认证
func (c *Client) G79AuthenticateWithCookie(cookieStr string) error {
	c.Cookie = cookieStr
	// 解析Cookie
	var cookieData CookieData
	err := json.Unmarshal([]byte(cookieStr), &cookieData)
	if err != nil {
		return fmt.Errorf("解析Cookie失败: %v", err)
	}

	var sauthData SauthData
	err = json.Unmarshal([]byte(cookieData.SauthJSON), &sauthData)
	if err != nil {
		return fmt.Errorf("解析SauthJSON失败: %v", err)
	}

	// 执行PE认证
	err = c.g79PerformPEAuthWithCookie(&sauthData, &cookieData)
	if err != nil {
		return fmt.Errorf("PE认证失败: %v", err)
	}

	// 获取用户详情
	userDetail, err := c.GetUserDetail()
	if err != nil {
		return fmt.Errorf("获取用户信息失败: %v", err)
	}

	c.UserDetail = &userDetail.Entity

	return nil
}

func (c *Client) G79AuthenticateWithCookieTest(cookieStr string, sp, tr int) error {
	c.Cookie = cookieStr
	var cookieData CookieData
	err := json.Unmarshal([]byte(cookieStr), &cookieData)
	if err != nil {
		return fmt.Errorf("解析Cookie失败: %v", err)
	}

	var sauthData SauthData
	err = json.Unmarshal([]byte(cookieData.SauthJSON), &sauthData)
	if err != nil {
		return fmt.Errorf("解析SauthJSON失败: %v", err)
	}

	err = c.g79PerformPEAuthWithCookieTest(&sauthData, &cookieData, sp, tr)
	if err != nil {
		return err
	}

	userDetail, err := c.GetUserDetail()
	if err != nil {
		return fmt.Errorf("获取用户信息失败: %v", err)
	}

	c.UserDetail = &userDetail.Entity

	return nil
}

// 使用Cookie执行PE认证（指定sp/tr参数，用于穷举测试）
func (c *Client) g79PerformPEAuthWithCookieTest(sauthData *SauthData, cookieData *CookieData, sp, tr int) error {
	if sauthData == nil {
		return fmt.Errorf("缺少 sauth 数据")
	}
	if c.G79LatestVersion == "" || c.patchResourcesHash == "" {
		return fmt.Errorf("缺少补丁信息，请重新初始化客户端")
	}

	required := map[string]string{
		"sdkuid":    sauthData.SDKUID,
		"sessionid": sauthData.SessionID,
		"udid":      sauthData.UDID,
		"deviceid":  sauthData.DeviceID,
	}
	for field, val := range required {
		if strings.TrimSpace(val) == "" {
			return fmt.Errorf("sauth_json 缺少字段 %s", field)
		}
	}

	seed := uuid.New().String()
	c.Seed = seed

	clientLoginSN, err := generateClientLoginSN()
	if err != nil {
		return fmt.Errorf("生成 client_login_sn 失败: %w", err)
	}

	sauthPayload := buildAndroidSauthPayload(sauthData, clientLoginSN)

	saDataPayload := buildAndroidSaDataPayload(c, sauthData, cookieData)
	saDataJSON, err := json.Marshal(saDataPayload)
	if err != nil {
		return fmt.Errorf("序列化 sa_data 失败: %w", err)
	}

	versionMessage := fmt.Sprintf("%s%s%s%s%s%s", c.EngineVersion, g79LibraryHash, c.G79LatestVersion, c.patchResourcesHash, g79SignatureHash, seed)
	sign, err := utils.PeAuthSign(versionMessage, sp, tr)
	if err != nil {
		return fmt.Errorf("计算签名失败: %w", err)
	}

	peauth := PeAuthData{
		EngineVersion: c.EngineVersion,
		ExtraParam:    "extra",
		Message:       versionMessage,
		PatchVersion:  c.G79LatestVersion,
		PayChannel:    g79AndroidMessageTag,
		SaData:        string(saDataJSON),
		SauthJSON:     sauthPayload,
		Seed:          seed,
		Sign:          sign,
	}

	payload, err := json.Marshal(peauth)
	if err != nil {
		return fmt.Errorf("序列化认证数据失败: %w", err)
	}

	encryptedPayload, err := G79HttpEncrypt(payload)
	if err != nil {
		return fmt.Errorf("加密认证数据失败: %w", err)
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.CoreServerURL+"/pe-authentication", strings.NewReader(hex.EncodeToString(encryptedPayload)))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return err
	}

	encryptedResp, err := hex.DecodeString(string(respBody))
	if err != nil {
		return err
	}

	decryptedResp, err := G79HttpDecrypt(encryptedResp)
	if err != nil {
		return err
	}

	validJSON := GetValidJSON(decryptedResp)

	var loginResp LoginResponse
	if err := json.Unmarshal(validJSON, &loginResp); err != nil {
		return fmt.Errorf("解析登录响应失败: %v, 响应内容: %s", err, string(validJSON))
	}

	if loginResp.Code != 0 {
		return fmt.Errorf("登录失败 (code: %d): %s", loginResp.Code, loginResp.Message)
	}

	c.SetCredentials(loginResp.Entity.EntityID, loginResp.Entity.Token)
	c.Seed = loginResp.Entity.Seed

	return nil
}

// 使用Cookie执行PE认证
func (c *Client) g79PerformPEAuthWithCookie(sauthData *SauthData, cookieData *CookieData) error {
	if sauthData == nil {
		return fmt.Errorf("缺少 sauth 数据")
	}
	if c.G79LatestVersion == "" || c.patchResourcesHash == "" {
		return fmt.Errorf("缺少补丁信息，请重新初始化客户端")
	}

	required := map[string]string{
		"sdkuid":    sauthData.SDKUID,
		"sessionid": sauthData.SessionID,
		"udid":      sauthData.UDID,
		"deviceid":  sauthData.DeviceID,
	}
	for field, val := range required {
		if strings.TrimSpace(val) == "" {
			return fmt.Errorf("sauth_json 缺少字段 %s", field)
		}
	}

	seed := uuid.New().String()
	c.Seed = seed

	clientLoginSN, err := generateClientLoginSN()
	if err != nil {
		return fmt.Errorf("生成 client_login_sn 失败: %w", err)
	}

	sauthPayload := buildAndroidSauthPayload(sauthData, clientLoginSN)
	//	_=sauthPayload

	saDataPayload := buildAndroidSaDataPayload(c, sauthData, cookieData)
	saDataJSON, err := json.Marshal(saDataPayload)
	if err != nil {
		return fmt.Errorf("序列化 sa_data 失败: %w", err)
	}

	versionMessage := fmt.Sprintf("%s%s%s%s%s%s", c.EngineVersion, g79LibraryHash, c.G79LatestVersion, c.patchResourcesHash, g79SignatureHash, seed)
	sign, err := utils.PeAuthSign(versionMessage, 3, 6) // 3.9 版本参数：sp=3, tr=6
	if err != nil {
		return fmt.Errorf("计算签名失败: %w", err)
	}

	//	var sauth map[string]any
	//	_ = json.Unmarshal([]byte(cookieData.SauthJSON), &sauth)
	peauth := PeAuthData{
		EngineVersion: c.EngineVersion,
		ExtraParam:    "extra",
		Message:       versionMessage,
		PatchVersion:  c.G79LatestVersion,
		PayChannel:    g79AndroidMessageTag,
		SaData:        string(saDataJSON),
		SauthJSON:     sauthPayload, //sauth,
		Seed:          seed,
		Sign:          sign,
	}

	payload, err := json.Marshal(peauth)
	if err != nil {
		return fmt.Errorf("序列化认证数据失败: %w", err)
	}
	//	fmt.Println(string(payload))

	encryptedPayload, err := G79HttpEncrypt(payload)
	if err != nil {
		return fmt.Errorf("加密认证数据失败: %w", err)
	}

	req, err := http.NewRequest("POST", c.ReleaseJSON.CoreServerURL+"/pe-authentication", strings.NewReader(hex.EncodeToString(encryptedPayload)))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return err
	}

	encryptedResp, err := hex.DecodeString(string(respBody))
	if err != nil {
		return err
	}

	decryptedResp, err := G79HttpDecrypt(encryptedResp)
	if err != nil {
		return err
	}

	validJSON := GetValidJSON(decryptedResp)
	//	fmt.Println(validJSON)

	var loginResp LoginResponse
	if err := json.Unmarshal(validJSON, &loginResp); err != nil {
		return fmt.Errorf("解析登录响应失败: %v, 响应内容: %s", err, string(validJSON))
	}

	if loginResp.Code != 0 {
		return fmt.Errorf("登录失败 (code: %d): %s", loginResp.Code, loginResp.Message)
	}

	c.SetCredentials(loginResp.Entity.EntityID, loginResp.Entity.Token)
	c.Seed = loginResp.Entity.Seed

	return nil
}

func generateClientLoginSN() (string, error) {
	buf, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(buf)), nil
}

func buildAndroidSauthPayload(sauthData *SauthData, clientLoginSN string) map[string]any {
	aimInfo := strings.TrimSpace(sauthData.AimInfo)
	if aimInfo == "" {
		aimInfo = `{"aim":"127.0.0.1","country":"CN","tz":"+0800","tzid":"","celluar_ip":"","operator":"","is_vpn_enabled":"false"}`
	}
	sdkVersion := strings.TrimSpace(sauthData.SDKVersion)
	if sdkVersion == "" {
		sdkVersion = "5.9.0"
	}
	ip := strings.TrimSpace(sauthData.IP)
	if ip == "" {
		ip = "127.0.0.1"
	}
	gameID := strings.TrimSpace(sauthData.GameID)
	if gameID == "" {
		gameID = "x19"
	}
	appChannel := strings.TrimSpace(sauthData.AppChannel)
	if appChannel == "" {
		appChannel = "netease"
	}
	loginChannel := strings.TrimSpace(sauthData.LoginChannel)
	if loginChannel == "" {
		loginChannel = appChannel
	}
	sourcePlatform := strings.TrimSpace(sauthData.SourcePlatform)
	if sourcePlatform == "" {
		sourcePlatform = "ad"
	}
	return map[string]any{
		"aim_info":           aimInfo,
		"app_channel":        appChannel,
		"client_login_sn":    clientLoginSN,
		"deviceid":           sauthData.DeviceID,
		"gameid":             gameID,
		"gas_token":          sauthData.GasToken,
		"get_access_token":   "1",
		"ip":                 ip,
		"is_unisdk_guest":    0,
		"login_channel":      loginChannel,
		"platform":           "ad",
		"sdk_version":        sdkVersion,
		"sdkuid":             sauthData.SDKUID,
		"sessionid":          sauthData.SessionID,
		"source_app_channel": appChannel,
		"source_platform":    sourcePlatform,
		"step":               g79AndroidStep,
		"step2":              g79AndroidStep2,
		"udid":               sauthData.UDID,
	}
}

func buildAndroidSaDataPayload(c *Client, sauthData *SauthData, cookieData *CookieData) map[string]any {
	sdkVersion := strings.TrimSpace(sauthData.SDKVersion)
	if sdkVersion == "" {
		sdkVersion = "5.16.0" // 3.9对应SDK版本
	}
	udid := sauthData.UDID
	if strings.TrimSpace(udid) == "" {
		udid = "0000000000000000"
	}
	appChannel := strings.TrimSpace(sauthData.AppChannel)
	if appChannel == "" {
		appChannel = "netease"
	}
	appVer := c.G79LatestVersion
	if appVer == "" {
		appVer = "3.9.23.298289"
	}

	return map[string]any{
		"app_channel":   appChannel,
		"app_ver":       appVer,
		"core_num":      "u0004",
		"cpu_digit":     "64",
		"cpu_hz":        "2465600",
		"cpu_name":      "placeholder",
		"device_height": "2400",
		"device_model":  "Xiaomi#23116PN5BC",
		"device_width":  "1080",
		"disk":          "",
		"emulator":      cookieData.emulatorFlag(),
		"first_udid":    udid,
		"is_guest":      cookieData.isGuestFlag(),
		"launcher_type": "PE_C++",
		"mac_addr":      cookieData.macAddress(),
		"network":       "CHANNEL_UNKNOW",
		"os_name":       "android",
		"os_ver":        "13",
		"ram":           cookieData.ramValue(),
		"rom":           cookieData.romValue(),
		"root":          false,
		"sdk_ver":       sdkVersion,
		"start_type":    "default",
		"udid":          udid,
	}
}

func (c *Client) G79AuthenticateWithPeAuth(peAuthHexStr string) error {
	peAuthEncryptedBytes, err := hex.DecodeString(peAuthHexStr)
	if err != nil {
		return fmt.Errorf("解析 PeAuth 失败: %v", err)
	}
	peAuthBytes, err := G79HttpDecrypt(peAuthEncryptedBytes)
	if err != nil {
		return fmt.Errorf("解密 PeAuth 失败: %v", err)
	}
	peAuthBytes = GetValidJSON(peAuthBytes)
	var peAuthData PeAuthData
	err = json.Unmarshal(peAuthBytes, &peAuthData)
	if err != nil {
		return fmt.Errorf("解析 PeAuth JSON 失败: %v", err)
	}

	// 发送认证请求
	sauthJSON, _ := json.Marshal(peAuthData.SauthJSON)
	cookieData := CookieData{
		SauthJSON: string(sauthJSON),
	}
	cookieStr, _ := json.Marshal(cookieData)
	c.Cookie = string(cookieStr)
	err = c.G79AuthenticateWithCookie(string(cookieStr))
	if err != nil {
		return fmt.Errorf("PE认证失败: %v", err)
	}
	return nil
}

type X19LoginOTPResponse struct {
	Response
	Entity X19LoginOTPEntity `json:"entity"`
}

type X19LoginOTPEntity struct {
	OTP          int    `json:"otp"`
	OTPToken     string `json:"otp_token"`
	AID          int    `json:"aid"`
	LockTime     int    `json:"lock_time"`
	OpenOTP      int    `json:"open_otp"`
	VerifyStatus int    `json:"verify_status"`
}

const charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// RandomStr 生成指定长度的随机字符串
func RandomStr(l int) string {
	// 初始化随机数生成器（只执行一次）
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// 预分配内存
	result := make([]byte, l)
	for i := range result {
		// 从 charset 中随机选一个字符
		result[i] = charset[rng.Intn(len(charset))]
	}
	return string(result)
}

func (c *Client) X19AuthenticateWithCookie(cookieStr string) error {
	c.Cookie = cookieStr
	// 解析Cookie
	var cookieData CookieData
	err := json.Unmarshal([]byte(cookieStr), &cookieData)
	if err != nil {
		return fmt.Errorf("解析Cookie失败: %v", err)
	}
	// 解析 sauth_json
	var sauthData SauthData
	err = json.Unmarshal([]byte(cookieData.SauthJSON), &sauthData)
	if err != nil {
		return fmt.Errorf("解析 sauth_json 失败: %v", err)
	}

	// 发送认证请求
	req, err := http.NewRequest("POST", c.X19ReleaseJSON.CoreServerURL+"/login-otp", strings.NewReader(cookieStr))
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := readResponseBody(resp)
	if err != nil {
		return err
	}
	var loginResp X19LoginOTPResponse
	if err := json.Unmarshal(respBody, &loginResp); err != nil {
		return fmt.Errorf("解析登录响应失败: %v, 响应内容: %s", err, string(respBody))
	}

	if loginResp.Code != 0 {
		return fmt.Errorf("登录失败 (code: %d): %s", loginResp.Code, loginResp.Message)
	}
	if loginResp.Entity.OTPToken == "" {
		return fmt.Errorf("登录响应缺少 otp_token")
	}

	const x19AuthOTPVersion = "1.14.6.45947"

	saData := map[string]any{
		"os_name":       "windows",
		"os_ver":        "Microsoft Windows 10",
		"mac_addr":      RandomStr(11),
		"udid":          RandomStr(23),
		"app_ver":       x19AuthOTPVersion,
		"sdk_ver":       "",
		"network":       "",
		"disk":          RandomStr(8),
		"is64bit":       "1",
		"video_card1":   "Video_cardl",
		"video_card2":   "",
		"video_card3":   "",
		"video_card4":   "",
		"launcher_type": "PC_java",
		"pay_channel":   "4399pc",
		"dotnet_ver":    "4.8.0",
		"cpu_type":      "Intel(R) Core(R) CPU i57200U 2.50GHz",
		"ram_size":      "21474836480",
		"device_width":  "1366",
		"device_height": "768",
		"os_detail":     "10",
	}
	saDataJSON, err := json.Marshal(saData)
	if err != nil {
		return fmt.Errorf("序列化 sa_data 失败: %v", err)
	}

	aimInfoJSON, err := json.Marshal(map[string]any{
		"aim":     "100.100.100.100",
		"country": "CN",
		"tz":      "+0800",
		"tzid":    "",
	})
	if err != nil {
		return fmt.Errorf("序列化 aim_info 失败: %v", err)
	}

	saAuthJSONBytes, err := json.Marshal(map[string]any{
		"gameid":        "x19",
		"login_channel": "netease",
		"app_channel":   "netease",
		"platform":      "pc",
		"sdkuid":        sauthData.SDKUID,
		"sessionid":     sauthData.SessionID,
		"sdk_version":   "4.10.0",
		"udid":          sauthData.UDID,
		"deviceid":      sauthData.DeviceID,
		"aim_info":      string(aimInfoJSON),
	})
	if err != nil {
		return fmt.Errorf("序列化 sauth_json 失败: %v", err)
	}

	authData := map[string]any{
		"sa_data":    string(saDataJSON),
		"sauth_json": string(saAuthJSONBytes),
		"version": map[string]any{
			"version":      x19AuthOTPVersion,
			"launcher_md5": "",
			"updater_md5":  "",
		},
		"otp_token":          loginResp.Entity.OTPToken,
		"aid":                strconv.Itoa(loginResp.Entity.AID),
		"sdkuid":             "",
		"hasMessage":         false,
		"hasGmail":           false,
		"otp_pwd":            loginResp.Entity.OTPToken,
		"lock_time":          0,
		"env":                "",
		"min_engine_version": "",
		"min_patch_version":  "",
		"unisdk_login_json":  "",
		"verify_status":      loginResp.Entity.VerifyStatus,
		"token":              "",
		"is_register":        true,
		"entity_id":          0,
	}

	authPayloadBytes, err := json.Marshal(authData)
	if err != nil {
		return fmt.Errorf("序列化认证载荷失败: %v", err)
	}
	authPayload := string(authPayloadBytes)
	encryptedAuthPayload, err := X19HttpEncrypt(authPayloadBytes)
	if err != nil {
		return fmt.Errorf("加密认证载荷失败: %v", err)
	}
	authReq, err := http.NewRequest("POST", c.X19ReleaseJSON.CoreServerURL+"/authentication-otp", bytes.NewReader(encryptedAuthPayload))
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}
	authReq.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	authReq.Header.Set("Accept-Encoding", "gzip")
	authReq.Header.Set("Content-Type", "application/json")
	authReq.Header.Set("user-id", "")
	authReq.Header.Set("user-token", CalculateDynamicToken("/authentication-otp", authPayload, ""))

	authResp, err := c.do(authReq)
	if err != nil {
		return fmt.Errorf("发送请求失败: %v", err)
	}
	defer authResp.Body.Close()

	authRespBody, err := readResponseBody(authResp)
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}

	decryptedAuthResp, err := X19HttpDecrypt(authRespBody)
	if err != nil {
		return fmt.Errorf("解密响应失败: %v", err)
	}

	validAuthRespJSON := GetValidJSON(decryptedAuthResp)
	var loginRespData LoginResponse
	if err := json.Unmarshal(validAuthRespJSON, &loginRespData); err != nil {
		return fmt.Errorf("解析登录响应失败: %v", err)
	}
	if loginRespData.Code != 0 {
		return fmt.Errorf("登录失败 (code: %d): %s", loginRespData.Code, loginRespData.Message)
	}

	// 设置用户凭证
	c.SetCredentials(loginRespData.Entity.EntityID, loginRespData.Entity.Token)
	c.Seed = loginRespData.Entity.Seed

	// 获取用户详情
	userDetail, err := c.GetUserDetail()
	if err != nil {
		return fmt.Errorf("获取用户信息失败: %v", err)
	}
	c.UserDetail = &userDetail.Entity
	c.ReleaseJSON.ApiGatewayUrl = c.X19ReleaseJSON.ApiGatewayGrayURL
	c.IsX19 = true
	return nil
}

var OSName = "android"

// 生成租赁服认证v2数据
func (c *Client) GenerateRentalGameAuthV2(serverID, clientKey string) ([]byte, error) {
	uid, err := c.GetUserIDInt()
	if err != nil {
		return nil, err
	}

	authv2 := map[string]any{
		"bit":           "64",
		"clientKey":     clientKey,
		"displayName":   c.UserDetail.Name,
		"engineVersion": c.EngineVersion,
		"netease_sid":   fmt.Sprintf("%s:RentalGame", serverID),
		"os_name":       OSName,
		"patchVersion":  c.G79LatestVersion,
		"uid":           uid,
	}

	return json.Marshal(authv2)
}

// 生成山头认证v2数据
func (c *Client) GenerateDomainGameAuthV2(serverID, clientKey string) ([]byte, error) {
	uid, err := c.GetUserIDInt()
	if err != nil {
		return nil, err
	}

	authv2 := map[string]any{
		"bit":           "64",
		"clientKey":     clientKey,
		"displayName":   c.UserDetail.Name,
		"engineVersion": c.EngineVersion,
		"netease_sid":   fmt.Sprintf("%s:DomainGame", serverID),
		"os_name":       OSName,
		"patchVersion":  c.G79LatestVersion,
		"uid":           uid,
	}

	return json.Marshal(authv2)
}

// 生成PC山头认证v2数据
func (c *Client) GeneratePCDomainGameAuthV2(serverID, clientKey string) ([]byte, error) {
	uid, err := c.GetUserIDInt()
	if err != nil {
		return nil, err
	}

	authv2 := map[string]any{
		"bit":           "64",
		"clientKey":     clientKey,
		"displayName":   c.UserDetail.Name,
		"engineVersion": c.EngineVersion,
		"netease_sid":   fmt.Sprintf("%s:DomainGame", serverID),
		"os_name":       "windows",
		"patchVersion":  "",
		"pcCheck":       "0",
		"platform":      "pc",
		"uid":           uid,
	}

	return json.Marshal(authv2)
}

// 生成大厅游戏认证v2数据（netease_sid: roomID:LobbyGame）
func (c *Client) GenerateLobbyGameAuthV2(roomID, clientKey string) ([]byte, error) {
	uid, err := c.GetUserIDInt()
	if err != nil {
		return nil, err
	}

	authv2 := map[string]any{
		"bit":           "64",
		"clientKey":     clientKey,
		"displayName":   c.UserDetail.Name,
		"engineVersion": c.EngineVersion,
		"netease_sid":   fmt.Sprintf("%s:LobbyGame", roomID),
		"os_name":       OSName,
		"patchVersion":  c.G79LatestVersion,
		"uid":           uid,
	}

	return json.Marshal(authv2)
}

// 生成PC大厅游戏认证v2数据（netease_sid: roomID:LobbyGame）
func (c *Client) GeneratePCLobbyGameAuthV2(roomID, clientKey string) ([]byte, error) {
	uid, err := c.GetUserIDInt()
	if err != nil {
		return nil, err
	}

	authv2 := map[string]any{
		"bit":           "64",
		"clientKey":     clientKey,
		"displayName":   c.UserDetail.Name,
		"engineVersion": c.EngineVersion,
		"netease_sid":   fmt.Sprintf("%s:LobbyGame", roomID),
		"os_name":       "windows",
		"patchVersion":  "",
		"pcCheck":       "0",
		"platform":      "pc",
		"uid":           uid,
	}

	return json.Marshal(authv2)
}

// 生成网络游戏认证v2数据（netease_sid: roomID:NetworkGame）
func (c *Client) GenerateNetworkGameAuthV2(roomID, clientKey string) ([]byte, error) {
	uid, err := c.GetUserIDInt()
	if err != nil {
		return nil, err
	}

	authv2 := map[string]any{
		"bit":           "64",
		"clientKey":     clientKey,
		"displayName":   c.UserDetail.Name,
		"engineVersion": c.EngineVersion,
		"netease_sid":   fmt.Sprintf("%s:NetworkGame", roomID),
		"os_name":       OSName,
		"patchVersion":  c.G79LatestVersion,
		"uid":           uid,
	}

	return json.Marshal(authv2)
}

// 发送认证v2请求
func (c *Client) SendAuthV2Request(authv2Data []byte) ([]byte, error) {
	// 实测 NetEase 官方：
	//   POST https://g79authobt.minecraft.cn/authentication-v2
	// 才是 G79 (OBT) 的 chain-info /authentication-v2 接口（其他域名+路径都是 404）。
	// 用占位 body / 占位 user-token / 占位 user-id 打它返回 500 空 body（content-length:0），
	// 属于"服务端解密/校验失败内部错误"，意味着：
	//   (a) 路径正确 (✅),
	//   (b) 请求只要签名/body/Link-Rental 绑定任一对不上就会 500 吞错误。
	// 因此这里不要穷举 27 条候选（会让你误判全 404），只在最常见的 4~5 条精确组合里回退。
	pathCandidates := []string{"/authentication-v2", "/authentication-v2/"}

	encryptedData, err := G79HttpEncrypt(authv2Data)
	if err != nil {
		return nil, err
	}
	hexBody := hex.EncodeToString(encryptedData)

	type attempt struct {
		label       string
		base        string
		path        string
		contentType string
	}
	candidates := []attempt{}
	addCandidate := func(label, base, path, ct string) {
		base = strings.TrimRight(base, "/")
		if base == "" || path == "" {
			return
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		for _, c := range candidates {
			if c.base == base && c.path == path && c.contentType == ct {
				return
			}
		}
		candidates = append(candidates, attempt{label: label, base: base, path: path, contentType: ct})
	}

	for _, p := range pathCandidates {
		if c.IsX19 {
			addCandidate("X19 CoreServer", c.X19ReleaseJSON.CoreServerURL, p, "text/plain; charset=utf-8")
			addCandidate("X19 AuthServer", c.X19ReleaseJSON.AuthServerURL, p, "text/plain; charset=utf-8")
			addCandidate("X19 ApiGatewayGray", c.X19ReleaseJSON.ApiGatewayGrayURL, p, "text/plain; charset=utf-8")
			addCandidate("X19 AuthServerCpp", c.X19ReleaseJSON.AuthServerCppURL, p, "text/plain; charset=utf-8")
			addCandidate("X19 CoreServer(application/json)", c.X19ReleaseJSON.CoreServerURL, p, "application/json")
		} else {
			// ⚠️ 重点：G79 租赁服 chain-info 接口就挂在 AuthServerURL 下，优先级必须第一。
			addCandidate("G79 AuthServer", c.ReleaseJSON.AuthServerURL, p, "text/plain; charset=utf-8")
			addCandidate("G79 AuthServer(application/json)", c.ReleaseJSON.AuthServerURL, p, "application/json")
			addCandidate("G79 ApiGateway", c.ReleaseJSON.ApiGatewayUrl, p, "text/plain; charset=utf-8")
			addCandidate("G79 ApiGatewayGray", c.ReleaseJSON.ApiGatewayGrayUrl, p, "text/plain; charset=utf-8")
			addCandidate("G79 CoreServer", c.ReleaseJSON.CoreServerURL, p, "text/plain; charset=utf-8")
			addCandidate("G79 CoreServerGray", c.ReleaseJSON.CoreServerGrayURL, p, "text/plain; charset=utf-8")
			addCandidate("G79 AuthServer(application/x-www-form-urlencoded)", c.ReleaseJSON.AuthServerURL, p, "application/x-www-form-urlencoded")
		}
	}

	var lastErr error
	for i, cand := range candidates {
		fullURL := cand.base + cand.path
		userTokenSign := CalculateDynamicToken(cand.path, string(authv2Data), c.UserToken)
		req, rerr := http.NewRequest("POST", fullURL, strings.NewReader(hexBody))
		if rerr != nil {
			lastErr = fmt.Errorf("[尝试%d/%d %s] build request: %w", i+1, len(candidates), cand.label, rerr)
			continue
		}
		req.Header.Set("User-Agent", "WPFLauncher/0.0.0.0")
		req.Header.Set("Accept-Encoding", "gzip")
		req.Header.Set("Content-Type", cand.contentType)
		req.Header.Set("user-id", c.UserID)
		req.Header.Set("user-token", userTokenSign)

		resp, derr := c.httpClient.Do(req)
		if derr != nil {
			lastErr = fmt.Errorf("[尝试%d/%d %s] POST %s (ct=%s): do=%w",
				i+1, len(candidates), cand.label, fullURL, cand.contentType, derr)
			continue
		}
		respBody, rerr := readResponseBody(resp)
		_ = resp.Body.Close()
		if rerr != nil {
			lastErr = fmt.Errorf("[尝试%d/%d %s] POST %s (ct=%s): read body=%w",
				i+1, len(candidates), cand.label, fullURL, cand.contentType, rerr)
			continue
		}
		if resp.StatusCode == 200 && len(respBody) > 0 {
			encryptedResp, herr := hex.DecodeString(string(respBody))
			if herr != nil {
				lastErr = fmt.Errorf("[尝试%d/%d %s] POST %s (ct=%s): status=200 但 body 非 hex=%q: %w",
					i+1, len(candidates), cand.label, fullURL, cand.contentType, trimForErr(respBody), herr)
				continue
			}
			decryptedResp, derr2 := G79HttpDecrypt(encryptedResp)
			if derr2 != nil {
				lastErr = fmt.Errorf("[尝试%d/%d %s] POST %s (ct=%s): status=200 解密失败: %w",
					i+1, len(candidates), cand.label, fullURL, cand.contentType, derr2)
				continue
			}
			return GetValidJSON(decryptedResp), nil
		}
		bodyStr := trimForErr(respBody)
		switch resp.StatusCode {
		case 401:
			lastErr = fmt.Errorf("[尝试%d/%d %s] AuthV2未授权: status=%d url=%s ct=%s body=%q。 "+
				"请先完成Link连接 + SendGameStart 再调用 AuthV2，或重新登录 G79 刷新 UserToken",
				i+1, len(candidates), cand.label, resp.StatusCode, fullURL, cand.contentType, bodyStr)
		case 403:
			lastErr = fmt.Errorf("[尝试%d/%d %s] AuthV2被拒绝: status=%d url=%s ct=%s body=%q。 "+
				"可能是租赁服未开 / 服务器号无效 / UserToken 无权限（重新登录获取新Token再试）",
				i+1, len(candidates), cand.label, resp.StatusCode, fullURL, cand.contentType, bodyStr)
		case 404:
			lastErr = fmt.Errorf("[尝试%d/%d %s] AuthV2路径不存在: status=404 url=%s ct=%s body=%q。 "+
				"会继续自动回退其他组合；如果所有候选都 404，请把完整错误贴给开发者追加 BaseURL。",
				i+1, len(candidates), cand.label, fullURL, cand.contentType, bodyStr)
		case 400:
			lastErr = fmt.Errorf("[尝试%d/%d %s] AuthV2请求格式错误: status=%d url=%s ct=%s body=%q。 "+
				"路径已对，重点排查：(1)netease_sid 应为 serverID:RentalGame；(2)Link GameStart 后是否进入了租赁服房间（EnterRentalServerWorld 是否 code=0）；(3)ClientKey 是否是合法 ECC P384 SPKI",
				i+1, len(candidates), cand.label, resp.StatusCode, fullURL, cand.contentType, bodyStr)
		case 500, 502, 503, 504:
			// NetEase G79 AuthServer 对"解密失败 / 缺少 Link 租赁服会话绑定 / user-token 签名错 / 请求体错"都统一返回 500 空 body，
			// 并不一定是服务端宕机。只要你看到走的是 G79 AuthServer + /authentication-v2，就说明路径正确，接下来需要核对：
			//   - UserToken 是否在登录后未过期（重新 SAuth 登录一般能修）
			//   - Link 是否已建立并 SendGameStart 成功
			//   - EnterRentalServerWorld 是否成功（mcserver_host/mcserver_port 都返回了）
			//   - ClientKey 是否合法 ECC P384 (P-384) SPKI（许多客户端需要你主动握手交换公钥，WebUI 默认自动生成一枚新的）
			lastErr = fmt.Errorf("[尝试%d/%d %s] AuthV2服务端返回内部错误: status=%d url=%s ct=%s body=%q。 "+
				"注意：该 status=500 并不一定是网易服务器崩，更多是「请求体解密失败 / user-token 签名错误 / Link 缺少租赁服会话绑定 / ClientKey 非法」。 "+
				"请依次核对：(1)先用 SAuth 重新登录刷新 UserToken；(2)Link 必须显示「Link + GameStart 成功」；"+
				"(3)先点「Enter 租赁服」确保 EnterRentalServerWorld code=0 且有 mcserver_host/port；(4)再点 AuthV2。",
				i+1, len(candidates), cand.label, resp.StatusCode, fullURL, cand.contentType, bodyStr)
		default:
			lastErr = fmt.Errorf("[尝试%d/%d %s] AuthV2响应异常: status=%d url=%s ct=%s body=%q",
				i+1, len(candidates), cand.label, resp.StatusCode, fullURL, cand.contentType, bodyStr)
		}
	}
	if lastErr == nil {
		return nil, fmt.Errorf("AuthV2: 未构造出任何候选请求（ReleaseJSON 字段均为空，请先 NewClient 初始化）")
	}
	return nil, lastErr
}

// trimForErr 把日志里的 body 限制到合理长度，避免整包 hex 刷满错误信息。
func trimForErr(b []byte) string {
	const max = 512
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + fmt.Sprintf("...(len=%d)", len(b))
}
