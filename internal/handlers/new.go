package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func RegisterNewRoutes(rg *gin.RouterGroup) {
	rg.GET("/new", func(c *gin.Context) {
		id := uuid.NewString()
		c.Data(http.StatusOK, "text/plain", []byte(id))
	})
}

// RegisterNewRoutesIfAbsent 与 RegisterNewRoutes 等价，仅语义不同：
// 表示「若未被其他模块占用才挂」。目前 router.go 里总是先挂 WebUI（已含 /new），
// 所以我们这里注册为独立的 noop-compat，保证函数存在即可，不需要真的再注册一次。
// 实际 RegisterNewRoutesIfAbsent = 空操作。
func RegisterNewRoutesIfAbsent(rg *gin.RouterGroup) {
	// WebUI/fbauthserver.go 已经注册了 GET /api/new 并且功能是生成 FB secret，
	// 比这里生成 uuid 更丰富。为避免路由冲突，不重复注册。
}
