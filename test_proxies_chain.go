//go:build ignore

package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// ========= 使用 proxy_chain.go 同款 TunneledDialer 架构 =========
// 目的：验证沙箱环境下 本地隧道(127.0.0.1:18080) + 用户代理 的双层代理链路
// 同时测试 明文/Basic 两种 Proxy-Authorization 格式

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

// ========== bufferedConn & tunneledDialer (拷贝自 proxy_chain.go) ==========

type bufferedConn struct {
	net.Conn
	buf []byte
}

func (bc *bufferedConn) Read(p []byte) (int, error) {
	if len(bc.buf) > 0 {
		n := copy(p, bc.buf)
		bc.buf = bc.buf[n:]
		return n, nil
	}
	return bc.Conn.Read(p)
}

type tunneledDialer struct {
	TunnelAddr string
	NextHop    string
	Dialer     net.Dialer
}

func (td *tunneledDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("tunneled dialer only supports tcp (got %q)", network)
	}
	target := td.NextHop
	if target == "" {
		target = addr
	}
	conn, err := td.Dialer.DialContext(ctx, "tcp", td.TunnelAddr)
	if err != nil {
		return nil, fmt.Errorf("tunnel dial %s: %w", td.TunnelAddr, err)
	}
	connectReq := "CONNECT " + target + " HTTP/1.1\r\n" +
		"Host: " + target + "\r\n" +
		"Proxy-Connection: keep-alive\r\n\r\n"
	if _, err := conn.Write([]byte(connectReq)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("tunnel CONNECT write: %w", err)
	}
	br := bufio.NewReaderSize(conn, 4096)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("tunnel CONNECT read: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("tunnel CONNECT %s rejected status=%d", target, resp.StatusCode)
	}
	if br.Buffered() == 0 {
		return conn, nil
	}
	left := make([]byte, br.Buffered())
	n, err := br.Read(left)
	if err != nil && err != io.EOF {
		conn.Close()
		return nil, fmt.Errorf("tunnel drain buffered: %w", err)
	}
	left = left[:n]
	return &bufferedConn{Conn: conn, buf: left}, nil
}

