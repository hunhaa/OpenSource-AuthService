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
	fmt.Printf("登录成功, 用户ID: %v, 昵称: %s\n", client.UserID, client.UserDetail.Name)
	// 搜索在线大厅房间
	keyword := "86020"
	searchResp, err := client.SearchOnlineLobbyRoomByKeyword(keyword, 10, 0)
	if err != nil {
		log.Fatalf("搜索在线大厅房间失败: %v", err)
	}
	if searchResp.Code != 0 {
		log.Fatalf("搜索在线大厅房间失败(%d): %s", searchResp.Code, searchResp.Message)
	}
	fmt.Printf("搜索在线大厅房间成功, 共找到 %d 个房间:\n", searchResp.Total.Int64())
	for _, room := range searchResp.Entities {
		fmt.Printf("- 房间: %v\n", room)
	}
}
