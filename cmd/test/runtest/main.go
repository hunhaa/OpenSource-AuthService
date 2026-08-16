package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/Yeah114/g79client"
	linkconnection "github.com/Yeah114/g79client/service/link_connection"
)

const clientPublicKey = "MHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEzmz6+EK8UC40g5XsqoAjqURAKP6uCAMmXJeEyzR/8BkZ1vVXpFTMF/AmBl3Tf+gvDFPJkT9Bm3bAO0IeXo+ssMOsJX4NFPLM4+YEohwJrJyRaMptmh1nvWue4J5+vbZW"

func testG79(name string, sauthJson string, serverCode string, password string) error {
	log.Printf("=== 测试G79: %s ===", name)

	cookieData := map[string]string{
		"sauth_json": sauthJson,
	}
	cookieBytes, _ := json.Marshal(cookieData)
	cookieStr := string(cookieBytes)

	client, err := g79client.NewClient()
	if err != nil {
		return fmt.Errorf("NewClient失败: %v", err)
	}

	log.Printf("当前版本: EngineVersion=%s, PatchVersion=%s", client.EngineVersion, client.G79LatestVersion)

	log.Printf("1. G79认证...")
	if err := client.G79AuthenticateWithCookie(cookieStr); err != nil {
		return fmt.Errorf("G79认证失败: %v", err)
	}
	log.Printf("✅ 认证成功! UserID=%s Name=%s", client.UserID, client.UserDetail.Name)

	log.Printf("2. 建立Link连接并发送GameStart (AuthV2前置步骤)...")
	svc, err := linkconnection.NewLinkConnectionService(client)
	if err != nil {
		return fmt.Errorf("创建Link服务失败: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	conn, err := svc.Dial(ctx)
	if err != nil {
		// 不致命，继续尝试
		log.Printf("⚠️  Link连接失败（继续尝试AuthV2）: %v", err)
	} else {
		defer conn.Close()
		if err := conn.SendGameStart(nil); err != nil {
			log.Printf("⚠️  GameStart失败（继续）: %v", err)
		} else {
			log.Printf("✅ Link+GameStart OK")
		}
	}

	log.Printf("3. 搜索服务器: %s", serverCode)
	searchResp, err := client.SearchRentalServerByName(serverCode)
	if err != nil {
		return fmt.Errorf("搜索服务器失败: %v", err)
	}
	if searchResp.Code != 0 {
		return fmt.Errorf("搜索服务器失败: code=%d, msg=%s", searchResp.Code, searchResp.Message)
	}
	if len(searchResp.Entities) == 0 {
		return fmt.Errorf("找不到服务器: %s", serverCode)
	}

	server := searchResp.Entities[0]
	serverID := server.EntityID
	log.Printf("✅ 找到服务器: ID=%s, Name=%s", serverID.String(), server.Name)

	log.Printf("4. EnterRentalServerWorld...")
	enterResp, err := client.EnterRentalServerWorld(serverID.String(), password)
	if err != nil {
		return fmt.Errorf("进入租赁服失败: %v", err)
	}
	if enterResp.Code != 0 {
		return fmt.Errorf("进入租赁服失败: code=%d, msg=%s", enterResp.Code, enterResp.Message)
	}

	ipAddr := fmt.Sprintf("%s:%v", enterResp.Entity.McserverHost, enterResp.Entity.McserverPort)
	log.Printf("✅ 进服地址: %s", ipAddr)

	log.Printf("5. GenerateRentalGameAuthV2...")
	authv2Data, err := client.GenerateRentalGameAuthV2(serverID.String(), clientPublicKey)
	if err != nil {
		return fmt.Errorf("生成AuthV2失败: %v", err)
	}

	log.Printf("6. SendAuthV2Request...")
	chainInfo, err := client.SendAuthV2Request(authv2Data)
	if err != nil {
		return fmt.Errorf("AuthV2请求失败: %v", err)
	}

	log.Printf("✅✅✅ AuthV2 成功! ChainInfo=%d bytes", len(chainInfo))
	if len(chainInfo) > 150 {
		log.Printf("预览: %s", string(chainInfo)[:150])
	}
	log.Printf("========== 全部成功! MC地址=%s ==========", ipAddr)
	return nil
}

func main() {
	sauth1 := "{\"gameid\": \"x19\", \"login_channel\": \"netease\", \"app_channel\": \"netease\", \"platform\": \"pc\", \"sdkuid\": \"aibgvakgg6ozaucj\", \"sessionid\": \"1-eyJzaSI6ICIwOGFjMzdlYjJjMGZiODRhYTAxYjFhMDgzODExYjc1YTliYTU4N2U2IiwgIm9kaSI6ICJhbWF3dWZ5YWF4dHUzdWZxLWQiLCAicyI6ICI4ZXdlYzR4NTU2Zjc0Y2syd2FubnE5ZmZoaXIzaDRoMSIsICJ1IjogImFpYmd2YWtnZzZvemF1Y2oiLCAidCI6IDIsICJwcnMiOiAxMjgsICJnX2kiOiAiYWVjZnJ4b2R5cWFhYWFqcCIsICJzYSI6IC05OX0g\", \"sdk_version\": \"5.9.0\", \"udid\": \"amawufyaaxtu3ufq-d\", \"deviceid\": \"amawufyaaxtu3ufq-d\", \"aim_info\": \"{\\\"aim\\\": \\\"127.0.0.1\\\", \\\"country\\\": \\\"CN\\\", \\\"tz\\\": \\\"+0800\\\", \\\"tzid\\\": \\\"\\\"}\", \"client_login_sn\": \"1b098d080b7d28b80f27445fa86a5998\", \"gas_token\": \"\", \"source_platform\": \"netease\", \"ip\": \"127.0.0.1\", \"nickname\": \"Nan_4504o\"}"
	sauth2 := "{\"gameid\": \"x19\", \"login_channel\": \"netease\", \"app_channel\": \"netease\", \"platform\": \"pc\", \"sdkuid\": \"aibgvakghcozaucw\", \"sessionid\": \"1-eyJzaSI6ICJjYjhiNTIxNzU1YWRhNjNhMjQ1YWVhZWY0ZjE5MzA3YWI0MTM5N2Q5IiwgIm9kaSI6ICJhbWF3dWZ5YWF4dHUzdWZxLWQiLCAicyI6ICI0cngwejBvYmdoYXgyNHEwZzBxbzRybHg3Z3V0emlzdiIsICJ1IjogImFpYmd2YWtnaGNvemF1Y3ciLCAidCI6IDIsICJwcnMiOiAxMjgsICJnX2kiOiAiYWVjZnJ4b2R5cWFhYWFqcCIsICJzYSI6IC05OX0g\", \"sdk_version\": \"5.9.0\", \"udid\": \"amawufyaaxtu3ufq-d\", \"deviceid\": \"amawufyaaxtu3ufq-d\", \"aim_info\": \"{\\\"aim\\\": \\\"127.0.0.1\\\", \\\"country\\\": \\\"CN\\\", \\\"tz\\\": \\\"+0800\\\", \\\"tzid\\\": \\\"\\\"}\", \"client_login_sn\": \"aab70ad6835997882100fb73eb742dde\", \"gas_token\": \"\", \"source_platform\": \"netease\", \"ip\": \"127.0.0.1\", \"nickname\": \"Nan_5816d\"}"

	serverID := "52258662"
	password := ""

	log.Println("========== G79 3.9 完整进服测试 ==========")

	if err := testG79("Nan_4504o", sauth1, serverID, password); err != nil {
		log.Printf("❌ %v", err)
	}

	log.Println()
	log.Println("========================================")
	log.Println()

	if err := testG79("Nan_5816d", sauth2, serverID, password); err != nil {
		log.Printf("❌ %v", err)
	}
}
