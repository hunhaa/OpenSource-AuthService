package usercenter

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/Yeah114/FunAuth/internal/db"
)

// ---- 密码哈希 ----
// 历史用户密码是 SHA256(username:salt:password) 的 hex；新用户密码使用 bcrypt。
// isBcryptHash 判断字符串是否为 bcrypt 哈希（$2a$/$2b$/$2y$ 开头，长度 60）

func isBcryptHash(s string) bool {
	return len(s) == 60 && (strings.HasPrefix(s, "$2a$") || strings.HasPrefix(s, "$2b$") || strings.HasPrefix(s, "$2y$"))
}

// hashPasswordBcrypt 用 bcrypt 哈希明文密码
func hashPasswordBcrypt(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// verifyPassword 校验密码：bcrypt 优先，否则回退到历史 SHA256(username:salt:password) hex
// legacySalt 用于兼容老用户（参考 internal/db/utils.go GenerateToken 中的固定盐）
const legacyPasswordSalt = "YeahYYDSYuansiAuthService"

func legacySHA256Hash(username, plain string) string {
	data := username + ":" + legacyPasswordSalt + ":" + plain
	h := sha256.New()
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func verifyPassword(stored, plain, username string) bool {
	if stored == "" {
		return false
	}
	if isBcryptHash(stored) {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(plain)) == nil
	}
	// 兼容历史 SHA256 hex
	return stored == legacySHA256Hash(username, plain)
}

// ---- 请求/响应类型 ----

type loginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type registerReq struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6,max=64"`
}

type profileUpdateReq struct {
	Nickname  *string `json:"nickname,omitempty"`
	AvatarURL *string `json:"avatar_url,omitempty"`
	Bio       *string `json:"bio,omitempty"`
}

type changePasswordReq struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6,max=64"`
}

// ---- Handlers ----

