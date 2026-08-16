package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/Yeah114/FunAuth/auth"
	"github.com/Yeah114/FunAuth/internal/db"
	"github.com/Yeah114/g79client"
	"github.com/Yeah114/g79client/account/mpay"
	"github.com/gin-gonic/gin"
)

var random = rand.New(rand.NewSource(time.Now().UnixNano()))

type AuthVerifyResponse struct {
	Status string `json:"status"`
}

func isCom4399User(user *db.User) bool {
	return user != nil && !user.IsX19
}

func ensureLoginMaterial(authenticatedUser *db.User) (*db.User, error) {
	if authenticatedUser == nil {
		return nil, fmt.Errorf("nil user")
	}
	if authenticatedUser.IsX19 && db.IsCom4399CredentialsPayload(authenticatedUser.Sauth) {
		if err := db.UpdateUserDeviceAndSauthToNull(authenticatedUser.UUID); err != nil {
			return nil, fmt.Errorf("清理无效 Sauth 失败: %w", err)
		}
		var err error
		authenticatedUser, err = db.GetUserByUUID(authenticatedUser.UUID)
		if err != nil {
			return nil, fmt.Errorf("重新获取用户信息失败: %w", err)
		}
	}
	if isCom4399User(authenticatedUser) {
		if err := db.EnsureCom4399CredentialsForUser(context.Background(), authenticatedUser.UUID); err != nil {
			return nil, err
		}
		return db.GetUserByUUID(authenticatedUser.UUID)
	}
	if strings.TrimSpace(authenticatedUser.Sauth) == "" {
		if err := db.CreateUserSauthFromDeviceID(authenticatedUser.UUID); err != nil {
			return nil, err
		}
		return db.GetUserByUUID(authenticatedUser.UUID)
	}
	return authenticatedUser, nil
}

func newAuthenticatedClient(authenticatedUser *db.User) (*g79client.Client, error) {
	if authenticatedUser == nil {
		return nil, fmt.Errorf("nil user")
	}
	if isCom4399User(authenticatedUser) {
		return db.AuthenticateCom4399User(context.Background(), authenticatedUser.UUID)
	}

	cli, err := g79client.NewClient()
	if err != nil {
		return nil, err
	}
	if err := cli.G79AuthenticateWithCookie(authenticatedUser.Sauth); err != nil {
		return nil, err
	}
	return cli, nil
}

func getOrCreateClient(authenticatedUser *db.User) (*g79client.Client, bool, error) {
	cache := GetClientCache()
	resetCachedClient := authenticatedUser != nil &&
		isCom4399User(authenticatedUser) &&
		!db.IsCom4399CredentialsPayload(authenticatedUser.Device)

	authenticatedUser, err := ensureLoginMaterial(authenticatedUser)
	if err != nil {
		return nil, false, err
	}
	if resetCachedClient {
		cache.RemoveClient(authenticatedUser.UUID)
	}

	if cached, ok := cache.GetClient(authenticatedUser.UUID); ok {
		if !isAuthenticatedG79Client(cached.Client) {
			cache.RemoveClient(authenticatedUser.UUID)
		} else {
			log.Printf("[ClientCache] 命中缓存, 用户: %s (UUID: %s)", authenticatedUser.Username, authenticatedUser.UUID)
			return cached.Client, true, nil
		}
	}

	cli, err := newAuthenticatedClient(authenticatedUser)
	if err != nil {
		return nil, false, err
	}
	if !isAuthenticatedG79Client(cli) {
		return nil, false, fmt.Errorf("G79AuthError : authenticated client is incomplete")
	}

	cache.SetClient(authenticatedUser.UUID, cli)

	return cli, false, nil
}

