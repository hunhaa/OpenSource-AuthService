//go:build ignore

package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ========= 测试目标 =========
// 复刻之前"方式A"的成功路径：
//   1. 所有 TCP 出网通过沙箱 iptables 自动走 127.0.0.1:18080 隧道（无需应用层处理）
//   2. 应用层直接设置 Transport.Proxy=ProxyFromEnvironment，并把 HTTPS_PROXY 设置成用户代理
//   3. 关键：ProxyConnectHeader 里写"明文 user:pass"而非 Basic base64
//
// 另外也测一下 4399 注册页（HTTPS，即 CONNECT 模式）

var testProxies = []string{
	"150.139.247.191:17303:ydl84074816:CiuEhvwj",
	"171.109.111.206:53677:ydl84074816:CiuEhvwj",
	"171.214.18.60:53589:ydl84074816:CiuEhvwj",
	"117.68.5.80:12251:ydl84074816:CiuEhvwj",
	"117.68.1.241:42687:ydl84074816:CiuEhvwj",
	"117.68.5.239:28354:ydl84074816:CiuEhvwj",
	"117.68.1.241:20539:ydl84074816:CiuEhvwj",
	"222.216.120.176:33807:ydl84074816:CiuEhvwj",
	"182.247.255.69:43405:ydl84074816:CiuEhvwj",
	"150.139.247.172:11349:ydl84074816:CiuEhvwj",
}

type testR struct {
	idx        int
	proxy      string
	binIP      string // httpbin返回的ip
	binOK      bool
	_4399OK    bool
	_4399Code  int
	err        string
	authFmt    string // "plain" 或 "basic"
}

func split4(s string) (ip, port, user, pass string) {
	parts := strings.Split(s, ":")
	if len(parts) == 4 {
		return parts[0], parts[1], parts[2], parts[3]
	}
	if len(parts) == 2 {
		return parts[0], parts[1], "", ""
	}
	return
}

// testAuth 测试指定的认证格式
func testAuth(idx int, proxy, ip, port, user, pass, authFmt string, ch chan<- testR) {
	res := testR{idx: idx, proxy: proxy, authFmt: authFmt}

	// 构造代理 URL
	var proxyURL *url.URL
	if user != "" {
		proxyURL, _ = url.Parse(fmt.Sprintf("http://%s:%s@%s:%s",
			url.PathEscape(user), url.PathEscape(pass), ip, port))
	} else {
		proxyURL, _ = url.Parse(fmt.Sprintf("http://%s:%s", ip, port))
	}

	// Transport 的关键设置
	transport := &http.Transport{
		Proxy:               http.ProxyURL(proxyURL),
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 12 * time.Second,
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
	}
	// 关键：覆盖默认的 Basic 认证，写明文
	if user != "" {
		authVal := user + ":" + pass
		if authFmt == "basic" {
			authVal = "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
		}
		// ProxyConnectHeader 是在 HTTPS CONNECT 阶段发送的头
		transport.ProxyConnectHeader = http.Header{
			"Proxy-Authorization": []string{authVal},
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   25 * time.Second,
	}

	// 1) 测 HTTPS: https://httpbin.org/ip
	func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://httpbin.org/ip", nil)
		req.Header.Set("User-Agent", "curl/8.5.0")
		resp, err := client.Do(req)
		if err != nil {
			res.err = fmt.Sprintf("httpbin: %v", err)
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			res.err = fmt.Sprintf("httpbin status=%d: %.120s", resp.StatusCode, string(b))
			return
		}
		res.binIP = strings.TrimSpace(string(b))
		res.binOK = true
	}()

	// 2) 测 4399 注册页 (HTTPS)
	if res.binOK {
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
				"https://ptlogin.4399.com/ptlogin/regFrame.do?appId=www_home&displayMode=popup&level=4&sec=1", nil)
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/128")
			req.Header.Set("Accept", "text/html,*/*;q=0.8")
			req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
			resp, err := client.Do(req)
			if err != nil {
				res.err = fmt.Sprintf("%s | 4399: %v", res.err, err)
				return
			}
			defer resp.Body.Close()
			res._4399Code = resp.StatusCode
			if resp.StatusCode == 200 {
				res._4399OK = true
			} else {
				b, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
				res.err = fmt.Sprintf("%s | 4399 status=%d: %.120s", res.err, resp.StatusCode, string(b))
			}
		}()
	}

	ch <- res
}

