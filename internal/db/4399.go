package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	httpproxy "github.com/Yeah114/FunAuth/utils/proxy"
	"github.com/Yeah114/g79client"
	account4399 "github.com/Yeah114/g79client/account/com4399"
	x19sdk "github.com/Yeah114/g79client/account/x19/sdk"
)

const (
	// A rate-limited proxy is discarded immediately. Keep trying the remaining
	// cached proxies before surfacing a registration failure.
	maxCom4399RegisterAttempts = 10
	checkEnterRoleID           = ""
	checkEnterHostID           = 8000
)

// Com4399ProxyEnabled 控制注册和登录4399时是否使用代理
// 默认为 false（关闭代理）
var Com4399ProxyEnabled = true

// SetCom4399ProxyEnabled 设置4399代理开关
func SetCom4399ProxyEnabled(enabled bool) {
	Com4399ProxyEnabled = enabled
}

// GetCom4399ProxyEnabled 获取4399代理开关状态
func GetCom4399ProxyEnabled() bool {
	return Com4399ProxyEnabled
}

type Com4399Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func IsCom4399CredentialsPayload(sauth string) bool {
	_, err := ParseCom4399Credentials(sauth)
	return err == nil
}

func IsX19Sauth(sauth string) bool {
	sauth = strings.TrimSpace(sauth)
	if sauth == "" {
		return false
	}
	if IsCom4399CredentialsPayload(sauth) {
		return false
	}

	_, sauthData, err := x19sdk.ParseCookieString(sauth)
	if err != nil || sauthData == nil {
		return false
	}

	loginChannel := strings.ToLower(strings.TrimSpace(sauthData.LoginChannel))
	appChannel := strings.ToLower(strings.TrimSpace(sauthData.AppChannel))
	sourcePlatform := strings.ToLower(strings.TrimSpace(sauthData.SourcePlatform))
	return loginChannel == "netease" || appChannel == "netease" || sourcePlatform == "pc"
}

func ParseCom4399Credentials(sauth string) (Com4399Credentials, error) {
	var creds Com4399Credentials
	if err := json.Unmarshal([]byte(strings.TrimSpace(sauth)), &creds); err != nil {
		return creds, fmt.Errorf("Com4399CredentialsError : parse credentials: %w", err)
	}
	creds.Username = strings.TrimSpace(creds.Username)
	creds.Password = strings.TrimSpace(creds.Password)
	if creds.Username == "" || creds.Password == "" {
		return creds, errors.New("Com4399CredentialsError : credentials are incomplete")
	}
	return creds, nil
}

func MarshalCom4399Credentials(username, password string) (string, error) {
	creds := Com4399Credentials{
		Username: strings.TrimSpace(username),
		Password: strings.TrimSpace(password),
	}
	if creds.Username == "" || creds.Password == "" {
		return "", errors.New("Com4399CredentialsError : username and password are required")
	}
	data, err := json.Marshal(creds)
	if err != nil {
		return "", fmt.Errorf("Com4399CredentialsError : marshal credentials: %w", err)
	}
	return string(data), nil
}

func EnsureCom4399CredentialsForUser(ctx context.Context, userUUID string) error {
	user, err := GetUserByUUID(userUUID)
	if err != nil {
		return fmt.Errorf("UserError : get user: %w", err)
	}
	// Migrate the legacy layout where credentials were stored in sauth.
	if IsCom4399CredentialsPayload(user.Sauth) {
		creds, err := ParseCom4399Credentials(user.Sauth)
		if err != nil {
			return err
		}
		cookie, err := loginCom4399Cookie(ctx, creds)
		if err != nil {
			return err
		}
		if err := updateUserCom4399LoginMaterial(userUUID, user.Sauth, cookie); err != nil {
			return fmt.Errorf("Com4399CredentialsError : migrate credentials: %w", err)
		}
		return nil
	}
	if IsCom4399CredentialsPayload(user.Device) {
		return nil
	}
	return RegisterCom4399CredentialsForUser(ctx, userUUID)
}

