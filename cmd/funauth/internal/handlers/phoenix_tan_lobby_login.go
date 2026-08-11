package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Yeah114/FunAuth/auth"
	"github.com/Yeah114/FunAuth/internal/db"
	"github.com/Yeah114/g79client"
	"github.com/gin-gonic/gin"
)

func RegisterPhoenixTanLobbyLoginRoute(api *gin.RouterGroup) {
	api.POST("/phoenix/tan_lobby_login", func(c *gin.Context) {
		rawAuthorization := c.GetHeader("Authorization")
		bearerToken := strings.TrimPrefix(rawAuthorization, "Bearer ")
		if bearerToken == "" {
			c.JSON(http.StatusOK, TanLobbyLoginResponse{Success: false, ErrorInfo: "TanLobbyLogin: Authorization header missing Bearer token"})
			return
		}

		var req TanLobbyLoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, TanLobbyLoginResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyLogin: 绑定请求体时出现问题, 原因是 %v", err)})
			return
		}

		var cli *g79client.Client
		var err error
		var authenticatedUser *db.User

		// 检查是否提供了FBToken
		if req.FBToken != "" {
			// 验证FBToken是否有效（在数据库中检查）
			authenticatedUser, err = db.ValidateFBToken(req.FBToken)
			if err != nil {
				c.JSON(http.StatusOK, TanLobbyLoginResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyLogin: 无效的FBToken, 原因是 %v", err)})
				return
			}

			// 如果Sauth为空，尝试创建
			if authenticatedUser.Sauth == "" {
				if err := db.CreateUserSauthFromDeviceID(authenticatedUser.UUID); err != nil {
					c.JSON(http.StatusOK, TanLobbyLoginResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyLogin: 创建Sauth失败, 原因是 %v", err)})
					return
				}

				// 重新获取用户信息以获取新创建的Sauth
				authenticatedUser, err = db.GetUserByUUID(authenticatedUser.UUID)
				if err != nil {
					c.JSON(http.StatusOK, TanLobbyLoginResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyLogin: 重新获取用户信息失败, 原因是 %v", err)})
					return
				}
			}

			authenticatedUser, err = ensureLoginMaterial(authenticatedUser)
			if err != nil {
				c.JSON(http.StatusOK, TanLobbyLoginResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyLogin: prepare login material failed: %v", err)})
				return
			}

			cli, _, err = getOrCreateClient(authenticatedUser)
			if err != nil {
				c.JSON(http.StatusOK, TanLobbyLoginResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyLogin: 初始化客户端时出现问题, 原因是 %v", err)})
				return
			}
		} else {
			c.JSON(http.StatusOK, TanLobbyLoginResponse{Success: false, ErrorInfo: "TanLobbyLogin: 请提供FBToken"})
			return
		}

		loginRes, err := auth.TanLobbyLogin(c.Request.Context(), cli, auth.TanLobbyLoginParams{
			RoomID: req.RoomID,
		})
		if err != nil {
			c.JSON(http.StatusOK, TanLobbyLoginResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyLogin: %v", err)})
			return
		}

		enableSkin := true
		var skinInfo SkinInfo
		if enableSkin {
			authSkinInfo, err := auth.GetSkinInfo(cli)
			if err != nil {
				c.JSON(http.StatusOK, TanLobbyLoginResponse{Success: false, ErrorInfo: fmt.Sprintf("TanLobbyLogin: 获取皮肤信息时出现问题, 原因是 %v", err)})
				return
			}
			skinInfo = SkinInfo{
				ItemID:          authSkinInfo.ItemID,
				SkinDownloadURL: authSkinInfo.SkinDownloadURL,
				SkinIsSlim:      authSkinInfo.SkinIsSlim,
			}
		}

		botLevel := 0
		if cli.UserDetail != nil {
			botLevel = int(cli.UserDetail.Level.Int64())
		}
		c.JSON(http.StatusOK, TanLobbyLoginResponse{
			Success:                true,
			ErrorInfo:              "",
			UserUniqueID:           loginRes.UserUniqueID,
			UserPlayerName:         loginRes.UserPlayerName,
			BotLevel:               botLevel,
			BotSkin:                skinInfo,
			BotComponent:           loginRes.BotComponent,
			RoomOwnerID:            loginRes.RoomOwnerID,
			RoomModDisplayName:     loginRes.RoomModDisplayName,
			RoomModDownloadURL:     loginRes.RoomModDownloadURL,
			RoomModEncryptKey:      loginRes.RoomModEncryptKey,
			RaknetServerAddress:    loginRes.RaknetServerAddress,
			RaknetRand:             loginRes.RaknetRand,
			RaknetAESRand:          loginRes.RaknetAESRand,
			EncryptKeyBytes:        loginRes.EncryptKeyBytes,
			DecryptKeyBytes:        loginRes.DecryptKeyBytes,
			SignalingServerAddress: loginRes.SignalingServerAddress,
			SignalingSeed:          loginRes.SignalingSeed,
			SignalingTicket:        loginRes.SignalingTicket,
		})
	})
}
