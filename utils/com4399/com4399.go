package com4399

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Yeah114/FunAuth/internal/db"
	httpproxy "github.com/Yeah114/FunAuth/utils/proxy"
	account4399 "github.com/Yeah114/g79client/account/com4399"
)

const maxProxyRetries = 10

var idCardPattern = regexp.MustCompile(`\d{17}[\dXx]`)

type NewCom4399CookieParams struct {
	Timeout     time.Duration
	Username    string
	Password    string
	RealName    string
	IDCard      string
	CaptchaCode string
}

type Com4399CookieRegister struct {
	ctx          context.Context
	timeout      time.Duration
	autoProxy    bool
	autoProxySet bool
	autoCaptcha  bool
	skipLogin    bool

	username         string
	password         string
	realName         string
	idCard           string
	captchaCode      string
	idCode           *db.IDCode
	httpClient       *http.Client
	ocrHTTPClient    *http.Client
	ocrLogging       bool
	realNameAttempts int

	gettingProxyCallback     func()
	gotProxyCallback         func(proxy string)
	generatedAccountCallback func(username, password string)
	gettingRealNameCallback  func()
	gotRealNameCallback      func(realName, idCard string)
	registeringCallback      func(username string)
	registeredCallback       func(uid, username string)
	loggingInCallback        func(username string)
	loggedInCallback         func()
	gotCookieCallback        func(cookieLength int)
	errorCallback            func(stage string, err error)
	captchaCallback          func(reason, verifyURL string) (string, error)
}

func NewCom4399Cookie(ctx context.Context) (string, error) {
	return NewCom4399CookieRegister().
		WithContext(ctx).
		RegisterCookie()
}

func NewCom4399CookieWithParams(ctx context.Context, params NewCom4399CookieParams) (string, error) {
	return NewCom4399CookieRegister().
		WithContext(ctx).
		WithTimeout(params.Timeout).
		WithAccount(params.Username, params.Password).
		WithRealName(params.RealName, params.IDCard).
		WithCaptchaCode(params.CaptchaCode).
		RegisterCookie()
}

func NewCom4399CookieRegister() *Com4399CookieRegister {
	return &Com4399CookieRegister{
		ctx:              context.Background(),
		timeout:          30 * time.Second,
		ocrLogging:       true,
		realNameAttempts: 1,
	}
}

func (r *Com4399CookieRegister) WithContext(ctx context.Context) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	if ctx != nil {
		r.ctx = ctx
	}
	return r
}

func (r *Com4399CookieRegister) WithTimeout(timeout time.Duration) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	if timeout > 0 {
		r.timeout = timeout
	}
	return r
}

func (r *Com4399CookieRegister) WithAutoProxy(enabled bool) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	r.autoProxy = enabled
	r.autoProxySet = true
	return r
}

func (r *Com4399CookieRegister) WithGettingProxyCallback(callback func()) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	r.gettingProxyCallback = callback
	return r
}

func (r *Com4399CookieRegister) WithGotProxyCallback(callback func(proxy string)) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	r.gotProxyCallback = callback
	return r
}

func (r *Com4399CookieRegister) WithAutoCaptcha(enabled bool) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	r.autoCaptcha = enabled
	return r
}

// WithSkipLogin returns after successful web registration and real-name
// verification instead of logging in to obtain a short-lived cookie.
func (r *Com4399CookieRegister) WithSkipLogin(skip bool) *Com4399CookieRegister {
	if r != nil {
		r.skipLogin = skip
	}
	return r
}

func (r *Com4399CookieRegister) WithGeneratedAccountCallback(callback func(username, password string)) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	r.generatedAccountCallback = callback
	return r
}

func (r *Com4399CookieRegister) WithGettingRealNameCallback(callback func()) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	r.gettingRealNameCallback = callback
	return r
}

func (r *Com4399CookieRegister) WithGotRealNameCallback(callback func(realName, idCard string)) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	r.gotRealNameCallback = callback
	return r
}

func (r *Com4399CookieRegister) WithRegisteringCallback(callback func(username string)) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	r.registeringCallback = callback
	return r
}

