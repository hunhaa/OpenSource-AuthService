package usercenter

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Yeah114/FunAuth/internal/db"
)

// systemSettings 系统设置缓存
var (
	systemSettingsMu     sync.RWMutex
	systemSettingsCache  *db.SystemSetting
	systemSettingsLoaded bool
)

// loadSystemSettings 读取系统设置（带内存缓存，管理员更新后调用 invalidateSystemSettings）
func loadSystemSettings() db.SystemSetting {
	systemSettingsMu.RLock()
	if systemSettingsLoaded && systemSettingsCache != nil {
		ss := *systemSettingsCache
		systemSettingsMu.RUnlock()
		return ss
	}
	systemSettingsMu.RUnlock()

	systemSettingsMu.Lock()
	defer systemSettingsMu.Unlock()
	if systemSettingsLoaded && systemSettingsCache != nil {
		return *systemSettingsCache
	}
	var ss db.SystemSetting
	if err := db.DB.Where("id = 1").First(&ss).Error; err != nil {
		// 不存在则返回默认值
		ss = db.SystemSetting{ID: 1, LoginEnabled: true, RegisterEnabled: true}
	}
	systemSettingsCache = &ss
	systemSettingsLoaded = true
	return ss
}

func invalidateSystemSettings() {
	systemSettingsMu.Lock()
	systemSettingsCache = nil
	systemSettingsLoaded = false
	systemSettingsMu.Unlock()
}

// HandleHealth GET /api/usercenter/health
func HandleHealth(c *gin.Context) {
	ok(c, gin.H{
		"ok":      true,
		"status":  "ok",
		"version": "funauth-usercenter",
		"time":    time.Now().Format(time.RFC3339),
	})
}

// HandleAdminGetSystemSettings GET /api/usercenter/admin/system-settings
func HandleAdminGetSystemSettings(c *gin.Context) {
	ss := loadSystemSettings()
	ok(c, ss)
}

// HandleAdminUpdateSystemSettings PUT /api/usercenter/admin/system-settings
func HandleAdminUpdateSystemSettings(c *gin.Context) {
	var req db.SystemSetting
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误")
		return
	}
	req.ID = 1
	req.UpdatedAt = time.Now()
	// upsert
	var existing db.SystemSetting
	findErr := db.DB.Where("id = 1").First(&existing).Error
	if findErr != nil {
		// 不存在则插入
		if err := db.DB.Create(&req).Error; err != nil {
			fail(c, http.StatusInternalServerError, "保存失败")
			return
		}
	} else {
		// 更新（保留 created 不动）
		if err := db.DB.Model(&db.SystemSetting{}).Where("id = 1").Updates(map[string]interface{}{
			"rate_limit_per_min":              req.RateLimitPerMin,
			"ip_whitelist":                     req.IPWhitelist,
			"ip_blacklist":                     req.IPBlacklist,
			"auto_blacklist_enabled":           req.AutoBlacklistEnabled,
			"auto_blacklist_threshold":         req.AutoBlacklistThreshold,
			"auto_blacklist_duration_minutes":  req.AutoBlacklistDurationMinutes,
			"login_enabled":                     req.LoginEnabled,
			"register_enabled":                  req.RegisterEnabled,
			"login_allowed_group_ids":          req.LoginAllowedGroupIDs,
			"maintenance_enabled":              req.MaintenanceEnabled,
			"maintenance_whitelist_ips":         req.MaintenanceWhitelistIPs,
			"maintenance_message":               req.MaintenanceMessage,
			"extra_config":                      req.ExtraConfig,
			"updated_at":                        time.Now(),
		}).Error; err != nil {
			fail(c, http.StatusInternalServerError, "保存失败")
			return
		}
	}
	invalidateSystemSettings()
	ss := loadSystemSettings()
	ok(c, ss)
}
