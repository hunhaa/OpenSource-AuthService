package main

import (
	"context"
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

var (
	captchaIDRegexp1 = regexp.MustCompile(`/ptlogin/captcha\.do\?captchaId=([A-Za-z0-9]+)`)
	captchaIDRegexp2 = regexp.MustCompile(`["']captchaId["']\s*[:=]\s*["']([A-Za-z0-9]+)["']`)
	sessionIDRegexp  = regexp.MustCompile(`["']sessionId["']\s*[:=]\s*["']([A-Za-z0-9]+)["']`)
	captchaReqRegexp = regexp.MustCompile(`captchaReq[A-Za-z0-9]{10,}`)
)

func main() {
	fmt.Println("========== 4399 注册流程深度诊断 ==========")
	fmt.Printf("时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("HTTPS_PROXY=%s\n\n", os.Getenv("HTTPS_PROXY"))

	// 使用 ProxyFromEnvironment 走本地隧道
	jar, _ := cookiejar.New(nil)
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		Jar:       jar,
		Timeout:   25 * time.Second,
	}

	ctx := context.Background()
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36"

	// ===== Step 1: 访问 www.4399.com 首页 =====
	fmt.Println("=== Step 1: 访问 www.4399.com 首页 ===")
	req1, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.4399.com/", nil)
	req1.Header.Set("User-Agent", ua)
	req1.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	resp1, err := client.Do(req1)
	if err != nil {
		fmt.Printf("❌ 首页失败: %v\n", err)
		return
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	fmt.Printf("✅ 首页 status=%d size=%d bytes\n", resp1.StatusCode, len(body1))
	fmt.Printf("   Cookies after home: %+v\n", dumpCookies(jar, "https://www.4399.com"))
	time.Sleep(800 * time.Millisecond)

	// ===== Step 2: 访问 regFrame.do =====
	fmt.Println("\n=== Step 2: 访问 regFrame.do（关键，看captchaId来源）===")
	regFrameURL := "https://ptlogin.4399.com/ptlogin/regFrame.do?appId=www_home&displayMode=popup&level=4&sec=1"
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, regFrameURL, nil)
	req2.Header.Set("User-Agent", ua)
	req2.Header.Set("Referer", "https://www.4399.com/")
	req2.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req2.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	resp2, err := client.Do(req2)
	if err != nil {
		fmt.Printf("❌ regFrame.do 失败: %v\n", err)
		return
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	fmt.Printf("✅ regFrame.do status=%d size=%d bytes\n", resp2.StatusCode, len(body2))
	fmt.Printf("   Cookies after regFrame:\n")
	for _, c := range jar.Cookies(mustURL("https://ptlogin.4399.com")) {
		fmt.Printf("     - %s = %s\n", c.Name, preview(c.Value, 50))
	}
	for _, c := range jar.Cookies(mustURL("https://www.4399.com")) {
		fmt.Printf("     - %s = %s\n", c.Name, preview(c.Value, 50))
	}

	// 保存完整响应到文件以便分析
	os.WriteFile("/tmp/regframe_response.html", body2, 0644)
	fmt.Printf("   📄 完整响应已保存到 /tmp/regframe_response.html\n")

	htmlStr := string(body2)

	// 搜索所有可能的 captchaId/sessionId
	fmt.Println("\n--- 在 regFrame HTML 中搜索 captchaId/sessionId ---")

	// 正则1: captcha.do?captchaId=xxx
	if m := captchaIDRegexp1.FindAllStringSubmatch(htmlStr, -1); len(m) > 0 {
		fmt.Printf("✅ captcha.do?captchaId= 匹配到 %d 处:\n", len(m))
		for i, mm := range m {
			if i > 3 {
				fmt.Printf("  ... 省略 %d 个\n", len(m)-i)
				break
			}
			fmt.Printf("  [%d] captchaId=%s\n", i+1, mm[1])
		}
	} else {
		fmt.Println("❌ 没有找到 captcha.do?captchaId= 格式")
	}

	// 正则2: "captchaId" : "xxx"
	if m := captchaIDRegexp2.FindAllStringSubmatch(htmlStr, -1); len(m) > 0 {
		fmt.Printf("✅ JSON格式 captchaId 匹配到 %d 处:\n", len(m))
		for i, mm := range m {
			fmt.Printf("  [%d] captchaId=%s\n", i+1, mm[1])
		}
	} else {
		fmt.Println("❌ 没有找到 JSON captchaId 格式")
	}

	// 正则3: "sessionId" : "xxx"
	if m := sessionIDRegexp.FindAllStringSubmatch(htmlStr, -1); len(m) > 0 {
		fmt.Printf("✅ JSON格式 sessionId 匹配到 %d 处:\n", len(m))
		for i, mm := range m {
			fmt.Printf("  [%d] sessionId=%s\n", i+1, mm[1])
		}
	} else {
		fmt.Println("❌ 没有找到 JSON sessionId 格式")
	}

	// 正则4: captchaReqxxxxxx (我们之前随机生成的格式)
	if m := captchaReqRegexp.FindAllString(htmlStr, -1); len(m) > 0 {
		fmt.Printf("✅ captchaReq... 匹配到 %d 处:\n", len(m))
		for i, mm := range m {
			if i > 3 {
				fmt.Printf("  ... 省略 %d 个\n", len(m)-i)
				break
			}
			fmt.Printf("  [%d] %s\n", i+1, mm)
		}
	} else {
		fmt.Println("❌ 没有找到 captchaReq... 格式（说明 captchaId 不是这个前缀！）")
	}

	// 搜索 script 标签里的 JS
	fmt.Println("\n--- 搜索内嵌 JS 中的关键变量 ---")
	scriptRegexp := regexp.MustCompile(`(?s)<script[^>]*>(.*?)</script>`)
	scripts := scriptRegexp.FindAllStringSubmatch(htmlStr, -1)
	fmt.Printf("找到 %d 个 script 标签\n", len(scripts))
	for i, sm := range scripts {
		js := sm[1]
		if strings.Contains(js, "captcha") || strings.Contains(js, "session") || strings.Contains(js, "Captcha") {
			fmt.Printf("\n  Script[%d] (len=%d) 包含关键字，前800字符:\n", i, len(js))
			fmt.Printf("  %s\n", preview(js, 800))
		}
	}

	// ===== Step 3: 尝试下载验证码 =====
	fmt.Println("\n=== Step 3: 尝试下载验证码图片 ===")

	// 先用正则1找第一个 captchaId
	var captchaId string
	if m := captchaIDRegexp1.FindStringSubmatch(htmlStr); len(m) >= 2 {
		captchaId = m[1]
		fmt.Printf("使用 captcha.do?captchaId= 提取到的: [%s]\n", captchaId)
	} else if m := captchaIDRegexp2.FindStringSubmatch(htmlStr); len(m) >= 2 {
		captchaId = m[1]
		fmt.Printf("使用 JSON captchaId 提取到的: [%s]\n", captchaId)
	} else {
		fmt.Println("⚠️  无法从 HTML 提取 captchaId，尝试用 sessionId + 其他方式...")
		fmt.Println("   尝试查找外部 JS 文件...")
		// 查找 src=/ptlogin/...js
		jsRegexp := regexp.MustCompile(`src=["'](/ptlogin/[^"']+\.js)["']`)
		jsMatches := jsRegexp.FindAllStringSubmatch(htmlStr, -1)
		for _, jm := range jsMatches {
			fmt.Printf("   发现外部JS: %s\n", jm[1])
		}
		// 兜底用随机，但打印警告
		captchaId = "captchaReqTEST1234567890123"
		fmt.Printf("   兜底使用测试ID: [%s]（很可能会失败）\n", captchaId)
	}

	if captchaId != "" {
		capURL := "https://ptlogin.4399.com/ptlogin/captcha.do?captchaId=" + url.QueryEscape(captchaId)
		req3, _ := http.NewRequestWithContext(ctx, http.MethodGet, capURL, nil)
		req3.Header.Set("User-Agent", ua)
		req3.Header.Set("Referer", regFrameURL)
		resp3, err := client.Do(req3)
		if err != nil {
			fmt.Printf("❌ 验证码下载失败: %v\n", err)
		} else {
			capBody, _ := io.ReadAll(resp3.Body)
			resp3.Body.Close()
			fmt.Printf("✅ 验证码 status=%d size=%d bytes\n", resp3.StatusCode, len(capBody))
			if len(capBody) > 10 {
				fmt.Printf("   前20字节(HEX): %x\n", capBody[:min(20, len(capBody))])
				// 保存到文件
				os.WriteFile("/tmp/captcha_test.png", capBody, 0644)
				fmt.Printf("   📄 已保存到 /tmp/captcha_test.png\n")
			}
			if resp3.StatusCode == 200 && len(capBody) < 200 {
				fmt.Println("   ⚠️  验证码图片太小 (<200 bytes)，可能 captchaId 无效！")
			}
			if resp3.StatusCode != 200 {
				fmt.Printf("   ❌ captchaId 无效! 响应: %s\n", preview(string(capBody), 200))
			}
		}
	}
	fmt.Println("\n========== 诊断完成 ==========")
}

func dumpCookies(jar *cookiejar.Jar, u string) string {
	uu, _ := url.Parse(u)
	cs := jar.Cookies(uu)
	if len(cs) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(cs))
	for _, c := range cs {
		parts = append(parts, fmt.Sprintf("%s=%s", c.Name, preview(c.Value, 20)))
	}
	return strings.Join(parts, "; ")
}

func mustURL(u string) *url.URL {
	uu, _ := url.Parse(u)
	return uu
}

func preview(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
