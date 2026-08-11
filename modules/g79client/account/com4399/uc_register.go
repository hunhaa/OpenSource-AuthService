package com4399

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Yuansi 于 2026-7-15 逆向

const ucRegisterURL = loginBaseURL + "/ptlogin/register.do"
const ucRegisterFrameURL = loginBaseURL + "/ptlogin/regFrame.do"
const ucRegisterRequestMinDelay = 8 * time.Second
const ucRegisterRequestMaxDelay = 12 * time.Second

// UCRegisterRequest is the single request used by UC registration, including real-name data.
type UCRegisterRequest struct {
	Username      string
	Password      string
	RealName      string
	IDCard        string
	Email         string
	WebRecordIDP  string
	CaptchaCode   string
	CaptchaID     string
	RegRequestID  string
	UseEncryption bool
}

type UCRegisterResult struct {
	Success  bool
	Status   int
	Username string
	Cookies  []*http.Cookie
}

type UCRegisterClient struct {
	httpClient *http.Client
	sessionID  string
}

func NewUCRegisterClient(client *http.Client) *UCRegisterClient {
	if client == nil {
		jar, _ := cookiejar.New(nil)
		client = &http.Client{Jar: jar, Timeout: requestTimeout}
	}
	if client.Jar == nil {
		client.Jar, _ = cookiejar.New(nil)
	}
	return &UCRegisterClient{httpClient: client}
}

func RegisterUC(ctx context.Context, req UCRegisterRequest) (*UCRegisterResult, error) {
	return NewUCRegisterClient(nil).Register(ctx, req)
}

func (c *UCRegisterClient) Register(ctx context.Context, req UCRegisterRequest) (*UCRegisterResult, error) {
	if err := validateUCRegisterRequest(req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.WebRecordIDP) == "" {
		record, err := generateUCWebRecordIDP()
		if err != nil {
			return nil, fmt.Errorf("com4399: 生成 webRecordIdP 失败: %w", err)
		}
		req.WebRecordIDP = record
	}
	if !req.UseEncryption {
		req.UseEncryption = true
	}
	for attempt := 0; attempt < 3; attempt++ {
		id := c.sessionID
		captcha := false
		if id == "" {
			var err error
			id, captcha, err = c.init(ctx, req.WebRecordIDP)
			if err != nil {
				return nil, err
			}
			c.sessionID = id
		}
		if req.CaptchaID == "" {
			req.CaptchaID = id
		}
		if captcha && strings.TrimSpace(req.CaptchaCode) == "" {
			if req.CaptchaID == "" {
				return nil, &UCRegisterCaptchaError{Reason: "验证码已捕获但缺少 captcha_id"}
			}
			log.Printf("[UC OCR] 下载验证码: captcha_id=%s", req.CaptchaID)
			code, ocrErr := RecognizeCaptchaWithHTTPClient(captchaURL(req.CaptchaID), c.httpClient)
			if ocrErr != nil {
				return nil, &UCRegisterCaptchaError{Reason: "验证码 OCR 失败", CaptchaID: req.CaptchaID, CaptchaURL: captchaURL(req.CaptchaID), Cause: ocrErr}
			}
			log.Printf("[UC OCR] 验证码识别完成: %s", code)
			req.CaptchaCode = code
		}
		if !waitUCRegisterDelay(ctx) {
			return nil, ctx.Err()
		}
		body, status, err := c.submit(ctx, req, id)
		if err != nil {
			return nil, err
		}
		message := ucRegisterMessage(html.UnescapeString(string(body)))
		captchaMessage := strings.Contains(message, "验证码")
		if !ucRegisterSuccess(body) && message != "" && !captchaMessage {
			return nil, ucRegisterError(status, body)
		}
		challenge := ucCaptchaError(body)
		if challenge == nil && captchaMessage {
			challenge = &UCRegisterCaptchaError{Reason: message, CaptchaID: id, CaptchaURL: captchaURL(id)}
		}
		if challenge != nil {
			if attempt == 2 {
				return nil, challenge
			}
			req.CaptchaCode, req.CaptchaID, c.sessionID = "", "", ""
			continue
		}
		if !ucRegisterSuccess(body) {
			return nil, ucRegisterError(status, body)
		}
		return &UCRegisterResult{Success: true, Status: status, Username: req.Username, Cookies: c.cookies()}, nil
	}
	return nil, errors.New("com4399: 注册重试次数耗尽")
}

