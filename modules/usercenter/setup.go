package usercenter

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/Yeah114/FunAuth/internal/db"
)

// setupPageBytes 在 init 时从 StaticFS 读取 setup.html，避免重复 //go:embed 冲突
var setupPageBytes = func() []byte {
	b, err := fs.ReadFile(StaticFS, "static/setup.html")
	if err != nil {
		return []byte("setup page not embedded: " + err.Error())
	}
	return b
}()

// setupExtra 网站信息（持久化到 system_settings.extra_config JSON）
type setupExtra struct {
	SiteName    string `json:"site_name"`
	Subtitle    string `json:"subtitle"`
	LogoURL     string `json:"logo_url"`
	FooterText  string `json:"footer_text"`
	AdminEmail  string `json:"admin_email,omitempty"`
}

// setupRequest 向导提交体
type setupRequest struct {
	// 数据库
	DBHost     string `json:"db_host"`
	DBPort     int    `json:"db_port"`
	DBUser     string `json:"db_user"`
	DBPassword string `json:"db_password"`
	DBName     string `json:"db_name"`
	// 管理员
	AdminUsername string `json:"admin_username"`
	AdminPassword string `json:"admin_password"`
	// 网站信息
	SiteName   string `json:"site_name"`
	Subtitle   string `json:"subtitle"`
	LogoURL    string `json:"logo_url"`
	FooterText string `json:"footer_text"`
	AdminEmail string `json:"admin_email"`
}

// RegisterSetupRoutes 注册 setup 向导路由（仅在 setup 模式下挂载）
func RegisterSetupRoutes(r *gin.Engine) {
	// GET /setup → 返回向导页面
	r.GET("/setup", handleSetupPage)
	// 静态资源（setup 页面用到的 css 仍走原 static 目录）
	if sub, err := fs.Sub(StaticFS, "static"); err == nil {
		r.StaticFS("/static", http.FS(sub))
	}

	api := r.Group("/api")
	{
		api.GET("/setup/status", handleSetupStatus) // 当前配置状态
		api.POST("/setup/test-db", handleTestDB)    // 测试数据库连接（不写文件）
		api.POST("/setup", handleSetup)            // 执行完整初始化
	}
}

// handleSetupPage 返回 setup.html
func handleSetupPage(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", setupPageBytes)
}

// handleSetupStatus GET /api/setup/status
// 返回当前配置是否就绪
func handleSetupStatus(c *gin.Context) {
	cfg, _ := db.LoadConfig()
	ready := false
	if env := strings.TrimSpace(os.Getenv("FUNAUTH_MYSQL_DSN")); env != "" {
		ready = true
	} else if cfg != nil {
		dsn := db.ResolveDSN(cfg)
		if !strings.Contains(dsn, ":密码@") && cfg.MySQL.Host != "" {
			ready = true
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":   true,
		"ready": ready,
	})
}

// handleTestDB POST /api/setup/test-db
// 只测试连接，不写文件、不初始化
type testDBReq struct {
	DBHost     string `json:"db_host"`
	DBPort     int    `json:"db_port"`
	DBUser     string `json:"db_user"`
	DBPassword string `json:"db_password"`
	DBName     string `json:"db_name"`
}

func handleTestDB(c *gin.Context) {
	var req testDBReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "msg": "参数错误"})
		return
	}
	if req.DBHost == "" || req.DBUser == "" || req.DBName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "msg": "host/user/database 不能为空"})
		return
	}
	port := req.DBPort
	if port <= 0 {
		port = 3306
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		req.DBUser, req.DBPassword, req.DBHost, port, req.DBName)

	gdb, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "msg": "连接失败: " + sanitizeErr(err.Error())})
		return
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "msg": "获取 DB 句柄失败"})
		return
	}
	defer sqlDB.Close()
	if err := sqlDB.Ping(); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "msg": "Ping 失败: " + sanitizeErr(err.Error())})
		return
	}
	// 顺便返回版本号
	var version string
	gdb.Raw("SELECT VERSION()").Scan(&version)
	c.JSON(http.StatusOK, gin.H{"ok": true, "msg": "连接成功", "data": gin.H{"version": version}})
}

// sanitizeErr 去除密码信息（DSN 出现在错误里时）
func sanitizeErr(s string) string {
	// DSN 形式：user:pass@tcp(...) → 隐藏 pass
	if at := strings.Index(s, "@"); at > 0 {
		if colon := strings.Index(s, ":"); colon >= 0 && colon < at {
			return s[:colon+1] + "***" + s[at:]
		}
	}
	return s
}

