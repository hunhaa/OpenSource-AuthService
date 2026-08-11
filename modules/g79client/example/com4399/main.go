package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Yeah114/g79client/account/com4399"
)

func main() {
	reader := bufio.NewReader(os.Stdin)

	username := strings.TrimSpace(os.Getenv("G79_4399_USERNAME"))
	password := os.Getenv("G79_4399_PASSWORD")
	if username == "" {
		fmt.Print("请输入 4399 用户名: ")
		value, _ := reader.ReadString('\n')
		username = strings.TrimSpace(value)
	}
	if password == "" {
		fmt.Print("请输入 4399 密码: ")
		value, _ := reader.ReadString('\n')
		password = strings.TrimSpace(value)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cookie, err := com4399.RegisterCookieWithPassword(ctx, username, password)
	if err != nil {
		var accountErr *com4399.AccountNotFoundError
		if errors.As(err, &accountErr) {
			fmt.Printf("4399 账号不存在或尚未注册: %s\n", username)
			return
		}
		var captchaErr *com4399.NeedCaptchaError
		if errors.As(err, &captchaErr) {
			fmt.Printf("4399 登录需要验证码: %s\n", captchaErr.Reason)
			if captchaErr.CaptchaURL != "" {
				fmt.Printf("验证码地址: %s\n", captchaErr.CaptchaURL)
			}
			if captchaErr.CaptchaID != "" {
				fmt.Printf("CaptchaID: %s\n", captchaErr.CaptchaID)
			}
			return
		}
		log.Fatalf("4399 登录失败: %v", err)
	}

	fmt.Println("4399 Cookie:")
	fmt.Println(cookie)
}
