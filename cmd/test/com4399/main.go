package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Yeah114/FunAuth/internal/db"
	com4399tools "github.com/Yeah114/FunAuth/utils/com4399"
)

func main() {
	if err := db.InitDB(); err != nil {
		panic(fmt.Errorf("init db: %w", err))
	}

	cookie, err := com4399tools.NewCom4399CookieRegister().
		WithContext(context.Background()).
		WithTimeout(5 * time.Second).
		WithAutoCaptcha(true).
		WithAutoProxy(true).
		WithGettingProxyCallback(func() { fmt.Println("正在获取代理") }).
		WithGotProxyCallback(func(proxy string) { fmt.Printf("已获取代理: %s\n", proxy) }).
		WithGeneratedAccountCallback(func(username, password string) {
			fmt.Printf("已生成 4399 账号: username=%s password=%s\n", username, password)
		}).
		WithGettingRealNameCallback(func() { fmt.Println("正在从数据库获取实名信息") }).
		WithGotRealNameCallback(func(realName, idCard string) {
			fmt.Printf("已获取实名信息: name=%s id=%s\n", realName, idCard)
		}).
		WithRegisteringCallback(func(username string) { fmt.Printf("正在注册 4399 账号: %s\n", username) }).
		WithRegisteredCallback(func(uid, username string) {
			if uid == "" {
				fmt.Printf("4399 账号注册成功: %s\n", username)
				return
			}
			fmt.Printf("4399 账号注册成功: %s (uid=%s)\n", username, uid)
		}).
		WithLoggingInCallback(func(username string) { fmt.Printf("正在登录 4399 账号并换取 Cookie: %s\n", username) }).
		WithLoggedInCallback(func() { fmt.Println("4399 账号登录成功") }).
		WithGotCookieCallback(func(cookieLength int) { fmt.Printf("已生成 4399 Cookie: length=%d\n", cookieLength) }).
		WithErrorCallback(func(stage string, err error) { fmt.Printf("%s失败: %v\n", stage, err) }).
		RegisterCookie()
	if err != nil {
		panic(err)
	}

	fmt.Println("\n========== Cookie ==========")
	fmt.Println(cookie)
}
