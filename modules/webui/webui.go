package webui

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Yeah114/g79client"
	linkconnection "github.com/Yeah114/g79client/service/link_connection"
	"github.com/gin-gonic/gin"
)

//go:embed static/*
var StaticFS embed.FS

const clientPublicKey = "MHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEzmz6+EK8UC40g5XsqoAjqURAKP6uCAMmXJeEyzR/8BkZ1vVXpFTMF/AmBl3Tf+gvDFPJkT9Bm3bAO0IeXo+ssMOsJX4NFPLM4+YEohwJrJyRaMptmh1nvWue4J5+vbZW"

// Session holds an authenticated g79 client.
type Session struct {
	Token        string              `json:"token"`
	Client       *g79client.Client   `json:"-"`
	LinkConn     *linkconnection.LinkConnection `json:"-"`
	LinkClose    func()              `json:"-"`
	CookieRaw    string              `json:"cookie_raw"`
	Nickname     string              `json:"nickname"`
	UserID       string              `json:"user_id"`
	EngineVer    string              `json:"engine_version"`
	PatchVer     string              `json:"patch_version"`
	CreatedAt    time.Time           `json:"created_at"`
	ExpiresAt    time.Time           `json:"expires_at"`
	mu           sync.Mutex
}

var (
	sessionStore sync.Map // map[string]*Session
	sessionTTL   = 24 * time.Hour
)

// API response envelope
type R struct {
	OK      bool   `json:"ok"`
	Msg     string `json:"msg,omitempty"`
	Data    any    `json:"data,omitempty"`
	Code    int    `json:"code,omitempty"`
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, R{OK: true, Data: data})
}
func fail(c *gin.Context, httpCode int, msg string, args ...any) {
	c.JSON(httpCode, R{OK: false, Msg: fmt.Sprintf(msg, args...)})
}

// generate a secure random token
func newToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// ---------- requests ----------

type AuthReq struct {
	Cookie string `json:"cookie"` // either {"sauth_json":"..."} string or raw sauth_json string
}

type SearchReq struct {
	Token  string `json:"token"`
	Query  string `json:"query"`
}

type EnterReq struct {
	Token    string `json:"token"`
	ServerID string `json:"server_id"`
	Password string `json:"password"`
}

type AuthV2Req struct {
	Token    string `json:"token"`
	ServerID string `json:"server_id"`
	Password string `json:"password"` // 租赁服密码（可选，有密码的服需要）
}

// ---------- helpers ----------

func cleanupExpired() {
	now := time.Now()
	sessionStore.Range(func(key, value any) bool {
		s, ok := value.(*Session)
		if !ok {
			sessionStore.Delete(key)
			return true
		}
		if now.After(s.ExpiresAt) {
			if s.LinkClose != nil { s.LinkClose() }
			sessionStore.Delete(key)
		}
		return true
	})
}

func loadSession(token string) (*Session, bool) {
	if token == "" {
		return nil, false
	}
	v, ok := sessionStore.Load(token)
	if !ok {
		return nil, false
	}
	s := v.(*Session)
	if time.Now().After(s.ExpiresAt) {
		if s.LinkClose != nil { s.LinkClose() }
		sessionStore.Delete(token)
		return nil, false
	}
	return s, true
}

func storeSession(s *Session) {
	sessionStore.Store(s.Token, s)
}

// ---------- routes ----------

