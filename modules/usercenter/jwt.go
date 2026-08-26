package usercenter

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/Yeah114/FunAuth/internal/db"
)

// jwtSecret JWT 签名密钥；从环境变量 FUNAUTH_JWT_SECRET 读取，未设置则启动时随机生成并打印警告
var jwtSecret []byte

func initJWTSecret() {
	if s := strings.TrimSpace(os.Getenv("FUNAUTH_JWT_SECRET")); s != "" {
		jwtSecret = []byte(s)
		return
	}
	// 随机生成 32 字节
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// 兜底：用时间戳
		jwtSecret = []byte(fmt.Sprintf("funauth-fallback-%d", time.Now().UnixNano()))
	} else {
		jwtSecret = b
	}
	fmt.Println("[usercenter] WARNING: FUNAUTH_JWT_SECRET 未设置，已随机生成 JWT 密钥（重启后旧 token 失效）")
}

// UserClaims JWT Claims
type UserClaims struct {
	UserUUID string `json:"user_uuid"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// IssueToken 签发 JWT
func IssueToken(user *db.User) (string, error) {
	if jwtSecret == nil {
		initJWTSecret()
	}
	now := time.Now()
	claims := UserClaims{
		UserUUID: user.UUID,
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "funauth",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(7 * 24 * time.Hour)),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(jwtSecret)
}

// ParseToken 解析并校验 JWT
func ParseToken(tokenStr string) (*UserClaims, error) {
	if jwtSecret == nil {
		initJWTSecret()
	}
	claims := &UserClaims{}
	t, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	if !t.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

// extractToken 从 Header 取 token：Authorization: Bearer <token> 或 X-Session-Token
func extractToken(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); h != "" {
		if strings.HasPrefix(strings.ToLower(h), "bearer ") {
			return strings.TrimSpace(h[7:])
		}
		return strings.TrimSpace(h)
	}
	if t := c.GetHeader("X-Session-Token"); t != "" {
		return strings.TrimSpace(t)
	}
	return ""
}

// CurrentUser 取当前用户对象（必须先经过 JwtAuthMiddleware）
func CurrentUser(c *gin.Context) *db.User {
	v, ok := c.Get("current_user")
	if !ok {
		return nil
	}
	u, _ := v.(*db.User)
	return u
}

// JwtAuthMiddleware JWT 认证中间件
func JwtAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := extractToken(c)
		if tokenStr == "" {
			fail(c, http.StatusUnauthorized, "未登录或 token 缺失")
			c.Abort()
			return
		}
		claims, err := ParseToken(tokenStr)
		if err != nil {
			fail(c, http.StatusUnauthorized, "token 无效或已过期")
			c.Abort()
			return
		}
		// 加载最新用户对象（保证 is_banned / 等状态实时）
		user, err := db.GetUserByUUID(claims.UserUUID)
		if err != nil {
			fail(c, http.StatusUnauthorized, "用户不存在")
			c.Abort()
			return
		}
		if user.IsBanned {
			fail(c, http.StatusForbidden, "账号已被封禁")
			c.Abort()
			return
		}
		c.Set("current_user", user)
		c.Set("current_uuid", user.UUID)
		c.Next()
	}
}

// OptionalAuthMiddleware 软认证：有 token 就解析，没有也放行（用于公开接口读取用户身份）
func OptionalAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := extractToken(c)
		if tokenStr == "" {
			c.Next()
			return
		}
		if claims, err := ParseToken(tokenStr); err == nil {
			if user, err := db.GetUserByUUID(claims.UserUUID); err == nil && !user.IsBanned {
				c.Set("current_user", user)
				c.Set("current_uuid", user.UUID)
			}
		}
		c.Next()
	}
}

// UserGroupIDs 获取用户所属的有效（未过期）身份组 ID 列表
func UserGroupIDs(userUUID string) ([]uint, error) {
	var ids []uint
	err := db.DB.Model(&db.UserGroup{}).
		Where("user_uuid = ? AND (expires_at IS NULL OR expires_at >= ?)", userUUID, time.Now()).
		Pluck("group_id", &ids).Error
	return ids, err
}

// UserPermissions 返回用户所有组的权限合并集合（去重）
func UserPermissions(userUUID string) ([]string, error) {
	ids, err := UserGroupIDs(userUUID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []string{}, nil
	}
	var groups []db.Group
	if err := db.DB.Where("id IN ?", ids).Find(&groups).Error; err != nil {
		return nil, err
	}
	set := map[string]struct{}{}
	for _, g := range groups {
		for _, p := range ParseStringArray(g.Permissions) {
			set[p] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out, nil
}

// HasPermission 判断用户是否拥有指定权限（"*" 表示超级管理员）
func HasPermission(userUUID string, perm string) bool {
	perms, err := UserPermissions(userUUID)
	if err != nil {
		return false
	}
	for _, p := range perms {
		if p == "*" || p == perm {
			return true
		}
	}
	return false
}

// RequirePermission 管理员权限中间件：perm="*" 表示任意管理员
func RequirePermission(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := CurrentUser(c)
		if user == nil {
			fail(c, http.StatusUnauthorized, "未登录")
			c.Abort()
			return
		}
		perms, err := UserPermissions(user.UUID)
		if err != nil {
			fail(c, http.StatusInternalServerError, "查询权限失败")
			c.Abort()
			return
		}
		allowed := false
		for _, p := range perms {
			if p == "*" {
				allowed = true
				break
			}
			if perm == "*" {
				continue // "*" 要求任意管理员权限（即 perms 含 "*"）
			}
			if p == perm {
				allowed = true
				break
			}
		}
		if !allowed {
			fail(c, http.StatusForbidden, "权限不足")
			c.Abort()
			return
		}
		c.Set("admin_perms", perms)
		c.Next()
	}
}

// ParseStringArray 把 JSON 字符串解析为字符串数组，失败返回空数组
func ParseStringArray(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return []string{}
	}
	return arr
}

// ParseStringArrayInto 把 JSON 字符串解析为 []uint（用于 visible_group_ids 等）
func ParseUintArray(raw string) []uint {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []uint{}
	}
	// 兼容数字可能以字符串形式存储
	var rawArr []json.Number
	if err := json.Unmarshal([]byte(raw), &rawArr); err != nil {
		// 再尝试 []uint
		var arr []uint
		if err2 := json.Unmarshal([]byte(raw), &arr); err2 == nil {
			return arr
		}
		return []uint{}
	}
	out := make([]uint, 0, len(rawArr))
	for _, n := range rawArr {
		if v, err := n.Int64(); err == nil && v > 0 {
			out = append(out, uint(v))
		}
	}
	return out
}

// ---- 以下为小工具 ----

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
