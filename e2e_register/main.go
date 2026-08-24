package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	aesKey = "lzYW5qaXVqa" // 16 bytes
)

// AES-CBC PKCS7 加密（4399 注册用）
func encryptAES(plaintext string) (string, error) {
	key := []byte(aesKey)
	iv := []byte(aesKey) // 4399 用 key 当 IV
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	padded := pkcs7Pad([]byte(plaintext), block.BlockSize())
	mode := cipher.NewCBCEncrypter(block, iv)
	ciphertext := make([]byte, len(padded))
	mode.CryptBlocks(ciphertext, padded)
	return hex.EncodeToString(ciphertext), nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - len(data)%blockSize
	padText := bytes.Repeat([]byte{byte(padLen)}, padLen)
	return append(data, padText...)
}

func main() {
	fmt.Println("========== 端到端注册测试：打印所有请求/响应 ==========")
	fmt.Printf("时间: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	jar, _ := cookiejar.New(nil)
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	client := &http.Client{Transport: transport, Jar: jar, Timeout: 30 * time.Second}
	ctx := context.Background()
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36"

	// =========================================
	// 配置测试数据
	// =========================================
	username := "testuser" + fmt.Sprintf("%d", time.Now().Unix()%100000)
	password := "Aa123456"
	realname := "张三"
	idcard := "110101199003074518" // 测试用的无效/有效SFZ？用一个通过校验算法的

	fmt.Printf("测试账号信息:\n")
	fmt.Printf("  用户名: %s\n", username)
	fmt.Printf("  密码: %s\n", password)
	fmt.Printf("  姓名: %s\n", realname)
	fmt.Printf("  SFZ:  %s\n\n", idcard)

	// =========================================
	// Step 1: 访问首页
	// =========================================
	fmt.Println("=== [1/6] 访问 www.4399.com ===")
	doRequest(ctx, client, "GET", "https://www.4399.com/", "", map[string]string{
		"User-Agent":      ua,
		"Accept-Language": "zh-CN,zh;q=0.9",
	}, false)
	time.Sleep(700 * time.Millisecond)

	// =========================================
	// Step 2: 访问 regFrame.do
	// =========================================
	fmt.Println("\n=== [2/6] 访问 regFrame.do ===")
	rfURL := "https://ptlogin.4399.com/ptlogin/regFrame.do?appId=www_home&displayMode=popup&level=4&sec=1"
	_, _, _ = doRequest(ctx, client, "GET", rfURL, "", map[string]string{
		"User-Agent":      ua,
		"Referer":         "https://www.4399.com/",
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Language": "zh-CN,zh;q=0.9",
	}, false)
	time.Sleep(900 * time.Millisecond)

	// 找 USESSIONID
	u4399, _ := url.Parse("https://ptlogin.4399.com")
	var usessionid string
	for _, c := range jar.Cookies(u4399) {
		if c.Name == "USESSIONID" {
			usessionid = c.Value
		}
	}
	fmt.Printf("  USESSIONID from Cookie: %s\n", usessionid)

	// =========================================
	// Step 3: 下载 regFrame.js 并分析 sessionId 来源
	// =========================================
	fmt.Println("\n=== [3/6] 分析 regFrame.js（sessionId 调用来源）===")
	regJSURL := "https://ptlogin.3304399.net/resource/regFrame.js?v=319"
	jsBody, _, _ := doRequest(ctx, client, "GET", regJSURL, "", map[string]string{
		"User-Agent": ua,
		"Referer":    rfURL,
	}, false)

	// 查找 sessionId 传入到 validation 函数的地方
	jsStr := string(jsBody)
	// 找 initCaptcha / getCaptcha / changeCaptcha 函数调用
	captchaFuncRegexp := regexp.MustCompile(`(?i)(change|init|get|refresh)?captcha[^(]{0,20}\(\s*["']?([A-Za-z0-9_\-\.]+)["']?`)
	matches := captchaFuncRegexp.FindAllStringSubmatch(jsStr, -1)
	fmt.Printf("  captcha 相关调用（共 %d 处）:\n", len(matches))
	for i, m := range matches {
		if i > 10 {
			break
		}
		fmt.Printf("  [%d] %s -> 传入参数=%s\n", i+1, preview(m[0], 80), m[2])
	}

	// 查找 var sid / var sessionId = 
	sidAssignRegexp := regexp.MustCompile(`var\s+(sessionId|sid|captchaId)\s*=\s*["']?([A-Za-z0-9_\-\.]+)["']?`)
	matches2 := sidAssignRegexp.FindAllStringSubmatch(jsStr, -1)
	if len(matches2) > 0 {
		fmt.Printf("  sessionId/sid 赋值:\n")
		for _, m := range matches2 {
			fmt.Printf("    var %s = %s\n", m[1], m[2])
		}
	}

	// 查找整个注册提交逻辑
	fmt.Println("\n  --- 注册提交函数 ---")
	submitRegexp := regexp.MustCompile(`(?i)(register|submit|doreg|postreg)[^}]{0,200}`)
	submits := submitRegexp.FindAllString(jsStr, -1)
	for i, s := range submits {
		if i > 3 {
			break
		}
		s = strings.TrimSpace(s)
		if len(s) > 250 {
			s = s[:250] + "..."
		}
		fmt.Printf("  [%d] %s\n", i+1, s)
	}
	os.WriteFile("/tmp/regFrame.js", jsBody, 0644)

	// =========================================
	// Step 4: 下载验证码图片
	// =========================================
	fmt.Println("\n=== [4/6] 下载验证码图片 ===")
	// 优先用 USESSIONID 做 captchaId
	captchaID := usessionid
	if captchaID == "" {
		captchaID = "captchaReq" + randStr(19)
	}
	fmt.Printf("  使用 captchaId: %s\n", captchaID)

	capURL := "https://ptlogin.4399.com/ptlogin/captcha.do?captchaId=" + url.QueryEscape(captchaID)
	capBody, capStatus, _ := doRequest(ctx, client, "GET", capURL, "", map[string]string{
		"User-Agent": ua,
		"Referer":    rfURL,
		"Accept":     "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8",
	}, false)
	if capStatus != 200 || len(capBody) < 200 {
		fmt.Printf("  ❌ 验证码异常！改用随机 captchaId 再试...\n")
		captchaID = "captchaReq" + randStr(19)
		capURL = "https://ptlogin.4399.com/ptlogin/captcha.do?captchaId=" + url.QueryEscape(captchaID)
		capBody, capStatus, _ = doRequest(ctx, client, "GET", capURL, "", map[string]string{
			"User-Agent": ua,
			"Referer":    rfURL,
		}, false)
		fmt.Printf("  新 captchaId=%s size=%d\n", captchaID, len(capBody))
	}
	os.WriteFile("/tmp/e2e_captcha.jpg", capBody, 0644)
	fmt.Printf("  📄 保存到 /tmp/e2e_captcha.jpg  (%d bytes)\n", len(capBody))
	time.Sleep(1000 * time.Millisecond)

	// =========================================
	// Step 5: 加密字段
	// =========================================
	fmt.Println("\n=== [5/6] AES 加密字段 ===")
	encPwd, _ := encryptAES(password)
	encName, _ := encryptAES(realname)
	encCard, _ := encryptAES(idcard)
	fmt.Printf("  密码明文: %s -> 密文: %s\n", password, preview(encPwd, 40))
	fmt.Printf("  姓名明文: %s -> 密文: %s\n", realname, preview(encName, 40))
	fmt.Printf("  SFZ明文:  %s -> 密文: %s\n", idcard, preview(encCard, 40))

	// =========================================
	// Step 6: 提交注册
	// =========================================
	fmt.Println("\n=== [6/6] 提交 register.do（关键！看详细响应）===")
	email := randStr(10) + "@qq.com"

	form := url.Values{}
	form.Set("postLoginHandler", "default")
	form.Set("displayMode", "popup")
	form.Set("appId", "www_home")
	form.Set("regMode", "reg_normal")
	form.Set("sessionId", "")
	form.Set("regIdcard", "true")
	form.Set("reg_eula_agree", "on")
	form.Set("autoLogin", "on")
	form.Set("level", "4")
	form.Set("sec", "1")
	form.Set("username", username)
	form.Set("password", encPwd)
	form.Set("passwordveri", encPwd)
	form.Set("realname", encName)
	form.Set("idcard", encCard)
	form.Set("email", email)
	form.Set("captcha_id", captchaID)
	// 注意：这里用 "aaaa" 作为验证码（错误的），目的是看看会返回什么错误信息
	// 如果返回「验证码错误」，说明 captchaId 是对的、流程是通的
	// 如果返回「请稍后再试」，说明在验证码校验之前就被风控拦截了
	form.Set("inputCaptcha", "aaaa")

	// 打印表单
	fmt.Printf("  提交表单:\n")
	for _, k := range []string{"username", "captcha_id", "inputCaptcha", "password", "realname", "idcard", "email", "sessionId"} {
		fmt.Printf("    %-16s = %s\n", k, preview(form.Get(k), 50))
	}

	respBody, status, headers := doRequest(ctx, client, "POST",
		"https://ptlogin.4399.com/ptlogin/register.do",
		form.Encode(),
		map[string]string{
			"User-Agent":                ua,
			"Referer":                   rfURL,
			"Origin":                    "https://ptlogin.4399.com",
			"Content-Type":              "application/x-www-form-urlencoded",
			"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			"Accept-Language":           "zh-CN,zh;q=0.9",
			"Sec-Fetch-Site":            "same-origin",
			"Sec-Fetch-Mode":            "navigate",
			"Sec-Fetch-User":            "?1",
			"Sec-Fetch-Dest":            "document",
			"Upgrade-Insecure-Requests": "1",
		},
		true,
	)

	fmt.Printf("\n  注册响应 status=%d\n", status)
	if loc := headers.Get("Location"); loc != "" {
		fmt.Printf("  Location: %s\n", loc)
	}
	html := string(respBody)
	// 提取消息
	msg := extractMsg(html)
	fmt.Printf("  提取消息: %s\n", msg)
	// 打印前2000字符
	fmt.Printf("\n  响应全文前 2000 字符:\n")
	fmt.Println("  " + strings.Repeat("-", 60))
	if len(html) > 2000 {
		fmt.Println(preview(html, 2000))
	} else {
		fmt.Println(html)
	}
	fmt.Println("  " + strings.Repeat("-", 60))

	// 保存全文
	os.WriteFile("/tmp/register_response.html", respBody, 0644)
	fmt.Printf("\n  📄 完整响应已保存到 /tmp/register_response.html\n")

	// 结果判断
	switch {
	case strings.Contains(html, "请稍后再试"):
		fmt.Println("\n  ❌ 结果: 返回「请稍后再试」—— 被风控拦截（在验证码校验之前）")
	case strings.Contains(html, "验证码错误"), strings.Contains(html, "验证码不正确"):
		fmt.Println("\n  ✅ 这是好消息！返回「验证码错误」—— 说明：")
		fmt.Println("     1) captchaId 正确绑定了 session")
		fmt.Println("     2) 加密字段格式正确")
		fmt.Println("     3) SFZ/姓名格式正确")
		fmt.Println("     4) 风控没有拦截！只要 OCR 识别正确就能注册成功！")
	case strings.Contains(html, "注册成功"):
		fmt.Println("\n  🎉 注册成功！")
	case strings.Contains(html, "用户名已被注册"):
		fmt.Println("\n  ⚠️  用户名已被注册（换个就行，说明流程通）")
	default:
		fmt.Println("\n  ⚠️  其他响应，请查看保存的 HTML")
	}
}

func doRequest(ctx context.Context, c *http.Client, method, rawURL, body string, headers map[string]string, verbose bool) ([]byte, int, http.Header) {
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, _ := http.NewRequestWithContext(ctx, method, rawURL, rdr)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	start := time.Now()
	resp, err := c.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		fmt.Printf("  ❌ 请求失败 (%s %s): %v (耗时 %.1fs)\n", method, preview(rawURL, 60), err, elapsed.Seconds())
		return nil, 0, nil
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if verbose {
		fmt.Printf("  🔹 %s %s -> status=%d 耗时=%.1fs size=%d bytes\n",
			method, preview(rawURL, 60), resp.StatusCode, elapsed.Seconds(), len(respBody))
	} else {
		fmt.Printf("  ✅ %s %s -> %d (%.1fs, %d bytes)\n",
			method, preview(rawURL, 55), resp.StatusCode, elapsed.Seconds(), len(respBody))
	}
	return respBody, resp.StatusCode, resp.Header
}

const alnum = "abcdefghijklmnopqrstuvwxyz0123456789"

func randStr(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alnum[time.Now().UnixNano()%int64(len(alnum))]
		time.Sleep(1)
	}
	return string(b)
}

func preview(s string, n int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func extractMsg(html string) string {
	// 尝试各种消息格式
	patterns := []struct {
		re *regexp.Regexp
	}{
		{regexp.MustCompile(`msgBox\s*\(\s*["']([^"']+)["']`)},
		{regexp.MustCompile(`showError\s*\(\s*["']([^"']+)["']`)},
		{regexp.MustCompile(`alert\s*\(\s*["']([^"']+)["']`)},
		{regexp.MustCompile(`<div[^>]*class="[^"]*(message|msg|error|tip)[^"]*"[^>]*>([^<]+)</div>`)},
	}
	for _, p := range patterns {
		m := p.re.FindStringSubmatch(html)
		if len(m) >= 2 {
			if len(m) >= 3 && m[2] != "" {
				return strings.TrimSpace(m[2])
			}
			return strings.TrimSpace(m[1])
		}
	}
	return "(未提取到消息，查看完整HTML)"
}
