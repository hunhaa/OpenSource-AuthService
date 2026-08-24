package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	g79 "github.com/Yeah114/g79client"
	link "github.com/Yeah114/g79client/service/link_connection"
)

var random = rand.New(rand.NewSource(time.Now().UnixNano()))

type CodeError struct {
	Code    int
	Message string
	Op      string
}

func (e *CodeError) Error() string {
	return fmt.Sprintf("%s: %s(%d)", e.Op, e.Message, e.Code)
}

func checkCode(code int, message string, op string) error {
	if code != 0 {
		return &CodeError{Code: code, Message: message, Op: op}
	}
	return nil
}

func ensureUserDetail(cli *g79.Client, nicknamePrefix string) error {
	if cli.UserDetail == nil {
		detail, err := cli.GetUserDetail()
		if err != nil {
			return fmt.Errorf("GetUserDetail: %w", err)
		}
		cli.UserDetail = &detail.Entity
	}
	if cli.UserDetail != nil && cli.UserDetail.Name == "" {
		// 辅助账号没有名字：先按 nklm_XXXXX 命名；若传了 legacyPrefix，会在冲突重试时兜底
		if err := g79.EnsureNicknameNKLMIfEmpty(cli, nicknamePrefix); err != nil {
			return fmt.Errorf("UpdateNickname: %w", err)
		}
	}
	return nil
}

func searchRoomByKeyword(cli *g79.Client, keyword string) (string, error) {
	searchResp, err := cli.SearchOnlineLobbyRoomByKeyword(keyword, 1, 0)
	if err != nil {
		return "", fmt.Errorf("SearchOnlineLobbyRoomByKeyword: %w", err)
	}
	if err := checkCode(searchResp.Code, searchResp.Message, "SearchOnlineLobbyRoomByKeyword"); err != nil {
		return "", err
	}
	if len(searchResp.Entities) == 0 {
		return "", fmt.Errorf("SearchOnlineLobbyRoomByKeyword: 找不到房间")
	}
	return searchResp.Entities[0].EntityID.String(), nil
}

func getRoomInfo(cli *g79.Client, roomCode string) (*g79.OnlineLobbyRoomGetResponse, error) {
	roomInfo, err := cli.GetOnlineLobbyRoom(roomCode)
	if err != nil {
		return nil, fmt.Errorf("GetOnlineLobbyRoom: %w", err)
	}
	if err := checkCode(roomInfo.Code, roomInfo.Message, "GetOnlineLobbyRoom"); err != nil {
		return nil, err
	}
	return roomInfo, nil
}

func enterLobbyRoom(cli *g79.Client, roomCode, password string, purchaseFunc func(string) (*g79.UserItemPurchaseResponse, error)) (string, error) {
	var enterResp *g79.OnlineLobbyRoomEnterResponse
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		var err error
		enterResp, err = cli.EnterOnlineLobbyRoom(roomCode, password)
		if err != nil {
			return "", fmt.Errorf("EnterOnlineLobbyRoom: %w", err)
		}
		if enterResp.Code != 501 {
			break
		}
		if attempt < maxRetries {
			_, _ = purchaseFunc("")
			time.Sleep(500 * time.Millisecond)
		}
	}
	if enterResp.Code == 501 {
		return "", fmt.Errorf("EnterOnlineLobbyRoom: %s(%d)", enterResp.Message, enterResp.Code)
	}
	if err := checkCode(enterResp.Code, enterResp.Message, "EnterOnlineLobbyRoom"); err != nil {
		return "", err
	}

	gameEnter, err := cli.OnlineLobbyGameEnter()
	if err != nil {
		return "", fmt.Errorf("OnlineLobbyGameEnter: %w", err)
	}
	if err := checkCode(gameEnter.Code, gameEnter.Message, "OnlineLobbyGameEnter"); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", gameEnter.Entity.ServerHost, gameEnter.Entity.ServerPort.Int64()), nil
}

func getChainInfo(cli *g79.Client, authv2Data []byte) (string, error) {
	chainInfo, err := cli.SendAuthV2Request(authv2Data)
	if err != nil {
		return "", fmt.Errorf("SendAuthV2Request: %w", err)
	}
	return string(chainInfo), nil
}