// handleSetup POST /api/setup
// 完整初始化：测试连接 → 写 config.json → 初始化 DB → 创建管理员 → 创建默认组 → 写网站信息
func handleSetup(c *gin.Context) {
	var req setupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "msg": "参数错误: " + err.Error()})
		return
	}
	if req.DBHost == "" || req.DBUser == "" || req.DBName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "msg": "数据库 host/user/database 不能为空"})
		return
	}
	if len(req.AdminUsername) < 3 || len(req.AdminUsername) > 50 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "msg": "管理员用户名长度需 3-50"})
		return
	}
	if len(req.AdminPassword) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "msg": "管理员密码至少 6 位"})
		return
	}
	if req.SiteName == "" {
		req.SiteName = "FunAuth 用户中心"
	}

	port := req.DBPort
	if port <= 0 {
		port = 3306
	}

	// 1) 测试连接
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		req.DBUser, req.DBPassword, req.DBHost, port, req.DBName)
	gdb, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "msg": "数据库连接失败: " + sanitizeErr(err.Error())})
		return
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "msg": "获取 DB 句柄失败"})
		return
	}
	defer sqlDB.Close()
	if err := sqlDB.Ping(); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "msg": "数据库 Ping 失败: " + sanitizeErr(err.Error())})
		return
	}

	// 2) 写 config.json
	cfg := &db.Config{
		MySQL: db.MySQLConfig{
			Host:     req.DBHost,
			Port:     port,
			Username: req.DBUser,
			Password: req.DBPassword,
			Database: req.DBName,
			Params:   "charset=utf8mb4&parseTime=True&loc=Local",
		},
	}
	path, err := db.SaveConfig(cfg)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "msg": "保存 config.json 失败: " + err.Error()})
		return
	}

	// 3) 初始化数据库（建表 / 修列）
	// 关闭旧连接，让 InitDBWithOptions 重新打开
	db.SetDSNOverride(dsn)
	if err := db.InitDBWithOptions(db.InitOptions{StartCom4399AccountPool: false}); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "msg": "初始化数据库失败: " + err.Error()})
		return
	}

	// 4) 创建管理员账号（若已存在则跳过）
	if err := createAdminAccount(req.AdminUsername, req.AdminPassword); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "msg": "创建管理员失败: " + err.Error()})
		return
	}

	// 5) 创建默认身份组（管理员组 / 普通用户组），并把管理员加入管理员组
	if err := ensureDefaultGroups(req.AdminUsername); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "msg": "创建默认身份组失败: " + err.Error()})
		return
	}

	// 6) 写网站信息到 system_settings.extra_config
	extra := setupExtra{
		SiteName:   req.SiteName,
		Subtitle:    req.Subtitle,
		LogoURL:    req.LogoURL,
		FooterText: req.FooterText,
		AdminEmail: req.AdminEmail,
	}
	if err := writeSiteInfo(extra); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "msg": "保存网站信息失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":   true,
		"msg":  "初始化完成",
		"data": gin.H{"config_path": path, "next_step": "重启程序进入正式服务模式"},
	})
}

// createAdminAccount 创建管理员账号（已存在则跳过）
func createAdminAccount(username, password string) error {
	var existing db.User
	err := db.DB.Where("username = ?", username).First(&existing).Error
	if err == nil {
		// 已存在 → 更新密码为 bcrypt，并解封
		hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		return db.DB.Model(&db.User{}).Where("username = ?", username).Updates(map[string]interface{}{
			"password":   string(hash),
			"is_banned":  false,
			"updated_at": time.Now(),
		}).Error
	}
	// 不存在 → 创建
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	u := &db.User{
		UUID:     uuidStr(),
		Username: username,
		Password: string(hash),
		Token:    "",
	}
	if err := db.DB.Create(u).Error; err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	// 创建钱包
	if err := db.DB.Create(&db.Wallet{UserUUID: u.UUID, Balance: 0}).Error; err != nil {
		// 钱包失败不致命
		_ = err
	}
	return nil
}

// ensureDefaultGroups 创建默认身份组并把管理员加入管理员组
func ensureDefaultGroups(adminUsername string) error {
	// 管理员组（permissions=["*"]）
	var adminGroup db.Group
	if err := db.DB.Where("name = ?", "管理员").First(&adminGroup).Error; err != nil {
		adminGroup = db.Group{
			Name:        "管理员",
			Permissions: `["*"]`,
			Description: "超级管理员组",
			IsDefault:   false,
			SortOrder:   0,
		}
		if err := db.DB.Create(&adminGroup).Error; err != nil {
			return fmt.Errorf("create admin group: %w", err)
		}
	}
	// 普通用户组
	var userGroup db.Group
	if err := db.DB.Where("name = ?", "普通用户").First(&userGroup).Error; err != nil {
		userGroup = db.Group{
			Name:        "普通用户",
			Permissions: `[]`,
			Description: "默认用户组",
			IsDefault:   true,
			SortOrder:   1,
		}
		if err := db.DB.Create(&userGroup).Error; err != nil {
			return fmt.Errorf("create user group: %w", err)
		}
	}
	// 把管理员加入管理员组
	var admin db.User
	if err := db.DB.Where("username = ?", adminUsername).First(&admin).Error; err != nil {
		return fmt.Errorf("find admin user: %w", err)
	}
	// 检查关联是否已存在
	var count int64
	db.DB.Model(&db.UserGroup{}).Where("user_uuid = ? AND group_id = ?", admin.UUID, adminGroup.ID).Count(&count)
	if count == 0 {
		if err := db.DB.Create(&db.UserGroup{
			UserUUID: admin.UUID,
			GroupID: adminGroup.ID,
		}).Error; err != nil {
			return fmt.Errorf("assign admin to group: %w", err)
		}
	}
	return nil
}

// writeSiteInfo 把网站信息写入 system_settings.extra_config
func writeSiteInfo(extra setupExtra) error {
	b, err := json.Marshal(extra)
	if err != nil {
		return err
	}
	extraJSON := string(b)
	var ss db.SystemSetting
	findErr := db.DB.Where("id = 1").First(&ss).Error
	if findErr != nil {
		// 不存在 → 插入
		ss = db.SystemSetting{
			ID:              1,
			LoginEnabled:    true,
			RegisterEnabled: true,
			ExtraConfig:     extraJSON,
		}
		return db.DB.Create(&ss).Error
	}
	// 存在 → 更新 extra_config
	return db.DB.Model(&db.SystemSetting{}).Where("id = 1").Updates(map[string]interface{}{
		"extra_config": extraJSON,
		"updated_at":   time.Now(),
	}).Error
}

// uuidStr 生成 UUID（避免在 setup.go 引入 google/uuid 包，复用 db.GenerateUUID）
func uuidStr() string {
	return db.GenerateUUID("")
}
