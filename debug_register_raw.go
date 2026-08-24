//go:build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Yeah114/g79client/account/com4399"
)

func main() {
	proxyStr := "150.139.247.191:17303:ydl84074816:CiuEhvwj"
	log.Printf("===== 注册调试：保存完整响应HTML到文件 =====")

	rt, err := com4399.ParseProxyString(proxyStr)
	if err != nil {
		log.Fatalf("ParseProxyString: %v", err)
	}
	log.Printf("  ✓ Transport OK")

	jar, _ := cookiejar.New(nil)
	httpc := &http.Client{Transport: rt, Jar: jar, Timeout: 40 * time.Second}
	ctx := context.Background()
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36"

	// ---- 1. 预热 ----
	get(ctx, httpc, "GET", "https://www.4399.com/", "", ua, "")
	time.Sleep(800 * time.Millisecond)
	rfURL := "https://ptlogin.4399.com/ptlogin/regFrame.do?appId=www_home&displayMode=popup&level=4&sec=1"
	get(ctx, httpc, "GET", rfURL, "", ua, "https://www.4399.com/")
	time.Sleep(1200 * time.Millisecond)

	// ---- 2. 下载验证码 ----
	sid := "captchaReq" + randStr(19)
	capURL := "https://ptlogin.4399.com/ptlogin/captcha.do?captchaId=" + url.QueryEscape(sid)
	capBody := get(ctx, httpc, "GET", capURL, "", ua, rfURL)
	os.WriteFile("/tmp/debug_captcha.jpg", capBody, 0644)
	log.Printf("  ✓ 验证码保存到 /tmp/debug_captcha.jpg (%d bytes)", len(capBody))

	// ---- 3. OCR 识别(如果引擎可用) ----
	captcha := "aaaa"
	recognized, ocrErr := com4399.RecognizeCaptchaBytes(capBody)
	if ocrErr == nil && len(recognized) == 4 {
		captcha = strings.ToLower(recognized)
		log.Printf("  ✓ OCR 识别结果: %s  (将用这个提交)", captcha)
	} else {
		log.Printf("  ⚠️ OCR 失败: %v → 回退填 'aaaa' 故意错，用来判断流程是否通", ocrErr)
	}

	// ---- 4. 表单 ----
	suffix := time.Now().Unix() % 100000
	uname := fmt.Sprintf("testfun%d", suffix)
	encPwd, _ := com4399.EncryptAES4399("Aa123456")
	encName, _ := com4399.EncryptAES4399("张三")
	encCard, _ := com4399.EncryptAES4399("110101199003074518")
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
	form.Set("username", uname)
	form.Set("password", encPwd)
	form.Set("passwordveri", encPwd)
	form.Set("realname", encName)
	form.Set("idcard", encCard)
	form.Set("email", email)
	form.Set("captcha_id", sid)
	form.Set("inputCaptcha", captcha)
	form.Set("mainDivId", "popup_reg_div")
	form.Set("showRegInfo", "true")
	form.Set("includeFcmInfo", "false")
	form.Set("expandFcmInput", "true")
	form.Set("fcmFakeValidate", "false")
	form.Set("realnameValidate", "true")
	form.Set("userNameLabel", "4399用户名")
	form.Set("iframeId", "popup_reg_frame")
	form.Set("noEmail", "")
	form.Set("bizId", "")
	form.Set("gameId", "")
	form.Set("cid", "")
	form.Set("externalLogin", "qq")
	form.Set("aid", "")
	form.Set("ref", "")
	form.Set("css", "//www.4399.com/css/4399_index_skin.css")
	form.Set("redirectUrl", "")
	form.Set("crossDomainIFrame", "")
	form.Set("crossDomainUrl", "")

	log.Printf("  提交表单: user=%s / captcha=%s (OCR=%q) / sid=%s",
		uname, captcha, recognized, sid)

	time.Sleep(1500 * time.Millisecond)

	// ---- 5. POST register.do ----
	regURL := "https://ptlogin.4399.com/ptlogin/register.do"
	htmlBody := postForm(ctx, httpc, regURL, form.Encode(), ua, rfURL)

	savePath := "/tmp/debug_register_response.html"
	os.WriteFile(savePath, htmlBody, 0644)
	log.Printf("  ✓ register.do 响应已保存到 %s (%d bytes)", savePath, len(htmlBody))

	html := string(htmlBody)
	fmt.Println()
	fmt.Println("=============== register.do 响应诊断 ===============")
	fmt.Printf("status判断: 包含「请稍后再试」=%v\n", strings.Contains(html, "请稍后再试"))
	fmt.Printf("包含「验证码错误」=%v\n", strings.Contains(html, "验证码错误") || strings.Contains(html, "验证码不正确"))
	fmt.Printf("包含「用户名已被注册」=%v\n", strings.Contains(html, "用户名已被注册"))
	fmt.Printf("包含「注册成功」=%v\n", strings.Contains(html, "注册成功"))
	fmt.Printf("包含「身份证」相关提示=%v\n",
		strings.Contains(html, "身份证") || strings.Contains(html, "实名"))
	fmt.Println()

	// 查找 <script> 中的 msgBox/showError/alert
	fmt.Println("---- 脚本中的消息 ----")
	for _, pat := range []string{
		`msgBox\s*\(\s*['"]([^'"]+)`,
		`showError\s*\(\s*['"]([^'"]+)`,
		`alert\s*\(\s*['"]([^'"]+)`,
		`"msg"\s*:\s*"([^"]+)`,
		`errmsg["']?\s*[:=]\s*["']([^"']+)`,
	} {
		re, _ := compilePat(pat)
		if re != nil {
			matches := re.FindAllStringSubmatch(html, -1)
			for _, m := range matches {
				fmt.Printf("  匹配 [%s]: %q\n", pat[:20], m[1])
			}
		}
	}

	// 打印 前 1500 字符
	fmt.Println()
	fmt.Println("---- 响应全文前 1500 字符 ----")
	preview := html
	if len(preview) > 1500 {
		preview = preview[:1500]
	}
	fmt.Println(preview)
	fmt.Println("...(截断)")
}

