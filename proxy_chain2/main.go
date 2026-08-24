package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ChainedTransport: 双层 HTTP 代理链（沙箱专用）
// 请求路径: App -> LocalTunnel(127.0.0.1:18080) -> UserProxy(IP:PORT:user:pass) -> 公网
// 认证格式（该供应商特化）：Proxy-Authorization: user:pass （明文，不带 Basic）
type ChainedTransport struct {
	LocalProxy string // 本地隧道 "127.0.0.1:18080"
	UserProxy  string // 用户代理 "IP:PORT"
	UserAuth   string // 明文 user:pass
}

// dialChained: 通过本地隧道 CONNECT 用户代理，建立 TCP 隧道
func (ct *ChainedTransport) dialChained(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", ct.LocalProxy)
	if err != nil {
		return nil, fmt.Errorf("连本地隧道: %w", err)
	}
	// CONNECT 用户代理
	req := "CONNECT " + ct.UserProxy + " HTTP/1.1\r\nHost: " + ct.UserProxy + "\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("发CONNECT给本地: %w", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("读本地响应: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		conn.Close()
		return nil, fmt.Errorf("本地隧道CONNECT拒绝: %d", resp.StatusCode)
	}
	return conn, nil
}

func (ct *ChainedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	// 1. 建立 本地->用户代理 TCP 隧道
	conn, err := ct.dialChained(ctx)
	if err != nil {
		return nil, err
	}
	targetHost := req.URL.Host
	if req.URL.Port() == "" {
		if req.URL.Scheme == "https" {
			targetHost += ":443"
		} else {
			targetHost += ":80"
		}
	}

	if req.URL.Scheme == "https" {
		// HTTPS: 在 本地->用户代理 隧道上，再向用户代理发 CONNECT 目标主机，再做 TLS
		connectReq := "CONNECT " + targetHost + " HTTP/1.1\r\n" +
			"Host: " + targetHost + "\r\n" +
			"Proxy-Authorization: " + ct.UserAuth + "\r\n\r\n"
		if _, err := conn.Write([]byte(connectReq)); err != nil {
			conn.Close()
			return nil, fmt.Errorf("用户代理CONNECT发: %w", err)
		}
		br := bufio.NewReader(conn)
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("用户代理CONNECT读: %w", err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			buf, _ := io.ReadAll(io.LimitReader(br, 1024))
			conn.Close()
			return nil, fmt.Errorf("用户代理CONNECT拒绝 status=%d: %s", resp.StatusCode, string(buf))
		}
		// TLS 握手
		sn := strings.Split(targetHost, ":")[0]
		tc := tls.Client(conn, &tls.Config{ServerName: sn})
		if err := tc.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, fmt.Errorf("TLS握手: %w", err)
		}
		// 发送请求
		if err := req.Write(tc); err != nil {
			tc.Close()
			return nil, fmt.Errorf("写HTTPS请求: %w", err)
		}
		resp, err = http.ReadResponse(bufio.NewReader(tc), req)
		if err != nil {
			tc.Close()
			return nil, fmt.Errorf("读HTTPS响应: %w", err)
		}
		resp.Body = &closeWrap{ReadCloser: resp.Body, closer: tc}
		return resp, nil
	}

	// HTTP: 发送绝对 URI + 明文 Proxy-Authorization
	abs := req.URL.String()
	hdr := ""
	// 需要去掉 Host header 的重复
	for k, vs := range req.Header {
		if strings.EqualFold(k, "Host") || strings.EqualFold(k, "Proxy-Authorization") || strings.EqualFold(k, "Proxy-Connection") {
			continue
		}
		for _, v := range vs {
			hdr += fmt.Sprintf("%s: %s\r\n", k, v)
		}
	}
	full := fmt.Sprintf("%s %s HTTP/1.1\r\nHost: %s\r\n%sProxy-Authorization: %s\r\nProxy-Connection: close\r\n\r\n",
		req.Method, abs, req.URL.Host, hdr, ct.UserAuth)
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			conn.Close()
			return nil, err
		}
		full += string(body)
	}
	if _, err := conn.Write([]byte(full)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("写HTTP请求: %w", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("读HTTP响应: %w", err)
	}
	resp.Body = &closeWrap{ReadCloser: resp.Body, closer: conn}
	return resp, nil
}

