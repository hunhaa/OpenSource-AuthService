package com4399

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	mrand "math/rand/v2"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ---------- Direct Register (ptlogin.4399.com/ptlogin/register.do) ----------
// 与 MCQTSS / boluoreg 一致的轻量注册端点。
// 优势：不走 OAuth device-oauth 重流程，交互步骤少，降低风控率；
//       便于接入代理池、UA 随机、SFZ 随机、自定义 OCR。

const (
	directRegisterURL      = "https://ptlogin.4399.com/ptlogin/register.do"
	directCaptchaURL       = "https://ptlogin.4399.com/ptlogin/captcha.do?captchaId="
	directReferer          = "https://ptlogin.4399.com/ptlogin/regFrame.do"
	directLoginURL         = "https://ptlogin.4399.com/ptlogin/login.do?v=1"
	directCheckKidLoginURL = "http://ptlogin.4399.com/ptlogin/checkKidLoginUserCookie.do"
	directSdkInfoURL       = "https://microgame.5054399.net/v2/service/sdk/info?callback="
)

type DirectRegisterRequest struct {
	Username string
	Password string
	RealName string // 中文姓名
	IDCard   string // 18 位身份证号
	Captcha  string // 可选，留空内部自动识别
	SessionID string // 可选，留空会生成 captchaReq+19 位随机
	UserAgent string // 可选，留空用随机
	Transport http.RoundTripper // 可选，代理通过 &http.Transport{Proxy:http.ProxyURL(proxyURL)}
}

type DirectRegisterResult struct {
	Username      string
	Password      string
	RealName      string
	IDCard        string
	Message       string
	Success       bool
	CookieString  string // 登录生成 sauth 后填入
	DisplayMessage string
}

var capAlphabet = []byte("abcdefghijklmnopqrstuvwxyz0123456789")
var userAlphabet = []byte("abcdefghijklmnopqrstuvwxyz0123456789")

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = crand.Read(b)
	return b
}

func randomString(bucket []byte, n int) string {
	buf := make([]byte, n)
	for i := range buf {
		b := randomBytes(1)
		buf[i] = bucket[int(b[0])%len(bucket)]
	}
	return string(buf)
}

func randomSessionID() string {
	return "captchaReq" + randomString(capAlphabet, 19)
}

func randomUserAgent() string {
	list := []string{
		"Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Mobile Safari/537.36",
		"Mozilla/5.0 (Linux; Android 12; SM-G991B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Mobile Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Linux; Android 14; MI 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Mobile Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 16_7_8 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (Linux; Android 11; Redmi K40) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Mobile Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 15_8 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.6 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Mobile Safari/537.36",
	}
	return list[mrand.IntN(len(list))]
}

func newDirectHTTPClient(transport http.RoundTripper) *http.Client {
	if transport == nil {
		transport = &http.Transport{
			IdleConnTimeout: 30 * time.Second,
		}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
	}
}

var captchaIDRegexp = regexp.MustCompile(`/ptlogin/captcha\.do\?captchaId=([\w\d]+)`)

