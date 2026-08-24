package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	account4399 "github.com/Yeah114/g79client/account/com4399"
)

var proxyList = []string{
	"117.68.5.239:21981:ydl84074816:CiuEhvwj",
	"61.160.216.105:21722:ydl84074816:CiuEhvwj",
	"122.246.28.237:15755:ydl84074816:CiuEhvwj",
	"61.160.216.155:11553:ydl84074816:CiuEhvwj",
	"122.246.28.203:20147:ydl84074816:CiuEhvwj",
	"150.139.247.220:31731:ydl84074816:CiuEhvwj",
	"218.65.97.103:22347:ydl84074816:CiuEhvwj",
	"222.216.120.156:59697:ydl84074816:CiuEhvwj",
	"150.139.247.191:16075:ydl84074816:CiuEhvwj",
	"171.214.18.54:51776:ydl84074816:CiuEhvwj",
}

func parseProxy(raw string) *url.URL {
	parts := stringsSplitN(raw, ":", 4)
	if len(parts) != 4 {
		return nil
	}
	u, _ := url.Parse(fmt.Sprintf("http://%s:%s@%s:%s",
		url.QueryEscape(parts[2]), url.QueryEscape(parts[3]), parts[0], parts[1]))
	return u
}

func stringsSplitN(s, sep string, n int) []string {
	var parts []string
	cur := ""
	count := 0
	for _, r := range s {
		if count >= n-1 {
			cur += string(r)
			continue
		}
		if string(r) == sep {
			parts = append(parts, cur)
			cur = ""
			count++
		} else {
			cur += string(r)
		}
	}
	parts = append(parts, cur)
	return parts
}

type checkResult struct {
	Proxy      string
	OK         bool
	Step       string
	TimeMs     int64
	Err        string
	StatusCode int
	Size       int
}

func checkProxy(raw string) checkResult {
	r := checkResult{Proxy: raw}
	u := parseProxy(raw)
	if u == nil {
		r.Err = "parse failed"
		return r
	}
	tr := &http.Transport{
		Proxy:               http.ProxyURL(u),
		IdleConnTimeout:     15 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Step 1: 访问 4399 首页（通过代理），看能否出网
	t0 := time.Now()
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	r.Step = "step1_www_4399"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.4399.com/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	resp, err := client.Do(req)
	if err != nil {
		r.Err = err.Error()
		r.TimeMs = time.Since(t0).Milliseconds()
		return r
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	r.StatusCode = resp.StatusCode
	r.TimeMs = time.Since(t0).Milliseconds()
	if resp.StatusCode != 200 {
		r.Err = fmt.Sprintf("status=%d", resp.StatusCode)
		return r
	}
	r.Step = "step2_captcha"
	t0 = time.Now()
	capURL := "https://ptlogin.4399.com/ptlogin/captcha.do?captchaId=captchaReq" + fmt.Sprintf("%d", time.Now().UnixNano())[:19]
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, capURL, nil)
	req2.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	req2.Header.Set("Referer", "https://ptlogin.4399.com/ptlogin/regFrame.do")
	resp2, err := client.Do(req2)
	if err != nil {
		r.Err = err.Error()
		r.TimeMs = time.Since(t0).Milliseconds()
		return r
	}
	data, _ := io.ReadAll(io.LimitReader(resp2.Body, 1<<20))
	resp2.Body.Close()
	r.Size = len(data)
	r.StatusCode = resp2.StatusCode
	r.TimeMs = time.Since(t0).Milliseconds()
	if resp2.StatusCode != 200 || len(data) < 300 {
		r.Err = fmt.Sprintf("status=%d size=%d", resp2.StatusCode, len(data))
		return r
	}

	r.Step = "ALL_OK"
	r.OK = true
	return r
}

func main() {
	log.Printf("=== 4399 代理连通性测试 (共 %d 个) ===\n", len(proxyList))

	var wg sync.WaitGroup
	results := make([]checkResult, len(proxyList))
	sem := make(chan struct{}, 5)
	for i, p := range proxyList {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, p string) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = checkProxy(p)
		}(i, p)
	}
	wg.Wait()

	fmt.Println("")
	fmt.Printf("%-45s %-8s %-6s %-18s %-10s %s\n", "PROXY", "OK?", "MS", "STEP", "STATUS", "ERROR/SIZE")
	fmt.Println("----------------------------------------------------------------------------------------------------")
	alive := 0
	for _, r := range results {
		okStr := "❌"
		if r.OK {
			okStr = "✅"
			alive++
		}
		extra := r.Err
		if extra == "" {
			extra = fmt.Sprintf("size=%d", r.Size)
		}
		fmt.Printf("%-45s %-8s %-6d %-18s %-10d %s\n", r.Proxy, okStr, r.TimeMs, r.Step, r.StatusCode, extra)
	}
	fmt.Println("----------------------------------------------------------------------------------------------------")
	fmt.Printf("通过 %d / %d\n\n", alive, len(proxyList))

	if alive == 0 {
		log.Println("所有代理连通性检查都未通过，跳过注册测试")
		return
	}

	// 用第一个活代理尝试注册一次
	var firstAlive checkResult
	for _, r := range results {
		if r.OK {
			firstAlive = r
			break
		}
	}
	log.Printf("=== 用代理 %s 尝试注册一次 ===\n", firstAlive.Proxy)
	u := parseProxy(firstAlive.Proxy)
	tr := &http.Transport{
		Proxy:               http.ProxyURL(u),
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 15 * time.Second,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	dreq := &account4399.DirectRegisterRequest{
		Username:  "test" + fmt.Sprintf("%d", time.Now().Unix())[6:],
		Password:  "Aa123456",
		RealName:  "张三",
		IDCard:    "110101199003078533",
		Transport: tr,
	}
	log.Printf("  用户名: %s, 身份证: %s %s", dreq.Username, dreq.RealName, dreq.IDCard)
	res, err := account4399.DirectRegister(ctx, dreq)
	if err != nil {
		log.Printf("  ❌ 注册失败: err=%v", err)
		if res != nil {
			log.Printf("     msg=%s", res.DisplayMessage)
		}
	} else if res != nil {
		if res.Success {
			log.Printf("  ✅ 注册成功！ username=%s password=%s", res.Username, res.Password)
		} else {
			log.Printf("  ❌ 未成功：msg=%s", res.DisplayMessage)
		}
	}
}