func (c *UCRegisterClient) init(ctx context.Context, record string) (string, bool, error) {
	initFields := url.Values{
		"bizId": {""}, "redirectUrl": {""}, "css": {"https://uc.img4399.com/root/css/ptlogin.css?8928ab0"},
		"gameId": {""}, "noEmail": {"false"}, "autoLogin": {"false"}, "cid": {""}, "aid": {""}, "ref": {""},
		"mainDivId": {"popup_reg_div"}, "includeFcmInfo": {"false"}, "externalLogin": {"qq"}, "expandFcmInput": {"false"},
	}
	v := url.Values{"regMode": {"reg_normal"}, "postLoginHandler": {"refreshParent"}, "displayMode": {"embed"}, "appId": {"u4399"}, "regIdcard": {"false"}, "level": {"4"}, "fcmFakeValidate": {"true"}, "userNameLabel": {"4399用户名"}}
	if record != "" {
		u, _ := url.Parse(ucRegisterFrameURL)
		_ = u
		c.setCookie(ctx, "webRecordIdP", record)
	}
	for key, values := range initFields {
		for _, value := range values {
			v.Set(key, value)
		}
	}
	v.Set("userNameLabel", "4399用户名")
	body, status, err := c.do(ctx, http.MethodGet, ucRegisterFrameURL+"?"+v.Encode(), nil)
	_ = status
	if err != nil {
		return "", false, fmt.Errorf("com4399: 初始化注册 session 失败: %w", err)
	}
	id := firstMatch(string(body), `(?is)<input[^>]*\bname\s*=\s*["']sessionId["'][^>]*\bvalue\s*=\s*["']([^"']*)["']`)
	if id == "" {
		id = firstMatch(string(body), `(?is)<input[^>]*\bvalue\s*=\s*["']([^"']*)["'][^>]*\bname\s*=\s*["']sessionId["']`)
	}
	h := string(body)
	has := regexp.MustCompile(`(?i)\bname\s*=\s*["']inputCaptcha["']`).MatchString(h) && id != ""
	if !strings.Contains(strings.ToLower(h), "name=\"sessionid\"") && !strings.Contains(strings.ToLower(h), "name='sessionid'") {
		return "", has, fmt.Errorf("com4399: 注册初始化响应缺少 sessionId 字段: status=%d body=%s", status, h)
	}
	return id, has, nil
}