// tryAutoChangeSauth 在当前用户的 SAuth 认证失败后自动更换账号并重试。
func tryAutoChangeSauth(c *gin.Context, cli **g79client.Client, authenticatedUser **db.User) bool {
	const maxSauthAttempts = 10
	cache := GetClientCache()

	for i := 0; i < maxSauthAttempts; i++ {
		sauthPool, changeErr := db.ChangeUserSauth((*authenticatedUser).UUID)
		if changeErr != nil {
			if errors.Is(changeErr, db.ErrNoAvailableSauth) {
				break
			}
			continue
		}

		user, userErr := db.GetUserByUUID((*authenticatedUser).UUID)
		if userErr != nil {
			continue
		}
		*authenticatedUser = user

		newCli, createErr := newAuthenticatedClient(*authenticatedUser)
		if createErr != nil {
			_ = db.MarkSauthAsBanned(sauthPool.ID)
			continue
		}

		if authErr := newCli.G79AuthenticateWithCookie((*authenticatedUser).Sauth); authErr != nil {
			_ = db.MarkSauthAsBanned(sauthPool.ID)
			continue
		}

		*cli = newCli
		cache.SetClient((*authenticatedUser).UUID, newCli)
		return true
	}

	// 自动池耗尽后进入常规注册。该路径会保存新 device；若需要验证码，
	// 下次请求会优先使用该 device 继续获取 Sauth。
	if err := db.UpdateUserDeviceAndSauthToNull((*authenticatedUser).UUID); err != nil {
		c.JSON(http.StatusOK, LoginResponse{
			SuccessStates: false,
			Message:       Message{Information: fmt.Sprintf("Login: 清空旧认证信息失败, 原因是 %v", err)},
		})
		return false
	}
	if err := db.CreateUserSauthWithDeviceID((*authenticatedUser).UUID); err != nil {
		c.JSON(http.StatusOK, LoginResponse{
			SuccessStates: false,
			Message:       Message{Information: fmt.Sprintf("Login: 自动换号失败后常规注册 Sauth 失败, 原因是 %v", err)},
		})
		return false
	}

	user, err := db.GetUserByUUID((*authenticatedUser).UUID)
	if err != nil {
		c.JSON(http.StatusOK, LoginResponse{
			SuccessStates: false,
			Message:       Message{Information: fmt.Sprintf("Login: 重新获取用户信息失败, 原因是 %v", err)},
		})
		return false
	}
	*authenticatedUser = user

	newCli, err := newAuthenticatedClient(*authenticatedUser)
	if err != nil {
		c.JSON(http.StatusOK, LoginResponse{
			SuccessStates: false,
			Message:       Message{Information: fmt.Sprintf("Login: 常规注册 Sauth 认证失败, 原因是 %v", err)},
		})
		return false
	}

	*cli = newCli
	cache.SetClient((*authenticatedUser).UUID, newCli)
	return true

}

