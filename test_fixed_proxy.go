//go:build ignore

package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Yeah114/g79client/account/com4399"
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

type result struct {
	idx       int
	proxy     string
	binIP     string
	binOK     bool
	_4399OK   bool
	_4399Code int
	err       string
	dur       time.Duration
}

func short(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 60 {
		return s[:60] + "..."
	}
	if s == "" {
		return "(none)"
	}
	return s
}

func testOne(idx int, proxy string, ch chan<- result) {
	t0 := time.Now()
	res := result{idx: idx, proxy: proxy}

	rt, err := com4399.ParseProxyString(proxy)
	if err != nil {
		res.err = fmt.Sprintf("ParseProxyString: %v", err)
		ch <- res
		return
	}
	client := &http.Client{Transport: rt, Timeout: 30 * time.Second}

	// 1) httpbin 出口IP
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

	// 2) 4399 regFrame
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

	res.dur = time.Since(t0)
	ch <- res
}

func main() {
	log.Printf("===== 验证修复后的 ParseProxyString（明文认证 + 双层链）=====")
	log.Printf("代理数: %d\n", len(testProxies))

	ch := make(chan result, len(testProxies))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, p := range testProxies {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, p string) {
			defer wg.Done()
			defer func() { <-sem }()
			testOne(i, p, ch)
		}(i, p)
	}
	go func() { wg.Wait(); close(ch) }()

	results := make([]result, 0, len(testProxies))
	for r := range ch {
		results = append(results, r)
	}
	// 按 idx 排序
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].idx > results[j].idx {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	pass := 0
	var firstGood string
	fmt.Println()
	for _, r := range results {
		st := "  ✗"
		if r.binOK && r._4399OK {
			st = "  ✓ PASS"
			pass++
			if firstGood == "" {
				firstGood = r.proxy
			}
		} else if r.binOK {
			st = "  △"
		}
		proxyStr := r.proxy
		if len(proxyStr) > 42 {
			proxyStr = proxyStr[:42]
		}
		fmt.Printf("%s [%02d] %-42s  出口=%-20s  4399=%v(%d)  %v\n",
			st, r.idx+1, proxyStr, short(r.binIP), r._4399OK, r._4399Code, r.dur.Round(100*time.Millisecond))
		if r.err != "" && !(r.binOK && r._4399OK) {
			fmt.Printf("        Err: %s\n", r.err)
		}
	}

	fmt.Println()
	fmt.Println("================ 汇总 ================")
	fmt.Printf("代理可用: %d / %d\n", pass, len(testProxies))
	if firstGood != "" {
		fmt.Printf("\n✓ 第一个可用代理:\n   export TEST_PROXY=%q\n", firstGood)
	} else {
		fmt.Println("\n⚠️  这批仍全部失败 → 大概率代理全部过期（短效通常1-5分钟）")
		fmt.Println("  请用户提供新一批刚提取的代理即可立刻验证。")
	}
}
