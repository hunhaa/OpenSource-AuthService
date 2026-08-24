package handlers

import (
	"fmt"
	"net/http"

	"github.com/Yeah114/FunAuth/auth"
	"github.com/Yeah114/FunAuth/internal/db"
	"github.com/gin-gonic/gin"
)

func RegisterPhoenixTanLobbyCreateRoute(api *gin.RouterGroup) {
	api.POST("/phoenix/tan_lobby_create", func(c *gin.Context) {
		var req TanLobbyCreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, TanLobbyCreateResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyCreate: 绑定请求体时出现问题, 原因是 %v", err)})
			return
		}
		if req.FBToken == "" {
			c.JSON(http.StatusOK, TanLobbyCreateResponse{Success: false, ErrorInfo: "TanLobbyCreate: 请提供FBToken"})
			return
		}

		// login_token 是 Phoenix 登录返回的 FBToken，需要先换取数据库中的 Sauth Cookie。
		authenticatedUser, err := db.ValidateFBToken(req.FBToken)
		if err != nil {
			c.JSON(http.StatusOK, TanLobbyCreateResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyCreate: 无效的FBToken, 原因是 %v", err)})
			return
		}

		cli, _, err := getOrCreateClient(authenticatedUser)
		if err != nil {
			c.JSON(http.StatusOK, TanLobbyCreateResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyCreate: 使用 Cookie 认证时出现问题, 原因是 %v", err)})
			return
		}

		createRes, err := auth.TanLobbyCreate(c.Request.Context(), cli)
		if err != nil {
			c.JSON(http.StatusOK, TanLobbyCreateResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyCreate: %v", err)})
			return
		}

		c.JSON(http.StatusOK, TanLobbyCreateResponse{
			Success:                true,
			ErrorInfo:              "",
			UserUniqueID:           createRes.UserUniqueID,
			UserPlayerName:         createRes.UserPlayerName,
			RaknetServerAddress:    createRes.RaknetServerAddress,
			RaknetRand:             createRes.RaknetRand,
			RaknetAESRand:          createRes.RaknetAESRand,
			EncryptKeyBytes:        createRes.EncryptKeyBytes,
			DecryptKeyBytes:        createRes.DecryptKeyBytes,
			SignalingServerAddress: createRes.SignalingServerAddress,
			SignalingSeed:          createRes.SignalingSeed,
			SignalingTicket:        createRes.SignalingTicket,
		})
	})
}