type closeWrap struct {
	io.ReadCloser
	closer io.Closer
}

func (c *closeWrap) Close() error {
	e1 := c.ReadCloser.Close()
	e2 := c.closer.Close()
	if e1 != nil {
		return e1
	}
	return e2
}

type Proxy struct {
	IP   string
	Port string
	User string
	Pass string
}

func parseProxy(raw string) *Proxy {
	parts := strings.Split(raw, ":")
	if len(parts) == 4 {
		return &Proxy{IP: parts[0], Port: parts[1], User: parts[2], Pass: parts[3]}
	}
	return nil
}

func testOne(ctx context.Context, idx int, p *Proxy, wg *sync.WaitGroup, results chan<- string) {
	defer wg.Done()
	r := fmt.Sprintf("[%2d] %s:%s ", idx, p.IP, p.Port)
	ct := &ChainedTransport{
		LocalProxy: "127.0.0.1:18080",
		UserProxy:  p.IP + ":" + p.Port,
		UserAuth:   p.User + ":" + p.Pass,
	}
	client := &http.Client{Transport: ct, Timeout: 35 * time.Second}

	// 1. 出口IP
	s := time.Now()
	req1, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://httpbin.org/ip", nil)
	req1.Header.Set("User-Agent", "Mozilla/5.0")
	resp1, err := client.Do(req1)
	if err != nil {
		results <- r + fmt.Sprintf("❌出口失败(%.1fs):%v", time.Since(s).Seconds(), shortErr(err))
		return
	}
	b1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	r += fmt.Sprintf("✅出口=%s(%.1fs) ", strings.TrimSpace(string(b1)), time.Since(s).Seconds())

	// 2. 4399首页
	s2 := time.Now()
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.4399.com/", nil)
	req2.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/129 Safari/537.36")
	req2.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	resp2, err := client.Do(req2)
	if err != nil {
		results <- r + fmt.Sprintf("❌首页(%.1fs):%v", time.Since(s2).Seconds(), shortErr(err))
		return
	}
	b2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	r += fmt.Sprintf("✅首页=%d(%.1fs,%d) ", resp2.StatusCode, time.Since(s2).Seconds(), len(b2))

	// 3. regFrame.do
	s3 := time.Now()
	req3, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://ptlogin.4399.com/ptlogin/regFrame.do?appId=www_home&displayMode=popup&level=4&sec=1", nil)
	req3.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/129 Safari/537.36")
	req3.Header.Set("Referer", "https://www.4399.com/")
	resp3, err := client.Do(req3)
	if err != nil {
		results <- r + fmt.Sprintf("❌regFrame(%.1fs):%v", time.Since(s3).Seconds(), shortErr(err))
		return
	}
	b3, _ := io.ReadAll(resp3.Body)
	resp3.Body.Close()
	r += fmt.Sprintf("✅regFrame=%d(%.1fs,%d)", resp3.StatusCode, time.Since(s3).Seconds(), len(b3))
	results <- r
}

func shortErr(err error) string {
	s := err.Error()
	if len(s) > 60 {
		return s[:60] + "..."
	}
	return s
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
	fmt.Println("========== 链式代理批量测试 v2（修复认证: 明文 user:pass）==========")
	fmt.Printf("时间: %s\n链路: App -> 127.0.0.1:18080 -> 用户代理 -> 公网\n共 %d 个代理\n\n",
		time.Now().Format("2006-01-02 15:04:05"), len(proxiesRaw))

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	results := make(chan string, len(proxiesRaw))
	for i, raw := range proxiesRaw {
		p := parseProxy(raw)
		if p == nil {
			continue
		}
		wg.Add(1)
		go testOne(ctx, i+1, p, &wg, results)
	}
	go func() { wg.Wait(); close(results) }()

	pass := 0
	total := 0
	for r := range results {
		total++
		if strings.Contains(r, "regFrame=200") {
			pass++
		}
		fmt.Println(r)
	}
	fmt.Printf("\n========== 汇总 ==========\n")
	fmt.Printf("总计: %d | 全部通过: %d (%.0f%%) | 失败: %d\n",
		total, pass, float64(pass)/float64(total)*100, total-pass)
}
