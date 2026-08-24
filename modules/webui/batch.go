package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Yeah114/g79client"
	linkconnection "github.com/Yeah114/g79client/service/link_connection"
	"github.com/gin-gonic/gin"
)
// ---------- 请求结构体 ----------

type BatchAccount struct {
	Cookie   string `json:"cookie"`
	Nickname string `json:"nickname"`
}

type BatchEnterReq struct {
	Accounts []BatchAccount `json:"accounts"`
	ServerID string         `json:"server_id"`
	Password string         `json:"password"`
}

type BatchAuthV2Req struct {
	Tokens   []string `json:"tokens"`
	ServerID string   `json:"server_id"`
}

// ---------- 服务器号 → 服务器 ID ----------

// lookupServerNameToEntityID 将服务器号（纯数字，例如 52258662）或服务器名查询为 server entity_id。
// 输入若本身就是 entity_id 样式（含非数字字符，或 > 36 位 UUID-like）也会原样返回。
func lookupServerNameToEntityID(serverNameOrID string) (string, error) {
	serverNameOrID = strings.TrimSpace(serverNameOrID)
	if serverNameOrID == "" {
		return "", fmt.Errorf("服务器号不能为空")
	}
	// 如果看起来像纯数字服务器号（<= 32 位）→ 走搜索
	isPureDigits := true
	for _, ch := range serverNameOrID {
		if ch < '0' || ch > '9' {
			isPureDigits = false
			break
		}
	}
	if !isPureDigits && len(serverNameOrID) >= 16 {
		// 不是纯数字且比较长，大概率就是 entity_id 直接返回
		return serverNameOrID, nil
	}
	// 用临时客户端搜索（注意：SearchRentalServerByName 走的是已 releaseJSON.WebServerUrl 的接口）
	cli, err := g79client.NewClient()
	if err != nil {
		return "", fmt.Errorf("初始化临时客户端失败: %w", err)
	}
	resp, err := cli.SearchRentalServerByName(serverNameOrID)
	if err != nil {
		return "", fmt.Errorf("搜索租赁服失败: %w", err)
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("搜索租赁服返回 code=%d msg=%s", resp.Code, resp.Message)
	}
	if len(resp.Entities) == 0 {
		return "", fmt.Errorf("未找到服务器号=%s 对应的租赁服，请检查服务器号是否正确", serverNameOrID)
	}
	// 优先找 Name(纯数字服号) 或 ServerName 完全匹配，否则返回第一个
	for _, e := range resp.Entities {
		if e.Name.String() == serverNameOrID || e.ServerName == serverNameOrID {
			return e.EntityID.String(), nil
		}
	}
	return resp.Entities[0].EntityID.String(), nil
}