// RegisterRoutes 注册 WebUI API。
// 参数 api: 统一 API 前缀路由组（通常是 /api）。
// 参数 engine: 根级 gin.Engine（用于挂 /ui 静态资源与首页重定向，不受 api 前缀影响）。
func RegisterRoutes(api *gin.RouterGroup, engine *gin.Engine) {
	go func() {
		t := time.NewTicker(15 * time.Minute)
		defer t.Stop()
		for range t.C {
			cleanupExpired()
		}
	}()

	// FastBuilder / NeoOmega 验证服务器兼容端点（不加 /webui 前缀，保持协议路径）
	RegisterFBAuthRoutes(api)

	g := api.Group("/webui")
	{
		g.GET("/status", HandleStatus)
		g.POST("/auth", HandleAuth)
		g.GET("/info/:token", HandleInfo)
		g.POST("/link/start", HandleLinkStart)
		g.POST("/link/stop", HandleLinkStop)
		g.POST("/search", HandleSearch)
		g.POST("/rental/enter", HandleRentalEnter)
		g.POST("/rental/authv2", HandleRentalAuthV2)
		g.POST("/logout", HandleLogout)

		// 批量塞入
		g.POST("/batch/enter", HandleBatchEnter)
		g.POST("/batch/authv2", HandleBatchAuthV2)

		// 批量注册机
		g.POST("/batch/register", HandleBatchRegister)
		g.POST("/batch/register/export", HandleExportBatchRegisterCSV)
		g.POST("/batch/sfz/upload", HandleUploadSFZ)
		g.POST("/batch/proxy/upload", HandleUploadProxies)
		g.POST("/batch/proxy/check", HandleCheckProxies)
		g.GET("/batch/stats", HandleStatsBatch)

		// 账号操作（改昵称/简介/头像/皮肤/动态/活力签到）
		RegisterAccountOpsRoutes(g)

		// 服务器号 → 服务器 ID 自动转换（无需登录态，用临时 g79client 查搜索接口）
		g.POST("/rental/lookup-server-name", HandleRentalLookupServerName)

		// 注册 (Bunker 风格)
		RegisterRegisterRoutes(g)
	}

	// 静态控制台 & 根跳转 挂在根路径，不重复加 /api 前缀
	if engine != nil {
		staticRoot, _ := fs.Sub(StaticFS, "static")
		engine.StaticFS("/ui", http.FS(staticRoot))
		engine.GET("/", func(c *gin.Context) {
			c.Redirect(http.StatusMovedPermanently, "/ui/")
		})
	} else {
		// 兼容：没有根 engine 时也挂到 api 所在 group 的顶层（多了 /api 前缀）
		staticRoot, _ := fs.Sub(StaticFS, "static")
		api.StaticFS("/ui", http.FS(staticRoot))
	}
}

// RegisterRoutesCompat 兼容单参数版 (供 router.go 在没有 engine 引用时使用)
// 静态页面会挂到 /api/ui/*，首页挂到 /api/ → /api/ui/ 跳转
func RegisterRoutesCompat(r *gin.RouterGroup) {
	RegisterRoutes(r, nil)
}

// ---------- handlers ----------

func HandleStatus(c *gin.Context) {
	cli, err := g79client.NewClient()
	if err != nil {
		fail(c, 500, "初始化客户端失败: %v", err)
		return
	}
	total := 0
	sessionStore.Range(func(_, _ any) bool {
		total++
		return true
	})
	ok(c, gin.H{
		"engine_version":   cli.EngineVersion,
		"patch_version":    cli.G79LatestVersion,
		"auth_server":      cli.ReleaseJSON.AuthServerURL,
		"core_server":      cli.ReleaseJSON.CoreServerURL,
		"active_sessions":  total,
		"time":             time.Now().Format(time.RFC3339),
	})
}

func HandleAuth(c *gin.Context) {
	var req AuthReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "参数错误: %v", err)
		return
	}
	cookie := req.Cookie
	if cookie == "" {
		fail(c, 400, "cookie 不能为空")
		return
	}

	// normalize: allow raw sauth_json, auto-wrap if not JSON object with "sauth_json" key
	trimmed := trimBytes([]byte(cookie))
	if len(trimmed) == 0 {
		fail(c, 400, "cookie 为空")
		return
	}
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(cookie), &m) != nil {
		// not a JSON object: likely the raw sauth_json string
		wrapped, _ := json.Marshal(map[string]string{"sauth_json": cookie})
		cookie = string(wrapped)
	} else if _, has := m["sauth_json"]; !has {
		// JSON object but missing key: assume the object itself IS sauth_json
		wrapped, _ := json.Marshal(map[string]string{"sauth_json": cookie})
		cookie = string(wrapped)
	}

	cli, err := g79client.NewClient()
	if err != nil {
		fail(c, 500, "NewClient失败: %v", err)
		return
	}
	if err := cli.G79AuthenticateWithCookie(cookie); err != nil {
		fail(c, 401, "认证失败: %v", err)
		return
	}

	token := newToken()
	if token == "" {
		fail(c, 500, "生成token失败")
		return
	}

	// extract nickname from cookie (if any). 辅助账号没名字时自动改成 nklm_XXXXX
	nickname := ""
	if cli.UserDetail != nil {
		nickname = cli.UserDetail.Name
	}
	if strings.TrimSpace(nickname) == "" {
		if err := g79client.EnsureNicknameNKLMIfEmpty(cli, "NKLM"); err == nil && cli.UserDetail != nil {
			nickname = cli.UserDetail.Name
		}
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

	// 签发 FBToken 供 ToolDelta / NeoOmega 作为验证服务器使用
	fbtoken := IssueFBTokenBySession(token)

	ok(c, gin.H{
		"token":          token,
		"fb_token":       fbtoken,
		"user_id":        cli.UserID,
		"nickname":       nickname,
		"level":          safeLevel(cli),
		"engine_version": cli.EngineVersion,
		"patch_version":  cli.G79LatestVersion,
		"expires_at":     s.ExpiresAt.Format(time.RFC3339),
	})
}

