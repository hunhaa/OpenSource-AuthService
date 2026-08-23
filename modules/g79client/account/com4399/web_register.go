package com4399

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	webRegisterClientID             = "a9a16636dbaeb917e2ffb16f0d52006e"
	webRegisterCallbackReturnURL    = "https://h.4399.com/wap/user.htm"
	webRegisterUserAgent            = "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Mobile Safari/537.36"
	webRegisterXRequestedWith       = "mark.via.gp"
	webRegisterCryptoPassphrase     = "lzYW5qaXVqa"
	maxWebRegisterPleaseWaitRetries = 6          // 请稍后再试 的重试次数
	webRegisterRealNameSubmitDelay  = 500 * time.Millisecond
	webRegisterPleaseWaitRetryDelay = 5 * time.Second // 每次请稍后再试后的等待时间（之前是 2s，太短会被再次限流）
)

var (
	ErrInvalidRegisterInput = errors.New("com4399: invalid register input")
	ErrUsernameExists       = errors.New("com4399: username already exists")
	ErrRegisterRejected     = errors.New("com4399: register rejected")
	ErrRealNameRejected     = errors.New("com4399: real-name rejected")
	ErrRiskControlTriggered = errors.New("com4399: risk control triggered")

	registerUsernamePattern = regexp.MustCompile(`^[\w@]{3,20}$`)
	registerPasswordPattern = regexp.MustCompile(`^[\w\.(!@#$%&)]{6,20}$`)
	registerIDCardPattern   = regexp.MustCompile(`^(\d{15}|\d{17}[\dXx])$`)
)

// WebRegisterRequest 描述网页版 4399 账号注册参数。
type WebRegisterRequest struct {
	Username    string
	Password    string
	RealName    string
	IDCard      string
	CaptchaCode string
}

// WebRegisterResult 描述网页版 4399 注册结果。
type WebRegisterResult struct {
	UID               string         `json:"uid"`
	Username          string         `json:"username"`
	DisplayName       string         `json:"display_name"`
	AccessToken       string         `json:"access_token,omitempty"`
	CallbackURL       string         `json:"callback_url"`
	RealNameSubmitted bool           `json:"real_name_submitted"`
	Cookies           []*http.Cookie `json:"cookies,omitempty"`
}

// WebRegisterCookieResult 描述注册后自动 com4399 登录换取 Cookie 的结果。
type WebRegisterCookieResult struct {
	Register *WebRegisterResult `json:"register"`
	Cookie   string             `json:"cookie"`
}

// WebCaptchaRequiredError 表示网页版注册需要图形验证码。
type WebCaptchaRequiredError struct {
	Message      string
	CaptchaID    string
	CaptchaURL   string
	RegRequestID string
}

func (e *WebCaptchaRequiredError) Error() string {
	if e == nil || e.Message == "" {
		return "com4399: 网页注册需要验证码"
	}
	return "com4399: " + e.Message
}

// WebRegisterClient 封装网页版 4399 注册流程。
type WebRegisterClient struct {
	httpClient *http.Client
	userAgent  string
	pending    *webRegistrationPage
}

type webRegistrationPage struct {
	fields     map[string]string
	captchaURL string
}

type webFormField struct {
	Name  string
	Value string
}

// NewWebRegisterClient 创建网页版注册客户端。
func NewWebRegisterClient(httpClient *http.Client) *WebRegisterClient {
	if httpClient == nil {
		jar, _ := cookiejar.New(nil)
		httpClient = &http.Client{Jar: jar, Timeout: requestTimeout}
	}
	if httpClient.Jar == nil {
		httpClient.Jar, _ = cookiejar.New(nil)
	}
	return &WebRegisterClient{httpClient: httpClient, userAgent: webRegisterUserAgent}
}

// RegisterWeb 执行网页版 4399 账号注册。
func RegisterWeb(ctx context.Context, req WebRegisterRequest) (*WebRegisterResult, error) {
	return NewWebRegisterClient(nil).Register(ctx, req)
}