func get(ctx context.Context, c *http.Client, method, u, body, ua, ref string) []byte {
	r, _ := http.NewRequestWithContext(ctx, method, u, strings.NewReader(body))
	r.Header.Set("User-Agent", ua)
	r.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	r.Header.Set("sec-ch-ua", `"Chromium";v="129", "Not=A?Brand";v="24", "Google Chrome";v="129"`)
	r.Header.Set("sec-ch-ua-mobile", "?0")
	r.Header.Set("sec-ch-ua-platform", `"Windows"`)
	r.Header.Set("Accept-Encoding", "gzip, deflate, br")
	if ref != "" {
		r.Header.Set("Referer", ref)
	}
	r.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	resp, err := c.Do(r)
	if err != nil {
		log.Printf("  ✗ %s %s: %v", method, shortURL(u), err)
		return nil
	}
	defer resp.Body.Close()
	b, _ := readAll(resp)
	log.Printf("  ✓ %s %s → %d (%d bytes)", method, shortURL(u), resp.StatusCode, len(b))
	return b
}

func postForm(ctx context.Context, c *http.Client, u, form, ua, ref string) []byte {
	r, _ := http.NewRequestWithContext(ctx, "POST", u, strings.NewReader(form))
	r.Header.Set("User-Agent", ua)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	r.Header.Set("sec-ch-ua", `"Chromium";v="129", "Not=A?Brand";v="24", "Google Chrome";v="129"`)
	r.Header.Set("sec-ch-ua-mobile", "?0")
	r.Header.Set("sec-ch-ua-platform", `"Windows"`)
	r.Header.Set("Accept-Encoding", "gzip, deflate, br")
	r.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("Sec-Fetch-Mode", "navigate")
	r.Header.Set("Sec-Fetch-User", "?1")
	r.Header.Set("Sec-Fetch-Dest", "document")
	r.Header.Set("Origin", "https://ptlogin.4399.com")
	r.Header.Set("Upgrade-Insecure-Requests", "1")
	if ref != "" {
		r.Header.Set("Referer", ref)
	}
	resp, err := c.Do(r)
	if err != nil {
		log.Printf("  ✗ POST %s: %v", shortURL(u), err)
		return nil
	}
	defer resp.Body.Close()
	b, _ := readAll(resp)
	log.Printf("  ✓ POST %s → %d (%d bytes)", shortURL(u), resp.StatusCode, len(b))
	return b
}

func readAll(resp *http.Response) ([]byte, error) {
	// 处理 gzip 等编码需要 compress/gzip，不过这里简化
	buf := make([]byte, 0, 64*1024)
	tmp := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf, nil
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
func shortURL(u string) string {
	if len(u) > 60 {
		return u[:60] + "..."
	}
	return u
}

func compilePat(p string) (*myRegexp, error) {
	// 简单实现：用strings.Contains找关键子串
	return &myRegexp{pattern: p}, nil
}

type myRegexp struct {
	pattern string
}

func (r *myRegexp) FindAllStringSubmatch(s string, n int) [][]string {
	// 这里用非常简化的正则解析：寻找 ["](...)[\"] 形式
	var out [][]string
	pat := r.pattern
	// 找到前缀关键
	var prefix, suffix string
	if idx := strings.Index(pat, `[^'"]`); idx > 0 {
		prefix = strings.ReplaceAll(pat[:idx], `\s*`, "")
		prefix = strings.ReplaceAll(prefix, `\(`, "(")
		prefix = strings.ReplaceAll(prefix, `['"]`, `"`)
	}
	_ = suffix
	// 简化版：直接找常见的关键字位置
	keywords := []string{"msgBox(", "showError(", "alert(", `msg":`, "errmsg", "message", "请稍后再试", "验证码错误", "注册成功", "用户名已被注册", "身份证"}
	for _, kw := range keywords {
		idx := strings.Index(s, kw)
		if idx >= 0 {
			start := idx
			end := idx + 60
			if end > len(s) {
				end = len(s)
			}
			// 提取引号内的内容
			rest := s[start:end]
			// 找第一个 " 或 ' 后面的内容
			for _, q := range []byte{'"', '\''} {
				q1 := strings.IndexByte(rest, q)
				if q1 >= 0 {
					q2 := strings.IndexByte(rest[q1+1:], q)
					if q2 >= 0 {
						content := rest[q1+1 : q1+1+q2]
						if len(content) > 0 && len(content) < 100 {
							out = append(out, []string{"", content})
						}
					}
				}
			}
		}
	}
	return out
}
