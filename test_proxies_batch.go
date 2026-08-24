//go:build ignore

package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Yeah114/g79client/account/com4399"
)

var proxies = []string{
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

func main() {
	log.Printf("===== 批量代理测试（10个短效动态代理）=====")
	log.Printf("格式: IP:PORT:USER:PASS  认证: 明文 user:pass  隧道: 127.0.0.1:18080")
	fmt.Println()

	var wg sync.WaitGroup
	results := make([]string, len(proxies))
	mu := &sync.Mutex{}

	for i, p := range proxies {
		wg.Add(1)
		go func(idx int, proxyStr string) {
			defer wg.Done()
			rt, err := com4399.ParseProxyString(proxyStr)
			if err != nil {
				mu.Lock()
				results[idx] = fmt.Sprintf("  [%2d] %-45s  ❌ ParseProxyString: %v", idx+1, proxyStr, err)
				mu.Unlock()
				return
			}
			client := &http.Client{Transport: rt, Timeout: 20 * time.Second}

			// 1) httpbin.org/ip 查出口IP
			ctx1, c1 := context.WithTimeout(context.Background(), 18*time.Second)
			defer c1()
			req1, _ := http.NewRequestWithContext(ctx1, "GET", "https://httpbin.org/ip", nil)
			resp1, err := client.Do(req1)
			var httpbinIP, status4399 string
			if err != nil {
				httpbinIP = fmt.Sprintf("httpbin ERR: %v", shortErr(err))
			} else {
				b, _ := io.ReadAll(resp1.Body)
				resp1.Body.Close()
				httpbinIP = fmt.Sprintf("httpbin=%s (%s)", trim(string(b), 60), resp1.Status)
			}

			// 2) GET www.4399.com 看能否连通注册站点
			ctx2, c2 := context.WithTimeout(context.Background(), 18*time.Second)
			defer c2()
			req2, _ := http.NewRequestWithContext(ctx2, "GET", "https://www.4399.com/", nil)
			req2.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/128.0 Safari/537.36")
			resp2, err := client.Do(req2)
			if err != nil {
				status4399 = fmt.Sprintf("4399 ERR: %v", shortErr(err))
			} else {
				resp2.Body.Close()
				status4399 = fmt.Sprintf("4399=%s", resp2.Status)
			}

			mu.Lock()
			ok := resp1 != nil && resp1.StatusCode == 200 && resp2 != nil && resp2.StatusCode == 200
			pass := "❌"
			if ok {
				pass = "✅"
			}
			results[idx] = fmt.Sprintf("  [%2d] %s %-45s\n           → %s\n           → %s",
				idx+1, pass, proxyStr, httpbinIP, status4399)
			mu.Unlock()
		}(i, p)
	}
	wg.Wait()

	fmt.Println("===== 结果汇总 =====")
	passN := 0
	for _, r := range results {
		fmt.Println(r)
		if len(r) > 8 && r[8:10] == "✅" {
			passN++
		}
	}
	fmt.Printf("\n通过率: %d/10\n", passN)
}

func shortErr(err error) string {
	s := fmt.Sprintf("%v", err)
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}

func trim(s string, n int) string {
	s = compact(s)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func compact(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if len(out) > 0 && out[len(out)-1] != ' ' {
				out = append(out, ' ')
			}
		} else {
			out = append(out, c)
		}
	}
	return string(out)
}
