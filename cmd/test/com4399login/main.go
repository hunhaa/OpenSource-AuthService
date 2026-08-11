package main

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/Yeah114/FunAuth/internal/db"
	com4399tools "github.com/Yeah114/FunAuth/utils/com4399"
	httpproxy "github.com/Yeah114/FunAuth/utils/proxy"
	"github.com/Yeah114/g79client"
	x19sdk "github.com/Yeah114/g79client/account/x19/sdk"
)

var nicknameRandom = rand.New(rand.NewSource(time.Now().UnixNano()))

const (
	maxCookieRegisterAttempts = 5
	defaultCheckEnterRoleID   = ""
	defaultCheckEnterHostID   = 8000
)

func main() {
	if err := db.InitDB(); err != nil {
		panic(fmt.Errorf("init db: %w", err))
	}

	fmt.Println("正在注册 4399 账号并登录 G79...")

	cookie, err := registerUsableCom4399Cookie(context.Background())
	if err != nil {
		panic(fmt.Errorf("register cookie: %w", err))
	}

	fmt.Println("\n正在用 Cookie 登录 G79...")
	client, err := loginG79(context.Background(), cookie)
	if err != nil {
		panic(fmt.Errorf("login g79: %w", err))
	}

	fmt.Println("\n========== G79 机器人信息 ==========")
	fmt.Printf("用户ID:   %s\n", client.UserID)
	fmt.Printf("用户Token: %s\n", client.UserToken)
	if err := ensureNickname(client); err != nil {
		fmt.Printf("设置昵称失败: %v\n", err)
	}
	if client.UserDetail != nil {
		fmt.Printf("用户名:   %s\n", client.UserDetail.Name)
		fmt.Printf("等级:     %v\n", client.UserDetail.Level)
		fmt.Printf("性别:     %s\n", client.UserDetail.Gender)
		fmt.Printf("签名:     %s\n", client.UserDetail.Signature)
		fmt.Printf("账号:     %s\n", client.UserDetail.Account)
	}
	fmt.Printf("IsX19:    %v\n", client.IsX19)
	fmt.Println("====================================")

	fmt.Print("\n按回车键退出...")
	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadString('\n')
}

func registerCom4399Cookie(ctx context.Context) (string, error) {
	return com4399tools.NewCom4399CookieRegister().
		WithContext(ctx).
		WithAutoCaptcha(true).
		WithAutoProxy(true).
		WithGettingProxyCallback(func() { fmt.Println("正在获取代理") }).
		WithGotProxyCallback(func(proxy string) { fmt.Printf("已获取代理: %s\n", proxy) }).
		WithGeneratedAccountCallback(func(username, password string) {
			fmt.Printf("已生成 4399 账号: username=%s password=%s\n", username, password)
		}).
		WithGettingRealNameCallback(func() { fmt.Println("正在从数据库获取实名信息") }).
		WithGotRealNameCallback(func(realName, idCard string) {
			fmt.Println("已获取实名信息")
		}).
		WithRegisteringCallback(func(username string) { fmt.Printf("正在注册 4399 账号: %s\n", username) }).
		WithRegisteredCallback(func(uid, username string) {
			if uid == "" {
				fmt.Printf("4399 账号注册成功: %s\n", username)
				return
			}
			fmt.Printf("4399 账号注册成功: %s (uid=%s)\n", username, uid)
		}).
		WithLoggingInCallback(func(username string) {
			fmt.Printf("正在登录 4399 账号并换取 Cookie: %s\n", username)
		}).
		WithLoggedInCallback(func() { fmt.Println("4399 账号登录成功") }).
		WithGotCookieCallback(func(cookieLength int) { fmt.Printf("已生成 4399 Cookie: length=%d\n", cookieLength) }).
		WithErrorCallback(func(stage string, err error) { fmt.Printf("%s失败: %v\n", stage, err) }).
		RegisterCookie()
}

func registerUsableCom4399Cookie(ctx context.Context) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= maxCookieRegisterAttempts; attempt++ {
		cookie, err := registerCom4399Cookie(ctx)
		if err != nil {
			lastErr = err
			continue
		}

		if err := checkCom4399CookieUsable(cookie); err != nil {
			lastErr = err
			fmt.Printf("4399 Cookie precheck failed, retrying registration (%d/%d): %v\n", attempt, maxCookieRegisterAttempts, err)
			continue
		}

		return cookie, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unknown error")
	}
	return "", fmt.Errorf("no usable 4399 cookie after %d attempts: %w", maxCookieRegisterAttempts, lastErr)
}

func checkCom4399CookieUsable(cookie string) error {
	client := x19sdk.NewClient(httpproxy.NewLoginHTTPClient(45 * time.Second))
	result, err := client.CheckEnterWithCookie(cookie, &x19sdk.CookieCheckEnterOptions{
		RoleID: defaultCheckEnterRoleID,
		HostID: defaultCheckEnterHostID,
	})
	if err != nil {
		return err
	}
	if result == nil || result.UniSauth == nil {
		return fmt.Errorf("uni_sauth response is nil")
	}
	if result.UniSauth.Code != 200 || result.UniSauth.Subcode != 0 {
		return fmt.Errorf(
			"uni_sauth failed: code=%d subcode=%d msg=%s",
			result.UniSauth.Code,
			result.UniSauth.Subcode,
			strings.TrimSpace(result.UniSauth.Msg),
		)
	}
	return nil
}

func loginG79(ctx context.Context, cookie string) (*g79client.Client, error) {
	client, err := g79client.NewClient()
	if err != nil {
		return nil, fmt.Errorf("new g79 client: %w", err)
	}

	x19Err := client.X19AuthenticateWithCookie(cookie)
	if x19Err == nil {
		detail, err := client.GetUserDetail()
		if err == nil {
			client.UserDetail = &detail.Entity
		}
		return client, nil
	}

	err = client.G79AuthenticateWithCookie(cookie)
	if err != nil {
		return nil, fmt.Errorf("g79 auth failed: x19_err=%v, g79_err=%w", x19Err, err)
	}

	detail, err := client.GetUserDetail()
	if err == nil {
		client.UserDetail = &detail.Entity
	}
	return client, nil
}

func ensureNickname(cli *g79client.Client) error {
	if cli == nil {
		return fmt.Errorf("g79 client is nil")
	}
	if cli.UserDetail == nil {
		detail, err := cli.GetUserDetail()
		if err != nil {
			return fmt.Errorf("GetUserDetail: %w", err)
		}
		cli.UserDetail = &detail.Entity
	}
	if cli.UserDetail != nil && strings.TrimSpace(cli.UserDetail.Name) != "" {
		return nil
	}

	var lastErr error
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("HZ%08d", nicknameRandom.Intn(100000000))
		if err := cli.UpdateNickname(name); err != nil {
			lastErr = err
			time.Sleep(time.Duration(200+i*100) * time.Millisecond)
			continue
		}
		if cli.UserDetail != nil {
			cli.UserDetail.Name = name
		}
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unknown error")
	}
	return fmt.Errorf("UpdateNickname: auto retry failed: %w", lastErr)
}
