package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Proxy struct {
	IP   string
	Port string
	User string
	Pass string
	Raw  string
}

func parseProxy(raw string) *Proxy {
	parts := strings.Split(raw, ":")
	if len(parts) == 4 {
		return &Proxy{IP: parts[0], Port: parts[1], User: parts[2], Pass: parts[3], Raw: raw}
	} else if len(parts) == 2 {
		return &Proxy{IP: parts[0], Port: parts[1], Raw: raw}
	}
	return nil
}

func (p *Proxy) ProxyURL() string {
	if p.User != "" {
		return fmt.Sprintf("http://%s:%s@%s:%s", p.User, p.Pass, p.IP, p.Port)
	}
	return fmt.Sprintf("http://%s:%s", p.IP, p.Port)
}

var captchaIDRegexp = regexp.MustCompile(`captcha\.do\?captchaId=([\w\d]+)|captchaId["':\s]+([\w\d]+)`)

func testProxy(ctx context.Context, idx int, p *Proxy, wg *sync.WaitGroup, results chan<- string) {
	defer wg.Done()
	result := fmt.Sprintf("[%2d] %s:%s ", idx, p.IP, p.Port)

	proxyURL, err := url.Parse(p.ProxyURL())
	if err != nil {
		results <- result + fmt.Sprintf("❌ URL解析失败: %v", err)
		return
	}
	transport := &http.Transport{
		Proxy:               http.ProxyURL(proxyURL),
		IdleConnTimeout:     15 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	client := &http.Client{Transport: transport, Timeout: 20 * time.Second}

	// 测试1: 出口IP
	start := time.Now()
	req1, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://httpbin.org/ip", nil)
	resp1, err := client.Do(req1)
	if err != nil {
		results <- result + fmt.Sprintf("❌ TCP不通 (%.1fs): %v", time.Since(start).Seconds(), err)
		return
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	result += fmt.Sprintf("✅出口=%s(%.1fs) ", strings.TrimSpace(string(body1)), time.Since(start).Seconds())

	// 测试2: 4399首页
	start2 := time.Now()
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.4399.com/", nil)
	req2.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/129 Safari/537.36")
	req2.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	resp2, err := client.Do(req2)
	if err != nil {
		results <- result + fmt.Sprintf("❌ 4399首页失败 (%.1fs): %v", time.Since(start2).Seconds(), err)
		return
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	title := ""
	if ti := strings.Index(string(body2), "<title>"); ti >= 0 {
		if te := strings.Index(string(body2)[ti:], "</title>"); te > 0 {
			title = string(body2)[ti+7 : ti+te]
		}
	}
	result += fmt.Sprintf("✅首页=%d(%.1fs)%s ", resp2.StatusCode, time.Since(start2).Seconds(), title)

	// 测试3: regFrame.do + 提取captchaId
	start3 := time.Now()
	req3, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://ptlogin.4399.com/ptlogin/regFrame.do?appId=www_home&displayMode=popup&level=4&sec=1", nil)
	req3.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/129 Safari/537.36")
	req3.Header.Set("Referer", "https://www.4399.com/")
	resp3, err := client.Do(req3)
	if err != nil {
		results <- result + fmt.Sprintf("❌ regFrame失败 (%.1fs): %v", time.Since(start3).Seconds(), err)
		return
	}
	body3, _ := io.ReadAll(resp3.Body)
	resp3.Body.Close()
	cid := ""
	if m := captchaIDRegexp.FindSubmatch(body3); len(m) >= 2 {
		for i := 1; i < len(m); i++ {
			if len(m[i]) > 0 {
				cid = string(m[i])
				break
			}
		}
	}
	result += fmt.Sprintf("✅regFrame=%d(%.1fs) captchaId=[%s]", resp3.StatusCode, time.Since(start3).Seconds(), cid)
	results <- result
}

func main() {
	proxiesRaw := []string{
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
	fmt.Println("========== 短效动态代理批量测试 ==========")
	fmt.Printf("时间: %s\n共 %d 个代理\n\n", time.Now().Format("2006-01-02 15:04:05"), len(proxiesRaw))

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	results := make(chan string, len(proxiesRaw))
	for i, raw := range proxiesRaw {
		p := parseProxy(raw)
		if p == nil {
			continue
		}
		wg.Add(1)
		go testProxy(ctx, i+1, p, &wg, results)
	}
	go func() { wg.Wait(); close(results) }()

	pass := 0
	total := 0
	for r := range results {
		total++
		if strings.Contains(r, "regFrame=") && strings.Contains(r, "✅regFrame=200") {
			pass++
		}
		fmt.Println(r)
	}
	fmt.Printf("\n========== 汇总 ==========\n")
	fmt.Printf("总计: %d | regFrame成功: %d (%.0f%%) | 失败: %d\n",
		total, pass, float64(pass)/float64(total)*100, total-pass)
}
