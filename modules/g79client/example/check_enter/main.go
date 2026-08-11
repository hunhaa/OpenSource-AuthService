package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	x19sdk "github.com/Yeah114/g79client/account/x19/sdk"
	examplecookie "github.com/Yeah114/g79client/example/cookie"
)

func main() {
	cookie := examplecookie.Value()
	sdkClient := x19sdk.NewClient(nil)
	roleID := os.Getenv("G79_ROLE_ID")
	hostID, err := parseHostID(os.Getenv("G79_HOST_ID"))
	if err != nil {
		log.Fatalf("解析 G79_HOST_ID 失败: %v", err)
	}

	result, err := sdkClient.CheckEnterWithCookie(cookie, &x19sdk.CookieCheckEnterOptions{
		RoleID: roleID,
		HostID: hostID,
	})
	if err != nil {
		log.Fatalf("执行 cookie 检测失败: %v", err)
	}

	uniResp := result.UniSauth
	if uniResp == nil {
		log.Fatal("uni_sauth 响应为空")
	}
	fmt.Printf("uni_sauth: code=%d subcode=%d status=%s aid=%d sdkuid=%s\n", uniResp.Code, uniResp.Subcode, uniResp.Status, uniResp.AID, uniResp.SDKUID)
	if uniResp.Msg != "" {
		fmt.Printf("uni_sauth msg: %s\n", uniResp.Msg)
	}

	if uniResp.Code != 200 || uniResp.Subcode != 0 {
		fmt.Println("cookie 未通过 uni_sauth 校验，无法继续 check_enter")
		return
	}

	if result.DecodedUniSDKLoginPayload == nil {
		log.Fatal("uni_sauth 成功，但未返回可解码的 unisdk_login_json")
	}

	fmt.Printf("unisdk_login_json: username=%s realname_status=%s is_adult=%s\n",
		result.DecodedUniSDKLoginPayload.Username,
		result.DecodedUniSDKLoginPayload.RealnameMsg.RealnameStatus,
		result.DecodedUniSDKLoginPayload.RealnameMsg.IsAdult,
	)

	if roleID == "" {
		fmt.Println("cookie 已通过 uni_sauth 校验；如需继续检测 check_enter，请设置环境变量 G79_ROLE_ID")
		return
	}
	if result.CheckEnter == nil {
		log.Fatal("roleID 已提供，但 check_enter 响应为空")
	}

	checkResp := result.CheckEnter
	fmt.Printf("check_enter: code=%d subcode=%d status=%s rm=%s\n", checkResp.Code, checkResp.Subcode, checkResp.Status, checkResp.RM)
	if checkResp.Msg != "" {
		fmt.Printf("check_enter msg: %s\n", checkResp.Msg)
	}
}

func parseHostID(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}

	hostID, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	if hostID <= 0 {
		return 0, fmt.Errorf("hostid 必须大于 0")
	}
	return hostID, nil
}
