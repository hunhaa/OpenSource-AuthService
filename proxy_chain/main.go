package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// proxyDial: 通过本地HTTP隧道代理，用 CONNECT 方法建立 TCP 连接到目标地址
// 实现链式代理：应用 -> localProxy(127.0.0.1:18080) -> target (用户的短效动态代理)
func proxyDial(ctx context.Context, localProxy, targetAddr, user, pass string) (net.Conn, error) {
	// 1. 连接到本地隧道代理
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", localProxy)
	if err != nil {
		return nil, fmt.Errorf("connect local proxy %s: %w", localProxy, err)
	}

	// 2. 发送 CONNECT targetAddr HTTP/1.1 请求（建立到用户代理的TCP隧道）
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", targetAddr, targetAddr)
	// 如果本地隧道需要认证（这里不用，本地隧道一般不需要）
	if user != "" && localProxy == "" {
		auth := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		connectReq += fmt.Sprintf("Proxy-Authorization: Basic %s\r\n", auth)
	}
	connectReq += "\r\n"

	if _, err := conn.Write([]byte(connectReq)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send CONNECT: %w", err)
	}

	// 3. 读取 CONNECT 响应
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read CONNECT resp: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("CONNECT rejected: status=%d", resp.StatusCode)
	}

	// 4. TCP 隧道建立成功！现在可以把 conn 当成直接连接到 targetAddr
	return conn, nil
}

// chainedTransport: 实现 http.RoundTripper，支持双层 HTTP 代理链
// 对于 HTTP 请求：GET http://host/path HTTP/1.1 -> 本地 -> 用户代理 -> 目标
// 对于 HTTPS 请求：CONNECT 用户代理 -> 再 CONNECT 目标主机，然后 TLS
type chainedTransport struct {
	localProxyAddr  string // 本地隧道，如 "127.0.0.1:18080"
	userProxyURL    *url.URL
	userProxyAuth   string // Basic xxx
	innerTransport  *http.Transport
}

func newChainedTransport(localProxyAddr string, userProxy *url.URL) *chainedTransport {
	ct := &chainedTransport{
		localProxyAddr: localProxyAddr,
		userProxyURL:   userProxy,
	}
	if userProxy.User != nil {
		auth := userProxy.User.Username()
		if p, ok := userProxy.User.Password(); ok {
			auth += ":" + p
		}
		ct.userProxyAuth = "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
	}
	ct.innerTransport = &http.Transport{
		// 自定义 DialContext：通过本地隧道建立到用户代理的 TCP 隧道
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// addr 是请求的目标主机（由 inner transport 传入）。但因为我们要走第二层代理，
			// 实际上 HTTP 请求会被发送到用户代理，HTTPS 会 CONNECT 用户代理，
			// 所以这里不应该 dial addr，而应该先 CONNECT 到用户代理，再...
			// 实际上，对于双层代理，我们需要自定义更完整的 RoundTrip，而不是仅 Dial。
			// 简化方案：直接用 httpproxy库风格的手写 RoundTrip
			return nil, fmt.Errorf("not used, see RoundTrip")
		},
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	return ct
}

// RoundTrip 实现完整双层代理请求
func (ct *chainedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	userProxyHost := ct.userProxyURL.Host // IP:PORT

	// Step 1: 通过本地隧道建立到用户代理的 TCP 隧道
	conn, err := proxyDial(ctx, ct.localProxyAddr, userProxyHost, "", "")
	if err != nil {
		return nil, fmt.Errorf("链式代理 Step1 失败: %w (本地=%s 用户代理=%s)", err, ct.localProxyAddr, userProxyHost)
	}

	// Step 2: 根据请求是 HTTP 还是 HTTPS，分别处理
	targetHost := req.URL.Host
	if req.URL.Port() == "" {
		if req.URL.Scheme == "https" {
			targetHost += ":443"
		} else {
			targetHost += ":80"
		}
	}

	if req.URL.Scheme == "https" {
		// HTTPS: 在已建立的(本地->用户代理)TCP隧道上，向用户代理发送 CONNECT targetHost
		// 然后在这个双重隧道上进行 TLS 握手
		connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", targetHost, targetHost)
		if ct.userProxyAuth != "" {
			connectReq += fmt.Sprintf("Proxy-Authorization: %s\r\n", ct.userProxyAuth)
		}
		connectReq += "\r\n"
		if _, err := conn.Write([]byte(connectReq)); err != nil {
			conn.Close()
			return nil, fmt.Errorf("链式代理 Step2a CONNECT 发送失败: %w", err)
		}
		br := bufio.NewReader(conn)
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("链式代理 Step2b 读取 CONNECT 响应失败: %w", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			conn.Close()
			buf := make([]byte, 512)
			n, _ := br.Read(buf)
			return nil, fmt.Errorf("链式代理 Step2c 用户代理 CONNECT 拒绝: status=%d body=%s", resp.StatusCode, string(buf[:n]))
		}
		// 现在 conn 是双层隧道：本地->用户代理->目标主机。在这个连接上做 TLS 握手
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         strings.Split(targetHost, ":")[0],
			InsecureSkipVerify: false,
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, fmt.Errorf("链式代理 Step2d TLS 握手失败: %w", err)
		}
		// 现在 tlsConn 是加密的，把请求写过去
		if err := req.Write(tlsConn); err != nil {
			tlsConn.Close()
			return nil, fmt.Errorf("链式代理 Step2e 写请求失败: %w", err)
		}
		// 读响应
		resp, err = http.ReadResponse(bufio.NewReader(tlsConn), req)
		if err != nil {
			tlsConn.Close()
			return nil, fmt.Errorf("链式代理 Step2f 读响应失败: %w", err)
		}
		// 注意：不能简单关闭 tlsConn，需要在 resp.Body 关闭时同时关闭
		resp.Body = &closeBoth{ReadCloser: resp.Body, c: tlsConn}
		return resp, nil
	}

	// HTTP: 在已建立的(本地->用户代理)TCP隧道上，发送完整绝对URL请求，并带用户代理认证
	absURL := req.URL.String()
	// 构造新的请求报文：绝对 URI
	httpReqLine := fmt.Sprintf("%s %s HTTP/1.1\r\n", req.Method, absURL)
	// Host header
	headers := ""
	for k, vs := range req.Header {
		for _, v := range vs {
			headers += fmt.Sprintf("%s: %s\r\n", k, v)
		}
	}
	// 加上 Proxy-Authorization
	if ct.userProxyAuth != "" {
		headers += fmt.Sprintf("Proxy-Authorization: %s\r\n", ct.userProxyAuth)
	}
	// Connection: close 简化处理
	headers += "Connection: close\r\n\r\n"

	fullReq := httpReqLine + headers
	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			conn.Close()
			return nil, err
		}
		fullReq += string(bodyBytes)
	}

	if _, err := conn.Write([]byte(fullReq)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("链式代理 HTTP 写请求失败: %w", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("链式代理 HTTP 读响应失败: %w", err)
	}
	resp.Body = &closeBoth{ReadCloser: resp.Body, c: conn}
	return resp, nil
}

