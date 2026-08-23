package com4399

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	apiBaseURL        = "https://m.4399api.com"
	loginBaseURL      = "https://ptlogin.4399.com"
	requestTimeout    = 15 * time.Second
	defaultUserAgent  = "Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/120.0 Mobile Safari/537.36"
	oauthSDKVersion   = "3.12.2.503"
	oauthGameKey      = "115716"
	oauthCallbackPath = "/openapi/oauth-callback.html?gamekey=44770&game_key=115716"
)

var captchaIDPattern = regexp.MustCompile(`(?i)name\s*=\s*["']captcha_id["']\s+value\s*=\s*["']([^"']+)["']`)
var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

// Client 封装 4399 手游渠道 OAuth 登录流程。
type Client struct {
	httpClient *http.Client
	apiBaseURL string
	loginURL   string
	userAgent  string
}

// NewClient 创建默认 4399 客户端。
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: requestTimeout}
	}
	if httpClient.CheckRedirect == nil {
		httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return &Client{
		httpClient: httpClient,
		apiBaseURL: apiBaseURL,
		loginURL:   loginBaseURL,
		userAgent:  defaultUserAgent,
	}
}

// SetUserAgent 允许自定义请求使用的 User-Agent。
func (c *Client) SetUserAgent(userAgent string) {
	if strings.TrimSpace(userAgent) != "" {
		c.userAgent = userAgent
	}
}

// RegisterDevice 注册 4399 OAuth 设备信息。
func RegisterDevice(ctx context.Context) (*Device, error) {
	return NewClient(nil).RegisterDevice(ctx)
}

// LoginWithPassword 使用默认客户端登录，并返回可生成 Cookie 的用户对象。
func LoginWithPassword(ctx context.Context, username, password string) (*User, error) {
	return NewClient(nil).LoginWithPassword(ctx, username, password)
}

// LoginCookieWithPassword 使用默认客户端登录，并直接返回 g79client 可消费的 Cookie 字符串。
func LoginCookieWithPassword(ctx context.Context, username, password string) (string, error) {
	user, err := LoginWithPassword(ctx, username, password)
	if err != nil {
		return "", err
	}
	return user.CookieString()
}

// RegisterCookieWithPassword 注册临时 4399 OAuth 设备、完成密码登录，并返回 Cookie 字符串。
func RegisterCookieWithPassword(ctx context.Context, username, password string) (string, error) {
	return NewClient(nil).RegisterCookieWithPassword(ctx, username, password)
}

// RegisterDevice 注册 4399 OAuth 设备信息。
func (c *Client) RegisterDevice(ctx context.Context) (*Device, error) {
	if c == nil {
		return nil, errors.New("com4399: client 未初始化")
	}
	device := &Device{
		DeviceIdentifier:   generateIdentifier(),
		DeviceIdentifierSM: generateIdentifier(),
		UDID:               uuid.NewString(),
		client:             c,
	}
	state, err := device.oauth(ctx)
	if err != nil {
		return nil, err
	}
	device.State = state
	return device, nil
}

// LoginWithPassword 注册临时设备并用密码登录 4399。
func (c *Client) LoginWithPassword(ctx context.Context, username, password string) (*User, error) {
	device, err := c.RegisterDevice(ctx)
	if err != nil {
		return nil, err
	}
	return device.LoginWithPassword(ctx, username, password)
}

// RegisterCookieWithPassword 注册临时 4399 OAuth 设备、完成密码登录，并返回 Cookie 字符串。
func (c *Client) RegisterCookieWithPassword(ctx context.Context, username, password string) (string, error) {
	user, err := c.LoginWithPassword(ctx, username, password)
	if err != nil {
		return "", err
	}
	return user.CookieString()
}

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, int, http.Header, error) {
	return c.do(ctx, http.MethodGet, rawURL, "", "")
}

func (c *Client) postForm(ctx context.Context, rawURL string, form url.Values) ([]byte, int, http.Header, error) {
	return c.do(ctx, http.MethodPost, rawURL, form.Encode(), "application/x-www-form-urlencoded")
}