func (r *Com4399CookieRegister) WithRegisteredCallback(callback func(uid, username string)) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	r.registeredCallback = callback
	return r
}

func (r *Com4399CookieRegister) WithLoggingInCallback(callback func(username string)) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	r.loggingInCallback = callback
	return r
}

func (r *Com4399CookieRegister) WithLoggedInCallback(callback func()) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	r.loggedInCallback = callback
	return r
}

func (r *Com4399CookieRegister) WithGotCookieCallback(callback func(cookieLength int)) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	r.gotCookieCallback = callback
	return r
}

func (r *Com4399CookieRegister) WithErrorCallback(callback func(stage string, err error)) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	r.errorCallback = callback
	return r
}

func (r *Com4399CookieRegister) WithCaptchaCallback(callback func(reason, verifyURL string) (string, error)) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	r.captchaCallback = callback
	return r
}

func (r *Com4399CookieRegister) WithAccount(username, password string) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	r.username = strings.TrimSpace(username)
	r.password = strings.TrimSpace(password)
	return r
}

func (r *Com4399CookieRegister) WithRealName(realName, idCard string) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	r.realName = strings.TrimSpace(realName)
	r.idCard = strings.TrimSpace(idCard)
	return r
}

func (r *Com4399CookieRegister) WithCaptchaCode(captchaCode string) *Com4399CookieRegister {
	if r == nil {
		return r
	}
	r.captchaCode = strings.TrimSpace(captchaCode)
	return r
}

// WithHTTPClient makes the whole registration flow use a caller-owned client.
func (r *Com4399CookieRegister) WithHTTPClient(client *http.Client) *Com4399CookieRegister {
	if r != nil && client != nil {
		r.httpClient = client
	}
	return r
}

// WithOCRHTTPClient optionally uses a separate client for captcha image
// downloads while registration requests keep using WithHTTPClient.
func (r *Com4399CookieRegister) WithOCRHTTPClient(client *http.Client) *Com4399CookieRegister {
	if r != nil && client != nil {
		r.ocrHTTPClient = client
	}
	return r
}

func (r *Com4399CookieRegister) WithOCRLogging(enabled bool) *Com4399CookieRegister {
	if r != nil {
		r.ocrLogging = enabled
	}
	return r
}

// WithRealNameAttempts controls submissions using the same idcode record.
func (r *Com4399CookieRegister) WithRealNameAttempts(attempts int) *Com4399CookieRegister {
	if r != nil && attempts > 0 {
		r.realNameAttempts = attempts
	}
	return r
}