func HandleInfo(c *gin.Context) {
	token := c.Param("token")
	s, ok0 := loadSession(token)
	if !ok0 {
		fail(c, 404, "会话不存在或已过期")
		return
	}
	ok(c, gin.H{
		"token":        s.Token,
		"user_id":      s.UserID,
		"nickname":     s.Nickname,
		"engine":       s.EngineVer,
		"patch":        s.PatchVer,
		"link_active":  s.LinkConn != nil,
		"created_at":   s.CreatedAt,
		"expires_at":   s.ExpiresAt,
	})
}

func HandleLogout(c *gin.Context) {
	var req struct{ Token string }
	_ = c.ShouldBindJSON(&req)
	if req.Token == "" {
		fail(c, 400, "缺少token")
		return
	}
	if s, ok := loadSession(req.Token); ok {
		if s.LinkClose != nil { s.LinkClose() }
		sessionStore.Delete(req.Token)
	}
	ok(c, "done")
}

func HandleLinkStart(c *gin.Context) {
	var req struct{ Token string }
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "参数错误: %v", err)
		return
	}
	s, ok0 := loadSession(req.Token)
	if !ok0 {
		fail(c, 404, "会话不存在或已过期")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.LinkConn != nil {
		ok(c, gin.H{"link": "already_active"})
		return
	}
	svc, err := linkconnection.NewLinkConnectionService(s.Client)
	if err != nil {
		fail(c, 500, "创建Link服务失败: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := svc.Dial(ctx)
	if err != nil {
		fail(c, 504, "Link连接失败: %v", err)
		return
	}
	if err := conn.SendGameStart(nil); err != nil {
		conn.Close()
		fail(c, 500, "Link建立成功但SendGameStart失败: %v", err)
		return
	}
	s.LinkConn = conn
	s.LinkClose = func() {
		defer func() { _ = recover() }()
		conn.Close()
	}
	ok(c, gin.H{"link": "ok"})
}

func HandleLinkStop(c *gin.Context) {
	var req struct{ Token string }
	_ = c.ShouldBindJSON(&req)
	s, ok0 := loadSession(req.Token)
	if !ok0 {
		fail(c, 404, "会话不存在")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.LinkClose != nil {
		s.LinkClose()
	}
	s.LinkConn = nil
	s.LinkClose = nil
	ok(c, "stopped")
}

func HandleSearch(c *gin.Context) {
	var req SearchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "参数错误: %v", err)
		return
	}
	s, ok0 := loadSession(req.Token)
	if !ok0 {
		fail(c, 401, "会话不存在或已过期")
		return
	}
	if req.Query == "" {
		fail(c, 400, "query 不能为空")
		return
	}
	resp, err := s.Client.SearchRentalServerByName(req.Query)
	if err != nil {
		fail(c, 500, "搜索失败: %v", err)
		return
	}
	if resp.Code != 0 {
		fail(c, 422, "搜索失败 code=%d msg=%s", resp.Code, resp.Message)
		return
	}
	ok(c, resp)
}

func HandleRentalEnter(c *gin.Context) {
	var req EnterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "参数错误: %v", err)
		return
	}
	s, ok0 := loadSession(req.Token)
	if !ok0 {
		fail(c, 401, "会话不存在或已过期")
		return
	}
	if req.ServerID == "" {
		fail(c, 400, "server_id 不能为空")
		return
	}
	resp, err := s.Client.EnterRentalServerWorld(req.ServerID, req.Password)
	if err != nil {
		fail(c, 500, "EnterRentalServerWorld失败: %v", err)
		return
	}
	if resp.Code != 0 {
		fail(c, 422, "进入租赁服失败 code=%d msg=%s", resp.Code, resp.Message)
		return
	}
	e := resp.Entity
	ipAddr := fmt.Sprintf("%s:%v", e.McserverHost, e.McserverPort.String())
	ok(c, gin.H{
		"server_id": req.ServerID,
		"ip":        ipAddr,
		"host":      e.McserverHost,
		"port":      e.McserverPort.String(),
		"raw":       e,
	})
}