func (c *Client) do(ctx context.Context, method, rawURL, body, contentType string) ([]byte, int, http.Header, error) {
	if c == nil {
		return nil, 0, nil, errors.New("com4399: client 未初始化")
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, strings.NewReader(body))
	if err != nil {
		return nil, 0, nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, resp.Header, err
	}
	return payload, resp.StatusCode, resp.Header, nil
}

// Device 描述 4399 OAuth 设备凭据。
type Device struct {
	DeviceIdentifier   string `json:"device-id"`
	DeviceIdentifierSM string `json:"device-id-sm"`
	UDID               string `json:"device-udid"`
	State              string `json:"device-state"`

	client *Client
}

// LoginOptions 描述 4399 密码登录附加参数。
type LoginOptions struct {
	Captcha   string
	CaptchaID string
	Retry     int // 验证码相关重试（上限 2）
	// PleaseWaitRetry 用于"请稍后再试"类风控重试，独立于验证码重试
	// 上限由 loginMaxPleaseWaitRetries 控制，不占 Retry 名额
	PleaseWaitRetry int
}

const (
	loginMaxPleaseWaitRetries  = 5
	loginPleaseWaitRetryDelay  = 5 * time.Second
)

// LoginWithPassword 使用设备凭据登录 4399。
func (d *Device) LoginWithPassword(ctx context.Context, username, password string) (*User, error) {
	return d.LoginWithPasswordOptions(ctx, username, password, LoginOptions{})
}

// LoginWithPasswordOptions 使用设备凭据登录 4399，支持携带验证码。
func (d *Device) LoginWithPasswordOptions(ctx context.Context, username, password string, options LoginOptions) (*User, error) {
	if err := d.ensureClient(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(username) == "" || password == "" {
		return nil, errors.New("com4399: username 与 password 不能为空")
	}
	if options.Retry > 2 {
		return nil, errors.New("com4399: 登录重试次数过多")
	}

	state, err := d.client.requestState(ctx)
	if err != nil {
		return nil, err
	}
	form := buildLoginForm(strings.ToLower(username), password, state, d.DeviceIdentifier)
	if options.Captcha != "" && options.CaptchaID != "" {
		form.Set("captcha", options.Captcha)
		form.Set("captcha_id", options.CaptchaID)
	}

	loginEndpoint := fmt.Sprintf("%s/oauth2/loginAndAuthorize.do?channel=&sdk=op&sdk_version=%s", d.client.loginURL, oauthSDKVersion)
	body, _, headers, err := d.client.postForm(ctx, loginEndpoint, form)
	if err != nil {
		return nil, err
	}
	html := string(body)
	if strings.Contains(html, "验证码") {
		captchaErr := newNeedCaptchaError(html)
		// 尝试自动 OCR 识别
		if needCaptcha, ok := captchaErr.(*NeedCaptchaError); ok && needCaptcha.CaptchaURL != "" {
			if ocrResult, ocrErr := RecognizeCaptcha(needCaptcha.CaptchaURL); ocrErr == nil {
				log.Printf("[OCR] 登录验证码自动识别结果: %s", ocrResult)
				options.Captcha = ocrResult
				options.CaptchaID = needCaptcha.CaptchaID
				options.Retry++
				return d.LoginWithPasswordOptions(ctx, username, password, options)
			} else {
				log.Printf("[OCR] 登录验证码自动识别失败: %v", ocrErr)
			}
		}
		return nil, captchaErr
	}

	location := headers.Get("Location")
	if location == "" {
		message := extractHTMLMessage(html)
		if isAccountNotFoundPage(html, message) {
			return nil, &AccountNotFoundError{Username: username, Message: message}
		}
		if message == "" {
			message = preview(html)
		}
		// ---------- 请稍后再试 自动重试 ----------
		if isLoginPleaseWait(message, html) && options.PleaseWaitRetry < loginMaxPleaseWaitRetries {
			log.Printf("[4399登录] 请稍后再试 第%d/%d次，等待%v后重新登录… (msg=%q)",
				options.PleaseWaitRetry+1, loginMaxPleaseWaitRetries, loginPleaseWaitRetryDelay, message)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(loginPleaseWaitRetryDelay):
			}
			options.PleaseWaitRetry++
			// 清空 captcha，避免旧 captcha 干扰；重新 requestState 拿新 state/deviceId
			options.Captcha = ""
			options.CaptchaID = ""
			return d.LoginWithPasswordOptions(ctx, username, password, options)
		}
		// 添加调试输出
		log.Printf("[DEBUG] 登录失败,响应状态码: %d", 200)
		log.Printf("[DEBUG] 响应头: %v", headers)
		log.Printf("[DEBUG] HTML前500字符: %s", preview(html))
		log.Printf("[DEBUG] 提取的错误消息: %s", message)
		return nil, fmt.Errorf("com4399: 登录失败，响应缺少跳转地址: %s", message)
	}
	callbackURL, err := d.resolveLoginURL(location)
	if err != nil {
		return nil, err
	}
	if options.Captcha != "" && options.CaptchaID == "" {
		values := callbackURL.Query()
		values.Set("captcha", options.Captcha)
		callbackURL.RawQuery = values.Encode()
	}

	body, status, _, err := d.client.get(ctx, callbackURL.String())
	if err != nil {
		return nil, err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("com4399: OAuth 回调失败 status=%d body=%s", status, string(body))
	}
	content := string(body)
	if strings.Contains(content, "登录状态已失效，请重新登录") {
		state, err := d.oauth(ctx)
		if err != nil {
			return nil, err
		}
		d.State = state
		options.Retry++
		return d.LoginWithPasswordOptions(ctx, username, password, options)
	}
	if strings.Contains(content, "登录成功，但账号存在异常，需要验证") {
		return nil, &NeedCaptchaError{Reason: "登录成功，但账号存在异常，需要验证"}
	}

	var userInfo userInfoResponse
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, fmt.Errorf("com4399: 解析用户信息失败: %w", err)
	}
	if userInfo.Code != "100" || userInfo.Result == nil {
		return nil, fmt.Errorf("com4399: 登录失败: %s", string(body))
	}
	return newUser(userInfo.Result), nil
}