func (r *Com4399CookieRegister) RegisterCookie() (string, error) {
	if r == nil {
		return "", errors.New("com4399: register is nil")
	}
	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// 代理重试循环
	for proxyRetry := 0; proxyRetry < maxProxyRetries; proxyRetry++ {
		r.notifyGettingProxy()
		httpClient, proxyText, err := r.resolveHTTPClient(ctx)
		if err != nil {
			// 获取代理失败，刷新代理池后重试
			if proxyRetry < maxProxyRetries-1 {
				log.Printf("[com4399] 获取代理失败 (第 %d 次): %v，正在重试...", proxyRetry+1, err)
				continue
			}
			r.notifyError("获取代理", err)
			return "", err
		}
		r.notifyGotProxy(proxyText)

		req, err := r.buildRegisterRequest(ctx)
		if err != nil {
			r.notifyError("准备注册资料", err)
			return "", err
		}

		webClient := account4399.NewWebRegisterClient(httpClient)
		var registerResult *account4399.WebRegisterResult
		for {
			r.notifyRegistering(req.Username)
			registerResult, err = webClient.Register(ctx, req)
			if err == nil {
				break
			}
			var captchaErr *account4399.WebCaptchaRequiredError
			if errors.As(err, &captchaErr) && captchaErr != nil {
				if r.captchaCallback != nil {
					captcha, captchaInputErr := r.captchaCallback(strings.TrimSpace(captchaErr.Message), strings.TrimSpace(captchaErr.CaptchaURL))
					if captchaInputErr != nil {
						r.notifyError("注册 4399 账号", captchaInputErr)
						return "", captchaInputErr
					}
					req.CaptchaCode = strings.TrimSpace(captcha)
					continue
				}
				if r.autoCaptcha {
					ocrHTTPClient := r.ocrHTTPClient
					if ocrHTTPClient == nil {
						ocrHTTPClient = httpClient
					}
					captcha, ocrErr := RecognizeCaptchaWithHTTPClient(captchaErr.CaptchaURL, ocrHTTPClient, r.ocrLogging)
					if ocrErr != nil {
						r.notifyError("OCR 识别验证码", ocrErr)
						return "", fmt.Errorf("com4399: OCR 识别验证码失败: %w", ocrErr)
					}
					req.CaptchaCode = strings.TrimSpace(captcha)
					continue
				}
			}
			if errors.Is(err, account4399.ErrRealNameRejected) {
				if r.realNameAttempts > 1 {
					const maxRealNameRecords = 30
					for recordTry := 0; recordTry < maxRealNameRecords && registerResult == nil; recordTry++ {
						if recordTry > 0 {
							if r.idCode != nil && r.idCode.ID > 0 {
								if delErr := db.DeleteIDCode(r.idCode.ID); delErr != nil {
									log.Printf("[com4399] delete rejected real-name record failed: id=%d err=%v", r.idCode.ID, delErr)
								}
								r.idCode = nil
							}
							if r.gettingRealNameCallback != nil {
								r.gettingRealNameCallback()
							}
							newName, newIDCard, newIDCode, fetchErr := r.fetchRealNameFromDB(ctx)
							if fetchErr != nil {
								r.notifyError("get replacement real name", fetchErr)
								return "", fetchErr
							}
							req.RealName = newName
							req.IDCard = newIDCard
							r.idCode = newIDCode
							if r.gotRealNameCallback != nil {
								r.gotRealNameCallback(newName, newIDCard)
							}
						}
						for attempt := 0; attempt < r.realNameAttempts; attempt++ {
							if attempt > 0 && !waitForRealNameRetry(ctx) {
								return "", ctx.Err()
							}
							callbackURL, submitErr := webClient.SubmitRealName(ctx, req)
							if submitErr != nil {
								err = submitErr
								if errors.Is(submitErr, account4399.ErrRealNameRejected) {
									continue
								}
								r.notifyError("submit real name", submitErr)
								return "", submitErr
							}
							if followErr := webClient.FollowCallback(ctx, callbackURL); followErr != nil {
								r.notifyError("follow real-name callback", followErr)
								return "", followErr
							}
							registerResult = webClient.BuildRegisterResult(callbackURL)
							break
						}
					}
					if registerResult == nil {
						r.notifyError("real-name records exhausted", err)
						return "", err
					}
					break
				}
				// 账号已注册成功，只需重试实名提交
				const maxRealNameRetries = 30
				for retry := 0; retry < maxRealNameRetries; retry++ {
					if r.idCode != nil && r.idCode.ID > 0 {
						log.Printf("[com4399] 实名信息被拒绝，删除数据库记录: id=%d name=%s idcard=%s (%v)", r.idCode.ID, r.idCode.Name, r.idCode.IDCard, err)
						if delErr := db.DeleteIDCode(r.idCode.ID); delErr != nil {
							log.Printf("[com4399] 删除实名信息失败: %v", delErr)
						}
						r.idCode = nil
					}
					if r.gettingRealNameCallback != nil {
						r.gettingRealNameCallback()
					}
					newName, newIDCard, newIDCode, fetchErr := r.fetchRealNameFromDB(ctx)
					if fetchErr != nil {
						r.notifyError("重新获取实名信息", fetchErr)
						return "", fetchErr
					}
					req.RealName = newName
					req.IDCard = newIDCard
					r.idCode = newIDCode
					if r.gotRealNameCallback != nil {
						r.gotRealNameCallback(newName, newIDCard)
					}
					r.notifyRegistering(req.Username)
					callbackURL, realNameErr := webClient.SubmitRealName(ctx, req)
					if realNameErr != nil {
						err = realNameErr
						if errors.Is(realNameErr, account4399.ErrRealNameRejected) {
							continue // 继续重试
						}
						r.notifyError("提交实名信息", realNameErr)
						return "", realNameErr
					}
					if followErr := webClient.FollowCallback(ctx, callbackURL); followErr != nil {
						r.notifyError("授权回调", followErr)
						return "", followErr
					}
					registerResult = webClient.BuildRegisterResult(callbackURL)
					break // 实名成功，退出重试循环
				}
				if registerResult == nil {
					r.notifyError("实名重试", err)
					return "", err
				}
				break // 退出外层注册循环
			}
			// 检查是否是网络错误，需要换代理重试
			if isNetworkError(err) && r.hasFixedHTTPClient() {
				r.notifyError("固定代理网络错误", err)
				return "", err
			}
			if isNetworkError(err) && proxyRetry < maxProxyRetries-1 {
				log.Printf("[com4399] 注册网络错误 (第 %d 次，代理 %s): %v，换代理重试...", proxyRetry+1, proxyText, err)
				break // 跳出内层循环，进入下一次代理重试
			}
			r.notifyError("注册 4399 账号", err)
			return "", err
		}

		// 如果 registerResult 为 nil，说明是网络错误需要换代理
		if registerResult == nil {
			continue
		}

		r.notifyRegistered(registerResult)
		if r.skipLogin {
			return "", nil
		}

		r.notifyLoggingIn(req.Username)
		loginClient := account4399.NewClient(httpClient)
		cookie, err := loginClient.RegisterCookieWithPassword(ctx, req.Username, req.Password)
		if err != nil {
			// 检查是否是网络错误，需要换代理重试
			if isNetworkError(err) && r.hasFixedHTTPClient() {
				r.notifyError("固定代理网络错误", err)
				return "", err
			}
			if isNetworkError(err) && proxyRetry < maxProxyRetries-1 {
				log.Printf("[com4399] 登录网络错误 (第 %d 次，代理 %s): %v，换代理重试...", proxyRetry+1, proxyText, err)
				continue
			}
			r.notifyError("登录 4399 账号并换取 Cookie", err)
			return "", err
		}
		r.notifyLoggedIn()

		cookie = strings.TrimSpace(cookie)
		if cookie == "" {
			err := errors.New("com4399: login succeeded but cookie is empty")
			r.notifyError("生成 Cookie", err)
			return "", err
		}

		r.notifyGotCookie(len(cookie))
		return cookie, nil
	}

	return "", errors.New("com4399: exceeded maximum proxy retries")
}