func probeLocalTunnel(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func detectLocalTunnel() string {
	for _, ev := range []string{"HTTPS_PROXY", "HTTP_PROXY", "https_proxy", "http_proxy"} {
		v := strings.TrimSpace(os.Getenv(ev))
		if v == "" {
			continue
		}
		u, err := url.Parse(v)
		if err == nil && u.Host != "" {
			if probeLocalTunnel(u.Host) {
				return u.Host
			}
		}
		if probeLocalTunnel(v) {
			return v
		}
	}
	if probeLocalTunnel("127.0.0.1:18080") {
		return "127.0.0.1:18080"
	}
	return ""
}

// ========== 每种组合的测试结果 ==========

type caseRes struct {
	idx        int
	proxy      string
	authFmt    string // "plain" / "basic"
	useTunnel  bool   // 是否走双层代理
	tunnelAddr string
	binIP      string
	binOK      bool
	_4399OK    bool
	_4399Code  int
	err        string
	dur        time.Duration
}

func buildTransport(ip, port, user, pass, authFmt string, useTunnel bool, tunnelAddr string) (*http.Transport, error) {
	hostport := ip + ":" + port
	var proxyURL *url.URL
	if user != "" {
		proxyURL, _ = url.Parse(fmt.Sprintf("http://%s:%s@%s:%s",
			url.PathEscape(user), url.PathEscape(pass), ip, port))
	} else {
		proxyURL, _ = url.Parse(fmt.Sprintf("http://%s:%s", ip, port))
	}

	transport := &http.Transport{
		Proxy:               http.ProxyURL(proxyURL),
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 12 * time.Second,
	}

	if user != "" {
		var authVal string
		if authFmt == "basic" {
			authVal = "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
		} else {
			authVal = user + ":" + pass // 明文
		}
		transport.ProxyConnectHeader = http.Header{
			"Proxy-Authorization": []string{authVal},
		}
	}

	if useTunnel && tunnelAddr != "" {
		td := &tunneledDialer{
			TunnelAddr: tunnelAddr,
			NextHop:    hostport,
			Dialer: net.Dialer{
				Timeout:   15 * time.Second,
				KeepAlive: 30 * time.Second,
			},
		}
		transport.DialContext = td.DialContext
	} else {
		transport.DialContext = (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext
	}

	return transport, nil
}

func runCase(idx int, proxy, ip, port, user, pass, authFmt string, useTunnel bool, tunnelAddr string, ch chan<- caseRes) {
	t0 := time.Now()
	res := caseRes{
		idx:        idx,
		proxy:      proxy,
		authFmt:    authFmt,
		useTunnel:  useTunnel,
		tunnelAddr: tunnelAddr,
	}

	transport, err := buildTransport(ip, port, user, pass, authFmt, useTunnel, tunnelAddr)
	if err != nil {
		res.err = fmt.Sprintf("build transport: %v", err)
		ch <- res
		return
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}

	// 1) httpbin.org/ip
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

	// 2) 4399 注册页
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

func shortIP(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 50 {
		return s[:50] + "..."
	}
	if s == "" {
		return "(none)"
	}
	return s
}

func main() {
	tunnel := detectLocalTunnel()
	log.Printf("===== 双层代理链测试 =====")
	log.Printf("本地隧道探测: %s", func() string {
		if tunnel == "" {
			return "未发现 (将直连用户代理)"
		}
		return tunnel + "  (将构造双层代理链)"
	}())
	log.Printf("测试维度: %d 代理 × 2认证(明文/Basic) × %s\n",
		len(testProxies), func() string {
			if tunnel == "" {
				return "1模式(直连)"
			}
			return "2模式(直连/双层)"
		}())

	// 构造测试 case
	type testCase struct {
		idx       int
		proxy     string
		ip, port  string
		user, pass string
		authFmt   string
		useTunnel bool
	}
	var cases []testCase
	for i, p := range testProxies {
		ip, port, user, pass := split4(p)
		if ip == "" {
			continue
		}
		for _, af := range []string{"plain", "basic"} {
			// 直连模式（不走本地隧道）
			cases = append(cases, testCase{i, p, ip, port, user, pass, af, false})
			// 双层模式（走本地隧道）
			if tunnel != "" {
				cases = append(cases, testCase{i, p, ip, port, user, pass, af, true})
			}
		}
	}

	ch := make(chan caseRes, len(cases))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)

	for _, c := range cases {
		wg.Add(1)
		sem <- struct{}{}
		go func(c testCase) {
			defer func() { <-sem }()
			defer wg.Done()
			runCase(c.idx, c.proxy, c.ip, c.port, c.user, c.pass, c.authFmt, c.useTunnel, tunnel, ch)
		}(c)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()

	// 收集结果
	type perProxy struct {
		plainDirect, basicDirect, plainTunnel, basicTunnel *caseRes
	}
	byProxy := make(map[int]*perProxy, len(testProxies))
	for r := range ch {
		if byProxy[r.idx] == nil {
			byProxy[r.idx] = &perProxy{}
		}
		pp := byProxy[r.idx]
		switch {
		case !r.useTunnel && r.authFmt == "plain":
			pp.plainDirect = &r
		case !r.useTunnel && r.authFmt == "basic":
			pp.basicDirect = &r
		case r.useTunnel && r.authFmt == "plain":
			pp.plainTunnel = &r
		case r.useTunnel && r.authFmt == "basic":
			pp.basicTunnel = &r
		}
	}

	// 输出
	fmt.Println()
	passCount := 0
	successCases := []caseRes{}
	for i := 0; i < len(testProxies); i++ {
		pp := byProxy[i]
		fmt.Printf("───── [%02d] %s ─────\n", i+1, testProxies[i])

		printOne := func(label string, r *caseRes) {
			if r == nil {
				return
			}
			st := "  ✗"
			if r.binOK && r._4399OK {
				st = "  ✓ PASS"
				passCount++
				successCases = append(successCases, *r)
			} else if r.binOK {
				st = "  △ httpbinOK"
			}
			fmt.Printf("%s %-16s auth=%-6s 出口=%s   4399=%v(%d)  %v\n",
				st, label, r.authFmt, shortIP(r.binIP), r._4399OK, r._4399Code, r.dur.Round(100*time.Millisecond))
			if r.err != "" && !(r.binOK && r._4399OK) {
				fmt.Printf("        Err: %s\n", r.err)
			}
		}
		printOne("直连-明文", pp.plainDirect)
		printOne("直连-Basic", pp.basicDirect)
		if tunnel != "" {
			printOne("双层-明文", pp.plainTunnel)
			printOne("双层-Basic", pp.basicTunnel)
		}
	}

	fmt.Println()
	fmt.Println("================ 汇总 ================")
	fmt.Printf("代理数: %d\n", len(testProxies))
	fmt.Printf("通过case数: %d / %d\n", passCount, len(cases))

	if passCount > 0 {
		fmt.Println("\n✓ 可用的代理+认证+链路组合：")
		for _, r := range successCases {
			mode := "直连"
			if r.useTunnel {
				mode = "双层(" + r.tunnelAddr + ")"
			}
			fmt.Printf("  %s    # auth=%s  mode=%s  出口=%s\n",
				r.proxy, r.authFmt, mode, shortIP(r.binIP))
		}
	} else {
		fmt.Println("\n⚠️  全部失败！")
		fmt.Println("  可能原因:")
		fmt.Println("  1) 代理已过期（短效通常1-5分钟）")
		fmt.Println("  2) 用户名/密码错误 ydl84074816:CiuEhvwj")
		fmt.Println("  3) 沙箱网络出口被拦截")
		if tunnel != "" {
			fmt.Println("\n  快速诊断——在终端执行这两条命令：")
			fmt.Printf("    # A. 检查隧道到用户代理的CONNEECT：\n")
			for _, p := range testProxies[:3] {
				ip, port, _, _ := split4(p)
				fmt.Printf("    curl -v -x http://127.0.0.1:18080 -p --proxy-user ydl84074816:CiuEhvwj https://%s:%s/ 2>&1 | head -20\n", ip, port)
			}
		}
	}
}