func handleAuthError(c *gin.Context, err error, cli **g79client.Client, authenticatedUser **db.User) bool {
	cache := GetClientCache()
	cache.RemoveClient((*authenticatedUser).UUID)

	var codeErr *auth.CodeError
	errText := err.Error()
	isBanned := strings.Contains(errText, "您已被禁止登录游戏") ||
		strings.Contains(errText, "服务器维护中，请稍候再试")
	if errors.As(err, &codeErr) && codeErr.Code == 32 {
		isBanned = true
	}

	if isBanned {

		if isCom4399User(*authenticatedUser) {
			_ = db.UpdateUserDeviceAndSauthToNull((*authenticatedUser).UUID)
			if createErr := db.CreateUserSauthFromDeviceID((*authenticatedUser).UUID); createErr != nil {
				c.JSON(http.StatusOK, LoginResponse{
					SuccessStates: false,
					Message:       Message{Information: fmt.Sprintf("Login: 创建 4399 账号失败, 原因是 %v", createErr)},
				})
				return false
			}

			user, userErr := db.GetUserByUUID((*authenticatedUser).UUID)
			if userErr != nil {
				c.JSON(http.StatusOK, LoginResponse{
					SuccessStates: false,
					Message:       Message{Information: fmt.Sprintf("Login: 重新获取用户信息失败, 原因是 %v", userErr)},
				})
				return false
			}
			*authenticatedUser = user

			newCli, createErr := newAuthenticatedClient(*authenticatedUser)
			if createErr != nil {
				c.JSON(http.StatusOK, LoginResponse{
					SuccessStates: false,
					Message:       Message{Information: fmt.Sprintf("Login: 使用新的 4399 账号认证失败, 原因是 %v", createErr)},
				})
				return false
			}

			*cli = newCli
			cache.SetClient((*authenticatedUser).UUID, newCli)
			return true
		}

		if (*authenticatedUser).AutoChangeSauth {
			return tryAutoChangeSauth(c, cli, authenticatedUser)
		}

		if updateErr := db.UpdateUserDeviceAndSauthToNull((*authenticatedUser).UUID); updateErr != nil {
		}

		if createErr := db.CreateUserSauthFromDeviceID((*authenticatedUser).UUID); createErr != nil {
			c.JSON(http.StatusOK, LoginResponse{
				SuccessStates: false,
				Message:       Message{Information: "Login: 辅助用户已被封禁 我们已为您丢弃辅助用户账号 请重新登录服务器"},
			})
			return false
		}

		user, userErr := db.GetUserByUUID((*authenticatedUser).UUID)
		if userErr != nil {
			c.JSON(http.StatusOK, LoginResponse{
				SuccessStates: false,
				Message:       Message{Information: "Login: 辅助用户已被封禁 我们已为您丢弃辅助用户账号 请重新登录服务器"},
			})
			return false
		}
		*authenticatedUser = user

		newCli, createErr := newAuthenticatedClient(*authenticatedUser)
		if createErr != nil {
			c.JSON(http.StatusOK, LoginResponse{
				SuccessStates: false,
				Message:       Message{Information: fmt.Sprintf("Login: 创建客户端失败: %v", createErr)},
			})
			return false
		}

		if authErr := newCli.G79AuthenticateWithCookie((*authenticatedUser).Sauth); authErr != nil {
			c.JSON(http.StatusOK, LoginResponse{
				SuccessStates: false,
				Message:       Message{Information: fmt.Sprintf("Login: 使用新的 Sauth 认证时出现问题, 原因是 %v", authErr)},
			})
			return false
		}

		*cli = newCli
		cache.SetClient((*authenticatedUser).UUID, newCli)
		return true
	}

	var verifyErr *mpay.NeedVerifyError
	if errors.As(err, &verifyErr) && (verifyErr.Code == 27003 || verifyErr.Code == 27004) {
		return HandleRealNameVerification(c, verifyErr, *cli, *authenticatedUser)
	}

	// PE 认证返回的 27003/27004 错误，包装成 NeedVerifyError 类型
	if !isCom4399User(*authenticatedUser) && (strings.Contains(err.Error(), "code: 27003") || strings.Contains(err.Error(), "code: 27004")) {
		verifyErr = &mpay.NeedVerifyError{Code: 27003, Reason: "PE认证需要实名"}
		if !HandleRealNameVerification(c, verifyErr, *cli, *authenticatedUser) {
			return false
		}
		// PE 认证实名成功后需要重新认证客户端
		newCli, authErr := newAuthenticatedClient(*authenticatedUser)
		if authErr != nil {
			c.JSON(http.StatusOK, LoginResponse{
				SuccessStates: false,
				Message:       Message{Information: fmt.Sprintf("Login: 实名后重新认证失败: %v", authErr)},
			})
			return false
		}
		*cli = newCli
		cache := GetClientCache()
		cache.SetClient((*authenticatedUser).UUID, newCli)
		return true
	}

	c.JSON(http.StatusOK, LoginResponse{
		SuccessStates: false,
		Message:       Message{Information: fmt.Sprintf("Login: 使用 Sauth 认证时出现问题, 原因是 %v", err)},
	})
	return false
}