// RegisterWebCookie 执行网页版 4399 账号注册，然后使用 com4399 登录并返回 Cookie。
func RegisterWebCookie(ctx context.Context, req WebRegisterRequest) (*WebRegisterCookieResult, error) {
	client := NewWebRegisterClient(nil)
	return client.RegisterAndLoginCookie(ctx, req)
}

// RegisterAndLoginCookie 执行网页版 4399 账号注册，然后使用 com4399 登录并返回 Cookie。
func (c *WebRegisterClient) RegisterAndLoginCookie(ctx context.Context, req WebRegisterRequest) (*WebRegisterCookieResult, error) {
	result, err := c.Register(ctx, req)
	if err != nil {
		return nil, err
	}
	cookie, err := RegisterCookieWithPassword(ctx, req.Username, req.Password)
	if err != nil {
		return nil, err
	}
	return &WebRegisterCookieResult{Register: result, Cookie: cookie}, nil
}

// Register 执行网页版 4399 账号注册。
func (c *WebRegisterClient) Register(ctx context.Context, req WebRegisterRequest) (*WebRegisterResult, error) {
	if err := validateWebRegisterRequest(req); err != nil {
		return nil, err
	}
	const maxOCRAttempts = 3
	for attempt := 0; attempt <= maxOCRAttempts; attempt++ {
		page, reused, err := c.registrationPageForAttempt(ctx, req)
		if err != nil {
			return nil, err
		}
		if !reused {
			exists, message, err := c.usernameExists(ctx, req.Username, page.fieldOrDefault("reg_mode", "reg_normal"))
			if err != nil {
				return nil, err
			}
			if exists {
				return nil, fmt.Errorf("%w: %s", ErrUsernameExists, message)
			}
		}
		err = c.submitRegistration(ctx, page, req)
		for retry := 0; isWebRegisterPleaseWait(err) && retry < maxWebRegisterPleaseWaitRetries; retry++ {
			log.Printf("[4399注册] 请稍后再试 第%d/%d次，等待%v后重新打开注册页…", retry+1, maxWebRegisterPleaseWaitRetries, webRegisterPleaseWaitRetryDelay)
			if !waitWebRegisterRetry(ctx) {
				return nil, ctx.Err()
			}
			// 请稍后再试 之后，老的 reg_req_id 已被风控标记，需要重新打开注册页拿新的 reg_req_id / sec
			newPage, nErr := c.openRegistrationPage(ctx)
			if nErr == nil {
				c.pending = nil
				page = newPage
			}
			err = c.submitRegistration(ctx, page, req)
		}
		if err != nil {
			var captchaErr *WebCaptchaRequiredError
			if errors.As(err, &captchaErr) && captchaErr.CaptchaURL != "" && attempt < maxOCRAttempts {
				ocrResult, ocrErr := RecognizeCaptcha(captchaErr.CaptchaURL)
				if ocrErr != nil {
					log.Printf("[OCR] 注册验证码自动识别失败: %v", ocrErr)
					return nil, err
				}
				log.Printf("[OCR] 注册验证码自动识别结果: %s", ocrResult)
				req.CaptchaCode = ocrResult
				// c.pending 已在 submitRegistration 中被设为含新 captcha_id 的页面，
				// 不要清空它，让 registrationPageForAttempt 复用该页面
				continue
			}
			return nil, err
		}
		c.pending = nil
		if !waitWebRegisterRealNameSubmit(ctx) {
			return nil, ctx.Err()
		}
		callbackURL, err := c.submitRealName(ctx, req)
		if err != nil {
			return nil, err
		}
		if err := c.followCallback(ctx, callbackURL); err != nil {
			return nil, err
		}
		return c.buildWebRegisterResult(callbackURL), nil
	}
	return nil, fmt.Errorf("com4399: 注册验证码自动识别失败: 超过最大重试次数 %d", maxOCRAttempts)
}

// SaveCaptchaImage 保存注册验证码图片。
func isWebRegisterPleaseWait(err error) bool {
	if err == nil || !errors.Is(err, ErrRegisterRejected) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "status=202") ||
		strings.Contains(message, "请稍后再试") ||
		strings.Contains(message, "please wait")
}