func (c *UCRegisterClient) submit(ctx context.Context, req UCRegisterRequest, sessionID string) ([]byte, int, error) {
	password, err := encryptUC(req.Password, req.UseEncryption)
	if err != nil {
		return nil, 0, err
	}
	name, err := encryptUC(req.RealName, req.UseEncryption)
	if err != nil {
		return nil, 0, err
	}
	id, err := encryptUC(req.IDCard, req.UseEncryption)
	if err != nil {
		return nil, 0, err
	}
	v := url.Values{
		"postLoginHandler": {"refreshParent"}, "displayMode": {"embed"}, "bizId": {""}, "appId": {"u4399"},
		"gameId": {""}, "cid": {""}, "externalLogin": {"qq"}, "aid": {""}, "ref": {""},
		"css": {"https://uc.img4399.com/root/css/ptlogin.css?8928ab0"}, "redirectUrl": {""}, "regMode": {"reg_normal"},
		"sessionId": {sessionID}, "regIdcard": {"true"}, "noEmail": {""}, "crossDomainIFrame": {""}, "crossDomainUrl": {""},
		"mainDivId": {"popup_reg_div"}, "showRegInfo": {"true"}, "includeFcmInfo": {"false"}, "expandFcmInput": {"true"},
		"fcmFakeValidate": {"false"}, "realnameValidate": {"true"}, "userNameLabel": {"4399用户名"}, "level": {"4"}, "sec": {"1"},
		"password": {password}, "passwordveri": {password}, "iframeId": {""}, "realname": {name},
		"idcard": {id}, "username": {req.Username}, "email": {req.Email}, "reg_eula_agree": {"on"},
	}
	/*
		name, err := encryptUC(req.RealName, req.UseEncryption)
		if err != nil {
			return nil, 0, err
		}
		id, err := encryptUC(req.IDCard, req.UseEncryption)
		if err != nil {
			return nil, 0, err
		}
		v := url.Values{"postLoginHandler": {"refreshParent"}, "displayMode": {"embed"}, "bizId": {""}, "appId": {"u4399"}, "gameId": {""}, "cid": {""}, "externalLogin": {"qq"}, "aid": {""}, "ref": {""}, "css": {"https://uc.img4399.com/root/css/ptlogin.css?8928ab0"}, "redirectUrl": {""}, "regMode": {"reg_normal"}, "sessionId": {""}, "regIdcard": {"true"}, "noEmail": {""}, "crossDomainIFrame": {""}, "crossDomainUrl": {""}, "mainDivId": {"popup_reg_div"}, "showRegInfo": {"true"}, "includeFcmInfo": {"false"}, "expandFcmInput": {"true"}, "fcmFakeValidate": {"false"}, "realnameValidate": {"true"}, "userNameLabel": {"4399鐢ㄦ埛鍚?}, "level": {"4"}, "sec": {"1"}, "password": {password}, "passwordveri": {password}, "iframeId": {"popup_reg_frame"}, "realname": {name}, "idcard": {id}, "username": {req.Username}, "email": {req.Email}, "reg_eula_agree": {"on"}}
	*/
	if req.CaptchaCode != "" {
		v.Set("inputCaptcha", req.CaptchaCode)
	}
	if req.RegRequestID != "" {
		v.Set("reg_req_id", req.RegRequestID)
	}
	return c.do(ctx, http.MethodPost, ucRegisterURL, strings.NewReader(v.Encode()))
}

func (c *UCRegisterClient) do(ctx context.Context, method, target string, body io.Reader) ([]byte, int, error) {
	r, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, 0, err
	}
	r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36")
	r.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	r.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	// Go's standard library has no Brotli decoder; advertise only encodings we decode.
	r.Header.Set("Accept-Encoding", "gzip, deflate")
	if body != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", loginBaseURL)
		r.Header.Set("Referer", ucRegisterURL)
	}
	resp, err := c.httpClient.Do(r)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	b, err = decodeUCResponseBody(b, resp.Header.Get("Content-Encoding"))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("com4399: 解压响应失败 encoding=%q: %w", resp.Header.Get("Content-Encoding"), err)
	}
	return b, resp.StatusCode, nil
}

