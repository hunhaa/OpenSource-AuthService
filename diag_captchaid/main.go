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

func main() {
	fmt.Println("========== 深度追踪：captchaId 的真实来源 ==========")

	jar, _ := cookiejar.New(nil)
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	client := &http.Client{Transport: transport, Jar: jar, Timeout: 25 * time.Second}
	ctx := context.Background()
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/129 Safari/537.36"

	// Step 1: 先种 cookie（首页 + regFrame）
	fmt.Println("Step 1: 预热会话...")
	client.Get("https://www.4399.com/")
	time.Sleep(500 * time.Millisecond)
	reqRF, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://ptlogin.4399.com/ptlogin/regFrame.do?appId=www_home&displayMode=popup&level=4&sec=1", nil)
	reqRF.Header.Set("User-Agent", ua)
	reqRF.Header.Set("Referer", "https://www.4399.com/")
	respRF, _ := client.Do(reqRF)
	bodyRF, _ := io.ReadAll(respRF.Body)
	respRF.Body.Close()

	// 打印 USESSIONID
	var usessionid string
	u4399, _ := url.Parse("https://ptlogin.4399.com")
	for _, c := range jar.Cookies(u4399) {
		if c.Name == "USESSIONID" {
			usessionid = c.Value
			fmt.Printf("  🔑 USESSIONID = %s\n", usessionid)
		}
	}
	time.Sleep(500 * time.Millisecond)

	// Step 2: 从 HTML 中提取所有外部 JS
	html := string(bodyRF)
	jsRegexp := regexp.MustCompile(`src=["']([^"']+\.js(?:\?[^"']*)?)["']`)
	jsMatches := jsRegexp.FindAllStringSubmatch(html, -1)
	fmt.Printf("\nStep 2: 发现 %d 个外部 JS 文件\n", len(jsMatches))
	validationJSURL := ""
	for _, jm := range jsMatches {
		jsu := jm[1]
		if strings.Contains(jsu, "validation") || strings.Contains(jsu, "captcha") || strings.Contains(jsu, "reg") {
			fmt.Printf("  ⭐ %s\n", jsu)
			if validationJSURL == "" && strings.Contains(jsu, "validation") {
				validationJSURL = jsu
			}
		} else {
			fmt.Printf("  - %s\n", jsu)
		}
	}

	// Step 3: 下载 validation.js 并分析
	if validationJSURL != "" {
		if !strings.HasPrefix(validationJSURL, "http") {
			if strings.HasPrefix(validationJSURL, "//") {
				validationJSURL = "https:" + validationJSURL
			} else if strings.HasPrefix(validationJSURL, "/") {
				validationJSURL = "https://ptlogin.4399.com" + validationJSURL
			} else {
				validationJSURL = "https://ptlogin.4399.com/ptlogin/" + validationJSURL
			}
		}
		fmt.Printf("\nStep 3: 分析 validation.js = %s\n", validationJSURL)
		reqJS, _ := http.NewRequestWithContext(ctx, http.MethodGet, validationJSURL, nil)
		reqJS.Header.Set("User-Agent", ua)
		reqJS.Header.Set("Referer", "https://ptlogin.4399.com/ptlogin/regFrame.do")
		respJS, err := client.Do(reqJS)
		if err != nil {
			fmt.Printf("  ❌ 下载失败: %v\n", err)
		} else {
			jsBody, _ := io.ReadAll(respJS.Body)
			respJS.Body.Close()
			fmt.Printf("  ✅ 下载成功 size=%d bytes\n", len(jsBody))
			os.WriteFile("/tmp/validation.js", jsBody, 0644)
			fmt.Printf("  📄 保存到 /tmp/validation.js\n")
			jsStr := string(jsBody)

			// 搜索 captchaId 相关逻辑
			fmt.Println("\n  --- captchaId 生成逻辑 ---")
			cidLineRegexp := regexp.MustCompile(`(?i)(captchaId|captcha_id|sessionId)[^;]{0,100}`)
			lines := cidLineRegexp.FindAllString(jsStr, -1)
			for i, l := range lines {
				if i > 20 {
					fmt.Printf("  ... (省略 %d 条)\n", len(lines)-i)
					break
				}
				l = strings.TrimSpace(l)
				if len(l) > 150 {
					l = l[:150] + "..."
				}
				fmt.Printf("  [%d] %s\n", i+1, l)
			}

			// 搜索 sessionId
			fmt.Println("\n  --- sessionId 相关逻辑 ---")
			sidLineRegexp := regexp.MustCompile(`(?i)sessionId[^;]{0,120}`)
			lines2 := sidLineRegexp.FindAllString(jsStr, -1)
			for i, l := range lines2 {
				if i > 15 {
					fmt.Printf("  ... (省略 %d 条)\n", len(lines2)-i)
					break
				}
				l = strings.TrimSpace(l)
				if len(l) > 160 {
					l = l[:160] + "..."
				}
				fmt.Printf("  [%d] %s\n", i+1, l)
			}
		}
	}

	// Step 4: 测试各种候选 captchaId
	fmt.Println("\nStep 4: 测试不同 captchaId 下载验证码，判断哪些是有效的")
	candidates := []struct {
		Name string
		ID   string
	}{
		{"USESSIONID 原值", usessionid},
		{"USESSIONID 去横杠", strings.ReplaceAll(usessionid, "-", "")},
		{"USESSIONID 大写", strings.ToUpper(strings.ReplaceAll(usessionid, "-", ""))},
		{"captchaReq+19位随机", "captchaReq" + randomString(19)},
		{"USESSIONID前缀+随机", usessionid[:8] + randomString(16)},
		{"纯UUID(无横杠)", strings.ReplaceAll(newUUID(), "-", "")},
		{"空字符串", ""},
	}

	// 关键：用同一个 client（同一个 cookie jar）来下载，看哪个 captchaId 对应的验证码能被服务端正确识别
	for i, cand := range candidates {
		capID := cand.ID
		// URL 编码
		capURL := "https://ptlogin.4399.com/ptlogin/captcha.do?captchaId=" + url.QueryEscape(capID)
		reqCap, _ := http.NewRequestWithContext(ctx, http.MethodGet, capURL, nil)
		reqCap.Header.Set("User-Agent", ua)
		reqCap.Header.Set("Referer", "https://ptlogin.4399.com/ptlogin/regFrame.do")
		startT := time.Now()
		respCap, err := client.Do(reqCap)
		elapsed := time.Since(startT)
		if err != nil {
			fmt.Printf("  [%d] %-25s -> ❌ 错误: %v (%.1fs)\n", i+1, cand.Name, err, elapsed.Seconds())
			continue
		}
		capBody, _ := io.ReadAll(respCap.Body)
		respCap.Body.Close()

		// 判断有效性：状态码+大小+响应头是否有特殊标记
		contentType := respCap.Header.Get("Content-Type")
		// 保存第一个有效图片
		if i == 0 && len(capBody) > 200 {
			os.WriteFile(fmt.Sprintf("/tmp/cap_test_%d.jpg", i+1), capBody, 0644)
		}

		status := "✅"
		note := ""
		if respCap.StatusCode != 200 {
			status = "❌"
			note = fmt.Sprintf(" | 响应=%s", preview(string(capBody), 100))
		} else if len(capBody) < 200 {
			status = "⚠️ "
			note = " | 图片太小"
		}

		fmt.Printf("  [%d] %-25s -> %s status=%3d size=%5d bytes ct=%s (%.1fs)%s\n",
			i+1, cand.Name, status, respCap.StatusCode, len(capBody), contentType, elapsed.Seconds(), note)
	}

	// Step 5: 关键测试 - 连续两次用相同 captchaId，验证 id 绑定性
	fmt.Println("\nStep 5: 验证 captchaId 的幂等性（同id下载两次）")
	testID := usessionid
	if testID == "" {
		testID = "captchaReq" + randomString(19)
	}
	for round := 1; round <= 2; round++ {
		capURL := "https://ptlogin.4399.com/ptlogin/captcha.do?captchaId=" + url.QueryEscape(testID)
		reqCap, _ := http.NewRequestWithContext(ctx, http.MethodGet, capURL, nil)
		reqCap.Header.Set("User-Agent", ua)
		respCap, _ := client.Do(reqCap)
		capBody, _ := io.ReadAll(respCap.Body)
		respCap.Body.Close()
		suffix := "diff"
		if round == 1 {
			os.WriteFile("/tmp/cap_sameid_1.jpg", capBody, 0644)
		} else {
			// 简单对比前几个字节
			data1, _ := os.ReadFile("/tmp/cap_sameid_1.jpg")
			if len(data1) == len(capBody) && len(data1) > 10 {
				eq := true
				for bi := 0; bi < 100; bi++ {
					if data1[bi] != capBody[bi] {
						eq = false
						break
					}
				}
				if eq {
					suffix = "SAME!"
				}
			}
		}
		fmt.Printf("  第%d次下载: id=%s size=%d bytes -> %s\n", round, preview(testID, 30), len(capBody), suffix)
	}

	fmt.Println("\n========== 完成 ==========")
}

const alnum = "abcdefghijklmnopqrstuvwxyz0123456789"

func randomString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alnum[time.Now().UnixNano()%int64(len(alnum))]
		time.Sleep(1 * time.Nanosecond)
	}
	return string(b)
}

func newUUID() string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(time.Now().UnixNano() % 256)
		time.Sleep(1 * time.Nanosecond)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[0], b[1], b[2], b[3], b[4], b[5], b[6], b[7],
		b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15])
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