func handleDomainGame(ctx context.Context, cli *g79.Client, inviteCode string, clientPublicKey string, isPC bool) (string, string, error) {
	resp, err := cli.GetOtherDomainServers()
	if err != nil {
		return "", "", fmt.Errorf("GetOtherDomainServers: %w", err)
	}
	for _, server := range resp.Entities {
		if _, delErr := cli.DeleteOtherDomainServer(server.Sid); delErr != nil {
			return "", "", fmt.Errorf("DeleteOtherDomainServer: %w", delErr)
		}
	}

	inviteResp, err := cli.JoinDomainServerWithInviteCode(inviteCode)
	if err != nil {
		return "", "", fmt.Errorf("JoinDomainServerWithInviteCode: %w", err)
	}
	if err := checkCode(inviteResp.Code, inviteResp.Message, "JoinDomainServerWithInviteCode"); err != nil {
		return "", "", err
	}

	serversResp, err := cli.GetOtherDomainServers()
	if err != nil {
		return "", "", fmt.Errorf("GetOtherDomainServers(after join): %w", err)
	}
	if len(serversResp.Entities) == 0 {
		return "", "", fmt.Errorf("GetOtherDomainServers: 加入后未找到山头服务器")
	}
	serverID := serversResp.Entities[0].Sid

	enterResp, err := cli.RequestEnterDomainServer(serverID)
	if err != nil {
		return "", "", fmt.Errorf("RequestEnterDomainServer(sid=%s): %w", serverID, err)
	}
	if err := checkCode(enterResp.Code, enterResp.Message, "RequestEnterDomainServer"); err != nil {
		return "", "", err
	}
	ipAddress := fmt.Sprintf("%s:%d", enterResp.Entity.ServerHost, enterResp.Entity.ServerPort.Int64())

	var authv2Data []byte
	if isPC {
		authv2Data, err = cli.GeneratePCDomainGameAuthV2(serverID, clientPublicKey)
	} else {
		authv2Data, err = cli.GenerateDomainGameAuthV2(serverID, clientPublicKey)
	}
	if err != nil {
		return "", "", fmt.Errorf("GenerateDomainGameAuthV2: %w", err)
	}

	chainInfo, err := getChainInfo(cli, authv2Data)
	if err != nil {
		return "", "", err
	}

	leaveResp, err := cli.RequestLeaveDomainServer(serverID)
	if err != nil {
		return "", "", fmt.Errorf("RequestLeaveDomainServer(sid=%s): %w", serverID, err)
	}
	if enterResp.Code != 0 {
		return "", "", fmt.Errorf("RequestLeaveDomainServer: %s(%d)", leaveResp.Message, leaveResp.Code)
	}

	if _, delErr := cli.DeleteOtherDomainServer(serverID); delErr != nil {
		return "", "", fmt.Errorf("DeleteOtherDomainServer(after auth-v2): %w", delErr)
	}

	return ipAddress, chainInfo, nil
}

func handleLobbyGame(cli *g79.Client, roomCode, password, clientPublicKey string, isPC bool) (string, string, error) {
	if len(roomCode) != 19 {
		var err error
		roomCode, err = searchRoomByKeyword(cli, roomCode)
		if err != nil {
			return "", "", err
		}
	}

	roomInfo, err := getRoomInfo(cli, roomCode)
	if err != nil {
		return "", "", err
	}

	var roomMap *g79.UserItemPurchaseResponse
	if isPC {
		roomMap, err = cli.UserItemPurchase(roomInfo.Entity.ResID.String())
	} else {
		var purchaseResp *g79.PurchaseItemResponse
		purchaseResp, err = cli.PurchaseItem(roomInfo.Entity.ResID.String())
		if purchaseResp != nil {
			roomMap = &g79.UserItemPurchaseResponse{}
			roomMap.Code = purchaseResp.Code
			roomMap.Message = purchaseResp.Message
		}
	}
	if err != nil {
		return "", "", fmt.Errorf("PurchaseItem: %w", err)
	}
	if !(roomMap.Code == 0 || roomMap.Code == 502 || roomMap.Code == 44) {
		return "", "", fmt.Errorf("PurchaseItem: %s(%d)", roomMap.Message, roomMap.Code)
	}

	var purchaseFunc func(string) (*g79.UserItemPurchaseResponse, error)
	if isPC {
		purchaseFunc = cli.UserItemPurchase
	} else {
		purchaseFunc = func(s string) (*g79.UserItemPurchaseResponse, error) {
			resp, err := cli.PurchaseItem(s)
			if err != nil {
				return nil, err
			}
			result := &g79.UserItemPurchaseResponse{}
			result.Code = resp.Code
			result.Message = resp.Message
			return result, nil
		}
	}

	ipAddress, err := enterLobbyRoom(cli, roomCode, password, purchaseFunc)
	if err != nil {
		return "", "", err
	}

	var authv2Data []byte
	if isPC {
		authv2Data, err = cli.GeneratePCLobbyGameAuthV2(roomInfo.Entity.ResID.String(), clientPublicKey)
	} else {
		authv2Data, err = cli.GenerateLobbyGameAuthV2(roomCode, clientPublicKey)
	}
	if err != nil {
		return "", "", fmt.Errorf("GenerateLobbyGameAuthV2: %w", err)
	}

	chainInfo, err := getChainInfo(cli, authv2Data)
	if err != nil {
		return "", "", err
	}

	return ipAddress, chainInfo, nil
}