func waitForRealNameRetry(ctx context.Context) bool {
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	// 检查常见的网络错误类型
	errStr := err.Error()
	// 超时错误
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
		return true
	}
	// 连接错误
	if strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "connection closed") || strings.Contains(errStr, "no route to host") {
		return true
	}
	// 代理错误
	if strings.Contains(errStr, "proxy") || strings.Contains(errStr, "dial") {
		return true
	}
	// EOF 错误（连接被意外关闭）
	if strings.Contains(errStr, "EOF") {
		return true
	}
	// TLS/SSL 错误
	if strings.Contains(errStr, "tls") || strings.Contains(errStr, "certificate") || strings.Contains(errStr, "handshake") {
		return true
	}
	// HTTP 状态码 5xx (服务器错误) 可能是代理问题
	if strings.Contains(errStr, "500") || strings.Contains(errStr, "502") || strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504") || strings.Contains(errStr, "529") {
		return true
	}
	// 上下文超时
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	return false
}

func (r *Com4399CookieRegister) notifyGettingProxy() {
	if r != nil && r.gettingProxyCallback != nil {
		r.gettingProxyCallback()
	}
}

func (r *Com4399CookieRegister) notifyGotProxy(proxy string) {
	if r != nil && r.gotProxyCallback != nil {
		r.gotProxyCallback(strings.TrimSpace(proxy))
	}
}

