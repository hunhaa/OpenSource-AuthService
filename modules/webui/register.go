package webui

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	account4399 "github.com/Yeah114/g79client/account/com4399"
)

// ---------- 请求 ----------

type Com4399RegisterReq struct {
	Username string `json:"username"` // 留空自动生成
	Password string `json:"password"` // 留空自动生成
	RealName string `json:"real_name"`
	IDCard   string `json:"id_card"`

	// SkipRegister: 若已经注册过账号，只提供 user/pass 直接登录获取 Sauth
	SkipRegister bool   `json:"skip_register"`
	LoginOnly    bool   `json:"login_only"` // 与 SkipRegister 同义，兼容前端命名
}

type RegPresetResp struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ---------- 随机账号生成 ----------

func randomLetters(n int, alphabet string) (string, error) {
	if n <= 0 { return "", errors.New("n<=0") }
	b := make([]byte, n)
	max := big.NewInt(int64(len(alphabet)))
	for i := range b {
		v, err := rand.Int(rand.Reader, max)
		if err != nil { return "", err }
		b[i] = alphabet[v.Int64()]
	}
	return string(b), nil
}

func genUsername() string {
	if s, err := randomLetters(12, "abcdefghijklmnopqrstuvwxyz0123456789"); err == nil {
		return s
	}
	return fmt.Sprintf("fuser%d", time.Now().UnixNano()%1_000_000)
}

func genPassword() string {
	u, _ := randomLetters(2, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	l, _ := randomLetters(4, "abcdefghijklmnopqrstuvwxyz")
	d, _ := randomLetters(3, "0123456789")
	m, _ := randomLetters(4, "abcdefghijklmnopqrstuvwxyz0123456789")
	return u + l + d + m
}

// ---------- 路由接入 ----------
// （在 webui.go 的 RegisterRoutes 内部 g := api.Group("/webui") 组内继续挂载）
// 为了避免循环引用，我们在同一包内直接暴露一个注册子路由方法给 RegisterRoutes 调用。

// 这个文件被 webui.go 同包使用。在 RegisterRoutes 中添加：
//   RegisterRegisterRoutes(g)
func RegisterRegisterRoutes(g *gin.RouterGroup) {
	reg := g.Group("/register")
	{
		reg.POST("/preset", HandleRegisterPreset)
		reg.POST("/com4399", HandleRegisterCom4399)
	}
}

// ---------- handlers ----------

// HandleRegisterPreset 生成一组随机账号预设（用户名+强密码），前端用户可以直接接受或改。
func HandleRegisterPreset(c *gin.Context) {
	ok(c, RegPresetResp{
		Username: genUsername(),
		Password: genPassword(),
	})
}

// HandleRegisterCom4399 执行 4399 Web 注册 → 登录 → 返回 cookie (g79client 直接消费)。
// 等效于 com4399register 的单次注册 + Sauth 获取流程，无 MySQL 依赖。
func HandleRegisterCom4399(c *gin.Context) {
	var req Com4399RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "参数错误: %v", err)
		return
	}
	loginOnly := req.SkipRegister || req.LoginOnly

	// 1. 账号生成与校验
	if strings.TrimSpace(req.Username) == "" && loginOnly {
		fail(c, 400, "登录模式下 username 不能为空")
		return
	}
	if strings.TrimSpace(req.Username) == "" {
		req.Username = genUsername()
	}
	if strings.TrimSpace(req.Password) == "" {
		req.Password = genPassword()
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()

	result := gin.H{
		"username": req.Username,
		"password": req.Password,
	}

	// 2. 注册（除非 login_only）
	if !loginOnly {
		if strings.TrimSpace(req.RealName) == "" || strings.TrimSpace(req.IDCard) == "" {
			fail(c, 400, "注册 4399 需要实名：real_name 与 id_card 不能为空；如果已经有账号，请设 login_only=true 仅登录获取 Sauth")
			return
		}
		cli := account4399.NewWebRegisterClient(&http.Client{Timeout: 45 * time.Second})
		regReq := account4399.WebRegisterRequest{
			Username: req.Username,
			Password: req.Password,
			RealName: strings.TrimSpace(req.RealName),
			IDCard:   strings.TrimSpace(req.IDCard),
		}
		regResult, err := cli.Register(ctx, regReq)
		if err != nil {
			// 有时注册提交后账号已经能登录；如果错误提示"已存在"等，则尝试下一步登录，不直接失败
			msg := strings.ToLower(err.Error())
			canContinue := strings.Contains(msg, "exists") ||
				strings.Contains(msg, "用户名") && strings.Contains(msg, "存在") ||
				strings.Contains(msg, "username") && strings.Contains(msg, "already")
			if !canContinue {
				fail(c, 502, "4399 注册失败: %v", err)
				return
			}
			result["register_note"] = fmt.Sprintf("注册阶段提示但继续尝试登录: %v", err)
		} else if regResult != nil {
			result["register"] = gin.H{
				"uid":         regResult.UID,
				"username":    regResult.Username,
				"display_name": regResult.DisplayName,
			}
		}

		// 4399 注册/实名提交后需要同步延时
		time.Sleep(3 * time.Second)
	}

	// 3. 登录 → 生成 G79 可用 Cookie JSON
	cookie, err := account4399.LoginCookieWithPassword(ctx, req.Username, req.Password)
	if err != nil {
		fail(c, 502, "4399 登录获取 Sauth 失败: %v", err)
		return
	}
	result["cookie"] = cookie
	result["cookie_len"] = len(cookie)

	ok(c, result)
}
