package main

import (
	"fmt"
	"log"

	"github.com/Yeah114/g79client"
)

func main() {
	// 替换为你的 Cookie（与 example/main.go 相同来源）
	cookie := `{"emulator":1,"is_guest":false,"mac_addr":"50ff4536ae37361c7efdfa72516df1bf","ram":"1035337728","rom":"134208294912","sauth_json":"{\"aim_info\":\"{\\\"aim\\\":\\\"127.0.0.1\\\",\\\"country\\\":\\\"CN\\\",\\\"tz\\\":\\\"+0800\\\",\\\"tzid\\\":\\\"Asia/Shanghai\\\",\\\"celluar_ip\\\":\\\"\\\",\\\"operator\\\":\\\"\\\",\\\"is_vpn_enabled\\\":false}\",\"app_channel\":\"4399com\",\"client_login_sn\":\"49d1ce7208c345bda1f1e2ce40007fa6\",\"deviceid\":\"49d1ce7208c345bda1f1e2ce40007fa6\",\"gameid\":\"x19\",\"gas_token\":\"\",\"get_access_token\":\"1\",\"ip\":\"127.0.0.1\",\"is_unisdk_guest\":0,\"login_channel\":\"4399com\",\"platform\":\"ad\",\"realname\":\"{\\\"realname_type\\\":\\\"0\\\"}\",\"sdk_version\":\"1.0.0\",\"sdkuid\":\"1335815995\",\"sessionid\":\"1335815995|201c649e95feafcc4635e0ac1e79e7ad|44770||827707f9e58ab1c5a7cecc0ed1793028|a66a895be6f51e7ddfa7f6fb93f0297d|1780215120|4399\",\"source_app_channel\":\"4399com\",\"source_platform\":\"ad\",\"udid\":\"fc0b24ebaccd84e5\"}"}`

	client, err := g79client.NewClient()
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}

	// 认证
	if err := client.G79AuthenticateWithCookie(cookie); err != nil {
		log.Fatalf("认证失败: %v", err)
	}

	inviteCode := "E41666F00E8"
	_ = inviteCode
	resp, _ := client.GetOtherDomainServers()
	for _, server := range resp.Entities {
		_, _ = client.DeleteOtherDomainServer(server.Sid)
	}
	inviteResp, _ := client.JoinDomainServerWithInviteCode(inviteCode)
	if inviteResp.Code != 0 {
		panic(fmt.Errorf("进入失败 因为: %s", inviteResp.Message))
	}
	serversResp, _ := client.GetOtherDomainServers()
	serverID := serversResp.Entities[0].Sid
	fmt.Println(serverID)
	/*
	enterResp, _ := client.RequestEnterDomainServer(serverID)
	address := fmt.Sprintf("%s:%d", enterResp.Entity.ServerHost, enterResp.Entity.ServerPort.Int64())
	fmt.Println(address)
	*/
	/*
	authv2, err := client.GenerateDomainGameAuthV2(serverID, "MHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEzmz6+EK8UC40g5XsqoAjqURAKP6uCAMmXJeEyzR/8BkZ1vVXpFTMF/AmBl3Tf+gvDFPJkT9Bm3bAO0IeXo+ssMOsJX4NFPLM4+YEohwJrJyRaMptmh1nvWue4J5+vbZW")
	fmt.Println(string(authv2))
	chainInfo, err := client.SendAuthV2Request(authv2)
	if err != nil {
		log.Fatalf("发送认证v2请求失败: %v", err)
	}
	fmt.Println(string(chainInfo))
	*/

	voiceResp, err := client.GetCCVoiceLoginInfo(serverID)
	if err != nil {
		log.Fatalf("获取语音登录信息失败: %v", err)
	}
	if voiceResp.Code != 0 {
		log.Fatalf("获取语音登录信息失败: code=%d message=%q", voiceResp.Code, voiceResp.Message)
	}
	fmt.Println(voiceResp)
}