func (d *Device) oauth(ctx context.Context) (string, error) {
	deviceJSON, err := json.Marshal(oauthDevice{
		DeviceIdentifier:   d.DeviceIdentifier,
		DeviceIdentifierSM: d.DeviceIdentifierSM,
		UDID:               d.UDID,
	})
	if err != nil {
		return "", err
	}
	form := url.Values{}
	form.Set("usernames", "")
	form.Set("top_bar", "1")
	form.Set("state", "")
	form.Set("device", string(deviceJSON))

	body, status, _, err := d.client.postForm(ctx, d.client.apiBaseURL+"/openapiv2/oauth.html", form)
	if err != nil {
		return "", err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return "", fmt.Errorf("com4399: 注册 OAuth 设备失败 status=%d body=%s", status, string(body))
	}

	var resp oauthResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	if resp.Result == nil {
		return "", fmt.Errorf("com4399: OAuth 设备响应缺少 result: %s", string(body))
	}
	var result oauthResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("com4399: 解析 OAuth result 失败: %w", err)
	}
	state := queryValue(result.LoginURL, "state")
	if state == "" {
		return "", fmt.Errorf("com4399: OAuth 响应缺少 state: %s", string(body))
	}
	return state, nil
}

func (d *Device) ensureClient() error {
	if d == nil {
		return errors.New("com4399: 设备未初始化")
	}
	if d.client == nil {
		d.client = NewClient(nil)
	}
	if d.DeviceIdentifier == "" || d.DeviceIdentifierSM == "" || d.UDID == "" {
		return errors.New("com4399: 设备信息不完整")
	}
	return nil
}

func (d *Device) resolveLoginURL(location string) (*url.URL, error) {
	parsed, err := url.Parse(location)
	if err != nil {
		return nil, err
	}
	if parsed.IsAbs() {
		return parsed, nil
	}
	base, err := url.Parse(d.client.loginURL)
	if err != nil {
		return nil, err
	}
	return base.ResolveReference(parsed), nil
}

func (c *Client) requestState(ctx context.Context) (string, error) {
	body, status, _, err := c.get(ctx, c.apiBaseURL+oauthCallbackPath)
	if err != nil {
		return "", err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return "", fmt.Errorf("com4399: 请求 OAuth state 失败 status=%d body=%s", status, string(body))
	}
	var callback oauthCallback
	if err := json.Unmarshal(body, &callback); err != nil {
		return "", err
	}
	state := queryValue(callback.Result, "state")
	if state == "" {
		return "", fmt.Errorf("com4399: OAuth callback 缺少 state: %s", string(body))
	}
	return state, nil
}

