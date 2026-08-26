package usercenter

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

//go:embed static/*
var StaticFS embed.FS

// RegisterRoutes 注册用户中心所有路由
// api: 统一 API 前缀路由组（通常是 /api）
// engine: 根 gin.Engine（用于挂载 /uc 静态 SPA）
func RegisterRoutes(api *gin.RouterGroup, engine *gin.Engine) {
	// 初始化 JWT 密钥
	initJWTSecret()

	// CORS（开发期允许 *；生产由 Nginx 处理）
	if engine != nil {
		engine.Use(cors.New(cors.Config{
			AllowOriginFunc:  func(string) bool { return true },
			AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Session-Token"},
			ExposeHeaders:    []string{"Content-Length"},
			AllowCredentials: true,
		}))
	}

	uc := api.Group("/usercenter")
	{
		// 通用
		uc.GET("/health", HandleHealth)

		// 认证（公开）
		uc.POST("/login", HandleLogin)
		uc.POST("/register", HandleRegister)

		// 公告（公开）
		uc.GET("/announcements", HandleListAnnouncements)

		// 已认证用户接口
		auth := uc.Group("")
		auth.Use(JwtAuthMiddleware())
		{
			auth.POST("/logout", HandleLogout)
			auth.GET("/profile", HandleProfile)
			auth.PUT("/profile", HandleProfileUpdate)
			auth.POST("/profile/password", HandleChangePassword)
			auth.GET("/profile/wallet", HandleProfileWallet)
			auth.GET("/transactions", HandleTransactions)

			// 卡槽
			auth.GET("/slots/mine", HandleMySlots)
			auth.POST("/slots/:id/bind", HandleBindSlot)
			auth.POST("/slots/:id/unbind", HandleUnbindSlot)

			// 商店
			auth.GET("/shop/categories", HandleShopCategories)
			auth.GET("/shop/products", HandleShopProducts)
			auth.POST("/shop/orders", HandleCreateOrder)
			auth.GET("/shop/orders/mine", HandleMyOrders)

			// 兑换码
			auth.POST("/redeem", HandleRedeem)

			// 公告（列表已公开，无需再挂；这里保留私有查询占位）

			// API Key
			auth.GET("/api-keys/mine", HandleListMyAPIKeys)
			auth.POST("/api-keys", HandleCreateAPIKey)
			auth.DELETE("/api-keys/:id", HandleRevokeAPIKey)
		}

		// 管理员接口（两层中间件：JwtAuthMiddleware + RequirePermission("*")）
		admin := uc.Group("/admin")
		admin.Use(JwtAuthMiddleware(), RequirePermission("*"))
		{
			// 用户
			admin.GET("/users", HandleAdminListUsers)
			admin.PUT("/users/:uuid", HandleAdminUpdateUser)
			admin.POST("/users/:uuid/adjust-balance", HandleAdminAdjustBalance)
			admin.POST("/users/:uuid/adjust-quota", HandleAdminAdjustQuota)
			admin.POST("/users/:uuid/adjust-times", HandleAdminAdjustTimes)
			admin.POST("/users/:uuid/groups", HandleAdminSetUserGroups)

			// 身份组
			admin.GET("/groups", HandleAdminListGroups)
			admin.POST("/groups", HandleAdminCreateGroup)
			admin.PUT("/groups/:id", HandleAdminUpdateGroup)
			admin.DELETE("/groups/:id", HandleAdminDeleteGroup)

			// 商品
			admin.GET("/products", HandleAdminListProducts)
			admin.POST("/products", HandleAdminCreateProduct)
			admin.PUT("/products/:id", HandleAdminUpdateProduct)
			admin.DELETE("/products/:id", HandleAdminDeleteProduct)

			// 分类
			admin.GET("/categories", HandleAdminListCategories)
			admin.POST("/categories", HandleAdminCreateCategory)
			admin.PUT("/categories/:id", HandleAdminUpdateCategory)
			admin.DELETE("/categories/:id", HandleAdminDeleteCategory)

			// 订单
			admin.GET("/orders", HandleAdminListOrders)
			admin.PUT("/orders/:id/status", HandleAdminUpdateOrderStatus)

			// 兑换码
			admin.POST("/redeem-codes/generate", HandleAdminGenerateRedeemCodes)
			admin.GET("/redeem-codes", HandleAdminListRedeemCodes)

			// 卡槽
			admin.GET("/slots", HandleAdminListSlots)
			admin.POST("/slots/grant", HandleAdminGrantSlots)
			admin.DELETE("/slots/:id", HandleAdminDeleteSlot)

			// 公告
			admin.GET("/announcements", HandleAdminListAnnouncements)
			admin.POST("/announcements", HandleAdminCreateAnnouncement)
			admin.PUT("/announcements/:id", HandleAdminUpdateAnnouncement)
			admin.DELETE("/announcements/:id", HandleAdminDeleteAnnouncement)

			// 系统设置
			admin.GET("/system-settings", HandleAdminGetSystemSettings)
			admin.PUT("/system-settings", HandleAdminUpdateSystemSettings)

			// 流水
			admin.GET("/transactions", HandleAdminListTransactions)

			// API Key 查看用户
			admin.GET("/api-keys/:user_uuid", HandleAdminListUserAPIKeys)
		}
	}

	// 静态 SPA：挂在 /uc（保持与 /ui 旧控制台互不干扰）
	if engine != nil {
		staticRoot, err := fs.Sub(StaticFS, "static")
		if err == nil {
			engine.StaticFS("/uc", http.FS(staticRoot))
		}
	}
}
