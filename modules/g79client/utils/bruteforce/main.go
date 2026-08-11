package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/Yeah114/g79client"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	cookie := `{"sauth_json":"{\"aim_info\":\"{\\\"aim\\\":\\\"127.0.0.1\\\",\\\"country\\\":\\\"CN\\\",\\\"tz\\\":\\\"+0800\\\",\\\"tzid\\\":\\\"\\\"}\",\"app_channel\":\"netease\",\"client_login_sn\":\"DE410238EC7305D346806ED69947FB2C\",\"deviceid\":\"amawtpyaaxnskag5-d\",\"gameid\":\"x19\",\"gas_token\":\"\",\"get_access_token\":\"1\",\"ip\":\"127.0.0.1\",\"is_unisdk_guest\":1,\"login_channel\":\"netease\",\"platform\":\"pc\",\"sdk_version\":\"3.9.0\",\"sdkuid\":\"aibgtp5bu2mr4z5i\",\"sessionid\":\"1-eyJzaSI6ICI4NzVjMjliYjIzY2Q1Y2NjZTEwZTc1OWFlMzhmYWQyNjc5NDE3Njk3IiwgIm9kaSI6ICJhbWF3dHB5YWF4bnNrYWc1LWQiLCAicyI6ICJxMXM1dGZ6Z2w0a3A2N3ltaHQ0dDFybmpod3FsZzRnMSIsICJ1IjogImFpYmd0cDVidTJtcjR6NWkiLCAidCI6IDIsICJwcnMiOiAxMjgsICJnX2kiOiAiYWVjZnJ4b2R5cWFhYWFqcCIsICJzYSI6IC05OX0g\",\"source_app_channel\":\"netease\",\"source_platform\":\"pc\",\"udid\":\"k1h051bjq07vjwt8uovg6zlm18tjfpbe\"}"}`

	client, err := g79client.NewClient()
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}

	fmt.Printf("引擎版本: %s\n", client.EngineVersion)
	fmt.Printf("补丁版本: %s\n", client.G79LatestVersion)
	fmt.Println("\n开始穷举 sp 和 tr...")

	success := false
	for sp := 0; sp <= 4 && !success; sp++ {
		for tr := 1; tr <= 20 && !success; tr++ {
			fmt.Printf("测试 sp=%d, tr=%d ... ", sp, tr)
			err := client.G79AuthenticateWithCookieTest(cookie, sp, tr)
			if err == nil {
				fmt.Printf("✅ 成功！\n")
				fmt.Printf("认证成功，用户ID: %s\n", client.UserID)
				success = true

				jsonBytes, _ := json.MarshalIndent(map[string]int{"sp": sp, "tr": tr}, "", "  ")
				fmt.Printf("正确参数: %s\n", string(jsonBytes))
				break
			} else {
				fmt.Printf("❌ %v\n", err)
			}
			time.Sleep(time.Duration(3+rand.Intn(4)) * time.Second)
		}
	}

	if !success {
		fmt.Println("\n未找到正确的 sp/tr 组合，尝试更大范围的 tr...")
		for sp := 0; sp <= 4 && !success; sp++ {
			for tr := 21; tr <= 20 && !success; tr++ {
				fmt.Printf("测试 sp=%d, tr=%d ... ", sp, tr)
				err := client.G79AuthenticateWithCookieTest(cookie, sp, tr)
				if err == nil {
					fmt.Printf("✅ 成功！\n")
					fmt.Printf("认证成功，用户ID: %s\n", client.UserID)
					success = true

					jsonBytes, _ := json.MarshalIndent(map[string]int{"sp": sp, "tr": tr}, "", "  ")
					fmt.Printf("正确参数: %s\n", string(jsonBytes))
					break
				} else {
					fmt.Printf("❌ %v\n", err)
				}
				time.Sleep(time.Duration(1+rand.Intn(2)) * time.Second)
			}
		}
	}

	if !success {
		fmt.Println("\n穷举完成，未找到正确的组合")
	}
}