// getCaptchaImage 下载验证码图像
func getCaptchaImage(ctx context.Context, httpc *http.Client, ua, sid string) ([]byte, error) {
	u := directCaptchaURL + url.QueryEscape(sid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", directReferer)
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("验证码下载失败 status=%d body=%s", resp.StatusCode, string(b))
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(b) < 200 {
		return nil, fmt.Errorf("验证码图像太小 %d byte", len(b))
	}
	return b, nil
}

func doHTTP(ctx context.Context, httpc *http.Client, method, rawURL, body, ct, ua, referer string) ([]byte, int, http.Header, error) {
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rdr)
	if err != nil {
		return nil, 0, nil, err
	}
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := httpc.Do(req)
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

// ---------- 注册 ----------

// DirectRegister 执行轻量 ptlogin 直接注册
func DirectRegister(ctx context.Context, req *DirectRegisterRequest) (*DirectRegisterResult, error) {
	if req == nil {
		return nil, errors.New("com4399: DirectRegisterRequest 为空")
	}
	if !registerUsernamePattern.MatchString(req.Username) {
		return nil, fmt.Errorf("%w: username %q 不合法（3-20 位字母数字下划线/@）", ErrInvalidRegisterInput, req.Username)
	}
	if !registerPasswordPattern.MatchString(req.Password) {
		return nil, fmt.Errorf("%w: password 不合法（6-20 位）", ErrInvalidRegisterInput)
	}
	if !registerIDCardPattern.MatchString(req.IDCard) {
		return nil, fmt.Errorf("%w: idcard %q 不合法（15/18 位身份证号）", ErrInvalidRegisterInput, req.IDCard)
	}
	if strings.TrimSpace(req.RealName) == "" {
		return nil, fmt.Errorf("%w: realname 不能为空", ErrInvalidRegisterInput)
	}

	ua := req.UserAgent
	if ua == "" {
		ua = randomUserAgent()
	}
	sid := req.SessionID
	if sid == "" {
		sid = randomSessionID()
	}
	httpc := newDirectHTTPClient(req.Transport)

	// 获取并识别验证码（最多重试 3 次）
	// 注意：识别失败直接返回错误，禁止使用随机值蒙混——验证码错误会被 4399 风控标记
	captcha := req.Captcha
	if captcha == "" {
		var lastErr error
		for i := 0; i < 3; i++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			captchaImg, err := getCaptchaImage(ctx, httpc, ua, sid)
			if err != nil {
				lastErr = err
				time.Sleep(500 * time.Millisecond)
				continue
			}
			recognized, err := RecognizeCaptchaBytes(captchaImg)
			if err != nil {
				lastErr = err
				if errors.Is(err, ErrCaptchaEngineNotReady) {
					// OCR 引擎未就绪，无需再重试
					break
				}
				time.Sleep(500 * time.Millisecond)
				continue
			}
			if len(recognized) == 4 {
				captcha = strings.ToLower(recognized)
				break
			}
			lastErr = fmt.Errorf("识别结果 %q 不是 4 位，重试", recognized)
			time.Sleep(400 * time.Millisecond)
			sid = randomSessionID()
		}
		if captcha == "" {
			log.Printf("[4399-Direct] 验证码识别失败（放弃本次注册）: %v", lastErr)
			return nil, fmt.Errorf("%w: %v", ErrCaptchaFailed, lastErr)
		}
	}

	encPwd, err := encryptWebRegisterAES(req.Password)
	if err != nil {
		return nil, err
	}
	encName, err := encryptWebRegisterAES(strings.TrimSpace(req.RealName))
	if err != nil {
		return nil, err
	}
	encCard, err := encryptWebRegisterAES(strings.TrimSpace(req.IDCard))
	if err != nil {
		return nil, err
	}
	email := fmt.Sprintf("%s@qq.com", randomString(capAlphabet, 10))

	form := url.Values{}
	form.Set("postLoginHandler", "default")
	form.Set("displayMode", "popup")
	form.Set("bizId", "")
	form.Set("appId", "www_home")
	form.Set("gameId", "")
	form.Set("cid", "")
	form.Set("externalLogin", "qq")
	form.Set("aid", "")
	form.Set("ref", "")
	form.Set("css", "//www.4399.com/css/4399_index_skin.css")
	form.Set("redirectUrl", "")
	form.Set("regMode", "reg_normal")
	form.Set("sessionId", "")
	form.Set("regIdcard", "true")
	form.Set("noEmail", "")
	form.Set("crossDomainIFrame", "")
	form.Set("crossDomainUrl", "")
	form.Set("mainDivId", "popup_reg_div")
	form.Set("showRegInfo", "true")
	form.Set("includeFcmInfo", "false")
	form.Set("expandFcmInput", "true")
	form.Set("fcmFakeValidate", "false")
	form.Set("realnameValidate", "true")
	form.Set("userNameLabel", "4399用户名")
	form.Set("level", "4")
	form.Set("sec", "1")
	form.Set("iframeId", "popup_reg_frame")
	form.Set("email", email)
	form.Set("reg_eula_agree", "on")
	form.Set("autoLogin", "on")
	form.Set("username", req.Username)
	form.Set("password", encPwd)
	form.Set("passwordveri", encPwd)
	form.Set("realname", encName)
	form.Set("idcard", encCard)
	form.Set("inputCaptcha", captcha)
	form.Set("captcha_id", sid)

	body, status, _, err := doHTTP(ctx, httpc, http.MethodPost, directRegisterURL,
		form.Encode(), "application/x-www-form-urlencoded", ua, directReferer)
	if err != nil {
		return nil, err
	}
	html := string(body)

	msg := extractHTMLMessage(html)
	res := &DirectRegisterResult{
		Username: req.Username,
		Password: req.Password,
		RealName: req.RealName,
		IDCard:   req.IDCard,
		Message:  msg,
	}

	switch {
	case strings.Contains(html, "注册成功"):
		res.Success = true
		res.DisplayMessage = "注册成功"
	case strings.Contains(html, "验证码错误"), strings.Contains(html, "验证码不正确"), strings.Contains(html, "验证码超时"):
		res.DisplayMessage = "验证码错误"
	case strings.Contains(html, "用户名已被注册"), strings.Contains(html, "用户名已存在"):
		res.DisplayMessage = "用户名已被注册"
		return res, ErrUsernameExists
	case strings.Contains(html, "身份证实名帐号数量超过限制"), strings.Contains(html, "身份证实名账号数量超过限制"):
		res.DisplayMessage = "身份证实名账号数量超过限制"
		return res, ErrRealNameRejected
	case strings.Contains(html, "身份证实名过于频繁"):
		res.DisplayMessage = "身份证实名过于频繁"
		return res, ErrRiskControlTriggered
	case strings.Contains(html, "该姓名身份证提交验证过于频繁"):
		res.DisplayMessage = "该姓名身份证提交验证过于频繁"
		return res, ErrRiskControlTriggered
	case strings.Contains(html, "身份证异常或错误"), strings.Contains(html, "您的身份证异常或错误"):
		res.DisplayMessage = "您的身份证异常或错误"
		return res, ErrRealNameRejected
	case strings.Contains(html, "姓名身份证不匹配"):
		res.DisplayMessage = "姓名身份证不匹配"
		return res, ErrRealNameRejected
	case strings.Contains(html, "用户名包含敏感字符"):
		res.DisplayMessage = "用户名包含敏感字符"
		return res, ErrInvalidRegisterInput
	case strings.Contains(html, "身份证实名账号数量超过时段限制"):
		res.DisplayMessage = "身份证实名账号数量超过时段限制"
		return res, ErrRealNameRejected
	case strings.Contains(html, "请稍后再试"):
		res.DisplayMessage = "请稍后再试"
		return res, ErrRiskControlTriggered
	default:
		if msg == "" {
			msg = preview(html)
		}
		res.DisplayMessage = fmt.Sprintf("未知响应(status=%d): %s", status, msg)
		log.Printf("[DEBUG] DirectRegister 未知响应 前500字符: %s", preview(html))
		return res, ErrRegisterRejected
	}
	return res, nil
}