func RegisterPhoenixLoginRoute(api *gin.RouterGroup) {
	api.POST("/phoenix/login", func(c *gin.Context) {
		rawAuthorization := c.GetHeader("Authorization")
		bearerToken := strings.TrimPrefix(rawAuthorization, "Bearer ")
		if bearerToken == "" {
			c.JSON(http.StatusOK, LoginResponse{
				SuccessStates: false,
				Message:       Message{Information: "Login: Authorization header missing Bearer token"},
			})
			return
		}

		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, LoginResponse{
				SuccessStates: false,
				Message:       Message{Information: "Login: invalid request body"},
			})
			return
		}

		var cli *g79client.Client
		var authenticatedUser *db.User
		var err error

		if req.FBToken != "" {
			authenticatedUser, err = db.ValidateFBToken(req.FBToken)
			if err != nil {
				c.JSON(http.StatusOK, LoginResponse{
					SuccessStates: false,
					Message:       Message{Information: fmt.Sprintf("Login: 无效的FBToken, 原因是 %v", err)},
				})
				return
			}
		} else if req.UserName != "" && req.Password != "" {
			authenticatedUser, err = db.ValidateUserCredentials(req.UserName, req.Password)
			if err != nil {
				c.JSON(http.StatusOK, LoginResponse{
					SuccessStates: false,
					Message:       Message{Information: fmt.Sprintf("Login: 用户认证失败, 原因是 %v", err)},
				})
				return
			}
		} else {
			c.JSON(http.StatusOK, LoginResponse{
				SuccessStates: false,
				Message:       Message{Information: "Login: 请提供FBToken或用户名和密码"},
			})
			return
		}

		if req.ServerCode == "::DRY::" {
			resp := LoginResponse{
				SuccessStates:  true,
				Message:        Message{Information: "ok"},
				BotLevel:       0,
				BotSkin:        SkinInfo{},
				BotComponent:   make(map[string]*int),
				FBToken:        authenticatedUser.Token,
				MasterName:     "",
				RentalServerIP: "",
				ChainInfo:      "",
			}
			c.JSON(http.StatusOK, resp)
			return
		}

		if authenticatedUser.Sauth == "" {
			if err := db.CreateUserSauthFromDeviceID(authenticatedUser.UUID); err != nil {
				c.JSON(http.StatusOK, LoginResponse{
					SuccessStates: false,
					Message:       Message{Information: fmt.Sprintf("Login: 创建Sauth失败, 原因是 %v", err)},
				})
				return
			}
			authenticatedUser, err = db.GetUserByUUID(authenticatedUser.UUID)
			if err != nil {
				c.JSON(http.StatusOK, LoginResponse{
					SuccessStates: false,
					Message:       Message{Information: fmt.Sprintf("Login: 重新获取用户信息失败, 原因是 %v", err)},
				})
				return
			}
		}

		cli, fromCache, err := getOrCreateClient(authenticatedUser)
		if err != nil {
			if !handleAuthError(c, err, &cli, &authenticatedUser) {
				return
			}
		}

		if req.ServerCode != "" {
			isRentalOrDomain := strings.HasPrefix(req.ServerCode, "DomainGame:") ||
				strings.HasPrefix(req.ServerCode, "PCDomainGame:") ||
				(!strings.HasPrefix(req.ServerCode, "DomainGame:") && !strings.HasPrefix(req.ServerCode, "PCDomainGame:") && !strings.HasPrefix(req.ServerCode, "LobbyGame:") && !strings.HasPrefix(req.ServerCode, "PCLobbyGame:") && !strings.HasPrefix(req.ServerCode, "NetworkGame:") && req.ServerCode != "MainCity")

			if isRentalOrDomain {
				hasSlot, slotErr := db.HasSlotForServer(authenticatedUser.Username, req.ServerCode)
				if slotErr != nil {
					c.JSON(http.StatusOK, LoginResponse{
						SuccessStates: false,
						Message:       Message{Information: fmt.Sprintf("Login: 检查卡槽权限时出现问题, 原因是 %v", slotErr)},
					})
					return
				}
				if !hasSlot {
					c.JSON(http.StatusOK, LoginResponse{
						SuccessStates: false,
						Message:       Message{Information: "Login: 没有该服务器的卡槽权限"},
					})
					return
				}
			}
		}

		loginRes, err := auth.Login(c.Request.Context(), cli, auth.LoginParams{
			ServerCode:      req.ServerCode,
			ServerPassword:  req.ServerPassword,
			ClientPublicKey: req.ClientPublicKey,
			NicknamePrefix:  authenticatedUser.NicknamePrefix,
		})

		if err != nil && fromCache {
			var codeErr *auth.CodeError
			if err.Error() == "nil client" || (errors.As(err, &codeErr) && (codeErr.Code == 10 || codeErr.Code == 22 || codeErr.Code == 32)) {
				cache := GetClientCache()
				cache.RemoveClient(authenticatedUser.UUID)

				cli, fromCache, err = getOrCreateClient(authenticatedUser)
				if err != nil {
					if !handleAuthError(c, err, &cli, &authenticatedUser) {
						return
					}
				}

				loginRes, err = auth.Login(c.Request.Context(), cli, auth.LoginParams{
					ServerCode:      req.ServerCode,
					ServerPassword:  req.ServerPassword,
					ClientPublicKey: req.ClientPublicKey,
					NicknamePrefix:  authenticatedUser.NicknamePrefix,
				})
			}
		}

		if err != nil {
			c.JSON(http.StatusOK, LoginResponse{
				SuccessStates: false,
				Message:       Message{Information: fmt.Sprintf("Login: 登录到租赁服时出现问题, 原因是 %v", err)},
			})
			return
		}

		if cli.UserDetail != nil && strings.TrimSpace(cli.UserDetail.Name) == "" {
			// 辅助账号没有名字 → 自动改为 nklm_XXXXX（5 位随机数字）；冲突时回退到用户配置的 prefix
			_ = g79client.EnsureNicknameNKLMIfEmpty(cli, authenticatedUser.NicknamePrefix)
		}

		var skinInfo SkinInfo
		authSkinInfo, err := auth.GetSkinInfo(cli)
		if err != nil {
			c.JSON(http.StatusOK, LoginResponse{
				SuccessStates: false,
				Message:       Message{Information: fmt.Sprintf("Login: 获取皮肤信息时出现问题, 原因是 %v", err)},
			})
			return
		}
		skinInfo = SkinInfo{
			ItemID:          authSkinInfo.ItemID,
			SkinDownloadURL: authSkinInfo.SkinDownloadURL,
			SkinIsSlim:      authSkinInfo.SkinIsSlim,
		}

		resetSession(bearerToken)
		session := getSessionByBearer(c)
		if session == nil {
			c.JSON(http.StatusOK, LoginResponse{
				SuccessStates: false,
				Message:       Message{Information: fmt.Sprintf("Login: 无效的 Auth Bearer (%s)", rawAuthorization)},
			})
			return
		}
		session.Store(sessionKeyEntityID, loginRes.EntityID)
		session.Store(sessionKeyEngineVersion, loginRes.EngineVersion)
		session.Store(sessionKeyPatchVersion, loginRes.PatchVersion)
		session.Store(sessionKeyUserID, loginRes.UID)
		session.Store(sessionKeyIsPC, loginRes.IsPC)

		resp := LoginResponse{
			SuccessStates:  true,
			Message:        Message{Information: "ok"},
			BotLevel:       loginRes.BotLevel,
			BotSkin:        skinInfo,
			BotComponent:   loginRes.BotComponent,
			FBToken:        authenticatedUser.Token,
			MasterName:     loginRes.MasterName,
			RentalServerIP: loginRes.IP,
			ChainInfo:      loginRes.ChainInfo,
		}
		c.JSON(http.StatusOK, resp)
	})
}

