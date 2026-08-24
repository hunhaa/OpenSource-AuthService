package webui

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Yeah114/g79client"
	linkconnection "github.com/Yeah114/g79client/service/link_connection"
	account4399 "github.com/Yeah114/g79client/account/com4399"
	"github.com/gin-gonic/gin"
)

// --------------------------------------------------------------------
// FastBuilder / NeoOmega 验证服务器兼容实现
// 参考 neomega-core:  neomega/fbauth/client.go
// --------------------------------------------------------------------

const (
	fbSecretTTL       = 5 * time.Minute
	fbTokenPrefix     = "fbtk_"
)

var (
	fbSecretStore sync.Map // map[string]time.Time (bearer secret -> 过期时间)
	fbTokenStore  sync.Map // map[string]string      (fbtoken -> webuiSessionToken)
	fbAccStore    sync.Map // map[string]fbAccount   (fbUsername -> fbAcc{passwordSha256, fbtoken})
)

type fbAccount struct {
	PasswordSHA256 string `json:"p"`
	FBToken        string `json:"t"`
}

func newSecret() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

func newFBToken() string {
	b := make([]byte, 28)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return fbTokenPrefix + hex.EncodeToString(b)
}

// cleanupFBStores 定期清理过期的 bearer secret
func cleanupFBStores() {
	now := time.Now()
	fbSecretStore.Range(func(k, v any) bool {
		exp, ok := v.(time.Time)
		if !ok || now.After(exp) {
			fbSecretStore.Delete(k)
		}
		return true
	})
}

func verifyFBSecret(secret string) bool {
	if secret == "" {
		return false
	}
	v, ok := fbSecretStore.Load(secret)
	if !ok {
		return false
	}
	exp, ok := v.(time.Time)
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		fbSecretStore.Delete(secret)
		return false
	}
	// 一次性消费
	fbSecretStore.Delete(secret)
	return true
}

// parseBearer 从 Authorization: Bearer <secret> 头中取出 secret
func parseBearer(c *gin.Context) (secret string, ok bool) {
	h := c.GetHeader("Authorization")
	if h == "" {
		return "", false
	}
	const pre = "bearer "
	if len(h) < len(pre)+1 || !strings.EqualFold(h[:len(pre)], pre) {
		return "", false
	}
	return strings.TrimSpace(h[len(pre):]), true
}

// =====================================================================
// 路由挂载：/api/new, /api/phoenix/*
// =====================================================================

func RegisterFBAuthRoutes(api *gin.RouterGroup) {
	// 启动定期清理
	go func() {
		t := time.NewTicker(2 * time.Minute)
		defer t.Stop()
		for range t.C {
			cleanupFBStores()
		}
	}()

	api.GET("/new", HandleFBNewSecret)
	phoenix := api.Group("/phoenix")
	{
		phoenix.POST("/login", HandleFBPhoenixLogin)
		phoenix.GET("/transfer_start_type", HandleFBTransferStartType)
		phoenix.POST("/transfer_check_num", HandleFBTransferCheckNum)
	}
}

// =====================================================================
// 1. GET /api/new → 返回纯文本 secret（用于后续请求 Bearer）
// =====================================================================

func HandleFBNewSecret(c *gin.Context) {
	secret := newSecret()
	if secret == "" {
		c.String(500, "500 Internal Server Error\n\nfailed to generate secret")
		return
	}
	fbSecretStore.Store(secret, time.Now().Add(fbSecretTTL))
	c.String(200, "%s", secret)
}

// =====================================================================
// 2. POST /api/phoenix/login
// =====================================================================

type phoenixLoginReq struct {
	ClientPublicKey string `json:"client_public_key"`
	ServerCode      string `json:"server_code"`
	ServerPasscode  string `json:"server_passcode"`
	// 模式 A：ToolDelta 登录获取 fbtoken（server_code == ::DRY::）
	Username string `json:"username"`
	Password string `json:"password"` // sha256(password) 或明文（兼容两种）
	// 模式 B：NeoOmega / neomega-core 使用 fbtoken 生成 chainInfo
	LoginToken string `json:"login_token"`
}

