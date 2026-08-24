package handlers

import "github.com/gin-gonic/gin"

// 汇总注册，调用各自文件中的注册函数
func RegisterPhoenixRoutes(api *gin.RouterGroup) {
	RegisterPhoenixLoginRoute(api)
	RegisterPhoenixTransferCheckNumRoute(api)
	RegisterPhoenixTransferStartTypeRoute(api)

	RegisterPhoenixTanLobbyLoginRoute(api)
	RegisterPhoenixTanLobbyCreateRoute(api)
	RegisterPhoenixTanLobbyTransferServerRoute(api)
}

// RegisterPhoenixNonOverlapRoutes 只注册不与 WebUI/FBToken (fbauthserver.go) 冲突的端点：
// 排除 login / transfer_check_num / transfer_start_type，保留 tan_lobby_* 和 transfer_server。
func RegisterPhoenixNonOverlapRoutes(api *gin.RouterGroup) {
	RegisterPhoenixTanLobbyLoginRoute(api)
	RegisterPhoenixTanLobbyCreateRoute(api)
	RegisterPhoenixTanLobbyTransferServerRoute(api)
}