func RegisterCom4399CredentialsForUser(ctx context.Context, userUUID string) error {
	creds, err := GetCom4399AccountFromPool(ctx)
	if err != nil {
		return err
	}
	payload, err := MarshalCom4399Credentials(creds.Username, creds.Password)
	if err != nil {
		return err
	}
	cookie, err := loginCom4399Cookie(ctx, creds)
	if err != nil {
		return err
	}
	if err := updateUserCom4399LoginMaterial(userUUID, payload, cookie); err != nil {
		return fmt.Errorf("SauthError : save 4399 login material: %w", err)
	}
	return nil
}

func updateUserCom4399LoginMaterial(userUUID, credentials, cookie string) error {
	updates := map[string]interface{}{
		"sauth":  cookie,
		"device": credentials,
	}
	return DB.Model(&User{}).Where("uuid = ?", userUUID).Updates(updates).Error
}

// AuthenticateCom4399User authenticates with the stored Cookie first. When it
// expires, credentials from device are used to obtain and persist a new Cookie.
func AuthenticateCom4399User(ctx context.Context, userUUID string) (*g79client.Client, error) {
	user, err := GetUserByUUID(userUUID)
	if err != nil {
		return nil, fmt.Errorf("UserError : get user: %w", err)
	}
	if strings.TrimSpace(user.Sauth) != "" {
		client, err := authenticateCom4399Cookie(ctx, user.Sauth)
		if err == nil {
			return client, nil
		}
		if !IsCom4399CredentialsPayload(user.Device) {
			return nil, fmt.Errorf("Com4399CookieError : authenticate stored sauth: %w", err)
		}
	}

	creds, err := ParseCom4399Credentials(user.Device)
	if err != nil {
		return nil, err
	}
	cookie, err := loginCom4399Cookie(ctx, creds)
	if err != nil {
		return nil, err
	}
	if err := UpdateUserSauthOnly(userUUID, cookie); err != nil {
		return nil, fmt.Errorf("SauthError : refresh 4399 sauth: %w", err)
	}
	client, err := authenticateCom4399Cookie(ctx, cookie)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func authenticateCom4399Cookie(ctx context.Context, cookie string) (*g79client.Client, error) {
	g79HTTPClient, err := newAuthHTTPClient(ctx, httpproxy.AuthG79clientProxy)
	if err != nil {
		return nil, fmt.Errorf("ProxyError : get g79 proxy: %w", err)
	}
	client, err := g79client.NewClientWithHTTPClient(g79HTTPClient)
	if err != nil {
		return nil, fmt.Errorf("G79ClientError : new client: %w", err)
	}
	if err := client.G79AuthenticateWithCookie(cookie); err != nil {
		return nil, fmt.Errorf("G79AuthError : authenticate with cookie: %w", err)
	}
	if strings.TrimSpace(client.UserID) == "" || strings.TrimSpace(client.UserToken) == "" {
		return nil, errors.New("G79AuthError : authenticated client is incomplete")
	}
	return client, nil
}

func LoginCom4399G79Client(ctx context.Context, creds Com4399Credentials) (*g79client.Client, error) {
	client, _, err := loginCom4399G79Client(ctx, creds)
	return client, err
}

func loginCom4399G79Client(ctx context.Context, creds Com4399Credentials) (*g79client.Client, string, error) {
	cookie, err := loginCom4399Cookie(ctx, creds)
	if err != nil {
		return nil, "", err
	}
	client, err := authenticateCom4399Cookie(ctx, cookie)
	if err != nil {
		return nil, "", err
	}
	return client, cookie, nil
}

// loginCom4399Cookie returns as soon as 4399 supplies a Cookie. Callers persist
// that Cookie before attempting the separate G79 authentication step.
func loginCom4399Cookie(ctx context.Context, creds Com4399Credentials) (string, error) {
	if strings.TrimSpace(creds.Username) == "" || strings.TrimSpace(creds.Password) == "" {
		return "", errors.New("Com4399CredentialsError : credentials are incomplete")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !httpproxy.Auth4399Proxy {
		httpClient := httpproxy.NewHTTPClientWithProxy(nil, 45*time.Second)
		cookie, err := account4399.NewClient(httpClient).RegisterCookieWithPassword(ctx, creds.Username, creds.Password)
		if err != nil {
			return "", fmt.Errorf("Com4399LoginError : login account: %w", err)
		}
		return cookie, nil
	}

	proxyURLs, err := com4399PoolProxyURLs(ctx, maxCom4399LoginProxyAttempts)
	if err != nil {
		return "", err
	}
	loginCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan com4399LoginResult, len(proxyURLs))
	for _, proxyURL := range proxyURLs {
		proxyURL := proxyURL
		go func() {
			httpClient := httpproxy.NewHTTPClientWithProxy(proxyURL, 45*time.Second)
			cookie, err := account4399.NewClient(httpClient).RegisterCookieWithPassword(loginCtx, creds.Username, creds.Password)
			if err != nil && isCom4399PoolProxyFailure(err) {
				discardCom4399PoolProxy(proxyURL.String())
				log.Printf("[Com4399Pool] 登录失败，丢弃代理 IP: %s, 原因: %v", proxyURL, err)
			}
			results <- com4399LoginResult{cookie: strings.TrimSpace(cookie), err: err}
		}()
	}

	var lastErr error
	for range proxyURLs {
		result := <-results
		if result.err == nil && result.cookie != "" {
			cancel()
			return result.cookie, nil
		}
		if result.err != nil {
			lastErr = result.err
		}
	}
	if lastErr != nil {
		return "", fmt.Errorf("Com4399LoginError : login account: %w", lastErr)
	}
	return "", errors.New("Com4399LoginError : login account returned an empty cookie")
}

const maxCom4399LoginProxyAttempts = 8

type com4399LoginResult struct {
	cookie string
	err    error
}

func newFunAuthCom4399HTTPClient(ctx context.Context) (*http.Client, string, error) {
	if !httpproxy.Auth4399Proxy {
		return httpproxy.NewHTTPClientWithProxy(nil, 45*time.Second), "", nil
	}
	proxyURL, err := nextCom4399PoolProxyURL(ctx)
	if err != nil {
		return nil, "", err
	}
	return httpproxy.NewHTTPClientWithProxy(proxyURL, 45*time.Second), proxyURL.String(), nil
}

func newAuthHTTPClient(ctx context.Context, useProxy bool) (*http.Client, error) {
	if !useProxy {
		return httpproxy.NewHTTPClientWithProxy(nil, 45*time.Second), nil
	}
	proxyURL, err := httpproxy.RandomProxyURL(ctx)
	if err != nil {
		return nil, err
	}
	return httpproxy.NewHTTPClientWithProxy(proxyURL, 45*time.Second), nil
}

func registerUsableCom4399Account(ctx context.Context) (string, string, error) {
	var lastErr error
	for attempt := 0; attempt < maxCom4399RegisterAttempts; attempt++ {
		username, err := randomCom4399Username()
		if err != nil {
			return "", "", err
		}
		password, err := randomCom4399Password()
		if err != nil {
			return "", "", err
		}

		cookie, err := registerCom4399AccountCookie(ctx, username, password)
		if err != nil {
			lastErr = err
			continue
		}
		if err := checkCom4399CookieUsable(cookie); err != nil {
			lastErr = err
			continue
		}
		return username, password, nil
	}
	if lastErr == nil {
		lastErr = errors.New("Com4399RegisterError : unknown error")
	}
	return "", "", fmt.Errorf("Com4399RegisterError : register usable account: %w", lastErr)
}

func registerCom4399AccountCookie(ctx context.Context, username, password string) (string, error) {
	httpClient, proxyText, err := newCom4399RegisterHTTPClient(ctx)
	if err != nil {
		return "", err
	}

	if err := registerCom4399AccountUC(ctx, httpClient, username, password); err != nil {
		if proxyText != "" && isCom4399PoolProxyFailure(err) {
			discardCom4399PoolProxy(proxyText)
			log.Printf("[Com4399Pool] 捕获到请稍后再试，丢弃代理 IP: %s", proxyText)
		}
		return "", err
	}

	loginClient := account4399.NewClient(httpClient)
	cookie, err := loginClient.RegisterCookieWithPassword(ctx, username, password)
	if err != nil {
		if proxyText != "" && isCom4399PoolProxyFailure(err) {
			discardCom4399PoolProxy(proxyText)
			log.Printf("[Com4399Pool] 捕获到请稍后再试，丢弃代理 IP: %s", proxyText)
		}
		return "", fmt.Errorf("Com4399LoginError : login registered account: %w", err)
	}
	return strings.TrimSpace(cookie), nil
}

func registerCom4399AccountUC(ctx context.Context, httpClient *http.Client, username, password string) error {
	const maxRealNameRetries = 30
	for retry := 0; retry < maxRealNameRetries; retry++ {
		req, idCode, err := buildCom4399UCRegisterRequest(username, password)
		if err != nil {
			return err
		}

		_, err = account4399.NewUCRegisterClient(httpClient).Register(ctx, req)
		if err == nil {
			return nil
		}
		if !errors.Is(err, account4399.ErrRealNameRejected) {
			return fmt.Errorf("Com4399RegisterError : register account: %w", err)
		}
		if idCode.ID > 0 {
			_ = DeleteIDCode(idCode.ID)
		}
	}
	return errors.New("RealNameError : retry limit exceeded")
}

func buildCom4399UCRegisterRequest(username, password string) (account4399.UCRegisterRequest, *IDCode, error) {
	idCode, err := GetRandomIDCode()
	if err != nil {
		return account4399.UCRegisterRequest{}, nil, fmt.Errorf("RealNameError : get real name: %w", err)
	}
	realName := strings.TrimSpace(idCode.Name)
	idCard := strings.TrimSpace(idCode.IDCard)
	if realName == "" || idCard == "" {
		return account4399.UCRegisterRequest{}, idCode, errors.New("RealNameError : real name record is incomplete")
	}
	return account4399.UCRegisterRequest{
		Username: strings.TrimSpace(username),
		Password: strings.TrimSpace(password),
		RealName: realName,
		IDCard:   idCard,
	}, idCode, nil
}

func newCom4399RegisterHTTPClient(ctx context.Context) (*http.Client, string, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	proxyText := ""

	// 根据开关状态决定是否使用代理
	if httpproxy.Auth4399Proxy {
		proxyURL, err := nextCom4399PoolProxyURL(ctx)
		if err != nil {
			return nil, "", err
		}
		proxyText = proxyURL.String()
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, proxyText, nil
}

func isCom4399PoolProxyRateLimited(err error) bool {
	return err != nil && strings.Contains(err.Error(), "请稍后再试")
}

func isCom4399PoolProxyFailure(err error) bool {
	if isCom4399PoolProxyRateLimited(err) {
		return true
	}
	errText := strings.ToLower(err.Error())
	for _, marker := range []string{
		"connection reset",
		"timeout",
		"deadline exceeded",
		"proxyconnect",
		"no route to host",
		"network is unreachable",
		"connection refused",
	} {
		if strings.Contains(errText, marker) {
			return true
		}
	}
	return false
}

func com4399PoolProxyURLs(ctx context.Context, limit int) ([]*url.URL, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 {
		return nil, errors.New("ProxyError : proxy limit must be positive")
	}
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		proxies := httpproxy.HTTPProxies()
		urls := make([]*url.URL, 0, limit)
		for _, proxy := range proxies {
			if httpproxy.IsHTTPProxyBlacklisted(proxy) {
				continue
			}
			if !strings.Contains(proxy, "://") {
				proxy = "http://" + proxy
			}
			proxyURL, err := url.Parse(proxy)
			if err != nil || proxyURL.Host == "" {
				continue
			}
			urls = append(urls, proxyURL)
			if len(urls) == limit {
				break
			}
		}
		if len(urls) > 0 {
			return urls, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, errors.New("ProxyError : no available 4399 pool proxy after refresh wait")
		case <-ticker.C:
		}
	}
}

func nextCom4399PoolProxyURL(ctx context.Context) (*url.URL, error) {
	proxyURLs, err := com4399PoolProxyURLs(ctx, 1)
	if err != nil {
		return nil, err
	}
	return proxyURLs[0], nil
}

func discardCom4399PoolProxy(proxy string) {
	httpproxy.BlacklistHTTPProxy(proxy)
}

func checkCom4399CookieUsable(cookie string) error {
	return checkCom4399CookieUsableWithHTTPClient(cookie, httpproxy.NewLoginHTTPClient(45*time.Second))
}

func checkCom4399CookieUsableWithHTTPClient(cookie string, httpClient *http.Client) error {
	client := x19sdk.NewClient(httpClient)
	result, err := client.CheckEnterWithCookie(cookie, &x19sdk.CookieCheckEnterOptions{
		RoleID: checkEnterRoleID,
		HostID: checkEnterHostID,
	})
	if err != nil {
		return fmt.Errorf("UniSauthError : check enter: %w", err)
	}
	if result == nil || result.UniSauth == nil {
		return errors.New("UniSauthError : response is nil")
	}
	if result.UniSauth.Code != 200 || result.UniSauth.Subcode != 0 {
		return fmt.Errorf(
			"UniSauthError : failed: code=%d subcode=%d msg=%s",
			result.UniSauth.Code,
			result.UniSauth.Subcode,
			strings.TrimSpace(result.UniSauth.Msg),
		)
	}
	return nil
}

func randomCom4399Username() (string, error) {
	token, err := randomHex(8)
	if err != nil {
		return "", fmt.Errorf("Com4399CredentialsError : generate username: %w", err)
	}
	return "hz" + token, nil
}

func randomCom4399Password() (string, error) {
	token, err := randomHex(8)
	if err != nil {
		return "", fmt.Errorf("Com4399CredentialsError : generate password: %w", err)
	}
	return "Hz" + token + "9", nil
}

func randomHex(byteLen int) (string, error) {
	if byteLen <= 0 {
		return "", errors.New("RandomError : byte length must be positive")
	}
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("RandomError : read random bytes: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

const (
	com4399PoolMinSize     = 3
	com4399PoolPrefill     = 3
	com4399PoolRefillSize  = 2
	com4399PoolAccountType = "4399"
)

type com4399PoolAccount struct {
	id          uint
	credentials Com4399Credentials
}

// com4399AccountPool 4399 账号缓存池
var com4399AccountPool struct {
	accounts []com4399PoolAccount
	mu       sync.RWMutex
	filling  int32 // 正在注册的线程数
}

// InitCom4399AccountPool 初始化 4399 账号缓存池。启动时先恢复未发放的固化账号，
// 再按预热数量补足缓存。
func InitCom4399AccountPool() {
	accounts, err := loadPersistedCom4399PoolAccounts()
	if err != nil {
		log.Printf("[Com4399Pool] 读取固化账号失败: %v", err)
		accounts = nil
	}

	com4399AccountPool.mu.Lock()
	com4399AccountPool.accounts = accounts
	restored := len(accounts)
	com4399AccountPool.mu.Unlock()
	log.Printf("[Com4399Pool] 已读取固化账号: %d", restored)

	// 多线程预注册补足启动预热数量。
	var wg sync.WaitGroup
	for i := restored; i < com4399PoolPrefill; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			registerAndAddToPool(context.Background())
		}()
	}
	wg.Wait()
	com4399AccountPool.mu.RLock()
	count := len(com4399AccountPool.accounts)
	com4399AccountPool.mu.RUnlock()
	log.Printf("[Com4399Pool] 初始化完成，缓存数量: %d", count)

	// 启动后台定时补充监控
	go monitorAndRefillPool()
}

// registerAndAddToPool 注册账号并添加到缓存池
func registerAndAddToPool(ctx context.Context) {
	// 控制并发注册数量，避免过多并发（原子操作，避免竞态）
	if atomic.AddInt32(&com4399AccountPool.filling, 1) > 3 {
		atomic.AddInt32(&com4399AccountPool.filling, -1)
		return
	}
	defer atomic.AddInt32(&com4399AccountPool.filling, -1)

	username, password, err := registerUsableCom4399Account(ctx)
	if err != nil {
		log.Printf("[Com4399Pool] 注册失败: %v", err)
		return
	}

	account := &Account{Type: com4399PoolAccountType, Username: username, Password: password}
	if err := DB.Create(account).Error; err != nil {
		log.Printf("[Com4399Pool] 固化注册账号失败: %v", err)
		return
	}

	com4399AccountPool.mu.Lock()
	com4399AccountPool.accounts = append(com4399AccountPool.accounts, com4399PoolAccount{
		id:          account.ID,
		credentials: Com4399Credentials{Username: username, Password: password},
	})
	count := len(com4399AccountPool.accounts)
	com4399AccountPool.mu.Unlock()

	log.Printf("[Com4399Pool] 注册成功，当前缓存数量: %d", count)
}

// GetCom4399AccountFromPool 从缓存池获取账号，优先从缓存取，缓存不足时现场注册
func GetCom4399AccountFromPool(ctx context.Context) (Com4399Credentials, error) {
	// 先尝试从缓存池获取
	com4399AccountPool.mu.Lock()
	if len(com4399AccountPool.accounts) > 0 {
		account := com4399AccountPool.accounts[0]
		result := DB.Where("id = ? AND type = ?", account.id, com4399PoolAccountType).Delete(&Account{})
		if result.Error != nil {
			com4399AccountPool.mu.Unlock()
			return Com4399Credentials{}, fmt.Errorf("Com4399PoolError : remove persisted account: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			com4399AccountPool.mu.Unlock()
			return Com4399Credentials{}, errors.New("Com4399PoolError : persisted account no longer exists")
		}
		com4399AccountPool.accounts = com4399AccountPool.accounts[1:]
		remaining := len(com4399AccountPool.accounts)
		com4399AccountPool.mu.Unlock()

		log.Printf("[Com4399Pool] 从缓存获取账号，剩余缓存: %d", remaining)
		// 账号已从固化缓存移除，立即在后台注册一个替补，避免库存持续减少。
		go registerAndAddToPool(context.Background())
		return account.credentials, nil
	}
	com4399AccountPool.mu.Unlock()

	// 缓存池为空，现场注册
	username, password, err := registerUsableCom4399Account(ctx)
	if err != nil {
		return Com4399Credentials{}, err
	}
	return Com4399Credentials{Username: username, Password: password}, nil
}

func loadPersistedCom4399PoolAccounts() ([]com4399PoolAccount, error) {
	var persisted []Account
	if err := DB.Where("type = ?", com4399PoolAccountType).Order("id ASC").Find(&persisted).Error; err != nil {
		return nil, err
	}

	accounts := make([]com4399PoolAccount, 0, len(persisted))
	for _, account := range persisted {
		if account.Username == "" || account.Password == "" {
			log.Printf("[Com4399Pool] 跳过无效固化账号 ID: %d", account.ID)
			continue
		}
		accounts = append(accounts, com4399PoolAccount{
			id:          account.ID,
			credentials: Com4399Credentials{Username: account.Username, Password: account.Password},
		})
	}
	return accounts, nil
}

// monitorAndRefillPool 后台定时监控并补充缓存池
func monitorAndRefillPool() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		com4399AccountPool.mu.RLock()
		current := len(com4399AccountPool.accounts)
		com4399AccountPool.mu.RUnlock()

		// 如果缓存低于最小数量，补充
		if current < com4399PoolMinSize {
			need := com4399PoolMinSize - current
			log.Printf("[Com4399Pool] 缓存不足 (%d/%d)，补充 %d 个", current, com4399PoolMinSize, need)
			for i := 0; i < need; i++ {
				go registerAndAddToPool(context.Background())
			}
		}
	}
}
