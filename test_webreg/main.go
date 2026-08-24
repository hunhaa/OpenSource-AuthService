package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Yeah114/g79client/account/com4399"
)

func main() {
	fmt.Println("========== WebRegister(OAuth) 流程注册测试 ==========")
	fmt.Printf("时间: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	// 启用 com4399 的详细日志
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// 生成测试账号
	suffix := fmt.Sprintf("%d", time.Now().Unix()%100000)
	username := "testweb" + suffix
	password := "Aa123456"
	// 使用通过 GB/T 2260 校验的 SFZ
	realname := "王芳"
	idcard := "510107199708267511" // 29岁四川成都

	fmt.Printf("测试账号:\n")
	fmt.Printf("  用户名: %s\n", username)
	fmt.Printf("  密码:   %s\n", password)
	fmt.Printf("  姓名:   %s\n", realname)
	fmt.Printf("  SFZ:    %s\n", idcard)
	fmt.Printf("  注意:   Transport=nil 会走 ProxyFromEnvironment（本地隧道 127.0.0.1:18080）\n\n")

	req := com4399.WebRegisterRequest{
		Username: username,
		Password: password,
		RealName: realname,
		IDCard:   idcard,
		// CaptchaCode 留空，内部自动 OCR
		Transport: nil, // 走 ProxyFromEnvironment
	}

	start := time.Now()
	fmt.Println("🚀 开始调用 RegisterWeb (OAuth流程)...")
	fmt.Println("   内部流程: openRegistrationPage -> usernameExists -> submitRegistration")
	fmt.Println("           (含「请稍后再试」5秒间隔 × 6次重试)")
	fmt.Println("           -> submitRealName -> followCallback\n")

	result, err := com4399.RegisterWeb(ctx, req)
	elapsed := time.Since(start)

	fmt.Printf("\n========== 结果 ==========\n")
	fmt.Printf("总耗时: %.1f 秒\n", elapsed.Seconds())
	if err != nil {
		fmt.Printf("❌ 注册失败: %v\n", err)
		// 打印错误类型
		switch {
		case com4399.ErrUsernameExists == err || (err != nil && contains(err.Error(), "已被注册")):
			fmt.Println("   👉 用户名已存在（注册流程通！换用户名即可）")
		case com4399.ErrRealNameRejected == err || contains(err.Error(), "实名") || contains(err.Error(), "身份证"):
			fmt.Println("   👉 实名信息问题（注册流程通！换SFZ即可）")
		case com4399.ErrRiskControlTriggered == err || contains(err.Error(), "请稍后再试") || contains(err.Error(), "风控"):
			fmt.Println("   👉 被风控拦截（需要代理IP轮换 / 增加等待 / 更换SFZ）")
		case com4399.ErrCaptchaFailed == err || contains(err.Error(), "验证码"):
			fmt.Println("   👉 验证码问题（OCR未初始化或识别率低）")
		}
		os.Exit(1)
	}

	fmt.Printf("🎉 注册成功！\n")
	fmt.Printf("  UID:         %s\n", result.UID)
	fmt.Printf("  Username:    %s\n", result.Username)
	fmt.Printf("  DisplayName: %s\n", result.DisplayName)
	fmt.Printf("  CallbackURL: %s\n", result.CallbackURL[:min(80, len(result.CallbackURL))])
	fmt.Printf("  RealNameOK:  %v\n", result.RealNameSubmitted)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
