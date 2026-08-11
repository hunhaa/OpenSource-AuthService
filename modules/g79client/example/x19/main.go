package main

import (
	"log"

	"github.com/Yeah114/g79client"
)

func main() {
	client, err := g79client.NewClient()
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}

	cookie := `{"sauth_json":"<REPLACE_WITH_YOUR_COOKIE>"}`

	if err := client.X19AuthenticateWithCookie(cookie); err != nil {
		log.Fatalf("X19 登录失败: %v", err)
	}
	// 打印信息
	log.Printf("登录成功, 用户ID: %v, 昵称: %s", client.UserID, client.UserDetail.Name)
}