func handleRentalGame(ctx context.Context, cli *g79.Client, serverCode, password, clientPublicKey string) (string, string, error) {
	searchResp, err := cli.SearchRentalServerByName(serverCode)
	if err != nil {
		return "", "", fmt.Errorf("SearchRentalServerByName: %w", err)
	}
	if err := checkCode(searchResp.Code, searchResp.Message, "SearchRentalServerByName"); err != nil {
		return "", "", err
	}
	if len(searchResp.Entities) == 0 {
		return "", "", fmt.Errorf("SearchRentalServerByName: 找不到服务器")
	}
	serverEntity := searchResp.Entities[0]
	serverID := serverEntity.EntityID

	ownerID := strings.TrimSpace(serverEntity.OwnerID.String())
	var ownerName string
	if ownerID != "" {
		ownerInfo, err := cli.GetUserElseDetailMany([]string{ownerID})
		if err != nil {
			return "", "", fmt.Errorf("GetUserElseDetailMany(owner_id=%s): %w", ownerID, err)
		}
		if err := checkCode(ownerInfo.Code, ownerInfo.Message, "GetUserElseDetailMany"); err != nil {
			return "", "", err
		}
		if len(ownerInfo.Entities) == 0 {
			return "", "", fmt.Errorf("GetUserElseDetailMany: 未返回服主信息")
		}
		ownerName = ownerInfo.Entities[0].Nickname
	}
	if ownerName == "" {
		if ownerID != "" {
			ownerName = ownerID
		} else {
			ownerName = serverCode
		}
	}

	enterResp, err := cli.EnterRentalServerWorld(serverID.String(), password)
	if err != nil {
		return "", "", fmt.Errorf("EnterRentalServerWorld: %w", err)
	}
	if err := checkCode(enterResp.Code, enterResp.Message, "EnterRentalServerWorld"); err != nil {
		return "", "", err
	}
	ipAddress := fmt.Sprintf("%s:%d", enterResp.Entity.McserverHost, enterResp.Entity.McserverPort.Int64())

	var linkErr error
	const maxLinkRetries = 3
	for attempt := 1; attempt <= maxLinkRetries; attempt++ {
		linkErr = func() error {
			service, err := link.NewLinkConnectionService(cli)
			if err != nil {
				return fmt.Errorf("NewLinkConnectionService: %w", err)
			}

			dialCtx := ctx
			if dialCtx == nil {
				dialCtx = context.Background()
			}
			dialCtx, cancel := context.WithTimeout(dialCtx, 10*time.Second)
			defer cancel()
			conn, err := service.Dial(dialCtx)
			if err != nil {
				return fmt.Errorf("link.Dial: %w", err)
			}

			gameInfo := map[string]interface{}{
				"min_level": serverEntity.MinLevel.Int64(),
				"room_name": serverCode,
				"gameType":  "RentalGame",
				"res_name":  serverCode,
				"ownerName": ownerName,
				"ownerId":   ownerID,
				"id":        serverEntity.EntityID.String(),
			}
			gameInfoJSON, err := json.Marshal(gameInfo)
			if err != nil {
				conn.Close()
				return fmt.Errorf("marshal rental game info: %w", err)
			}
			gameStartPayload := map[string]interface{}{
				"game_info":    string(gameInfoJSON),
				"strict_mode":  true,
				"game_type":    10,
				"is_free_play": false,
				"game_id":      serverEntity.EntityID.String(),
				"play_iids":    []string{},
			}

			opCtx, opCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer opCancel()

			errCh := make(chan error, 1)
			go func() {
				if err := conn.SendGameStart(gameStartPayload); err != nil {
					errCh <- fmt.Errorf("SendGameStart: %w", err)
					return
				}
				if err := conn.Conn().Close(); err != nil {
					errCh <- fmt.Errorf("Close: %w", err)
					return
				}
				errCh <- nil
			}()

			select {
			case err := <-errCh:
				if err != nil {
					return err
				}
			case <-opCtx.Done():
				return fmt.Errorf("operation timed out: %w", opCtx.Err())
			}
			return nil
		}()

		if linkErr == nil {
			break
		}
		if attempt < maxLinkRetries {
			fmt.Printf("Link 连接失败 (尝试 %d/%d): %v，正在重试...\n", attempt, maxLinkRetries, linkErr)
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	if linkErr != nil {
		return "", "", fmt.Errorf("link 连接失败 (已重试 %d 次): %w", maxLinkRetries, linkErr)
	}

	authv2Data, err := cli.GenerateRentalGameAuthV2(serverID.String(), clientPublicKey)
	if err != nil {
		return "", "", fmt.Errorf("GenerateRentalGameAuthV2: %w", err)
	}

	chainInfo, err := getChainInfo(cli, authv2Data)
	if err != nil {
		return "", "", err
	}

	return ipAddress, chainInfo, nil
}

func Login(ctx context.Context, cli *g79.Client, p LoginParams) (LoginResult, error) {
	var result LoginResult
	if cli == nil {
		return result, fmt.Errorf("nil client")
	}

	if err := ensureUserDetail(cli, p.NicknamePrefix); err != nil {
		return result, err
	}

	var ipAddress, chainInfoStr string
	var err error

	if p.ServerCode == "" {
		return result, fmt.Errorf("server code is empty")
	}
	p.ServerPassword = strings.ReplaceAll(p.ServerPassword, "000000", "")

	if after, ok := strings.CutPrefix(p.ServerCode, "LobbyGame:"); ok && after != "" {
		ipAddress, chainInfoStr, err = handleLobbyGame(cli, after, p.ServerPassword, p.ClientPublicKey, false)
	} else if after, ok := strings.CutPrefix(p.ServerCode, "PCLobbyGame:"); ok && after != "" {
		newCli, err := g79.NewClient()
		if err != nil {
			return result, fmt.Errorf("NewClient: %w", err)
		}
		for {
			time.Sleep(time.Second)
			err = newCli.X19AuthenticateWithCookie(cli.Cookie)
			if err == nil {
				break
			}
			if strings.Contains(err.Error(), "操作过于频繁，请稍后重试") {
				continue
			}
		}
		cli = newCli
		ipAddress, chainInfoStr, err = handleLobbyGame(cli, after, p.ServerPassword, p.ClientPublicKey, true)
		result.IsPC = true
	} else if after, ok := strings.CutPrefix(p.ServerCode, "NetworkGame:"); ok && after != "" {
		serverAddress, err := cli.GetPeGameServerAddress(after)
		if err != nil {
			return result, fmt.Errorf("GetPeGameServerAddress: %w", err)
		}
		if err := checkCode(serverAddress.Code, serverAddress.Message, "GetPeGameServerAddress"); err != nil {
			return result, err
		}
		ipAddress = fmt.Sprintf("%s:%d", serverAddress.Entity.IP, serverAddress.Entity.Port.Int64())

		authv2Data, err := cli.GenerateNetworkGameAuthV2(after, p.ClientPublicKey)
		if err != nil {
			return result, fmt.Errorf("GenerateNetworkGameAuthV2: %w", err)
		}
		chainInfoStr, err = getChainInfo(cli, authv2Data)
		if err != nil {
			return result, err
		}
	} else if p.ServerCode == "MainCity" {
		_ = cli.LeaveEnteredGame()
		_, _ = cli.LeaveMainCity()
		mainCity, err := cli.EnterMainCity()
		if err != nil {
			return result, fmt.Errorf("EnterMainCity: %w", err)
		}
		if err := checkCode(mainCity.Code, mainCity.Message, "EnterMainCity"); err != nil {
			return result, err
		}
		ipAddress = fmt.Sprintf("%s:%d", mainCity.Entity.ServerHost, mainCity.Entity.ServerPort)
		authv2Data, err := cli.GenerateLobbyGameAuthV2(fmt.Sprintf("%d", mainCity.Entity.CityNo), p.ClientPublicKey)
		if err != nil {
			return result, fmt.Errorf("GenerateLobbyGameAuthV2: %w", err)
		}
		chainInfoStr, err = getChainInfo(cli, authv2Data)
		if err != nil {
			return result, err
		}
	} else if after, ok := strings.CutPrefix(p.ServerCode, "DomainGame:"); ok && after != "" {
		ipAddress, chainInfoStr, err = handleDomainGame(ctx, cli, after, p.ClientPublicKey, false)
	} else if after, ok := strings.CutPrefix(p.ServerCode, "PCDomainGame:"); ok && after != "" {
		ipAddress, chainInfoStr, err = handleDomainGame(ctx, cli, after, p.ClientPublicKey, true)
		result.IsPC = true
	} else {
		ipAddress, chainInfoStr, err = handleRentalGame(ctx, cli, p.ServerCode, p.ServerPassword, p.ClientPublicKey)
	}

	if err != nil {
		return result, err
	}

	result.UID = cli.UserID
	result.EntityID = cli.UserDetail.EntityID
	result.MasterName = cli.UserDetail.Name
	if result.MasterName == "" {
		result.MasterName = cli.UserID
	}
	result.ChainInfo = chainInfoStr
	result.IP = ipAddress
	result.BotLevel = int(cli.UserDetail.Level.Int64())
	result.EngineVersion = cli.EngineVersion
	result.PatchVersion = cli.G79LatestVersion

	return result, nil
}