func main() {
	log.Printf("===== 批量测试 %d 个代理 × 2种认证格式 (明文/Basic) =====", len(testProxies))
	log.Printf("重点：4399 注册页 HTTPS 可达 (CONNECT模式)\n")

	ch := make(chan testR, len(testProxies)*2)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 2) // 控制并发，避免代理被打

	for i, p := range testProxies {
		ip, port, user, pass := split4(p)
		if ip == "" {
			log.Printf("[%02d] 跳过(格式错误): %s", i+1, p)
			continue
		}
		// 两种认证格式并行测试
		for _, af := range []string{"plain", "basic"} {
			wg.Add(1)
			sem <- struct{}{}
			go func(ii int, pp, ipp, portp, userp, passp, afp string) {
				defer func() { <-sem }()
				defer wg.Done()
				testAuth(ii, pp, ipp, portp, userp, passp, afp, ch)
			}(i, p, ip, port, user, pass, af)
		}
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	type pair struct{ plain, basic testR }
	results := make(map[int]*pair, len(testProxies))
	for r := range ch {
		if _, ok := results[r.idx]; !ok {
			results[r.idx] = &pair{}
		}
		if r.authFmt == "plain" {
			results[r.idx].plain = r
		} else {
			results[r.idx].basic = r
		}
	}

	passCount := 0
	bestAuth := map[string]int{"plain": 0, "basic": 0}
	fmt.Println()
	for i := 0; i < len(testProxies); i++ {
		p := results[i]
		fmt.Printf("───── [%02d] %s ─────\n", i+1, testProxies[i])
		for _, r := range []testR{p.plain, p.basic} {
			if r.proxy == "" {
				continue
			}
			st := "  ✗"
			if r.binOK && r._4399OK {
				st = "  ✓ PASS"
				passCount++
				bestAuth[r.authFmt]++
			} else if r.binOK {
				st = "  △"
			}
			fmt.Printf("%s auth=%-6s  出口IP=%s   4399=%v(%d)\n",
				st, r.authFmt, shortIP(r.binIP), r._4399OK, r._4399Code)
			if r.err != "" && !(r.binOK && r._4399OK) {
				fmt.Printf("        Err: %s\n", r.err)
			}
		}
	}
	fmt.Printf("\n===== 汇总 =====\n代理数: %d, 4399可用(任一认证): %d   明文胜: %d, Basic胜: %d\n",
		len(testProxies), passCount, bestAuth["plain"], bestAuth["basic"])

	fmt.Println("\n\n推荐最佳代理（第一个可用的明文/Basic）：")
	printBest := func(auth string) {
		for i := 0; i < len(testProxies); i++ {
			p := results[i]
			var r testR
			if auth == "plain" {
				r = p.plain
			} else {
				r = p.basic
			}
			if r.binOK && r._4399OK {
				fmt.Printf("  %s  # auth=%s 出口=%s\n", testProxies[i], auth, shortIP(r.binIP))
			}
		}
	}
	printBest("plain")
	printBest("basic")

	// 如果全部失败，给一个诊断
	if passCount == 0 {
		fmt.Println("\n⚠️  全部失败，可能的原因：")
		fmt.Println("  1) 代理已过期（短效代理通常有效期很短，1-5分钟）")
		fmt.Println("  2) 用户名/密码错误（比如 ydl84074816:CiuEhvwj 是否正确？）")
		fmt.Println("  3) 沙箱出口被拦截——请确认以下命令可达：")
		fmt.Println("     curl -x http://127.0.0.1:18080 -sI https://ptlogin.4399.com/")
	}
}

func shortIP(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 60 {
		return s[:60] + "..."
	}
	if s == "" {
		return "(none)"
	}
	return s
}
