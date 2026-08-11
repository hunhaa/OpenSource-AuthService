package main

import (
	"fmt"
	"log"

	"github.com/Yeah114/g79client"
)

func main() {
	// 替换为你的 Cookie（与 example/main.go 相同来源）
	cookie := `{"emulator":1,"is_guest":false,"mac_addr":"27d0125bb34ba58f373c8c29fd280f9f","ram":"1035337728","rom":"134208294912","sauth_json":"{\"aim_info\":\"{\\\"aim\\\":\\\"127.0.0.1\\\",\\\"country\\\":\\\"CN\\\",\\\"tz\\\":\\\"+0800\\\",\\\"tzid\\\":\\\"Asia/Shanghai\\\",\\\"celluar_ip\\\":\\\"\\\",\\\"operator\\\":\\\"\\\",\\\"is_vpn_enabled\\\":false}\",\"app_channel\":\"4399com\",\"client_login_sn\":\"65df7d9bfaee4dddad28185bfdcbf599\",\"deviceid\":\"65df7d9bfaee4dddad28185bfdcbf599\",\"gameid\":\"x19\",\"gas_token\":\"\",\"get_access_token\":\"1\",\"ip\":\"127.0.0.1\",\"is_unisdk_guest\":0,\"login_channel\":\"4399com\",\"platform\":\"ad\",\"realname\":\"{\\\"realname_type\\\":\\\"0\\\"}\",\"sdk_version\":\"1.0.0\",\"sdkuid\":\"1389518990\",\"sessionid\":\"1389518990|f4f4c881a2f5a6a091d4adf34426018f|44770||a7dcf618cec58daa4ff2c2dd9fd26b31|96965144e0a9098206bd311349c6c617|1786591877|4399\",\"source_app_channel\":\"4399com\",\"source_platform\":\"ad\",\"udid\":\"f067e0d5057cba2b\"}"}`

	client, err := g79client.NewClient()
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}

	// 认证
	if err := client.G79AuthenticateWithCookie(cookie); err != nil {
		log.Fatalf("认证失败: %v", err)
	}

	inviteCode := "8927C164249"
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
	enterResp, _ := client.RequestEnterDomainServer(serverID)
	address := fmt.Sprintf("%s:%d", enterResp.Entity.ServerHost, enterResp.Entity.ServerPort.Int64())
	fmt.Println(address)
	authv2, err := client.GenerateDomainGameAuthV2(serverID, "MHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEzmz6+EK8UC40g5XsqoAjqURAKP6uCAMmXJeEyzR/8BkZ1vVXpFTMF/AmBl3Tf+gvDFPJkT9Bm3bAO0IeXo+ssMOsJX4NFPLM4+YEohwJrJyRaMptmh1nvWue4J5+vbZW")
	fmt.Println(string(authv2))
	chainInfo, err := client.SendAuthV2Request(authv2)
	if err != nil {
		log.Fatalf("发送认证v2请求失败: %v", err)
	}
	fmt.Println(string(chainInfo))
}