func waitWebRegisterRetry(ctx context.Context) bool {
	timer := time.NewTimer(webRegisterPleaseWaitRetryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func waitWebRegisterRealNameSubmit(ctx context.Context) bool {
	timer := time.NewTimer(webRegisterRealNameSubmitDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (c *WebRegisterClient) SaveCaptchaImage(ctx context.Context, captchaURL, path string) error {
	if strings.TrimSpace(captchaURL) == "" {
		return errors.New("com4399: captcha url is empty")
	}
	resp, body, err := c.webRequest(ctx, captchaURL, http.MethodGet, nil, map[string]string{"Referer": webAuthorizeSubmitURL()}, true)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("com4399: captcha image status %d", resp.StatusCode)
	}
	return os.WriteFile(path, body, 0o600)
}

func (c *WebRegisterClient) registrationPageForAttempt(ctx context.Context, req WebRegisterRequest) (*webRegistrationPage, bool, error) {
	if strings.TrimSpace(req.CaptchaCode) != "" && c.pending != nil {
		return c.pending, true, nil
	}
	c.pending = nil
	page, err := c.openRegistrationPage(ctx)
	return page, false, err
}

func (c *WebRegisterClient) openRegistrationPage(ctx context.Context) (*webRegistrationPage, error) {
	resp, body, err := c.webRequest(ctx, webAuthorizeLandingURL(), http.MethodGet, nil, nil, true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK || len(body) == 0 {
		return nil, fmt.Errorf("com4399: authorize landing status %d", resp.StatusCode)
	}
	form := []webFormField{
		{Name: "username", Value: ""}, {Name: "phone", Value: ""}, {Name: "phone_captcha", Value: ""},
		{Name: "css", Value: ""}, {Name: "show_close_button", Value: ""}, {Name: "response_type", Value: "TOKEN"},
		{Name: "client_id", Value: webRegisterClientID}, {Name: "show_4399", Value: ""}, {Name: "username_history", Value: ""},
		{Name: "uid", Value: ""}, {Name: "expand_ext_login_list", Value: ""}, {Name: "password", Value: ""},
		{Name: "ref", Value: ""}, {Name: "autoCreateAccount", Value: ""}, {Name: "scope", Value: "basic"},
		{Name: "bizId", Value: ""}, {Name: "show_ext_login", Value: "true"}, {Name: "reg_mode", Value: "reg_normal"},
		{Name: "_d", Value: ""}, {Name: "show_back_button", Value: ""}, {Name: "auto_scroll", Value: ""},
		{Name: "access_token", Value: ""}, {Name: "show_forget_password", Value: ""}, {Name: "auth_action", Value: "register"},
		{Name: "redirect_uri", Value: webRedirectURI()}, {Name: "show_topbar", Value: ""}, {Name: "aid", Value: ""}, {Name: "cid", Value: ""},
	}
	resp, body, err = c.webRequest(ctx, webAuthorizeSubmitURL(), http.MethodPost, form, map[string]string{
		"Origin": webOrigin(loginBaseURL), "Referer": webAuthorizeLandingURL(),
	}, true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("com4399: authorize submit status %d", resp.StatusCode)
	}
	page := webPageFromDocument(string(body), loginBaseURL)
	if page.fieldOrDefault("reg_req_id", "") == "" || page.fieldOrDefault("sec", "") == "" {
		return nil, fmt.Errorf("com4399: registration page is missing hidden fields")
	}
	return page, nil
}

func (c *WebRegisterClient) usernameExists(ctx context.Context, username, regMode string) (bool, string, error) {
	u, _ := url.Parse(loginBaseURL + "/ptlogin/isExist.do")
	query := u.Query()
	query.Set("username", username)
	query.Set("appId", "oauth")
	query.Set("regMode", regMode)
	query.Set("v", "2")
	u.RawQuery = query.Encode()
	resp, body, err := c.webRequest(ctx, u.String(), http.MethodGet, nil, nil, true)
	if err != nil {
		return false, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("com4399: username exists status %d", resp.StatusCode)
	}
	text := strings.TrimSpace(string(body))
	return text != "0", text, nil
}

func (c *WebRegisterClient) submitRegistration(ctx context.Context, page *webRegistrationPage, req WebRegisterRequest) error {
	encryptedPassword, err := encryptWebRegisterAES(req.Password)
	if err != nil {
		return err
	}
	form := []webFormField{{Name: "phone_captcha", Value: ""}, {Name: "password", Value: encryptedPassword}, {Name: "username", Value: req.Username}}
	if captchaID := page.fieldOrDefault("captcha_id", ""); captchaID != "" {
		form = append(form, webFormField{Name: "captcha", Value: strings.TrimSpace(req.CaptchaCode)}, webFormField{Name: "captcha_id", Value: captchaID})
	}
	form = append(form,
		webFormField{Name: "css", Value: page.fieldOrDefault("css", "")}, webFormField{Name: "show_close_button", Value: page.fieldOrDefault("show_close_button", "")},
		webFormField{Name: "response_type", Value: page.fieldOrDefault("response_type", "TOKEN")}, webFormField{Name: "client_id", Value: page.fieldOrDefault("client_id", webRegisterClientID)},
		webFormField{Name: "show_4399", Value: page.fieldOrDefault("show_4399", "")}, webFormField{Name: "username_history", Value: page.fieldOrDefault("username_history", "")},
		webFormField{Name: "uid", Value: page.fieldOrDefault("uid", "")}, webFormField{Name: "expand_ext_login_list", Value: page.fieldOrDefault("expand_ext_login_list", "")},
		webFormField{Name: "password", Value: ""}, webFormField{Name: "ref", Value: page.fieldOrDefault("ref", "")}, webFormField{Name: "autoCreateAccount", Value: page.fieldOrDefault("autoCreateAccount", "")},
		webFormField{Name: "scope", Value: page.fieldOrDefault("scope", "basic")}, webFormField{Name: "bizId", Value: page.fieldOrDefault("bizId", "")}, webFormField{Name: "show_ext_login", Value: page.fieldOrDefault("show_ext_login", "true")},
		webFormField{Name: "reg_mode", Value: page.fieldOrDefault("reg_mode", "reg_normal")}, webFormField{Name: "_d", Value: page.fieldOrDefault("_d", "")}, webFormField{Name: "show_back_button", Value: page.fieldOrDefault("show_back_button", "")},
		webFormField{Name: "auto_scroll", Value: page.fieldOrDefault("auto_scroll", "")}, webFormField{Name: "reg_req_id", Value: page.fieldOrDefault("reg_req_id", "")}, webFormField{Name: "access_token", Value: page.fieldOrDefault("access_token", "")},
		webFormField{Name: "show_forget_password", Value: page.fieldOrDefault("show_forget_password", "")}, webFormField{Name: "auth_action", Value: page.fieldOrDefault("auth_action", "REGISTER")}, webFormField{Name: "redirect_uri", Value: page.fieldOrDefault("redirect_uri", webRedirectURI())},
		webFormField{Name: "show_topbar", Value: page.fieldOrDefault("show_topbar", "")}, webFormField{Name: "aid", Value: page.fieldOrDefault("aid", "")}, webFormField{Name: "cid", Value: page.fieldOrDefault("cid", "")},
		webFormField{Name: "isInputRealname", Value: page.fieldOrDefault("isInputRealname", "false")}, webFormField{Name: "sec", Value: page.fieldOrDefault("sec", "1")},
	)
	resp, body, err := c.webRequest(ctx, loginBaseURL+"/oauth2/registerAndAuthorize.do", http.MethodPost, form, map[string]string{
		"Origin": webOrigin(loginBaseURL), "Referer": webAuthorizeSubmitURL(),
	}, false)
	if err != nil {
		return err
	}
	doc := string(body)
	if resp.StatusCode == http.StatusOK && webLooksLikeRealNamePageV2(doc) {
		return nil
	}
	if challenge := extractWebCaptchaChallenge(doc, loginBaseURL); challenge != nil {
		c.pending = webPageFromDocument(doc, loginBaseURL)
		message := firstNonEmptyString(extractWebPageErrors(doc)...)
		if message == "" {
			message = "网页注册需要验证码"
		}
		return &WebCaptchaRequiredError{Message: message, CaptchaID: challenge.CaptchaID, CaptchaURL: challenge.CaptchaURL, RegRequestID: challenge.RegRequestID}
	}
	message := firstNonEmptyString(extractWebPageErrors(doc)...)
	if strings.Contains(message, "验证码") || strings.Contains(message, "风险") || strings.Contains(message, "异常") {
		return fmt.Errorf("%w: %s", ErrRiskControlTriggered, message)
	}
	if message == "" {
		message = describeUnexpectedRegistrationResponse(resp, doc)
	}
	return fmt.Errorf("%w: %s", ErrRegisterRejected, message)
}

func (c *WebRegisterClient) SubmitRealName(ctx context.Context, req WebRegisterRequest) (string, error) {
	return c.submitRealName(ctx, req)
}

func (c *WebRegisterClient) submitRealName(ctx context.Context, req WebRegisterRequest) (string, error) {
	realName, err := encryptWebRegisterAES(strings.TrimSpace(req.RealName))
	if err != nil {
		return "", err
	}
	idCard, err := encryptWebRegisterAES(strings.TrimSpace(req.IDCard))
	if err != nil {
		return "", err
	}
	form := []webFormField{{Name: "sec", Value: "1"}, {Name: "realname", Value: realName}, {Name: "idcard", Value: idCard}, {Name: "bizId", Value: ""}, {Name: "isReg", Value: "true"}, {Name: "needValidate", Value: "true"}, {Name: "policy", Value: "on"}}
	resp, body, err := c.webRequest(ctx, loginBaseURL+"/oauth2/setIdcardAndRealname.do", http.MethodPost, form, map[string]string{
		"Origin": webOrigin(loginBaseURL), "Referer": loginBaseURL + "/oauth2/registerAndAuthorize.do",
	}, false)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther {
		location, err := resp.Location()
		if err != nil {
			return "", fmt.Errorf("com4399: missing callback location")
		}
		if strings.Contains(location.Path, "/unifiedLogin/user/login/callback") {
			return location.String(), nil
		}
		return "", fmt.Errorf("%w: unexpected redirect location %s", ErrRealNameRejected, location.String())
	}
	message := firstNonEmptyString(extractWebPageErrors(string(body))...)
	if message == "" {
		message = "real-name submission did not redirect to callback"
	}
	return "", fmt.Errorf("%w: %s", ErrRealNameRejected, message)
}

func (c *WebRegisterClient) FollowCallback(ctx context.Context, callbackURL string) error {
	return c.followCallback(ctx, callbackURL)
}

func (c *WebRegisterClient) followCallback(ctx context.Context, callbackURL string) error {
	resp, _, err := c.webRequest(ctx, callbackURL, http.MethodGet, nil, map[string]string{"Referer": webRegisterCallbackReturnURL}, true)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("com4399: callback follow status %d", resp.StatusCode)
	}
	return nil
}

func (c *WebRegisterClient) BuildRegisterResult(callbackURL string) *WebRegisterResult {
	return c.buildWebRegisterResult(callbackURL)
}

func (c *WebRegisterClient) buildWebRegisterResult(callbackURL string) *WebRegisterResult {
	result := &WebRegisterResult{CallbackURL: callbackURL, RealNameSubmitted: true, Cookies: c.collectWebCookies()}
	u, err := url.Parse(callbackURL)
	if err != nil {
		return result
	}
	query := u.Query()
	result.UID = query.Get("uid")
	result.Username = firstNonEmptyString(query.Get("username"), query.Get("display_name"))
	result.DisplayName = firstNonEmptyString(query.Get("display_name"), query.Get("username"))
	result.AccessToken = query.Get("access_token")
	return result
}

func (c *WebRegisterClient) webRequest(ctx context.Context, target, method string, form []webFormField, headers map[string]string, followRedirect bool) (*http.Response, []byte, error) {
	var body io.Reader
	if len(form) > 0 {
		body = strings.NewReader(encodeWebForm(form))
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7")
	if len(form) > 0 {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	applyWebBrowserHeaders(req, len(form) > 0)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	client := c.redirectClient(followRedirect)
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return resp, data, err
}

func (c *WebRegisterClient) redirectClient(follow bool) *http.Client {
	clone := *c.httpClient
	if follow {
		clone.CheckRedirect = nil
	} else {
		clone.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	}
	return &clone
}

func validateWebRegisterRequest(req WebRegisterRequest) error {
	switch {
	case !registerUsernamePattern.MatchString(req.Username):
		return fmt.Errorf("%w: username must match ^[\\w@]{3,20}$", ErrInvalidRegisterInput)
	case !registerPasswordPattern.MatchString(req.Password):
		return fmt.Errorf("%w: password must be 6-20 characters and match site rules", ErrInvalidRegisterInput)
	case strings.TrimSpace(req.RealName) == "":
		return fmt.Errorf("%w: real name is required", ErrInvalidRegisterInput)
	case utf8.RuneCountInString(strings.TrimSpace(req.RealName)) < 2:
		return fmt.Errorf("%w: real name is too short", ErrInvalidRegisterInput)
	case !isValidWebIDCard(req.IDCard):
		return fmt.Errorf("%w: invalid id card", ErrInvalidRegisterInput)
	default:
		return nil
	}
}

func isValidWebIDCard(id string) bool {
	if !registerIDCardPattern.MatchString(id) || len(id) == 15 {
		return false
	}
	factors := []int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	checksum := []byte{'1', '0', 'X', '9', '8', '7', '6', '5', '4', '3', '2'}
	sum := 0
	for i := 0; i < 17; i++ {
		if id[i] < '0' || id[i] > '9' {
			return false
		}
		sum += int(id[i]-'0') * factors[i]
	}
	last := id[17]
	if last == 'x' {
		last = 'X'
	}
	return checksum[sum%11] == last
}

func applyWebBrowserHeaders(req *http.Request, hasForm bool) {
	switch {
	case hasForm && strings.Contains(req.URL.Host, "ptlogin.4399.com"):
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
		req.Header.Set("Cache-Control", "max-age=0")
		req.Header.Set("Upgrade-Insecure-Requests", "1")
		req.Header.Set("X-Requested-With", webRegisterXRequestedWith)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		req.Header.Set("Sec-Fetch-User", "?1")
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("sec-ch-ua", `"Not(A:Brand";v="8", "Chromium";v="144", "Android WebView";v="144"`)
		req.Header.Set("sec-ch-ua-mobile", "?1")
		req.Header.Set("sec-ch-ua-platform", `"Android"`)
	default:
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	}
}

func webAuthorizeLandingURL() string {
	u, _ := url.Parse(loginBaseURL + "/oauth2/authorize.do")
	query := u.Query()
	query.Set("client_id", webRegisterClientID)
	query.Set("redirect_uri", webRedirectURI())
	query.Set("response_type", "token")
	query.Set("show_ext_login", "true")
	query.Set("loginRealNameLevel", "4")
	query.Set("regRealNameLevel", "4")
	u.RawQuery = query.Encode()
	return u.String()
}

func webAuthorizeSubmitURL() string { return loginBaseURL + "/oauth2/authorize.do?channel=" }
func webRedirectURI() string {
	return "https://h.api.4399.com/unifiedLogin/user/login/callback?callbackUrl=" + url.QueryEscape(webRegisterCallbackReturnURL)
}
func webOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Scheme + "://" + u.Host
}

func (p *webRegistrationPage) fieldOrDefault(name, fallback string) string {
	if p == nil {
		return fallback
	}
	if value, ok := p.fields[name]; ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func encodeWebForm(fields []webFormField) string {
	var b strings.Builder
	for i, field := range fields {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(url.QueryEscape(field.Name))
		b.WriteByte('=')
		b.WriteString(url.QueryEscape(field.Value))
	}
	return b.String()
}

func extractWebInputValues(doc string) map[string]string {
	inputPattern := regexp.MustCompile(`(?is)<input\b[^>]*>`)
	attrPattern := regexp.MustCompile(`([a-zA-Z_:][-a-zA-Z0-9_:.]*)\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))`)
	values := make(map[string]string)
	for _, input := range inputPattern.FindAllString(doc, -1) {
		attrs := make(map[string]string)
		for _, attr := range attrPattern.FindAllStringSubmatch(input, -1) {
			value := firstNonEmptyString(attr[3], attr[4], attr[5])
			attrs[strings.ToLower(attr[1])] = html.UnescapeString(value)
		}
		name := attrs["name"]
		if name != "" {
			if _, exists := values[name]; !exists {
				values[name] = attrs["value"]
			}
		}
	}
	return values
}

func webPageFromDocument(doc, baseURL string) *webRegistrationPage {
	challenge := extractWebCaptchaChallenge(doc, baseURL)
	page := &webRegistrationPage{fields: extractWebInputValues(doc)}
	if challenge != nil {
		page.captchaURL = challenge.CaptchaURL
	}
	return page
}

type webCaptchaChallenge struct{ CaptchaID, CaptchaURL, RegRequestID string }

func extractWebCaptchaChallenge(doc, baseURL string) *webCaptchaChallenge {
	fields := extractWebInputValues(doc)
	captchaID := strings.TrimSpace(fields["captcha_id"])
	if captchaID == "" && !strings.Contains(doc, `id="captcha_img"`) {
		return nil
	}
	imagePattern := regexp.MustCompile(`(?is)<img[^>]*id="captcha_img"[^>]*src="([^"]+)"`)
	matches := imagePattern.FindStringSubmatch(doc)
	captchaURL := ""
	if len(matches) > 1 {
		captchaURL = html.UnescapeString(strings.TrimSpace(matches[1]))
		if base, err := url.Parse(baseURL); err == nil {
			if parsed, err := url.Parse(captchaURL); err == nil {
				captchaURL = base.ResolveReference(parsed).String()
			}
		}
	}
	return &webCaptchaChallenge{CaptchaID: captchaID, CaptchaURL: captchaURL, RegRequestID: strings.TrimSpace(fields["reg_req_id"])}
}

func webLooksLikeRealNamePage(doc string) bool {
	return strings.Contains(doc, `id="set_register_idcard"`) || strings.Contains(doc, `/oauth2/setIdcardAndRealname.do`) || strings.Contains(doc, "身份认证")
}

func webLooksLikeRealNamePageV2(doc string) bool {
	if webLooksLikeRealNamePage(doc) {
		return true
	}
	lower := strings.ToLower(doc)
	if strings.Contains(lower, `/oauth2/setidcardandrealname.do`) ||
		strings.Contains(lower, `setidcardandrealname`) {
		return true
	}
	fields := extractWebInputValues(doc)
	_, hasRealName := fields["realname"]
	_, hasIDCard := fields["idcard"]
	return hasRealName && hasIDCard
}

func describeUnexpectedRegistrationResponse(resp *http.Response, doc string) string {
	parts := []string{"registration page did not transition to real-name form"}
	if resp != nil {
		parts = append(parts, fmt.Sprintf("status=%d", resp.StatusCode))
		if location := strings.TrimSpace(resp.Header.Get("Location")); location != "" {
			parts = append(parts, "location="+location)
		}
	}
	doc = strings.TrimSpace(doc)
	if doc == "" {
		parts = append(parts, "body=empty")
		return strings.Join(parts, " ")
	}
	if title := extractWebTitle(doc); title != "" {
		parts = append(parts, "title="+title)
	}
	if snippet := compactWebText(doc, 160); snippet != "" {
		parts = append(parts, "body="+snippet)
	}
	return strings.Join(parts, " ")
}

func extractWebTitle(doc string) string {
	titlePattern := regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	matches := titlePattern.FindStringSubmatch(doc)
	if len(matches) < 2 {
		return ""
	}
	return compactPlainText(matches[1], 80)
}

func compactWebText(doc string, limit int) string {
	bodyPattern := regexp.MustCompile(`(?is)<body[^>]*>(.*?)</body>`)
	if matches := bodyPattern.FindStringSubmatch(doc); len(matches) > 1 {
		doc = matches[1]
	}
	scriptPattern := regexp.MustCompile(`(?is)<(?:script|style)[^>]*>.*?</(?:script|style)>`)
	doc = scriptPattern.ReplaceAllString(doc, " ")
	tagPattern := regexp.MustCompile(`(?is)<[^>]+>`)
	return compactPlainText(tagPattern.ReplaceAllString(doc, " "), limit)
}

func compactPlainText(text string, limit int) string {
	text = strings.TrimSpace(html.UnescapeString(text))
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}
	if limit > 0 && len(text) > limit {
		text = text[:limit] + "..."
	}
	return text
}

func extractWebPageErrors(doc string) []string {
	idPattern := regexp.MustCompile(`(?is)<(?:p|div)[^>]*id="(?:username\d?_err_msg|captcha\d?_err_msg|protocol\d?_err_msg|login_err_msg)"[^>]*>(.*?)</(?:p|div)>`)
	warnPattern := regexp.MustCompile(`(?is)<div[^>]*class="[^"]*ipt_error_warntip[^"]*"[^>]*>.*?<p>(.*?)</p>`)
	tipPattern := regexp.MustCompile(`(?is)<(?:p|div|span)[^>]*(?:class|id)="[^"]*(?:error|err|warn|tip|message|msg)[^"]*"[^>]*>(.*?)</(?:p|div|span)>`)
	alertPattern := regexp.MustCompile(`(?is)alert\s*\(\s*["']([^"']+)["']\s*\)`)
	tagPattern := regexp.MustCompile(`(?is)<[^>]+>`)
	messages := make([]string, 0, 2)
	for _, match := range idPattern.FindAllStringSubmatch(doc, -1) {
		if text := strings.TrimSpace(html.UnescapeString(tagPattern.ReplaceAllString(match[1], ""))); text != "" {
			messages = append(messages, text)
		}
	}
	for _, match := range warnPattern.FindAllStringSubmatch(doc, -1) {
		if text := strings.TrimSpace(html.UnescapeString(tagPattern.ReplaceAllString(match[1], ""))); text != "" {
			messages = append(messages, text)
		}
	}
	for _, match := range tipPattern.FindAllStringSubmatch(doc, -1) {
		if text := strings.TrimSpace(html.UnescapeString(tagPattern.ReplaceAllString(match[1], ""))); text != "" {
			messages = append(messages, text)
		}
	}
	for _, match := range alertPattern.FindAllStringSubmatch(doc, -1) {
		if text := strings.TrimSpace(html.UnescapeString(match[1])); text != "" {
			messages = append(messages, text)
		}
	}
	return messages
}

func (c *WebRegisterClient) collectWebCookies() []*http.Cookie {
	targets := []string{loginBaseURL, "https://h.api.4399.com", webRegisterCallbackReturnURL}
	seen := make(map[string]bool)
	cookies := make([]*http.Cookie, 0)
	for _, target := range targets {
		u, err := url.Parse(target)
		if err != nil {
			continue
		}
		for _, cookie := range c.httpClient.Jar.Cookies(u) {
			key := cookie.Name + "\x00" + cookie.Value
			if !seen[key] {
				seen[key] = true
				cookies = append(cookies, cookie)
			}
		}
	}
	return cookies
}

func (c *WebRegisterClient) hasWebCookie(name string) bool {
	for _, cookie := range c.collectWebCookies() {
		if cookie.Name == name {
			return true
		}
	}
	return false
}

func encryptWebRegisterAES(plain string) (string, error) {
	salt := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key, iv := webEvpBytesToKey([]byte(webRegisterCryptoPassphrase), salt, 32, aes.BlockSize)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	padded := webPKCS7Pad([]byte(plain), aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)
	payload := append([]byte("Salted__"), salt...)
	payload = append(payload, ciphertext...)
	return base64.StdEncoding.EncodeToString(payload), nil
}

func webEvpBytesToKey(passphrase, salt []byte, keyLen, ivLen int) ([]byte, []byte) {
	total := keyLen + ivLen
	derived := make([]byte, 0, total)
	var block []byte
	for len(derived) < total {
		sum := md5.New()
		sum.Write(block)
		sum.Write(passphrase)
		sum.Write(salt)
		block = sum.Sum(nil)
		derived = append(derived, block...)
	}
	return derived[:keyLen], derived[keyLen:total]
}

func webPKCS7Pad(src []byte, blockSize int) []byte {
	padding := blockSize - len(src)%blockSize
	return append(src, bytes.Repeat([]byte{byte(padding)}, padding)...)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