type closeBoth struct {
	io.ReadCloser
	c io.Closer
}

func (cb *closeBoth) Close() error {
	e1 := cb.ReadCloser.Close()
	e2 := cb.c.Close()
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
	Raw  string
}

func parseProxy(raw string) *Proxy {
	parts := strings.Split(raw, ":")
	if len(parts) == 4 {
		return &Proxy{IP: parts[0], Port: parts[1], User: parts[2], Pass: parts[3], Raw: raw}
	}
	return nil
}

func testChained(ctx context.Context, idx int, p *Proxy, wg *sync.WaitGroup, results chan<- string) {
	defer wg.Done()
	result := fmt.Sprintf("[%2d] %s:%s ", idx, p.IP, p.Port)

	// 构造用户代理 URL
	userProxyURL, _ := url.Parse(fmt.Sprintf("http://%s:%s@%s:%s", p.User, p.Pass, p.IP, p.Port))

	// 构造链式 Transport
	ct := newChainedTransport("127.0.0.1:18080", userProxyURL)
	client := &http.Client{Transport: ct, Timeout: 30 * time.Second}

	// 测试：通过双层代理访问 httpbin.org/ip
	start := time.Now()
	req1, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://httpbin.org/ip", nil)
	req1.Header.Set("User-Agent", "Mozilla/5.0")
	resp1, err := client.Do(req1)
	if err != nil {
		results <- result + fmt.Sprintf("❌ 链式访问失败 (%.1fs): %v", time.Since(start).Seconds(), err)
		return
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	result += fmt.Sprintf("✅出口=%s(%.1fs) ", strings.TrimSpace(string(body1)), time.Since(start).Seconds())

	// 测试2: 访问4399首页
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
	result += fmt.Sprintf("✅首页=%d(%.1fs,%d bytes) ", resp2.StatusCode, time.Since(start2).Seconds(), len(body2))

	// 测试3: regFrame.do
	start3 := time.Now()
	req3, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://ptlogin.4399.com/ptlogin/regFrame.do?appId=www_home&displayMode=popup&level=4&sec=1", nil)
	req3.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/129 Safari/537.36")
	req3.Header.Set("Referer", "https://www.4399.com/")
	resp3, err := client.Do(req3)
	if err != nil {
		results <- result + fmt.Sprintf("❌ regFrame失败 (%.1fs): %v", time.Since(start3).Seconds(), err)
		return
	}
	body3, _ := io.ReadAll(resp3.Body)
	resp3.Body.Close()
	result += fmt.Sprintf("✅regFrame=%d(%.1fs,%d bytes)", resp3.StatusCode, time.Since(start3).Seconds(), len(body3))
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
	fmt.Println("========== 链式代理批量测试（本地隧道 -> 短效动态代理 -> 公网）==========")
	fmt.Printf("时间: %s\n本地隧道: 127.0.0.1:18080\n共 %d 个代理\n\n",
		time.Now().Format("2006-01-02 15:04:05"), len(proxiesRaw))

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	results := make(chan string, len(proxiesRaw))
	for i, raw := range proxiesRaw {
		p := parseProxy(raw)
		if p == nil {
			continue
		}
		wg.Add(1)
		go testChained(ctx, i+1, p, &wg, results)
	}
	go func() { wg.Wait(); close(results) }()

	pass := 0
	total := 0
	for r := range results {
		total++
		if strings.Contains(r, "regFrame=") && strings.Contains(r, "regFrame=200") {
			pass++
		}
		fmt.Println(r)
	}
	fmt.Printf("\n========== 汇总 ==========\n")
	fmt.Printf("总计: %d | regFrame成功: %d (%.0f%%) | 失败: %d\n",
		total, pass, 100*float64(pass)/float64(total), total-pass)
}