func (r *Com4399CookieRegister) notifyRegistering(username string) {
	if r != nil && r.registeringCallback != nil {
		r.registeringCallback(strings.TrimSpace(username))
	}
}

func (r *Com4399CookieRegister) notifyRegistered(result *account4399.WebRegisterResult) {
	if r == nil || r.registeredCallback == nil {
		return
	}
	uid := ""
	username := ""
	if result != nil {
		uid = strings.TrimSpace(result.UID)
		username = strings.TrimSpace(result.Username)
	}
	r.registeredCallback(uid, username)
}

func (r *Com4399CookieRegister) notifyLoggingIn(username string) {
	if r != nil && r.loggingInCallback != nil {
		r.loggingInCallback(strings.TrimSpace(username))
	}
}

func (r *Com4399CookieRegister) notifyLoggedIn() {
	if r != nil && r.loggedInCallback != nil {
		r.loggedInCallback()
	}
}

func (r *Com4399CookieRegister) notifyGotCookie(cookieLength int) {
	if r != nil && r.gotCookieCallback != nil {
		r.gotCookieCallback(cookieLength)
	}
}

func (r *Com4399CookieRegister) notifyError(stage string, err error) {
	if r != nil && r.errorCallback != nil {
		r.errorCallback(strings.TrimSpace(stage), err)
	}
}

func (r *Com4399CookieRegister) autoProxyEnabled() bool {
	if r != nil && r.autoProxySet {
		return r.autoProxy
	}
	return false
}

func (r *Com4399CookieRegister) resolveHTTPClient(ctx context.Context) (*http.Client, string, error) {
	if r != nil && r.httpClient != nil {
		return r.httpClient, "", nil
	}
	return newHTTPClient(ctx, r.timeout, r.autoProxyEnabled())
}

func (r *Com4399CookieRegister) hasFixedHTTPClient() bool {
	return r != nil && r.httpClient != nil
}

func (r *Com4399CookieRegister) buildRegisterRequest(ctx context.Context) (account4399.WebRegisterRequest, error) {
	username := strings.TrimSpace(r.username)
	if username == "" {
		generated, err := randomRegisterUsername()
		if err != nil {
			return account4399.WebRegisterRequest{}, err
		}
		username = generated
	}

	password := strings.TrimSpace(r.password)
	if password == "" {
		generated, err := randomRegisterPassword()
		if err != nil {
			return account4399.WebRegisterRequest{}, err
		}
		password = generated
	}
	if r.generatedAccountCallback != nil {
		r.generatedAccountCallback(username, password)
	}

	realName := strings.TrimSpace(r.realName)
	idCard := strings.TrimSpace(r.idCard)
	if realName == "" || idCard == "" {
		if r.gettingRealNameCallback != nil {
			r.gettingRealNameCallback()
		}
		centerRealName, centerIDCard, idCode, err := r.fetchRealNameFromDB(ctx)
		if err != nil {
			return account4399.WebRegisterRequest{}, err
		}
		if realName == "" {
			realName = centerRealName
		}
		if idCard == "" {
			idCard = centerIDCard
		}
		r.idCode = idCode
		if r.gotRealNameCallback != nil {
			r.gotRealNameCallback(realName, idCard)
		}
	}
	if realName == "" || idCard == "" {
		return account4399.WebRegisterRequest{}, errors.New("com4399: real name and id card are required")
	}

	return account4399.WebRegisterRequest{
		Username:    username,
		Password:    password,
		RealName:    realName,
		IDCard:      idCard,
		CaptchaCode: strings.TrimSpace(r.captchaCode),
	}, nil
}

func (r *Com4399CookieRegister) fetchRealNameFromDB(ctx context.Context) (string, string, *db.IDCode, error) {
	idCode, err := db.GetRandomIDCode()
	if err != nil {
		return "", "", nil, fmt.Errorf("com4399: get id code from db: %w", err)
	}
	realName := strings.TrimSpace(idCode.Name)
	idCard := strings.TrimSpace(idCode.IDCard)
	if realName == "" || idCard == "" {
		return "", "", nil, errors.New("com4399: idcode from db is incomplete")
	}
	return realName, idCard, idCode, nil
}