// HandleLogin POST /api/usercenter/login
func HandleLogin(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	// 系统设置：登录通道开关
	ss := loadSystemSettings()
	if !ss.LoginEnabled {
		fail(c, http.StatusForbidden, "登录通道已关闭")
		return
	}
	user, err := db.GetUserByUsername(req.Username)
	if err != nil || user == nil {
		fail(c, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if user.IsBanned {
		fail(c, http.StatusForbidden, "账号已被封禁")
		return
	}
	if !verifyPassword(user.Password, req.Password, user.Username) {
		fail(c, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	// 登录身份组限制
	if ids := ParseUintArray(ss.LoginAllowedGroupIDs); len(ids) > 0 {
		myIDs, _ := UserGroupIDs(user.UUID)
		mySet := map[uint]struct{}{}
		for _, id := range myIDs {
			mySet[id] = struct{}{}
		}
		ok := false
		for _, id := range ids {
			if _, has := mySet[id]; has {
				ok = true
				break
			}
		}
		if !ok {
			fail(c, http.StatusForbidden, "当前身份组不允许登录")
			return
		}
	}
	token, err := IssueToken(user)
	if err != nil {
		fail(c, http.StatusInternalServerError, "签发 token 失败")
		return
	}
	groups, _ := userGroupInfos(user.UUID)
	ok(c, gin.H{
		"token": token,
		"user":  publicUserView(user, groups),
	})
}

// HandleRegister POST /api/usercenter/register
func HandleRegister(c *gin.Context) {
	ss := loadSystemSettings()
	if !ss.RegisterEnabled {
		fail(c, http.StatusForbidden, "注册通道已关闭")
		return
	}
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误：用户名 3-50 字符，密码 6-64 字符")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		fail(c, http.StatusBadRequest, "用户名不能为空")
		return
	}
	// 用户名唯一性检查
	if existed, _ := db.GetUserByUsername(req.Username); existed != nil {
		fail(c, http.StatusConflict, "用户名已存在")
		return
	}
	hashed, err := hashPasswordBcrypt(req.Password)
	if err != nil {
		fail(c, http.StatusInternalServerError, "密码加密失败")
		return
	}
	now := time.Now()
	user := &db.User{
		UUID:     uuid.New().String(),
		Username: req.Username,
		Password: hashed,
		IsX19:    true,
		EndTime:  nil, // 永久
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	// 历史逻辑要求 token 字段非空，生成一个登录态无关的随机 token 占位
	user.Token = randHex(24)

	if err := db.DB.Create(user).Error; err != nil {
		fail(c, http.StatusInternalServerError, "创建用户失败")
		return
	}
	// 创建钱包
	if _, err := db.EnsureUserWallet(db.DB, user.UUID); err != nil {
		// 钱包失败不致命，记录但继续
		_ = err
	}
	// 自动分配默认身份组
	assignDefaultGroups(user.UUID)

	groups, _ := userGroupInfos(user.UUID)
	token, err := IssueToken(user)
	if err != nil {
		fail(c, http.StatusInternalServerError, "签发 token 失败")
		return
	}
	ok(c, gin.H{
		"token": token,
		"user":  publicUserView(user, groups),
	})
}

// HandleLogout POST /api/usercenter/logout （客户端删 token 即可）
func HandleLogout(c *gin.Context) {
	ok(c, gin.H{"message": "已退出"})
}

// HandleProfile GET /api/usercenter/profile
func HandleProfile(c *gin.Context) {
	user := CurrentUser(c)
	groups, _ := userGroupInfos(user.UUID)
	wallet, _ := db.EnsureUserWallet(db.DB, user.UUID)
	balance := int64(0)
	if wallet != nil {
		balance = wallet.Balance
	}
	// 统计卡槽数（总数 / 已绑）
	var totalSlots, boundSlots int64
	db.DB.Model(&db.Slot{}).Where("user_uuid = ?", user.UUID).Count(&totalSlots)
	db.DB.Model(&db.Slot{}).Where("user_uuid = ? AND server_code != ''", user.UUID).Count(&boundSlots)
	ok(c, gin.H{
		"user":         publicUserView(user, groups),
		"quota":        user.Quota,
		"times":        user.Times,
		"balance":      balance,
		"total_slots":  totalSlots,
		"bound_slots":  boundSlots,
	})
}

// HandleProfileUpdate PUT /api/usercenter/profile
func HandleProfileUpdate(c *gin.Context) {
	user := CurrentUser(c)
	var req profileUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	updates := map[string]interface{}{}
	if req.Nickname != nil {
		updates["nickname"] = truncate(*req.Nickname, 64)
	}
	if req.AvatarURL != nil {
		updates["avatar_url"] = truncate(*req.AvatarURL, 512)
	}
	if req.Bio != nil {
		updates["bio"] = truncate(*req.Bio, 1024)
	}
	if len(updates) == 0 {
		fail(c, http.StatusBadRequest, "没有可更新的字段")
		return
	}
	updates["updated_at"] = time.Now()
	if err := db.UpdateUser(user.UUID, updates); err != nil {
		fail(c, http.StatusInternalServerError, "更新失败")
		return
	}
	updated, _ := db.GetUserByUUID(user.UUID)
	groups, _ := userGroupInfos(user.UUID)
	ok(c, gin.H{"user": publicUserView(updated, groups)})
}

// HandleChangePassword POST /api/usercenter/profile/password
func HandleChangePassword(c *gin.Context) {
	user := CurrentUser(c)
	var req changePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误：新密码 6-64 字符")
		return
	}
	if !verifyPassword(user.Password, req.OldPassword, user.Username) {
		fail(c, http.StatusUnauthorized, "旧密码错误")
		return
	}
	hashed, err := hashPasswordBcrypt(req.NewPassword)
	if err != nil {
		fail(c, http.StatusInternalServerError, "密码加密失败")
		return
	}
	if err := db.UpdateUser(user.UUID, map[string]interface{}{
		"password":    hashed,
		"updated_at":  time.Now(),
	}); err != nil {
		fail(c, http.StatusInternalServerError, "更新密码失败")
		return
	}
	ok(c, gin.H{"message": "密码已更新"})
}

// HandleProfileWallet GET /api/usercenter/profile/wallet
func HandleProfileWallet(c *gin.Context) {
	user := CurrentUser(c)
	wallet, _ := db.EnsureUserWallet(db.DB, user.UUID)
	var txs []db.WalletTransaction
	db.DB.Where("user_uuid = ?", user.UUID).Order("created_at DESC").Limit(10).Find(&txs)
	balance := int64(0)
	if wallet != nil {
		balance = wallet.Balance
	}
	ok(c, gin.H{
		"balance":            balance,
		"transactions_last_10": txs,
	})
}

// HandleTransactions GET /api/usercenter/transactions?type=wallet|quota|times
func HandleTransactions(c *gin.Context) {
	user := CurrentUser(c)
	txType := c.DefaultQuery("type", "wallet")
	page, size := parsePage(c)
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	var total int64
	var list any
	base := db.DB.Where("user_uuid = ?", user.UUID)
	if startDate != "" {
		base = base.Where("created_at >= ?", startDate)
	}
	if endDate != "" {
		base = base.Where("created_at <= ?", endDate+" 23:59:59")
	}
	switch txType {
	case "wallet":
		var arr []db.WalletTransaction
		base2 := base.Session(&gorm.Session{}).Model(&db.WalletTransaction{}).Order("created_at DESC")
		total, _ = paginate(base2, &arr, page, size)
		list = arr
	case "quota":
		var arr []db.QuotaTransaction
		base2 := base.Session(&gorm.Session{}).Model(&db.QuotaTransaction{}).Order("created_at DESC")
		total, _ = paginate(base2, &arr, page, size)
		list = arr
	case "times":
		var arr []db.TimesTransaction
		base2 := base.Session(&gorm.Session{}).Model(&db.TimesTransaction{}).Order("created_at DESC")
		total, _ = paginate(base2, &arr, page, size)
		list = arr
	default:
		fail(c, http.StatusBadRequest, "type 必须是 wallet/quota/times")
		return
	}
	ok(c, pageResult(list, total, page, size))
}

// ---- 辅助 ----

// publicUserView 返回前端需要的用户公开字段
func publicUserView(u *db.User, groups []gin.H) gin.H {
	if u == nil {
		return gin.H{}
	}
	view := gin.H{
		"uuid":            u.UUID,
		"username":        u.Username,
		"nickname":        u.Nickname,
		"avatar_url":      u.AvatarURL,
		"bio":             u.Bio,
		"is_banned":       u.IsBanned,
		"isX19":           u.IsX19,
		"nickname_prefix": u.NicknamePrefix,
		"quota":           u.Quota,
		"times":           u.Times,
		"end_time":        u.EndTime,
		"created_at":      u.CreatedAt,
		"groups":          groups,
	}
	return view
}

// userGroupInfos 返回用户所属身份组信息（id/name/permissions/description/expires_at）
func userGroupInfos(userUUID string) ([]gin.H, error) {
	var ugs []db.UserGroup
	if err := db.DB.Where("user_uuid = ? AND (expires_at IS NULL OR expires_at >= ?)", userUUID, time.Now()).Find(&ugs).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []gin.H{}, nil
		}
		return nil, err
	}
	if len(ugs) == 0 {
		return []gin.H{}, nil
	}
	ids := make([]uint, 0, len(ugs))
	for _, g := range ugs {
		ids = append(ids, g.GroupID)
	}
	var groups []db.Group
	if err := db.DB.Where("id IN ?", ids).Find(&groups).Error; err != nil {
		return nil, err
	}
	gmap := map[uint]db.Group{}
	for _, g := range groups {
		gmap[g.ID] = g
	}
	out := make([]gin.H, 0, len(ugs))
	for _, ug := range ugs {
		g, has := gmap[ug.GroupID]
		if !has {
			continue
		}
		out = append(out, gin.H{
			"id":          g.ID,
			"name":        g.Name,
			"permissions": ParseStringArray(g.Permissions),
			"description": g.Description,
			"is_default":  g.IsDefault,
			"sort_order":  g.SortOrder,
			"expires_at":  ug.ExpiresAt,
		})
	}
	return out, nil
}

// assignDefaultGroups 把所有 is_default=1 的组自动分配给用户
func assignDefaultGroups(userUUID string) {
	var defaults []db.Group
	if err := db.DB.Where("is_default = 1").Find(&defaults).Error; err != nil {
		return
	}
	for _, g := range defaults {
		ug := db.UserGroup{
			UserUUID:     userUUID,
			GroupID:      g.ID,
			AutoAssigned: true,
		}
		db.DB.Create(&ug)
	}
}

// truncate 截断字符串到 maxLen
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