func failPhoenix(c *gin.Context, httpCode int, msg string, translation ...int) {
	resp := map[string]any{
		"success":   false,
		"message":   msg,
		"successed": false, // 兼容老拼写
	}
	if len(translation) > 0 {
		resp["translation"] = translation[0]
	}
	c.JSON(httpCode, resp)
}

func okPhoenix(c *gin.Context, extra map[string]any) {
	resp := map[string]any{
		"success":   true,
		"message":   "ok",
		"successed": true,
	}
	for k, v := range extra {
		resp[k] = v
	}
	c.JSON(200, resp)
}

// sha256Hex 计算字符串 sha256 小写 hex
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// isValidHex 检查字符串是否是合法的 64 位小写 hex（sha256）
func isSHA256Hex(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

func HandleFBPhoenixLogin(c *gin.Context) {
	// ---- 1. Bearer secret 校验（一次性） ----
	secret, ok := parseBearer(c)
	if !ok || !verifyFBSecret(secret) {
		// 兼容：为了方便调试，如果没有 secret 就只放行 localhost 请求
		if !isLocalhostRequest(c) {
			failPhoenix(c, 401, "invalid or expired bearer secret. please GET /api/new first.", -1)
			return
		}
	}

	body, _ := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	var req phoenixLoginReq
	if err := json.Unmarshal(body, &req); err != nil {
		failPhoenix(c, 400, "invalid json: "+err.Error(), -1)
		return
	}

	// ---- 2. 模式判断 ----
	isDRY := (strings.TrimSpace(req.ServerCode) == "::DRY::")
	hasFBToken := strings.TrimSpace(req.LoginToken) != ""

	// ---- 模式 A：DRY = 登录，返回 fbtoken ----
	if isDRY && !hasFBToken {
		handleFBLoginIssue(c, req)
		return
	}

	// ---- 模式 B：NeoOmega 请求生成 chainInfo ----
	if hasFBToken || req.ServerCode != "" {
		handleFBAuthV2(c, req)
		return
	}

	failPhoenix(c, 400, "invalid request: neither DRY login nor AuthV2", -1)
}

func isLocalhostRequest(c *gin.Context) bool {
	h := c.Request.Host
	return strings.HasPrefix(h, "localhost") ||
		strings.HasPrefix(h, "127.0.0.1") ||
		strings.HasPrefix(h, "[::1]")
}

// handleFBLoginIssue 处理 ToolDelta "账号密码登录 → 签发 fbtoken"
// 兼容两种子模式：
//   (A1) username/password 匹配已注册 4399 账号 → 登录拿 SAuth → 创建 webui session → 绑定 fbtoken 返回
//   (A2) 任意用户名/密码 → 作为"虚拟 fb 账号"签发 fbtoken；但是后续调用 AuthV2 没有真实 SAuth 会失败，
//        所以仅当 webui 登录产生的 fbtoken 能正常 AuthV2；我们这里通过账号密码真正调用 4399 登录，
//        确保签发出去的 fbtoken 拥有真实可用的 SAuth 会话。
func handleFBLoginIssue(c *gin.Context, req phoenixLoginReq) {
	user := strings.TrimSpace(req.Username)
	pass := strings.TrimSpace(req.Password)
	if user == "" || pass == "" {
		failPhoenix(c, 400, "username / password 不能为空", -1)
		return
	}

	// 1) 密码：是 sha256hex（ToolDelta fblike_sign_login 会 sha256(password)）就直接用，
	//    否则当明文处理转成 sha256hex 做比较
	passHash := pass
	if !isSHA256Hex(pass) {
		passHash = sha256Hex(pass)
	}

	// 快速路径：存在 fb 虚拟账号映射 且 密码 hash 匹配
	if v, ok := fbAccStore.Load(user); ok {
		acc, ok := v.(fbAccount)
		if ok && acc.PasswordSHA256 == passHash {
			// 检查 fbtoken 对应的 session 还活着
			if _, stillAlive := fbTokenStore.Load(acc.FBToken); stillAlive {
				okPhoenix(c, map[string]any{"token": acc.FBToken, "username": user})
				return
			}
		}
	}

	// 2) 真实网络路径：用 4399 账号密码登录拿 SAuth Cookie → 创建 webui 会话 → 签发 fbtoken
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cookie, err := account4399.LoginCookieWithPassword(ctx, user, pass)
	if err != nil {
		// 3) 不具备真实 4399 登录条件的情况下，允许"虚拟 fb 账号"存在的兜底：
		//    直接签发一个 fbtoken，同时把当前账号作为虚拟账号记录下来（密码存 hash）。
		//    该 fbtoken 后续若被使用会因没有 SAuth 而失败。用户应在 WebUI 使用 SAuth 登录来获取真正可用的 fbtoken。
		if v, exists := fbAccStore.LoadOrStore(user, fbAccount{PasswordSHA256: passHash, FBToken: ""}); exists {
			// 已存在账号且密码不对 → 拒绝
			if acc, ok := v.(fbAccount); ok && acc.PasswordSHA256 != passHash {
				failPhoenix(c, 401, fmt.Sprintf("4399 登录失败且本地虚拟账号密码不匹配: %v", err), -1)
				return
			}
		}
		fbt := newFBToken()
		fbAccStore.Store(user, fbAccount{PasswordSHA256: passHash, FBToken: fbt})
		okPhoenix(c, map[string]any{
			"token":        fbt,
			"username":     user,
			"note":         "虚拟 fb 账号（非 SAuth 登录）：该 Token 仅用于登录，后续 AuthV2 会因缺少真实网易 SAuth 失败。请通过 WebUI 使用 SAuth 登录获取 fbtoken 以拥有完整能力。",
		})
		return
	}

	// 3) 登录成功：创建 webui 会话 & 签发 fbtoken
	sessionToken, fbtoken, nickname, err := createSessionFromCookie(cookie, user)
	if err != nil {
		failPhoenix(c, 500, "认证成功但创建会话失败: "+err.Error(), -1)
		return
	}

	// 记录 fb 账号 → fbtoken 映射
	fbAccStore.Store(user, fbAccount{PasswordSHA256: passHash, FBToken: fbtoken})

	_ = sessionToken
	okPhoenix(c, map[string]any{
		"token":    fbtoken,
		"username": user,
		"nickname": nickname,
	})
}

// createSessionFromCookie 通过 Cookie 字符串创建一个 webui Session，并返回 (sessionToken, fbtoken, nickname)
func createSessionFromCookie(cookie string, tagName string) (string, string, string, error) {
	cli, err := g79client.NewClient()
	if err != nil {
		return "", "", "", err
	}
	if err := cli.G79AuthenticateWithCookie(cookie); err != nil {
		return "", "", "", fmt.Errorf("SAuth 认证失败: %w", err)
	}
	token := newToken()
	if token == "" {
		return "", "", "", fmt.Errorf("生成 webui token 失败")
	}
	nickname := ""
	if cli.UserDetail != nil {
		nickname = cli.UserDetail.Name
	}
	if strings.TrimSpace(nickname) == "" {
		// 辅助账号没有名字 → 改为 nklm_XXXXX
		if err := g79client.EnsureNicknameNKLMIfEmpty(cli, "NKLM"); err == nil && cli.UserDetail != nil {
			nickname = cli.UserDetail.Name
		}
	}
	if nickname == "" {
		nickname = tagName
	}
	s := &Session{
		Token:     token,
		Client:    cli,
		CookieRaw: cookie,
		Nickname:  nickname,
		UserID:    cli.UserID,
		EngineVer: cli.EngineVersion,
		PatchVer:  cli.G79LatestVersion,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(sessionTTL),
	}
	storeSession(s)

	// 生成 fbtoken 并绑定到 session
	fbt := newFBToken()
	fbTokenStore.Store(fbt, token)
	return token, fbt, nickname, nil
}

// IssueFBTokenBySession 给已有的 webui session 签发（或复用）一个 fbtoken。
// 返回 fbtoken。不存在会话时返回 ""。
func IssueFBTokenBySession(webuiToken string) string {
	_, ok := loadSession(webuiToken)
	if !ok {
		return ""
	}
	// 找已绑定的 fbtoken
	var found string
	fbTokenStore.Range(func(k, v any) bool {
		t, ok := v.(string)
		if ok && t == webuiToken {
			found = k.(string)
			return false
		}
		return true
	})
	if found != "" {
		return found
	}
	fbt := newFBToken()
	fbTokenStore.Store(fbt, webuiToken)
	return fbt
}

// handleFBAuthV2 处理 NeoOmega 使用 fbtoken + 服务器号获取 chainInfo 的请求
func handleFBAuthV2(c *gin.Context, req phoenixLoginReq) {
	fbt := strings.TrimSpace(req.LoginToken)
	if fbt == "" {
		failPhoenix(c, 400, "缺少 login_token (fbtoken)", -1)
		return
	}
	v, ok := fbTokenStore.Load(fbt)
	if !ok {
		failPhoenix(c, 401, "无效的 fbtoken", -1)
		return
	}
	sessionToken, _ := v.(string)
	s, ok := loadSession(sessionToken)
	if !ok {
		fbTokenStore.Delete(fbt)
		failPhoenix(c, 401, "fbtoken 对应会话已过期或不存在", -1)
		return
	}

	serverCode := strings.TrimSpace(req.ServerCode)
	if serverCode == "" || serverCode == "::DRY::" {
		failPhoenix(c, 400, "缺少有效 server_code", -1)
		return
	}

	// ---- 1) 确保 Link + GameStart 已建立 ----
	s.mu.Lock()
	if s.LinkConn == nil {
		svc, err := linkconnection.NewLinkConnectionService(s.Client)
		if err != nil {
			s.mu.Unlock()
			failPhoenix(c, 500, "创建 Link 服务失败: "+err.Error(), -1)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		conn, err := svc.Dial(ctx)
		if err != nil {
			s.mu.Unlock()
			failPhoenix(c, 504, "Link 连接失败: "+err.Error(), -1)
			return
		}
		if err := conn.SendGameStart(nil); err != nil {
			conn.Close()
			s.mu.Unlock()
			failPhoenix(c, 500, "Link SendGameStart 失败: "+err.Error(), -1)
			return
		}
		s.LinkConn = conn
		s.LinkClose = func() {
			defer func() { _ = recover() }()
			conn.Close()
		}
	}
	s.mu.Unlock()

	// ---- 2) EnterRentalServerWorld ----
	passcode := req.ServerPasscode
	enter, err := s.Client.EnterRentalServerWorld(serverCode, passcode)
	if err != nil {
		failPhoenix(c, 502, "EnterRentalServerWorld 失败: "+err.Error(), -1)
		return
	}
	if enter.Code != 0 {
		failPhoenix(c, 422, fmt.Sprintf("EnterRentalServerWorld code=%d msg=%s", enter.Code, enter.Message), -1)
		return
	}
	e := enter.Entity
	mcAddr := fmt.Sprintf("%s:%v", e.McserverHost, e.McserverPort.String())

	// ---- 3) 使用 NeoOmega 传过来的 client_public_key 生成 chainInfo ----
	pubKey := strings.TrimSpace(req.ClientPublicKey)
	if pubKey == "" {
		pubKey = clientPublicKey // fallback 到 webui 默认
	} else {
		// 兼容性：如果客户端给的是 PEM / DER / x509 / 原始 base64 的 384bit ECC key，
		//          g79client GenerateRentalGameAuthV2 内部会自动选择正确的编码方式，
		//          这里只做格式验证，不合法就 fallback 保证流程不中断
		if !looksValidPublicKey(pubKey) {
			pubKey = clientPublicKey
		}
	}
	authV2Data, err := s.Client.GenerateRentalGameAuthV2(serverCode, pubKey)
	if err != nil {
		failPhoenix(c, 500, "GenerateRentalGameAuthV2 失败: "+err.Error(), -1)
		return
	}
	chainInfo, err := s.Client.SendAuthV2Request(authV2Data)
	if err != nil {
		failPhoenix(c, 502, "SendAuthV2Request 失败: "+err.Error(), -1)
		return
	}

	extra := map[string]any{
		"chainInfo":              encodeB64(chainInfo),
		"chain_info_b64":         encodeB64(chainInfo),
		"chain_info_hex":         hex.EncodeToString(chainInfo),
		"chainInfoLength":        len(chainInfo),
		"MinecraftServerAddress": mcAddr,
		"serverAddress":          mcAddr,
		"ip":                     e.McserverHost,
		"port":                   e.McserverPort.Int64(),
		"port_str":               e.McserverPort.String(),
		"serverCode":             serverCode,
		"entity":                 e,
		"botName":                s.Nickname,
		"botUid":                 s.UserID,
		"respondTo":              s.UserID,
	}
	okPhoenix(c, extra)
}

// looksValidPublicKey 宽松校验：只要是 base64/hex/PEM 就认为合法
func looksValidPublicKey(k string) bool {
	k = strings.TrimSpace(k)
	if k == "" {
		return false
	}
	// PEM
	if strings.Contains(k, "-----BEGIN") {
		block, _ := pem.Decode([]byte(k))
		return block != nil && len(block.Bytes) > 0
	}
	// x509 DER（base64）: 尝试解析
	if raw, err := decodeB64OrHex(k); err == nil && len(raw) >= 32 {
		if _, err := x509.ParsePKIXPublicKey(raw); err == nil {
			return true
		}
		// 解析失败但数据量够，也放行（可能是原始 ECC point bytes）
		return true
	}
	return false
}

func decodeB64OrHex(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	// hex
	isHex := true
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			isHex = false
			break
		}
	}
	if isHex && len(s)%2 == 0 {
		return hex.DecodeString(s)
	}
	// base64（标准 / URL）
	return decodeB64String(s)
}

func decodeB64String(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	// try std first
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return nil, fmt.Errorf("base64 decode failed")
}

// portStrToInt 辅助：Uncertain 字符串 port 转 int，失败回退 0
func portStrToInt(v string) int {
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

// =====================================================================
// 3. /api/phoenix/transfer_start_type + transfer_check_num 兼容端点
//    NeoOmega 需要这两个端点完成"启动类型转换"交互。我们实现为回显/透传。
// =====================================================================

func HandleFBTransferStartType(c *gin.Context) {
	_, ok := parseBearer(c)
	if !ok && !isLocalhostRequest(c) {
		// 宽松：不强制 secret，避免 NeoOmega 版本差异
	}
	content := strings.TrimSpace(c.Query("content"))
	if content == "" {
		c.JSON(200, map[string]any{"success": true, "data": ""})
		return
	}
	c.JSON(200, map[string]any{
		"success": true,
		"data":    content,
	})
}

func HandleFBTransferCheckNum(c *gin.Context) {
	_, ok := parseBearer(c)
	_ = ok
	var r struct {
		Data string `json:"data"`
	}
	_ = c.ShouldBindJSON(&r)
	c.JSON(200, map[string]any{
		"success": true,
		"value":   r.Data,
	})
}