func HandleRealNameVerification(c *gin.Context, verifyErr *mpay.NeedVerifyError, cli *g79client.Client, authenticatedUser *db.User) bool {
	if verifyErr.Code != 27003 {
		c.JSON(http.StatusOK, LoginResponse{
			SuccessStates: false,
			Message:       Message{Information: fmt.Sprintf("Login: 需要验证: %v", verifyErr)},
		})
		return false
	}

	var cookieData g79client.CookieData
	if err := json.Unmarshal([]byte(authenticatedUser.Sauth), &cookieData); err != nil {
		c.JSON(http.StatusOK, LoginResponse{
			SuccessStates: false,
			Message:       Message{Information: fmt.Sprintf("Login: 解析 Cookie 失败: %v", err)},
		})
		return false
	}

	var sauthData g79client.SauthData
	if err := json.Unmarshal([]byte(cookieData.SauthJSON), &sauthData); err != nil {
		c.JSON(http.StatusOK, LoginResponse{
			SuccessStates: false,
			Message:       Message{Information: fmt.Sprintf("Login: 解析 Sauth 失败: %v", err)},
		})
		return false
	}

	maxAttempts := 100
	for attempt := 0; attempt < maxAttempts; attempt++ {
		idCode, err := db.GetRandomIDCode()
		if err != nil {
			c.JSON(http.StatusOK, LoginResponse{
				SuccessStates: false,
				Message:       Message{Information: fmt.Sprintf("Login: 获取身份证信息失败: %v", err)},
			})
			return false
		}

		device := &mpay.Device{
			ID:   sauthData.DeviceID,
			UDID: sauthData.UDID,
			Mac:  cookieData.MacAddr,
			Ram:  cookieData.RAM,
			Rom:  cookieData.ROM,
		}
		user := &mpay.User{
			Sauth: mpay.Sauth{
				DeviceID:  sauthData.DeviceID,
				SDKUID:    sauthData.SDKUID,
				SessionID: sauthData.SessionID,
				UDID:      sauthData.UDID,
			},
		}
		err = device.AuthRealName(context.Background(), user, idCode.Name, idCode.IDCard)
		if err == nil {
			db.DeleteIDCode(idCode.ID)
			return true
		}

		db.DeleteIDCode(idCode.ID)
	}

	c.JSON(http.StatusOK, LoginResponse{
		SuccessStates: false,
		Message:       Message{Information: "Login: 实名认证失败，已尝试所有可用的身份信息"},
	})
	//等待一秒 否则直接再次重新登录会返回操作过于频繁错误
	time.Sleep(1 * time.Second)

	return false
}