func buildLoginForm(username, password, state, deviceIdentifier string) url.Values {
	form := url.Values{}
	form.Set("isInputRealname", "false")
	form.Set("isValidRealname", "false")
	form.Set("sec", "1")
	form.Set("password", password)
	form.Set("username", username)
	form.Set("css", "")
	form.Set("show_close_button", "")
	form.Set("response_type", "TOKEN")
	form.Set("client_id", "40f9e9b95d6c71ba5c6e0bd14c0abeff")
	form.Set("show_4399", "")
	form.Set("username_history", "")
	form.Set("uid", "")
	form.Set("expand_ext_login_list", "")
	form.Set("ref", `{"game":"115716","channel":""}`)
	form.Set("autoCreateAccount", "")
	form.Set("scope", "basic")
	form.Set("bizId", "2100001792")
	form.Set("state", state)
	form.Set("show_ext_login", "")
	form.Set("reg_mode", "reg_phone")
	form.Set("_d", deviceIdentifier)
	form.Set("show_back_button", "")
	form.Set("auto_scroll", "")
	form.Set("access_token", "")
	form.Set("show_forget_password", "")
	form.Set("auth_action", "ORILOGIN")
	form.Set("redirect_uri", "https://m.4399api.com/openapi/oauth-callback.html?gamekey=44770&game_key=115716")
	form.Set("show_topbar", "false")
	form.Set("aid", "")
	form.Set("cid", "")
	return form
}

func queryValue(rawURL, key string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		values, parseErr := url.ParseQuery(strings.TrimPrefix(rawURL, "?"))
		if parseErr != nil {
			return ""
		}
		return values.Get(key)
	}
	return parsed.Query().Get(key)
}

func newNeedCaptchaError(html string) error {
	match := captchaIDPattern.FindStringSubmatch(html)
	if len(match) < 2 {
		return &NeedCaptchaError{Reason: "需要验证码"}
	}
	captchaID := match[1]
	return &NeedCaptchaError{
		Reason:     "需要验证码",
		CaptchaID:  captchaID,
		CaptchaURL: fmt.Sprintf("%s/ptlogin/captcha.do?captchaId=%s&xx=1", loginBaseURL, url.QueryEscape(captchaID)),
	}
}

func extractHTMLMessage(value string) string {
	value = html.UnescapeString(value)
	value = strings.ReplaceAll(value, "\r", "\n")
	value = htmlTagPattern.ReplaceAllString(value, "\n")
	lines := strings.Split(value, "\n")
	messages := make([]string, 0, 4)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "4399用户中心") {
			continue
		}
		if strings.Contains(line, "错误") || strings.Contains(line, "失败") || strings.Contains(line, "异常") || strings.Contains(line, "验证码") || strings.Contains(line, "密码") || strings.Contains(line, "账号") || strings.Contains(line, "用户名") || strings.Contains(line, "注册") {
			messages = append(messages, line)
		}
		if len(messages) >= 3 {
			break
		}
	}
	if len(messages) == 0 {
		return ""
	}
	return strings.Join(messages, "; ")
}

func isAccountNotFoundPage(rawHTML, message string) bool {
	content := strings.ToLower(rawHTML + "\n" + message)
	if strings.Contains(content, "用户名或密码错误") || strings.Contains(content, "密码错误") {
		return false
	}
	return strings.Contains(content, "show-register") ||
		strings.Contains(content, "register-form") ||
		strings.Contains(content, "没有账号") ||
		strings.Contains(content, "注册") && strings.Contains(content, "账号") && strings.Contains(content, "密码")
}

// isLoginPleaseWait 判断登录响应是否为风控"请稍后再试"（需要延时重试，而非账号错误）
func isLoginPleaseWait(message, html string) bool {
	if message == "" {
		return false
	}
	content := strings.ToLower(message + "\n" + html)
	return strings.Contains(content, "请稍后再试") ||
		strings.Contains(content, "please wait") ||
		strings.Contains(content, "status=202") ||
		strings.Contains(content, "操作太频繁") ||
		strings.Contains(content, "风控") ||
		strings.Contains(content, "异常") && strings.Contains(content, "稍后")
}

