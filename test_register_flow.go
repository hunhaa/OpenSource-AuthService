//go:build ignore

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
	proxyStr := "171.214.18.60:53589:ydl84074816:CiuEhvwj"
	log.Printf("===== DirectRegister 完整流程测试 =====")
	log.Printf("代理字符串: %s", proxyStr)
	log.Printf("OCR 资源检查:")
	for _, f := range []string{"common.onnx", "dict.txt", "libonnxruntime.so"} {
		if st, err := os.Stat("./ocr_resources/" + f); err == nil {
			log.Printf("  ✓ ./ocr_resources/%s  (%d bytes)", f, st.Size())
		} else {
			log.Printf("  ✗ ./ocr_resources/%s  缺失: %v", f, err)
		}
	}

	// 用 ParseProxyString 生成 Transport（验证新架构）
	rt, err := com4399.ParseProxyString(proxyStr)
	if err != nil {
		log.Fatalf("❌ ParseProxyString 失败: %v", err)
	}
	log.Printf("  ✓ ParseProxyString 成功，Transport: %T", rt)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	suffix := time.Now().Unix() % 100000
	req := &com4399.DirectRegisterRequest{
		Username:  fmt.Sprintf("testfun%d", suffix),
		Password:  "Aa123456",
		RealName:  "李娜",
		IDCard:    "420102199104282629", // 新生成的有效SFZ
		Captcha:   "aaaa", // 故意填错验证码，看返回什么
		Transport: rt,
	}

	t0 := time.Now()
	log.Printf("\n[请求] user=%s / pwd=%s / SFZ=%s / 姓名=%s / 故意填错验证码=aaaa",
		req.Username, req.Password, req.IDCard, req.RealName)

	result, err := com4399.DirectRegister(ctx, req)
	dur := time.Since(t0)

	fmt.Println()
	fmt.Println("=============== 结果 ===============")
	fmt.Printf("总耗时: %.1fs\n", dur.Seconds())
	if err != nil {
		fmt.Printf("❌ 返回 error: %v\n", err)
		classify(err.Error())
	} else {
		fmt.Printf("✅ 返回 success=%v\n", result.Success)
		fmt.Printf("   Message: %s\n", result.Message)
		fmt.Printf("   Display: %s\n", result.DisplayMessage)
		fmt.Printf("   CookieString 前80字: %.80s\n", result.CookieString)
	}
}

func classify(e string) {
	fmt.Println("\n--- 错误诊断 ---")
	switch {
	case ci(e, "请稍后再试") || ci(e, "risk") || ci(e, "riskcontrol"):
		fmt.Println("  ⚠️  [失败原因] 仍然被风控拦截！")
		fmt.Println("      需要优化：会话预热时间、请求头完整性、UA指纹、换更干净的代理、请求间隔")
	case ci(e, "验证码") || ci(e, "captcha") || ci(e, "captchaid"):
		fmt.Println("  ✅  [极佳!] 返回验证码相关错误")
		fmt.Println("      → 代理层 √（已能成功提交到 register.do）")
		fmt.Println("      → 会话预热 √（未被风控）")
		fmt.Println("      → AES加密 √（表单格式被正确解析）")
		fmt.Println("      → captchaId绑定正确 √")
		fmt.Println("      → **只要OCR识别正确验证码即可成功注册！**")
	case ci(e, "用户名已被注册"):
		fmt.Println("  ⚠️  [警告] 用户名重复 → 换个用户名就行，流程完全通")
	case ci(e, "proxy") || ci(e, "407") || ci(e, "authentication required") || ci(e, "eof") || ci(e, "timeout"):
		fmt.Println("  ⚠️  [失败原因] 代理连接问题")
		fmt.Println("      → 代理是否过期？（短效通常1-5分钟）")
		fmt.Println("      → 账号密码是否正确？（ydl84074816:CiuEhvwj）")
	case ci(e, "ocr") || ci(e, "onnx") || ci(e, "engine") || ci(e, "ApiVersion") || ci(e, "not ready"):
		fmt.Println("  ⚠️  [失败原因] OCR引擎初始化失败")
		fmt.Println("      → 需要重新编译正确版本的 onnxruntime_purego / libonnxruntime.so")
		fmt.Println("      → 或者手动填对 Captcha 字段，测试注册")
	case ci(e, "404") || ci(e, "not found"):
		fmt.Println("  ❌  [失败原因] 接口 404，register.do 路径错误")
	case ci(e, "wrong idcard") || ci(e, "身份证") || ci(e, "realname") || ci(e, "idcard") || ci(e, "实名"):
		fmt.Println("  ⚠️  [失败原因] 实名信息错误 → SFZ或姓名格式不对 (wrong idcard)")
		fmt.Println("      → SFZ校验算法不通过，或者该SFZ段被系统拉黑")
		fmt.Println("      → 换 tool_gensfz 生成的新SFZ+姓名组合再试")
	default:
		fmt.Println("  ❓ 其他错误，请检查完整日志")
	}
}

func ci(s, sub string) bool {
	if len(s) < len(sub) {
		return false
	}
	ls := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		ls[i] = c
	}
	lsub := make([]byte, len(sub))
	for i := 0; i < len(sub); i++ {
		c := sub[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		lsub[i] = c
	}
	n, m := len(ls), len(lsub)
	for i := 0; i+m <= n; i++ {
		match := true
		for j := 0; j < m; j++ {
			if ls[i+j] != lsub[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