func HandleRentalAuthV2(c *gin.Context) {
	var req AuthV2Req
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "参数错误: %v", err)
		return
	}
	s, ok0 := loadSession(req.Token)
	if !ok0 {
		fail(c, 401, "会话不存在或已过期")
		return
	}
	if req.ServerID == "" {
		fail(c, 400, "server_id 不能为空")
		return
	}

	// ---- 1) 确保 Link + GameStart 已建立 ----
	s.mu.Lock()
	if s.LinkConn == nil {
		svc, err := linkconnection.NewLinkConnectionService(s.Client)
		if err != nil {
			s.mu.Unlock()
			fail(c, 500, "创建Link服务失败: %v", err)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		conn, err := svc.Dial(ctx)
		if err != nil {
			s.mu.Unlock()
			fail(c, 504, "Link连接失败: %v", err)
			return
		}
		if err := conn.SendGameStart(nil); err != nil {
			conn.Close()
			s.mu.Unlock()
			fail(c, 500, "Link SendGameStart失败: %v", err)
			return
		}
		s.LinkConn = conn
		s.LinkClose = func() {
			defer func() { _ = recover() }()
			conn.Close()
		}
	}
	s.mu.Unlock()

	// ---- 2) EnterRentalServerWorld（绑定租赁服会话） ----
	enter, err := s.Client.EnterRentalServerWorld(req.ServerID, req.Password)
	if err != nil {
		fail(c, 500, "EnterRentalServerWorld失败: %v", err)
		return
	}
	if enter.Code != 0 {
		fail(c, 422, "进入租赁服失败 code=%d msg=%s", enter.Code, enter.Message)
		return
	}
	e := enter.Entity
	mcAddr := fmt.Sprintf("%s:%v", e.McserverHost, e.McserverPort.String())

	// ---- 3) 生成并发送 AuthV2（先 PC 后 PE） ----
	type variant struct {
		name string
		gen  func() ([]byte, error)
	}
	variants := []variant{
		{
			name: "PC版(os=windows,patch=\"\",platform=pc,pcCheck=0)",
			gen: func() ([]byte, error) {
				return s.Client.GeneratePCRentalGameAuthV2(req.ServerID, clientPublicKey)
			},
		},
		{
			name: "PE版(os=android,patch=latest,platform=android)",
			gen: func() ([]byte, error) {
				return s.Client.GenerateRentalGameAuthV2(req.ServerID, clientPublicKey)
			},
		},
	}

	var lastErr error
	for vi, v := range variants {
		data, gerr := v.gen()
		if gerr != nil {
			lastErr = fmt.Errorf("[%s] 生成AuthV2失败: %w", v.name, gerr)
			continue
		}
		chainInfo, aerr := s.Client.SendAuthV2Request(data)
		if aerr == nil {
			ok(c, gin.H{
				"server_id":       req.ServerID,
				"used_variant":    v.name,
				"chain_info_b64":  encodeB64(chainInfo),
				"chain_info_hex":  hex.EncodeToString(chainInfo),
				"chain_info_len":  len(chainInfo),
				"mc_addr":         mcAddr,
				"mc_host":         e.McserverHost,
				"mc_port":         e.McserverPort.String(),
			})
			return
		}
		lastErr = fmt.Errorf("[%s] AuthV2请求失败: %w", v.name, aerr)
		_ = vi
	}

	fail(c, 502, "AuthV2两种字段组合均失败。%v", lastErr)
}

// ---------- small utils ----------

func trimBytes(b []byte) []byte {
	start, end := 0, len(b)
	for start < end && (b[start] == ' ' || b[start] == '\t' || b[start] == '\n' || b[start] == '\r') {
		start++
	}
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\n' || b[end-1] == '\r') {
		end--
	}
	return b[start:end]
}

func safeLevel(c *g79client.Client) string {
	if c == nil || c.UserDetail == nil {
		return ""
	}
	return c.UserDetail.Level.String()
}

func encodeB64(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return encodeToString(b)
}
