package usercenter

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Yeah114/FunAuth/internal/db"
)

const apiKeyPrefix = "fa_"

type createAPIKeyReq struct {
	Remark    string `json:"remark,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// HandleListMyAPIKeys GET /api/usercenter/api-keys/mine
func HandleListMyAPIKeys(c *gin.Context) {
	user := CurrentUser(c)
	var keys []db.APIKey
	db.DB.Where("user_uuid = ?", user.UUID).Order("id DESC").Find(&keys)
	out := make([]gin.H, 0, len(keys))
	for _, k := range keys {
		out = append(out, gin.H{
			"id":           k.ID,
			"prefix":      apiKeyPrefix + k.Prefix,
			"remark":      k.Remark,
			"revoked":     k.Revoked,
			"created_at":  k.CreatedAt,
			"last_used_at": k.LastUsedAt,
			"expires_at":  k.ExpiresAt,
			"usage_count":  k.UsageCount,
		})
	}
	ok(c, gin.H{"list": out, "total": len(out)})
}

// HandleCreateAPIKey POST /api/usercenter/api-keys
// 一次性返回明文 key；后续不再展示
func HandleCreateAPIKey(c *gin.Context) {
	user := CurrentUser(c)
	var req createAPIKeyReq
	_ = c.ShouldBindJSON(&req)

	// 生成随机 key：fa_ + 32 字符 hex
	plain := apiKeyPrefix + randHex(32)
	prefix := plain[len(apiKeyPrefix):len(apiKeyPrefix)+8]
	keyHash := sha256Hex(plain)

	k := db.APIKey{
		UserUUID:  user.UUID,
		KeyHash:   keyHash,
		KeyPlain:  plain, // 一次性明文存储，便于管理员/用户后续核对（生产可改为只存 hash）
		Prefix:    prefix,
		Remark:    req.Remark,
		ExpiresAt: req.ExpiresAt,
	}
	if err := db.DB.Create(&k).Error; err != nil {
		fail(c, http.StatusInternalServerError, "创建 API Key 失败")
		return
	}
	ok(c, gin.H{
		"id":         k.ID,
		"key_plain": plain, // 仅此一次返回明文
		"prefix":     apiKeyPrefix + prefix,
		"remark":     k.Remark,
		"expires_at": k.ExpiresAt,
		"created_at": k.CreatedAt,
	})
}

// HandleRevokeAPIKey DELETE /api/usercenter/api-keys/:id
func HandleRevokeAPIKey(c *gin.Context) {
	user := CurrentUser(c)
	id := c.Param("id")
	res := db.DB.Model(&db.APIKey{}).
		Where("id = ? AND user_uuid = ?", id, user.UUID).
		Update("revoked", true)
	if res.Error != nil {
		fail(c, http.StatusInternalServerError, "撤销失败")
		return
	}
	if res.RowsAffected == 0 {
		fail(c, http.StatusNotFound, "API Key 不存在或不属于当前用户")
		return
	}
	ok(c, gin.H{"revoked": id})
}

// ---- Admin ----

// HandleAdminListUserAPIKeys GET /api/usercenter/admin/api-keys/:user_uuid
func HandleAdminListUserAPIKeys(c *gin.Context) {
	userUUID := c.Param("user_uuid")
	var keys []db.APIKey
	db.DB.Where("user_uuid = ?", userUUID).Order("id DESC").Find(&keys)
	out := make([]gin.H, 0, len(keys))
	for _, k := range keys {
		out = append(out, gin.H{
			"id":           k.ID,
			"prefix":       apiKeyPrefix + k.Prefix,
			"remark":       k.Remark,
			"revoked":      k.Revoked,
			"created_at":   k.CreatedAt,
			"last_used_at": k.LastUsedAt,
			"expires_at":   k.ExpiresAt,
			"usage_count":  k.UsageCount,
		})
	}
	ok(c, gin.H{"list": out, "total": len(out)})
}

// ValidateAPIKey 通过明文 key 校验，更新 last_used_at + usage_count
// 供外部 API 网关调用
func ValidateAPIKey(plainKey string) (*db.APIKey, error) {
	plainKey = strings.TrimSpace(plainKey)
	if !strings.HasPrefix(plainKey, apiKeyPrefix) {
		return nil, errors.New("invalid api key prefix")
	}
	hash := sha256Hex(plainKey)
	var k db.APIKey
	if err := db.DB.Where("key_hash = ?", hash).First(&k).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("api key not found")
		}
		return nil, err
	}
	if k.Revoked {
		return nil, errors.New("api key revoked")
	}
	if k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("api key expired")
	}
	// 异步更新使用信息（避免阻塞调用）
	go func(id uint) {
		db.DB.Model(&db.APIKey{}).Where("id = ?", id).Updates(map[string]interface{}{
			"last_used_at": time.Now(),
			"usage_count":  gorm.Expr("usage_count + 1"),
		})
	}(k.ID)
	return &k, nil
}
