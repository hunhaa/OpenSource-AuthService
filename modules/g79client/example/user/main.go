package main

import (
	"fmt"
	"log"

	"github.com/Yeah114/g79client"
	"github.com/Yeah114/g79client/example/login"
)

func main() {
	client, err := login.Login()
	if err != nil {
		log.Fatalf("登录失败: %v", err)
	}

	resp, err := client.GetOtherUserSettingList(g79client.GetOtherUserSettingListRequest{
		WithPetInfo: true,
		UserID:      "2233961007",
	})
	if err != nil {
		log.Fatalf("请求失败: %v", err)
	}

	// 简要打印
	fmt.Printf("返回: %v\n", resp)

	resp2, err := client.GetDownloadInfo("4680532973461110889")
	if err != nil {
		log.Fatalf("请求失败: %v", err)
	}

	// 简要打印
	fmt.Printf("返回: %v\n", resp2)
}