func waitUCRegisterDelay(ctx context.Context) bool {
	delay := ucRegisterRequestMinDelay
	if offset, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(ucRegisterRequestMaxDelay-ucRegisterRequestMinDelay)+1)); err == nil {
		delay += time.Duration(offset.Int64())
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func decodeUCResponseBody(body []byte, encoding string) ([]byte, error) {
	encoding = strings.ToLower(strings.TrimSpace(strings.Split(encoding, ";")[0]))
	if encoding == "" || encoding == "identity" {
		return body, nil
	}
	var reader io.ReadCloser
	var err error
	switch encoding {
	case "gzip":
		reader, err = gzip.NewReader(bytes.NewReader(body))
	case "deflate":
		reader, err = zlib.NewReader(bytes.NewReader(body))
		if err != nil {
			reader = flateReader(bytes.NewReader(body))
			err = nil
		}
	default:
		return nil, fmt.Errorf("不支持的 Content-Encoding %q", encoding)
	}
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

type flateReadCloser struct{ io.ReadCloser }

func flateReader(r io.Reader) io.ReadCloser {
	return flateReadCloser{ReadCloser: structReadCloser{Reader: flate.NewReader(r)}}
}

type structReadCloser struct{ io.Reader }

func (structReadCloser) Close() error { return nil }
func (c *UCRegisterClient) setCookie(ctx context.Context, name, value string) {
	u, _ := url.Parse(loginBaseURL)
	c.httpClient.Jar.SetCookies(u, []*http.Cookie{{Name: name, Value: value}})
}
func (c *UCRegisterClient) cookies() []*http.Cookie {
	u, _ := url.Parse(loginBaseURL)
	return c.httpClient.Jar.Cookies(u)
}

type UCRegisterCaptchaError struct {
	Reason, CaptchaID, CaptchaURL string
	Cause                         error
}

func (e *UCRegisterCaptchaError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("com4399: %s captcha_id=%s captcha_url=%s: %v", e.Reason, e.CaptchaID, e.CaptchaURL, e.Cause)
	}
	return fmt.Sprintf("com4399: %s captcha_id=%s captcha_url=%s", e.Reason, e.CaptchaID, e.CaptchaURL)
}
func (e *UCRegisterCaptchaError) Unwrap() error { return e.Cause }

func ucCaptchaError(body []byte) *UCRegisterCaptchaError {
	s := html.UnescapeString(string(body))
	if !regexp.MustCompile(`(?i)\bname\s*=\s*["']inputCaptcha["']`).MatchString(s) {
		return nil
	}
	id := firstMatch(s, `(?i)/ptlogin/captcha\.do\?captchaId=([\w-]+)`)
	if id == "" {
		id = firstMatch(s, `(?i)<input[^>]*\bname\s*=\s*["']sessionId["'][^>]*\bvalue\s*=\s*["']([\w-]+)["']`)
	}
	if id == "" {
		return nil
	}
	return &UCRegisterCaptchaError{Reason: "需要验证码或验证码错误", CaptchaID: id, CaptchaURL: captchaURL(id)}
}
func ucRegisterSuccess(b []byte) bool {
	s := html.UnescapeString(string(b))
	return strings.Contains(s, "注册成功") || strings.Contains(s, "注册成功")
}
func ucRegisterError(status int, b []byte) error {
	raw := html.UnescapeString(string(b))
	message := ucRegisterMessage(raw)
	if message == "" {
		message = ucRegisterPlainText(raw)
	}
	if strings.Contains(message, "用户名已") {
		return fmt.Errorf("%w: %s", ErrUsernameExists, message)
	}
	if isDiscardableUCIDCodeError(message) {
		return fmt.Errorf("%w: %s", ErrRealNameRejected, message)
	}
	return fmt.Errorf("%w: status=%d %s", ErrRegisterRejected, status, preview(message))
}

func ucRegisterMessage(s string) string {
	match := regexp.MustCompile(`(?is)<div\s+id=["']Msg["'][^>]*>(.*?)</div>`).FindStringSubmatch(s)
	if len(match) < 2 {
		return ""
	}
	return ucRegisterPlainText(match[1])
}

func ucRegisterPlainText(s string) string {
	s = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

func isDiscardableUCIDCodeError(message string) bool {
	return strings.Contains(message, "姓名身份证提交验证过于频繁") ||
		strings.Contains(message, "姓名身份证不匹配") ||
		strings.Contains(message, "身份证实名账号数量超过限制")
}
func validateUCRegisterRequest(r UCRegisterRequest) error {
	if strings.TrimSpace(r.Username) == "" || r.Password == "" || strings.TrimSpace(r.RealName) == "" || strings.TrimSpace(r.IDCard) == "" {
		return fmt.Errorf("%w: username/password/realname/idcard are required", ErrInvalidRegisterInput)
	}
	return nil
}
func encryptUC(s string, enabled bool) (string, error) {
	if !enabled {
		return s, nil
	}
	return encryptWebRegisterAES(s)
}
func captchaURL(id string) string {
	if id == "" {
		return ""
	}
	return loginBaseURL + "/ptlogin/captcha.do?captchaId=" + url.QueryEscape(id)
}
func firstMatch(s, pattern string) string {
	m := regexp.MustCompile(pattern).FindStringSubmatch(s)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func generateUCWebRecordIDP() (string, error) {
	parts := []int{8, 7, 5}
	result := make([]string, len(parts))
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	for i, length := range parts {
		buf := make([]byte, length)
		for j := range buf {
			value, err := randomHexString(1)
			if err != nil {
				return "", err
			}
			idx := int(value[0]) % len(chars)
			buf[j] = chars[idx]
		}
		result[i] = string(buf)
	}
	return strings.Join(result, "-"), nil
}