func parseRealNamePayload(payload string) (string, string) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return "", ""
	}

	var raw any
	if json.Unmarshal([]byte(payload), &raw) == nil {
		realName, idCard := realNameFields(raw)
		if realName != "" || idCard != "" {
			return realName, idCard
		}
	}

	if realName, idCard := realNameText(payload); realName != "" || idCard != "" {
		return realName, idCard
	}
	return "", ""
}

func realNameText(payload string) (string, string) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return "", ""
	}

	for _, sep := range []string{",", "\uFF0C", "\n", "\t", "|", " ", "\r\n"} {
		parts := strings.Split(payload, sep)
		if len(parts) >= 2 {
			realName := cleanRealNameText(parts[0])
			idCard := idCardPattern.FindString(parts[1])
			if idCard != "" {
				return realName, idCard
			}
		}
	}

	idCard := idCardPattern.FindString(payload)
	if idCard != "" {
		realName := cleanRealNameText(strings.Replace(payload, idCard, "", 1))
		return realName, idCard
	}
	return "", ""
}

func cleanRealNameText(text string) string {
	text = strings.TrimSpace(text)
	for _, prefix := range []string{"real_name", "realName", "realname", "name", "\u59D3\u540D"} {
		text = strings.TrimPrefix(text, prefix)
	}
	return strings.Trim(text, ":\uFF1A,\uFF0C;\uFF1B \t\r\n")
}

func realNameFields(value any) (string, string) {
	switch typed := value.(type) {
	case map[string]any:
		realName := firstStringField(typed, "real_name", "realName", "realname", "name", "\u59D3\u540D")
		idCard := firstStringField(typed, "id_card", "idCard", "id_card_number", "idCardNumber", "id_num", "idNum", "idno", "idNo", "idcard", "idCard", "card", "card_no", "cardNo", "identity", "identity_card", "identityCard", "number", "id", "\u8EAB\u4EFD\u8BC1", "\u8EAB\u4EFD\u8BC1\u53F7")
		if realName != "" || idCard != "" {
			return realName, idCard
		}
		for _, key := range []string{"data", "result"} {
			if nested, ok := typed[key]; ok {
				if realName, idCard := realNameFields(nested); realName != "" || idCard != "" {
					return realName, idCard
				}
			}
		}
		for _, nested := range typed {
			if realName, idCard := realNameFields(nested); realName != "" || idCard != "" {
				return realName, idCard
			}
		}
	case []any:
		if len(typed) >= 2 {
			return strings.TrimSpace(fmt.Sprint(typed[0])), strings.TrimSpace(fmt.Sprint(typed[1]))
		}
		for _, nested := range typed {
			if realName, idCard := realNameFields(nested); realName != "" || idCard != "" {
				return realName, idCard
			}
		}
	case string:
		return realNameText(typed)
	}
	return "", ""
}

func firstStringField(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func randomRegisterUsername() (string, error) {
	token, err := randomHex(8)
	if err != nil {
		return "", fmt.Errorf("com4399: generate username: %w", err)
	}
	return "hz" + token, nil
}

func randomRegisterPassword() (string, error) {
	token, err := randomHex(8)
	if err != nil {
		return "", fmt.Errorf("com4399: generate password: %w", err)
	}
	return "Hz" + token + "9", nil
}

func randomHex(byteLen int) (string, error) {
	if byteLen <= 0 {
		return "", errors.New("com4399: random byte length must be positive")
	}
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func newHTTPClient(ctx context.Context, timeout time.Duration, autoProxy bool) (*http.Client, string, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if !autoProxy {
		return &http.Client{
			Timeout:   timeout,
			Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}, "", nil
	}

	proxyURL, err := httpproxy.RandomProxyURL(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("com4399: random proxy: %w", err)
	}

	transport.Proxy = http.ProxyURL(proxyURL)
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, proxyURL.String(), nil
}
