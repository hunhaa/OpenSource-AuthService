package main

import (
	"fmt"
	"log"

	"github.com/Yeah114/g79client"
)

func main() {
	// 替换为你的 Cookie（与 example/main.go 相同来源）
	cookie := `{"sauth_json":"<REPLACE_WITH_YOUR_COOKIE>"}`

	client, err := g79client.NewClient()
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}

	// 认证
	if err := client.X19AuthenticateWithCookie(cookie); err != nil {
		log.Fatalf("认证失败: %v", err)
	}

	// 进入在线大厅房间
	// 注意：示例 roomID 为占位符，请替换为有效房间ID；password 可为空字符串
	roomID := "4683186036680040534"
	password := ""

	// 先查询房间详情
	roomInfo, err := client.GetOnlineLobbyRoom(roomID)
	if err != nil {
		log.Fatalf("查询房间失败: %v", err)
	}
	if roomInfo.Code != 0 {
		log.Fatalf("查询房间失败: %s", roomInfo.Message)
	}
	fmt.Println(roomInfo.Entity.ResID.String())
	fmt.Println("查询房间成功")

	/*
	// 购买房间地图
	roomMap, err := client.PurchaseItem(roomInfo.Entity.ResID.String())
	if err != nil {
		log.Fatalf("购买房间地图失败: %v", err)
	}
	if roomMap.Code != 0 && roomMap.Code != 502 && roomMap.Code != 44 {
		log.Fatalf("购买房间地图失败(%d): %s", roomMap.Code, roomMap.Message)
	}
		*/
	buyResult, err := client.UserItemPurchase(roomInfo.Entity.ResID.String())
	if err != nil {
		log.Fatalf("获取购买结果失败: %v", err)
	}
	fmt.Println(buyResult)
	fmt.Println("购买房间地图成功")

	resp, err := client.EnterOnlineLobbyRoom(roomID, password)
	if err != nil {
		log.Fatalf("请求失败: %v", err)
	}

	if resp.Code != 0 {
		log.Fatalf("进入失败: %s", resp.Message)
	}

	fmt.Printf("进入成功，room_id: %s\n", resp.Entity.RoomID.String())

	// 进入在线大厅游戏
	gameEnter, err := client.OnlineLobbyGameEnter()
	if err != nil {
		log.Fatalf("进入在线大厅游戏失败: %v", err)
	}
	if gameEnter.Code != 0 {
		log.Fatalf("进入在线大厅游戏失败(%d): %s", gameEnter.Code, gameEnter.Message)
	}
	fmt.Printf("在线大厅游戏连接地址: %v:%v\n", gameEnter.Entity.ServerHost, gameEnter.Entity.ServerPort)

	// 生成 LobbyGame AuthV2（clientKey 需从游戏握手获取，这里仅示例）
	// 注意：下面 clientKey 为占位符
	lobbyClientKey := "MHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEzmz6+EK8UC40g5XsqoAjqURAKP6uCAMmXJeEyzR/8BkZ1vVXpFTMF/AmBl3Tf+gvDFPJkT9Bm3bAO0IeXo+ssMOsJX4NFPLM4+YEohwJrJyRaMptmh1nvWue4J5+vbZW"
	authv2Lobby, err := client.GeneratePCLobbyGameAuthV2("4641943113293411081", lobbyClientKey)
	if err != nil {
		log.Fatalf("生成 LobbyGame AuthV2 失败: %v", err)
	}
	chainLobby, err := client.SendAuthV2Request(authv2Lobby)
	if err != nil {
		log.Fatalf("发送 LobbyGame AuthV2 失败: %v", err)
	}
	fmt.Printf("LobbyGame 认证链: %s\n", string(chainLobby))
}