// ---------- 组合：注册 → 登录 → 生成 SAuth Cookie ----------

// DirectRegisterAndLoginCookie 一步完成注册 + 登录生成 sauth
func DirectRegisterAndLoginCookie(ctx context.Context, req *DirectRegisterRequest) (string, *DirectRegisterResult, error) {
	res, err := DirectRegister(ctx, req)
	if err != nil {
		return "", res, err
	}
	if !res.Success {
		return "", res, fmt.Errorf("注册未成功: %s", res.DisplayMessage)
	}
	cookie, err := DirectLoginSAuth(ctx, req)
	if err != nil {
		return "", res, fmt.Errorf("登录生成sauth失败: %w", err)
	}
	res.CookieString = cookie
	return cookie, res, nil
}

// ---------- 登录生成 SAuth ----------

type sdkLoginData struct {
	Username string
	UID      string
	Token    string
	Time     string
}

func parseSdkLoginData(body []byte) (*sdkLoginData, error) {
	// body 形如 callback({"code":0,"data":...}) 或纯 JSON
	raw := strings.TrimSpace(string(body))
	if idx := strings.Index(raw, "{"); idx >= 0 {
		raw = raw[idx:]
	}
	if idx := strings.LastIndex(raw, "}"); idx >= 0 {
		raw = raw[:idx+1]
	}
	var wrap struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			SdkLoginData string `json:"sdk_login_data"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &wrap); err != nil {
		return nil, fmt.Errorf("sdk/info JSON解析失败: %w body=%s", err, preview(raw))
	}
	if wrap.Data.SdkLoginData == "" {
		return nil, fmt.Errorf("sdk/info 返回无 sdk_login_data (code=%d msg=%s)", wrap.Code, wrap.Msg)
	}
	d := &sdkLoginData{}
	for _, kv := range strings.Split(wrap.Data.SdkLoginData, "&") {
		kv2 := strings.SplitN(kv, "=", 2)
		if len(kv2) != 2 {
			continue
		}
		v, _ := url.QueryUnescape(kv2[1])
		switch kv2[0] {
		case "username":
			d.Username = v
		case "uid":
			d.UID = v
		case "token":
			d.Token = v
		case "time":
			d.Time = v
		}
	}
	if d.UID == "" || d.Token == "" || d.Time == "" || d.Username == "" {
		return nil, fmt.Errorf("sdk_login_data 字段缺失(%+v)", d)
	}
	return d, nil
}

type sauthParts struct {
	SdkUserID string
	SessionID string
	Timestamp string
	UserID    string
	RealName  string
}

func newUUIDUpper() string {
	buf := make([]byte, 16)
	_, _ = crand.Read(buf)
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X%02X",
		buf[0], buf[1], buf[2], buf[3], buf[4], buf[5], buf[6], buf[7],
		buf[8], buf[9], buf[10], buf[11], buf[12], buf[13], buf[14], buf[15])
}

func buildSAuthJSON(p sauthParts) (string, error) {
	aimInfo := map[string]string{
		"aim":     "127.0.0.1",
		"country": "CN",
		"tz":      "0800",
		"tzid":    "",
	}
	aimB, err := json.Marshal(aimInfo)
	if err != nil {
		return "", err
	}
	realName := map[string]string{"realname_type": "0"}
	realB, err := json.Marshal(realName)
	if err != nil {
		return "", err
	}
	sauth := map[string]any{
		"gameid":           "x19",
		"login_channel":    "4399pc",
		"app_channel":      "4399pc",
		"platform":         "pc",
		"sdkuid":           p.SdkUserID,
		"sessionid":        p.SessionID,
		"sdk_version":      "1.0.0",
		"udid":             newUUIDUpper(),
		"deviceid":         newUUIDUpper(),
		"aim_info":         string(aimB),
		"client_login_sn":  newUUIDUpper(),
		"gas_token":        "",
		"source_platform":  "pc",
		"ip":               "127.0.0.1",
		"userid":           p.UserID,
		"realname":         string(realB),
		"timestamp":        p.Timestamp,
	}
	_ = p.RealName
	b, err := json.Marshal(sauth)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DirectLoginSAuth 账号密码登录生成 g79client 可消费的 cookie JSON 字符串
func DirectLoginSAuth(ctx context.Context, req *DirectRegisterRequest) (string, error) {
	ua := req.UserAgent
	if ua == "" {
		ua = randomUserAgent()
	}
	httpc := newDirectHTTPClient(req.Transport)
	jar, _ := cookiejar.New(nil)
	httpc.Jar = jar

	// 1) verify.do 检测
	sessionUUID := newUUIDUpper()
	verifyURL := fmt.Sprintf(
		"https://ptlogin.4399.com/ptlogin/verify.do?username=%s&appId=kid_wdsj&t=%s&inputWidth=iptw2&v=1",
		url.QueryEscape(req.Username),
		hex.EncodeToString(randomBytes(16)),
	)
	verifyBody, _, _, err := doHTTP(ctx, httpc, http.MethodGet, verifyURL, "", "", ua, "")
	if err != nil {
		return "", err
	}
	match := captchaIDRegexp.FindSubmatch(verifyBody)
	var captcha string
	var verifySession string
	if len(match) >= 2 {
		verifySession = string(match[1])
		capImg, e := getCaptchaImage(ctx, httpc, ua, verifySession)
		if e == nil {
			if rec, e2 := RecognizeCaptchaBytes(capImg); e2 == nil && len(rec) == 4 {
				captcha = strings.ToLower(rec)
			} else {
				log.Printf("[4399-DirectLogin] 登录阶段验证码识别失败: cap=%v ocr=%v — 取消本次登录（不使用随机值避免风控）", e, e2)
				return "", fmt.Errorf("%w: 登录验证码识别失败 capImgErr=%v ocrErr=%v", ErrCaptchaFailed, e, e2)
			}
		}
	}

	// 2) login.do
	u4399, _ := url.Parse("https://ptlogin.4399.com")
	jar.SetCookies(u4399, []*http.Cookie{
		{Name: "ptusertype", Value: "kid_wdsj.4399_login", Path: "/", Domain: ".4399.com"},
		{Name: "USESSIONID", Value: sessionUUID, Path: "/", Domain: ".4399.com"},
	})

	loginPayload := url.Values{}
	loginPayload.Set("postLoginHandler", "default")
	loginPayload.Set("externalLogin", "qq")
	loginPayload.Set("bizId", "2100001792")
	loginPayload.Set("appId", "kid_wdsj")
	loginPayload.Set("gameId", "wd")
	loginPayload.Set("sec", "1")
	loginPayload.Set("password", req.Password)
	loginPayload.Set("username", req.Username)
	if captcha != "" && verifySession != "" {
		loginPayload.Set("redirectUrl", "")
		loginPayload.Set("sessionId", verifySession)
		loginPayload.Set("inputCaptcha", captcha)
	}
	_, status, _, err := doHTTP(ctx, httpc, http.MethodPost, directLoginURL,
		loginPayload.Encode(), "application/x-www-form-urlencoded", ua, "")
	if err != nil {
		return "", err
	}
	if status != 200 && status != 302 {
		return "", fmt.Errorf("登录失败 status=%d", status)
	}
	var uauth string
	for _, ck := range jar.Cookies(u4399) {
		if ck.Name == "Uauth" {
			uauth = ck.Value
			break
		}
	}
	if uauth == "" {
		return "", fmt.Errorf("登录失败：未获取 Uauth cookie（账号密码错误或IP被风控）")
	}
	parts := strings.Split(uauth, "|")
	randTime := parts[len(parts)-1]
	// 部分 Uauth 字段可能不是 | 分割，兜底取整串
	if randTime == "" {
		randTime = uauth
	}

	checkURL := fmt.Sprintf(
		"https://ptlogin.4399.com/ptlogin/checkKidLoginUserCookie.do?appId=kid_wdsj&gameUrl=http://cdn.h5wan.4399sj.com/microterminal-h5-frame?game_id=500352&rand_time=%s&nick=null&onLineStart=false&show=1&isCrossDomain=1&retUrl=http%%253A%%252F%%252Fptlogin.4399.com%%252Fresource%%252Fucenter.html",
		url.QueryEscape(randTime),
	)
	// 3) 跳转跟随，取最终 URL 的 queryStr
	checkReq, err := http.NewRequestWithContext(ctx, http.MethodPost, checkURL, nil)
	if err != nil {
		return "", err
	}
	checkReq.Header.Set("User-Agent", ua)
	checkResp, err := httpc.Do(checkReq)
	if err != nil {
		return "", fmt.Errorf("checkKidLogin请求失败: %w", err)
	}
	defer checkResp.Body.Close()
	io.Copy(io.Discard, checkResp.Body)
	finalQuery := ""
	if checkResp.Request != nil && checkResp.Request.URL != nil {
		finalQuery = checkResp.Request.URL.RawQuery
	}
	if finalQuery == "" {
		if idx := strings.Index(checkURL, "?"); idx >= 0 {
			finalQuery = checkURL[idx+1:]
		}
	}

	sdkInfoURL := directSdkInfoURL + "&queryStr=" + url.QueryEscape(finalQuery)
	sdkBody, _, _, err := doHTTP(ctx, httpc, http.MethodGet, sdkInfoURL, "", "", ua, "")
	if err != nil {
		return "", fmt.Errorf("sdk/info请求失败: %w", err)
	}
	sdkData, err := parseSdkLoginData(sdkBody)
	if err != nil {
		return "", err
	}
	sauth, err := buildSAuthJSON(sauthParts{
		SdkUserID: sdkData.UID,
		SessionID: sdkData.Token,
		Timestamp: sdkData.Time,
		UserID:    sdkData.Username,
		RealName:  req.RealName,
	})
	if err != nil {
		return "", err
	}
	wrapped, err := json.Marshal(map[string]string{"sauth_json": sauth})
	if err != nil {
		return "", err
	}
	return string(wrapped), nil
}
