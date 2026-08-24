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
	"strings"
	"time"
)

// 单层代理简化版：直接通过 本地隧道 去连用户代理
// 对 HTTP 请求：通过 本地隧道 CONNECT 用户代理 -> 然后发送给用户代理的 CONNECT target（即使是HTTP，也走CONNECT用户代理，然后在用户代理层处理）
// 更简单的实现：把所有请求当成 HTTPS 风格，都用 CONNECT 到用户代理，然后在那个 TCP 上发送请求

func main() {
	fmt.Println("========== 单个链式代理详细调试 ==========")

	// 选择代理9: 182.247.255.69:43405 （刚才返回了407，说明TCP通了）
	proxyIP := "182.247.255.69"
	proxyPort := "43405"
	proxyUser := "ydl84074816"
	proxyPass := "CiuEhvwj"
	proxyAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(proxyUser+":"+proxyPass))
	localProxy := "127.0.0.1:18080"

	fmt.Printf("本地隧道: %s\n", localProxy)
	fmt.Printf("用户代理: http://%s:%s@%s:%s\n", proxyUser, "*****", proxyIP, proxyPort)
	fmt.Printf("认证头: Proxy-Authorization: %s\n\n", proxyAuth)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// ======= 测试1: 检查本地隧道是否接受 CONNECT 到用户代理 =======
	fmt.Println("[Test1] 本地隧道 CONNECT 用户代理...")
	var dialer net.Dialer
	conn1, err := dialer.DialContext(ctx, "tcp", localProxy)
	if err != nil {
		fmt.Printf("  ❌ 连不上本地隧道: %v\n", err)
		return
	}
	defer conn1.Close()
	target := proxyIP + ":" + proxyPort
	connectStr := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	fmt.Printf("  发送:\n%s\n", preview(connectStr, 200))
	conn1.Write([]byte(connectStr))
	br := bufio.NewReader(conn1)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		fmt.Printf("  ❌ 读响应失败: %v\n", err)
		return
	}
	resp.Body.Close()
	fmt.Printf("  CONNECT 响应状态: %d %s\n", resp.StatusCode, resp.Status)
	if resp.StatusCode != 200 {
		fmt.Println("  ❌ 本地隧道拒绝 CONNECT 用户代理")
		return
	}
	fmt.Println("  ✅ 成功：现在 conn1 是 本地隧道->用户代理 的 TCP 隧道！\n")

	// ======= 测试2: 验证用户代理是否需要认证 =======
	fmt.Println("[Test2] 在 conn1 上发送 HTTP 请求（带认证）访问 http://httpbin.org/ip ...")
	// HTTP 代理协议：发送 GET http://httpbin.org/ip HTTP/1.1 + Proxy-Authorization
	getReq := "GET http://httpbin.org/ip HTTP/1.1\r\n" +
		"Host: httpbin.org\r\n" +
		"User-Agent: Mozilla/5.0\r\n" +
		"Proxy-Authorization: " + proxyAuth + "\r\n" +
		"Accept: */*\r\n" +
		"Proxy-Connection: close\r\n\r\n"
	fmt.Printf("  发送请求 (前160字):\n  %s\n", preview(getReq, 160))
	conn1.Write([]byte(getReq))
	resp2, err := http.ReadResponse(br, nil)
	if err != nil {
		fmt.Printf("  ❌ 读响应失败: %v\n", err)
		return
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	fmt.Printf("  响应 status=%d size=%d bytes:\n  %s\n\n", resp2.StatusCode, len(body2), preview(string(body2), 200))

	if resp2.StatusCode != 200 {
		fmt.Println("  ⚠️  HTTP访问失败，尝试 HTTPS (CONNECT) 方式...")
	}

	// ======= 测试3: 访问 HTTPS 目标 =======
	fmt.Println("[Test3] 在 conn1 上，通过用户代理 CONNECT 到 www.4399.com:443 ...")
	// 重新建立 conn，因为上一个可能被关了
	conn3, err := dialer.DialContext(ctx, "tcp", localProxy)
	if err != nil {
		fmt.Printf("  ❌ 本地隧道连接失败: %v\n", err)
		return
	}
	defer conn3.Close()
	fmt.Fprintf(conn3, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	resp3a, _ := http.ReadResponse(bufio.NewReader(conn3), nil)
	resp3a.Body.Close()
	if resp3a.StatusCode != 200 {
		fmt.Printf("  ❌ 本地隧道 CONNECT 用户代理失败: %d\n", resp3a.StatusCode)
		return
	}

	// 现在 conn3 是 本地->用户代理。通过用户代理 CONNECT 到 ptlogin.4399.com:443
	httpsTarget := "ptlogin.4399.com:443"
	connectHTTPS := "CONNECT " + httpsTarget + " HTTP/1.1\r\n" +
		"Host: " + httpsTarget + "\r\n" +
		"Proxy-Authorization: " + proxyAuth + "\r\n\r\n"
	fmt.Printf("  发送给用户代理 CONNECT:\n%s\n", preview(connectHTTPS, 200))
	br3 := bufio.NewReader(conn3)
	fmt.Fprint(conn3, connectHTTPS)
	resp3b, err := http.ReadResponse(br3, nil)
	if err != nil {
		fmt.Printf("  ❌ 读用户代理 CONNECT 响应失败: %v\n", err)
		return
	}
	resp3b.Body.Close()
	fmt.Printf("  用户代理 CONNECT 响应: status=%d\n", resp3b.StatusCode)
	if resp3b.StatusCode != 200 {
		buf := make([]byte, 512)
		n, _ := br3.Read(buf)
		fmt.Printf("  响应 body=%s\n", string(buf[:n]))
		fmt.Println("  ❌ 用户代理拒绝 CONNECT 目标")
		return
	}
	fmt.Println("  ✅ 双重隧道建立成功！现在做 TLS 握手...")

	tlsConn := tls.Client(conn3, &tls.Config{ServerName: "ptlogin.4399.com"})
	if err := tlsConn.Handshake(); err != nil {
		fmt.Printf("  ❌ TLS 握手失败: %v\n", err)
		return
	}
	defer tlsConn.Close()
	fmt.Println("  ✅ TLS 握手成功！发送 HTTPS 请求...")

	fmt.Fprint(tlsConn, "GET /ptlogin/regFrame.do?appId=www_home&displayMode=popup&level=4&sec=1 HTTP/1.1\r\n"+
		"Host: ptlogin.4399.com\r\n"+
		"User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/129 Safari/537.36\r\n"+
		"Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8\r\n"+
		"Accept-Language: zh-CN,zh;q=0.9\r\n"+
		"Connection: close\r\n\r\n")
	resp3c, err := http.ReadResponse(bufio.NewReader(tlsConn), nil)
	if err != nil {
		fmt.Printf("  ❌ 读 HTTPS 响应失败: %v\n", err)
		return
	}
	body3, _ := io.ReadAll(resp3c.Body)
	resp3c.Body.Close()
	fmt.Printf("\n  🎉 最终响应: status=%d size=%d bytes\n", resp3c.StatusCode, len(body3))
	if resp3c.StatusCode == 200 {
		fmt.Println("  ✅✅✅ 链式代理完全可用！现在可以用短效动态代理换IP绕过风控了！")
		// 找 USESSIONID
		cookieRE := `"Set-Cookie"`
		_ = cookieRE
		// 打印 Set-Cookie
		for k, v := range resp3c.Header {
			if strings.ToLower(k) == "set-cookie" {
				for _, sv := range v {
					if strings.Contains(sv, "USESSIONID") {
						fmt.Printf("  🍪 %s\n", preview(sv, 100))
					}
				}
			}
		}
	}
}

func preview(s string, n int) string {
	s = strings.TrimSpace(s)
	lines := strings.Split(s, "\n")
	filtered := make([]string, 0, len(lines))
	for _, l := range lines {
		if l = strings.TrimRight(l, "\r "); l != "" {
			filtered = append(filtered, l)
		}
	}
	out := strings.Join(filtered, "\n")
	if len(out) > n {
		return out[:n] + "..."
	}
	return out
}