func generateIdentifier() string {
	timestamp := time.Now().Format("200601021504")
	randomValue := make([]byte, 32)
	if _, err := rand.Read(randomValue); err != nil {
		randomValue = []byte(fmt.Sprintf("%d", time.Now().UnixNano()))
	}
	hash := sha256.Sum256([]byte(uuid.NewString() + timestamp + hex.EncodeToString(randomValue)))
	return timestamp + hex.EncodeToString(hash[:])[:50]
}

func preview(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 200 {
		return value
	}
	return value[:200]
}

type oauthDevice struct {
	DeviceIdentifier   string `json:"DEVICE_IDENTIFIER"`
	ScreenResolution   string `json:"SCREEN_RESOLUTION"`
	DeviceModel        string `json:"DEVICE_MODEL"`
	DeviceModelVersion string `json:"DEVICE_MODEL_VERSION"`
	SystemVersion      string `json:"SYSTEM_VERSION"`
	PlatformType       string `json:"PLATFORM_TYPE"`
	SDKVersion         string `json:"SDK_VERSION"`
	GameKey            string `json:"GAME_KEY"`
	GameVersion        string `json:"GAME_VERSION"`
	BID                string `json:"BID"`
	Runtime            string `json:"RUNTIME"`
	CanalIdentifier    string `json:"CANAL_IDENTIFIER"`
	UDID               string `json:"UDID"`
	Debug              string `json:"DEBUG"`
	NetworkType        string `json:"NETWORK_TYPE"`
	GameBoxVersion     string `json:"GAME_BOX_VERSION"`
	VIPInfo            string `json:"VIP_INFO"`
	Team               int    `json:"TEAM"`
	DeviceIdentifierSM string `json:"DEVICE_IDENTIFIER_SM"`
	UID                string `json:"UID"`
}

func (d oauthDevice) MarshalJSON() ([]byte, error) {
	type alias oauthDevice
	if d.ScreenResolution == "" {
		d.ScreenResolution = "3200*1384"
	}
	if d.DeviceModel == "" {
		d.DeviceModel = "Oppo A5 2022"
	}
	if d.DeviceModelVersion == "" {
		d.DeviceModelVersion = "14"
	}
	if d.SystemVersion == "" {
		d.SystemVersion = "14"
	}
	if d.PlatformType == "" {
		d.PlatformType = "Android"
	}
	if d.SDKVersion == "" {
		d.SDKVersion = oauthSDKVersion
	}
	if d.GameKey == "" {
		d.GameKey = oauthGameKey
	}
	if d.GameVersion == "" {
		d.GameVersion = "3.1.5.260925"
	}
	if d.BID == "" {
		d.BID = "com.netease.mc.m4399"
	}
	if d.Runtime == "" {
		d.Runtime = "Origin"
	}
	if d.Debug == "" {
		d.Debug = "false"
	}
	if d.NetworkType == "" {
		d.NetworkType = "WIFI"
	}
	if d.Team == 0 {
		d.Team = 2
	}
	return json.Marshal(alias(d))
}

type oauthResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Result  json.RawMessage `json:"result"`
}

type oauthResult struct {
	LoginURL            string `json:"login_url"`
	LoginURLBackup      string `json:"login_url_backup"`
	LoginURLPhone       string `json:"login_url_phone"`
	LoginURLBackupPhone string `json:"login_url_backup_phone"`
}

type oauthCallback struct {
	Result string `json:"result"`
}

type userInfoResponse struct {
	Code    string    `json:"code"`
	Message string    `json:"message"`
	Result  *UserInfo `json:"result"`
}

// UserInfo 描述 4399 OAuth 回调返回的账号信息。
type UserInfo struct {
	UID         int64  `json:"uid"`
	Username    string `json:"username"`
	Nick        string `json:"nick"`
	AccessToken string `json:"access_token"`
	State       string `json:"state"`
	AuthCode    string `json:"code"`
	AccountType string `json:"account_type"`
}
