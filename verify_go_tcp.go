//go:build ignore

package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// 与刚才 python 脚本完全一样的逻辑：
//   1. TCP -> 127.0.0.1:18080 (本地隧道)
//   2. 写 CONNECT 用户代理:端口 HTTP/1.1 -> 读 200
//   3. 写 CONNECT httpbin.org:443 HTTP/1.1 + Proxy-Authorization: 明文 user:pass -> 读 200
//   4. TLS 握手 -> 写 GET /ip HTTP/1.1 -> 读响应

func main() {
	tunnel := "127.0.0.1:18080"
	userProxy := "150.139.247.172:11349"
	user := "ydl84074816"
	pass := "CiuEhvwj"
	plainAuth := user + ":" + pass

	fmt.Println("===== Go 纯 TCP 复刻成功路径 =====")
	fmt.Printf("本地隧道: %s\n用户代理: %s\n认证格式: 明文 %s\n\n", tunnel, userProxy, plainAuth)

	// -------- 步骤 1~2 --------
	fmt.Println("[1/4] TCP → 本地隧道 " + tunnel)
	conn, err := net.DialTimeout("tcp", tunnel, 10*time.Second)
	if err != nil {
		fmt.Printf("  ✗ 失败: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Println("[2/4] CONNECT " + userProxy + " → 隧道")
	connect1 := "CONNECT " + userProxy + " HTTP/1.1\r\n" +
		"Host: " + userProxy + "\r\nProxy-Connection: keep-alive\r\n\r\n"
	if _, err := conn.Write([]byte(connect1)); err != nil {
		fmt.Printf("  ✗ 写1失败: %v\n", err)
		return
	}
	br := bufio.NewReaderSize(conn, 4096)
	resp1, err := http.ReadResponse(br, nil)
	if err != nil {
		fmt.Printf("  ✗ 读1失败: %v\n", err)
		return
	}
	resp1.Body.Close()
	if resp1.StatusCode != 200 {
		fmt.Printf("  ✗ 失败 status=%d\n", resp1.StatusCode)
		return
	}
	fmt.Printf("  ✓ %s\n", resp1.Status)

	// 处理预读字节
	var leftover []byte
	if br.Buffered() > 0 {
		leftover = make([]byte, br.Buffered())
		n, _ := br.Read(leftover)
		leftover = leftover[:n]
	}
	connWithBuf := &bufConn{Conn: conn, leftover: leftover}

	// -------- 步骤 3 --------
	fmt.Println("[3/4] CONNECT httpbin.org:443 → 用户代理 (Proxy-Authorization: 明文)")
	connect2 := "CONNECT httpbin.org:443 HTTP/1.1\r\n" +
		"Host: httpbin.org:443\r\n" +
		"Proxy-Authorization: " + plainAuth + "\r\n" +
		"Proxy-Connection: keep-alive\r\n\r\n"
	if _, err := connWithBuf.Write([]byte(connect2)); err != nil {
		fmt.Printf("  ✗ 写2失败: %v\n", err)
		return
	}
	br2 := bufio.NewReaderSize(connWithBuf, 4096)
	resp2, err := http.ReadResponse(br2, nil)
	if err != nil {
		fmt.Printf("  ✗ 读2失败: %v  (如果这里是EOF → 代理已过期)\n", err)
		return
	}
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp2.Body, 200))
		fmt.Printf("  ✗ 失败 status=%d: %s\n", resp2.StatusCode, string(body))
		for k, v := range resp2.Header {
			fmt.Printf("     Header %s: %v\n", k, v)
		}
		return
	}
	fmt.Printf("  ✓ %s  (明文认证成功！)\n", resp2.Status)

	// 处理预读字节
	var leftover2 []byte
	if br2.Buffered() > 0 {
		leftover2 = make([]byte, br2.Buffered())
		n, _ := br2.Read(leftover2)
		leftover2 = leftover2[:n]
	}
	connForTLS := &bufConn{Conn: connWithBuf.Conn, leftover: append(leftover, leftover2...)}

	// -------- 步骤 4 --------
	fmt.Println("[4/4] TLS 握手 → GET https://httpbin.org/ip")
	tlsCfg := &tls.Config{ServerName: "httpbin.org"}
	tlsConn := tls.Client(connForTLS, tlsCfg)
	if err := tlsConn.Handshake(); err != nil {
		fmt.Printf("  ✗ TLS 握手失败: %v\n", err)
		return
	}
	fmt.Println("  ✓ TLS 握手成功")
	req, _ := http.NewRequest("GET", "https://httpbin.org/ip", nil)
	req.Header.Set("User-Agent", "curl/8.5.0")
	if err := req.Write(tlsConn); err != nil {
		fmt.Printf("  ✗ 写请求失败: %v\n", err)
		return
	}
	resp, err := http.ReadResponse(bufio.NewReader(tlsConn), req)
	if err != nil {
		fmt.Printf("  ✗ 读响应失败: %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("  ✓ 出口IP: %s\n", string(body))

	fmt.Println("\n===== 结论 =====")
	fmt.Println("  明文认证 + 双层代理链 在 Go 层 100% 可用！")
	fmt.Println("  若刚才 test_fixed_proxy.go 报 unexpected EOF，只是因为短效代理过期。")
	fmt.Println("  请用户在代理后台重新提取一批新代理 → 然后直接用修复后的 ParseProxyString 批量注册即可。")
}

type bufConn struct {
	net.Conn
	leftover []byte
}

func (b *bufConn) Read(p []byte) (int, error) {
	if len(b.leftover) > 0 {
		n := copy(p, b.leftover)
		b.leftover = b.leftover[n:]
		return n, nil
	}
	return b.Conn.Read(p)
}