// HandleRentalLookupServerName 前端批量塞入时，输入服务器号自动转换为服务器ID
func HandleRentalLookupServerName(c *gin.Context) {
	var req struct {
		ServerName string `json:"server_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "参数错误: %v", err)
		return
	}
	id, err := lookupServerNameToEntityID(req.ServerName)
	if err != nil {
		fail(c, 422, "%v", err)
		return
	}
	ok(c, gin.H{
		"server_name": req.ServerName,
		"entity_id":   id,
	})
}

// ---------- 响应结构体 ----------

type BatchEnterResult struct {
	Index    int    `json:"index"`
	Nickname string `json:"nickname"`
	UserID   string `json:"user_id"`
	Token    string `json:"token"`
	Status   string `json:"status"` // "ok" / "error"
	IP       string `json:"ip,omitempty"`
	ServerID string `json:"server_id,omitempty"` // 实际服务器 ID（UUID），用于后续 AuthV2
	Error    string `json:"error,omitempty"`
}

type BatchAuthV2Result struct {
	Index    int    `json:"index"`
	Nickname string `json:"nickname"`
	Status   string `json:"status"` // "ok" / "error"
	ChainLen int    `json:"chain_info_len,omitempty"`
	ChainB64 string `json:"chain_info_b64,omitempty"`
	Variant  string `json:"variant,omitempty"`
	Error    string `json:"error,omitempty"`
}

// normalizeCookie 规范化 cookie 格式，兼容原始 sauth_json 字符串
func normalizeCookie(cookie string) string {
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(cookie), &m) != nil {
		wrapped, _ := json.Marshal(map[string]string{"sauth_json": cookie})
		return string(wrapped)
	} else if _, has := m["sauth_json"]; !has {
		wrapped, _ := json.Marshal(map[string]string{"sauth_json": cookie})
		return string(wrapped)
	}
	return cookie
}

// HandleBatchEnter 批量认证+Link+进入租赁服
// 对每个 cookie 依次执行：认证 → 设置昵称 → Link+GameStart → EnterRentalServerWorld
// 参数 server_id 既支持真实 entity_id，也支持服务器号（纯数字），会自动 lookup 转换
func HandleBatchEnter(c *gin.Context) {
	var req BatchEnterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "参数错误: %v", err)
		return
	}
	if len(req.Accounts) == 0 {
		fail(c, 400, "accounts 不能为空")
		return
	}
	if req.ServerID == "" {
		fail(c, 400, "server_id 不能为空")
		return
	}

	// 自动将服务器号（纯数字）转换为真实 entity_id
	realServerID, lookupErr := lookupServerNameToEntityID(req.ServerID)
	if lookupErr != nil {
		fail(c, 422, "服务器号转换失败: %v", lookupErr)
		return
	}
	serverIDUsed := realServerID

	results := make([]BatchEnterResult, len(req.Accounts))
	for i, acc := range req.Accounts {
		r := BatchEnterResult{Index: i, Nickname: acc.Nickname}
		cookie := normalizeCookie(acc.Cookie)
		if cookie == "" {
			r.Status = "error"
			r.Error = "cookie 为空"
			results[i] = r
			continue
		}

		// 1. 认证
		cli, err := g79client.NewClient()
		if err != nil {
			r.Status = "error"
			r.Error = fmt.Sprintf("NewClient失败: %v", err)
			results[i] = r
			continue
		}
		if err := cli.G79AuthenticateWithCookie(cookie); err != nil {
			r.Status = "error"
			r.Error = fmt.Sprintf("认证失败: %v", err)
			results[i] = r
			continue
		}
		r.UserID = cli.UserID

		// 2. 设置昵称（如果提供了自定义昵称）
		nickname := strings.TrimSpace(acc.Nickname)
		if nickname != "" {
			curNick := ""
			if cli.UserDetail != nil {
				curNick = cli.UserDetail.Name
			}
			if curNick != nickname {
				if err := cli.UpdateNickname(nickname); err != nil {
					// 昵称设置失败不中断流程，继续往下走
					r.Error = fmt.Sprintf("昵称设置失败(忽略): %v; ", err)
				}
			}
		} else {
			// 没提供昵称但当前为空时自动补 NKLM 前缀
			curNick := ""
			if cli.UserDetail != nil {
				curNick = cli.UserDetail.Name
			}
			if strings.TrimSpace(curNick) == "" {
				_ = g79client.EnsureNicknameNKLMIfEmpty(cli, "NKLM")
				if cli.UserDetail != nil {
					nickname = cli.UserDetail.Name
					r.Nickname = nickname
				}
			}
		}
		if r.Nickname == "" && cli.UserDetail != nil {
			r.Nickname = cli.UserDetail.Name
		}

		// 3. 创建会话
		token := newToken()
		s := &Session{
			Token:     token,
			Client:    cli,
			CookieRaw: cookie,
			Nickname:  r.Nickname,
			UserID:    cli.UserID,
			EngineVer: cli.EngineVersion,
			PatchVer:  cli.G79LatestVersion,
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(sessionTTL),
		}

		// 4. Link + GameStart
		svc, err := linkconnection.NewLinkConnectionService(cli)
		if err != nil {
			r.Status = "error"
			r.Error = fmt.Sprintf("%s创建Link服务失败: %v", r.Error, err)
			results[i] = r
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		conn, err := svc.Dial(ctx)
		if err != nil {
			cancel()
			r.Status = "error"
			r.Error = fmt.Sprintf("%sLink连接失败: %v", r.Error, err)
			results[i] = r
			continue
		}
		if err := conn.SendGameStart(nil); err != nil {
			cancel()
			conn.Close()
			r.Status = "error"
			r.Error = fmt.Sprintf("%sSendGameStart失败: %v", r.Error, err)
			results[i] = r
			continue
		}
		cancel()
		s.LinkConn = conn
		s.LinkClose = func() {
			defer func() { _ = recover() }()
			conn.Close()
		}
		storeSession(s)
		r.Token = token

		// 5. EnterRentalServerWorld
		resp, err := cli.EnterRentalServerWorld(serverIDUsed, req.Password)
		if err != nil {
			r.Status = "error"
			r.Error = fmt.Sprintf("%sEnterRentalServerWorld失败: %v", r.Error, err)
			results[i] = r
			continue
		}
		if resp.Code != 0 {
			r.Status = "error"
			r.Error = fmt.Sprintf("%s进入租赁服失败 code=%d msg=%s", r.Error, resp.Code, resp.Message)
			results[i] = r
			continue
		}
		e := resp.Entity
		r.IP = fmt.Sprintf("%s:%v", e.McserverHost, e.McserverPort.String())
		r.ServerID = e.ServerID // 保存实际服务器 ID（UUID），用于后续 AuthV2
		r.Status = "ok"
		if r.Error != "" {
			r.Error = strings.TrimSpace(r.Error)
		}
		results[i] = r
	}

	okCount := 0
	for _, r := range results {
		if r.Status == "ok" {
			okCount++
		}
	}
	ok(c, gin.H{
		"results":    results,
		"total":      len(results),
		"ok_count":   okCount,
		"fail_count": len(results) - okCount,
		"server_id":  serverIDUsed,
		"server_name_lookup": gin.H{
			"input":   req.ServerID,
			"resolved": realServerID,
		},
	})
}

// HandleBatchAuthV2 批量生成 AuthV2 ChainInfo
// 对每个 token 依次执行：加载会话 → 检查Link → 生成AuthV2数据 → 发送请求
// 参数 server_id 既支持真实 entity_id，也支持服务器号（纯数字），会自动 lookup 转换
func HandleBatchAuthV2(c *gin.Context) {
	var req BatchAuthV2Req
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "参数错误: %v", err)
		return
	}
	if len(req.Tokens) == 0 {
		fail(c, 400, "tokens 不能为空")
		return
	}
	if req.ServerID == "" {
		fail(c, 400, "server_id 不能为空")
		return
	}

	// 自动将服务器号（纯数字）转换为真实 entity_id
	realServerID, lookupErr := lookupServerNameToEntityID(req.ServerID)
	if lookupErr != nil {
		fail(c, 422, "服务器号转换失败: %v", lookupErr)
		return
	}
	serverIDUsed := realServerID

	results := make([]BatchAuthV2Result, len(req.Tokens))
	for i, tok := range req.Tokens {
		r := BatchAuthV2Result{Index: i}
		s, ok0 := loadSession(tok)
		if !ok0 {
			r.Status = "error"
			r.Error = "会话不存在或已过期"
			results[i] = r
			continue
		}
		r.Nickname = s.Nickname

		if s.LinkConn == nil {
			r.Status = "error"
			r.Error = "未启动Link连接"
			results[i] = r
			continue
		}

		// 依次尝试 PC 版和 PE 版
		type variant struct {
			name string
			gen  func() ([]byte, error)
		}
		variants := []variant{
			{
				name: "PC版",
				gen: func() ([]byte, error) {
					return s.Client.GeneratePCRentalGameAuthV2(serverIDUsed, clientPublicKey)
				},
			},
			{
				name: "PE版",
				gen: func() ([]byte, error) {
					return s.Client.GenerateRentalGameAuthV2(serverIDUsed, clientPublicKey)
				},
			},
		}

		var lastErr error
		success := false
		for _, v := range variants {
			data, gerr := v.gen()
			if gerr != nil {
				lastErr = fmt.Errorf("[%s] 生成失败: %w", v.name, gerr)
				continue
			}
			chainInfo, aerr := s.Client.SendAuthV2Request(data)
			if aerr == nil {
				r.Status = "ok"
				r.ChainLen = len(chainInfo)
				r.ChainB64 = encodeB64(chainInfo)
				r.Variant = v.name
				success = true
				break
			}
			lastErr = fmt.Errorf("[%s] %w", v.name, aerr)
		}
		if !success {
			r.Status = "error"
			r.Error = fmt.Sprintf("%v", lastErr)
		}
		results[i] = r
	}

	okCount := 0
	for _, r := range results {
		if r.Status == "ok" {
			okCount++
		}
	}
	ok(c, gin.H{
		"results":    results,
		"total":      len(results),
		"ok_count":   okCount,
		"fail_count": len(results) - okCount,
		"server_id":  serverIDUsed,
		"server_name_lookup": gin.H{
			"input":   req.ServerID,
			"resolved": realServerID,
		},
	})
}
