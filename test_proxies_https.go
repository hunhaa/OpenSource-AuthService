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

	com4399 "github.com/Yeah114/g79client/account/com4399"
)

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

type r struct {
	idx      int
	proxy    string
	binOK    bool
	binIP    string
	_4399OK  bool
	_4399Len int
	err      string
}

func main() {
	log.Printf("===== 批量测试 %d 个代理 (仅HTTPS端点) =====", len(testProxies))
	log.Printf("目标: https://httpbin.org/ip  +  https://ptlogin.4399.com/ (4399注册页)\n")

	var wg sync.WaitGroup
	ch := make(chan r, len(testProxies))
	sem := make(chan struct{}, 3)

	for i, p := range testProxies {
		wg.Add(1)
		sem <- struct{}{}
		go func(ii int, pp string) {
			defer func() { <-sem }()
			defer wg.Done()
			res := r{idx: ii, proxy: pp}

			tr, err := com4399.ParseProxyString(pp)
			if err != nil {
				res.err = err.Error()
				ch <- res
				return
			}
			client := &http.Client{
				Transport: tr,
				Timeout:   30 * time.Second,
			}

			// 1. https://httpbin.org/ip (出口IP)
			func() {
				ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
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
					res.err = fmt.Sprintf("httpbin status=%d: %.200s", resp.StatusCode, string(b))
					return
				}
				res.binOK = true
				res.binIP = string(b)
			}()

			// 2. https://ptlogin.4399.com/ptlogin/regFrame.do (4399注册页)
			if res.binOK {
				func() {
					ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
					defer cancel()
					req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
						"https://ptlogin.4399.com/ptlogin/regFrame.do?appId=www_home&displayMode=popup&level=4&sec=1", nil)
					req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/128")
					req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
					resp, err := client.Do(req)
					if err != nil {
						res.err = fmt.Sprintf("%s | 4399: %v", res.err, err)
						return
					}
					defer resp.Body.Close()
					b, _ := io.ReadAll(resp.Body)
					res._4399Len = len(b)
					if resp.StatusCode == 200 && len(b) > 5000 {
						res._4399OK = true
					} else {
						res.err = fmt.Sprintf("%s | 4399 status=%d bodyLen=%d", res.err, resp.StatusCode, len(b))
					}
				}()
			}

			ch <- res
		}(i, p)
	}
	go func() { wg.Wait(); close(ch) }()

	okCnt := 0
	results := make([]r, 0, len(testProxies))
	for res := range ch {
		results = append(results, res)
	}
	// 按 idx 排
	for i := 0; i < len(testProxies); i++ {
		var res *r
		for k := range results {
			if results[k].idx == i {
				res = &results[k]
				break
			}
		}
		if res == nil {
			continue
		}
		st := "  ✗"
		if res.binOK && res._4399OK {
			st = "  ✓ PASS"
			okCnt++
		} else if res.binOK {
			st = "  △ PARTIAL (httpbin ok, 4399 no)"
		}
		fmt.Printf("%s [%02d] %s\n", st, i+1, res.proxy)
		if res.binOK {
			fmt.Printf("        出口IP: %s\n", trim(res.binIP, 80))
		}
		if res._4399OK {
			fmt.Printf("        4399注册页: OK (body=%d bytes)\n", res._4399Len)
		}
		if res.err != "" && !(res.binOK && res._4399OK) {
			fmt.Printf("        Err: %s\n", res.err)
		}
		fmt.Println()
	}

	fmt.Printf("===== 汇总: 共 %d 个代理，HTTPS 完全可用 (4399可访问): %d/%d =====\n",
		len(testProxies), okCnt, len(testProxies))
}

func trim(s string, n int) string {
	s = short(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
func short(s string) string {
	for len(s) > 0 && (s[0] == '\n' || s[0] == ' ' || s[0] == '\r' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == ' ' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
