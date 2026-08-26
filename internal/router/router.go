package router

import (
	"io"
	"log"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/Yeah114/FunAuth/internal/handlers"
	uc "github.com/Yeah114/FunAuth/modules/usercenter"
	webui "github.com/Yeah114/FunAuth/modules/webui"
)

func NewRouter() *gin.Engine {
	// 确保 gin 的默认日志写到 stdout
	gin.DefaultWriter = io.MultiWriter(os.Stdout)
	gin.DefaultErrorWriter = io.MultiWriter(os.Stdout)
	log.SetOutput(os.Stdout)

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	_ = r.SetTrustedProxies([]string{"127.0.0.1"})

	api := r.Group("/api")

	// WebUI 的 RegisterRoutes 内部会注册 /api/new + /api/phoenix/*（FBToken 版本），
	// 与 handlers 的路由路径完全相同但功能更全（支持 FBToken、sauth→fbtoken）。
	// 因此：只要挂 WebUI，就不再重复挂 handlers 的同名路由，避免 gin panic "handlers are already registered"。
	// 同时 handlers 的 Phoenix 功能（fixed cookie phoenix_login 等）在 WebUI 里已经有等价实现。
	webui.RegisterRoutes(api, r)

	// 用户中心（API: /api/usercenter/*；前端 SPA: /uc/）
	uc.RegisterRoutes(api, r)

	// 未被 WebUI 占用的 Phoenix 扩展（tan_lobby_*、transfer_* 等）仍然挂 handlers。
	// 注意：RegisterPhoenixRoutes 内部已按路由粒度去重不可能，只能在注册前先检查。
	// 由于 webui.RegisterFBAuthRoutes 只占了 login / transfer_check_num / transfer_start_type，
	// handlers.RegisterPhoenixRoutes 里剩下的 tan_lobby_* / transfer_server 不会冲突，
	// 但为了绝对安全（gin 不允许同路径二次注册），我们拆成独立注册器。
	handlers.RegisterPhoenixNonOverlapRoutes(api)
	handlers.RegisterNewRoutesIfAbsent(api)

	return r
}
