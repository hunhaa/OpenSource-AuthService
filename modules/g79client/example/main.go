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
	// 初始化随机数种子
	rand.Seed(time.Now().UnixNano())

	fmt.Println("启动 G79 客户端...")

	// Debug: 打印 patch version 获取的数据（已禁用动态获取，使用固定值）
	fmt.Println("\n=== Debug: Patch Version 数据 ===")
	patchMeta, err := g79client.GetGlobalG79PatchMetadata()
	if err != nil {
		log.Printf("获取 patch metadata 失败: %v", err)
	} else {
		fmt.Printf("Patch Version: %s\n", patchMeta.Version)
		fmt.Printf("Resources Hash: %s\n", patchMeta.ResourcesHash)
	}

	// Debug: 打印 G79 Release JSON
	fmt.Println("\n=== Debug: G79 Release JSON ===")
	releaseJSON, err := g79client.GetGlobalG79ReleaseJSON()
	if err != nil {
		log.Printf("获取 G79 Release JSON 失败: %v", err)
	} else {
		data, _ := json.MarshalIndent(releaseJSON, "", "  ")
		fmt.Printf("%s\n", data)
	}

	// Debug: 打印 X19 Release JSON
	fmt.Println("\n=== Debug: X19 Release JSON ===")
	x19JSON, err := g79client.GetGlobalX19ReleaseJSON()
	if err != nil {
		log.Printf("获取 X19 Release JSON 失败: %v", err)
	} else {
		data, _ := json.MarshalIndent(x19JSON, "", "  ")
		fmt.Printf("%s\n", data)
	}

	fmt.Println("\n=== 开始登录测试 ===")

	// Cookie字符串
	cookie := `{"sauth_json":"{\"aim_info\":\"{\\\"aim\\\":\\\"127.0.0.1\\\",\\\"country\\\":\\\"CN\\\",\\\"tz\\\":\\\"+0800\\\",\\\"tzid\\\":\\\"\\\"}\",\"realname\":\"{\\\"realname_type\\\":2}\",\"gameid\":\"x19\",\"app_channel\":\"4399com\",\"login_channel\":\"4399com\",\"platform\":\"ad\",\"sdk_version\":\"3.12.2\",\"sdkuid\":\"1238535092\",\"sessionid\":\"1238535092|32a562a04cd0cf89402565990aedf661|44770||1392aa39257d5b57c1e5ecabcef6ff5e|74ce2d2dc7614da129cc4247cddc8ae3|1786809861|4399\",\"udid\":\"ee8829520004c055\",\"client_login_sn\":\"243572b925c86dc6\",\"deviceid\":\"b77333e5de1cccaa\"}"}`

	client, err := g79client.NewClient()
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}

	// 执行认证
	fmt.Println("开始认证...")
	err = client.G79AuthenticateWithCookie(cookie)
	if err != nil {
		log.Fatalf("认证失败: %v", err)
	}
	fmt.Printf("认证成功，用户ID: %s\n", client.UserID)

	// 获取用户详情
	fmt.Println("获取用户信息...")
	username := client.UserDetail.Name
	if username == "" {
		// 生成随机用户名
		name := fmt.Sprintf("HZ%08d", rand.Intn(100000000))
		err = client.UpdateNickname(name)
		if err != nil {
			log.Fatalf("修改名字失败: %v", err)
		}
		username = name
	}

	growthLevel := client.UserDetail.Level
	fmt.Printf("用户名: %s\n", username)
	fmt.Printf("等级: %v\n", growthLevel)

	// 搜索租赁服
	fmt.Println("搜索租赁服...")
	searchResp, err := client.SearchRentalServerByName("48285363")
	if err != nil {
		log.Fatalf("搜索租赁服失败: %v", err)
	}

	if searchResp.Code != 0 || len(searchResp.Entities) == 0 {
		log.Fatalf("获取租赁服信息失败")
	}

	serverID := searchResp.Entities[0].EntityID

	// 进入租赁服世界
	fmt.Println("进入租赁服...")
	enterResp, err := client.EnterRentalServerWorld(serverID.String(), "123456")
	if err != nil {
		log.Fatalf("进入租赁服失败: %v", err)
	}

	if enterResp.Code != 0 {
		log.Fatalf("获取租赁服地址失败: %s", enterResp.Message)
	}

	ipAddress := fmt.Sprintf("%s:%v", enterResp.Entity.McserverHost, enterResp.Entity.McserverPort)
	fmt.Printf("服务器地址: %s\n", ipAddress)

	// 生成认证v2数据
	fmt.Println("生成认证信息...")
	authv2Data, err := client.GenerateRentalGameAuthV2(serverID.String(), "MHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEzmz6+EK8UC40g5XsqoAjqURAKP6uCAMmXJeEyzR/8BkZ1vVXpFTMF/AmBl3Tf+gvDFPJkT9Bm3bAO0IeXo+ssMOsJX4NFPLM4+YEohwJrJyRaMptmh1nvWue4J5+vbZW")
	if err != nil {
		log.Fatalf("生成认证v2数据失败: %v", err)
	}

	// 发送认证v2请求
	chainInfo, err := client.SendAuthV2Request(authv2Data)
	if err != nil {
		log.Fatalf("发送认证v2请求失败: %v", err)
	}

	fmt.Printf("认证链信息: %s\n", string(chainInfo))
	fmt.Println("运行完成!")
}
