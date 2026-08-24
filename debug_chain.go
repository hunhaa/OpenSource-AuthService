//go:build ignore

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

// 手工模拟双层代理的完整过程，打印每一步的实际字节
func main() {
	localTunnel := "127.0.0.1:18080"
	userProxy := "150.139.247.191:17303"
	user := "ydl84074816"
	pass := "CiuEhvwj"
	target := "httpbin.org:443"
	userAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))

	fmt.Println("== Step 1: TCP dial 本地隧道", localTunnel, "==")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", localTunnel)
	if err != nil {
		fmt.Println("FAIL dial tunnel:", err)
		return
	}
	fmt.Println("OK connected. Local addr:", conn.LocalAddr())

	fmt.Println("\n== Step 2: CONNECT 用户代理", userProxy, "to tunnel ==")
	step2 := "CONNECT " + userProxy + " HTTP/1.1\r\nHost: " + userProxy + "\r\n\r\n"
	fmt.Printf("SEND [%d bytes]:\n%s\n", len(step2), step2)
	if _, err := conn.Write([]byte(step2)); err != nil {
		fmt.Println("FAIL step2 write:", err)
		return
	}
	br := bufio.NewReader(conn)
	resp2, err := http.ReadResponse(br, nil)
	if err != nil {
		fmt.Println("FAIL step2 read:", err)
		return
	}
	fmt.Printf("RECV status=%d\n", resp2.StatusCode)
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		buf, _ := io.ReadAll(io.LimitReader(br, 512))
		fmt.Println("extra body:", string(buf))
		return
	}
	fmt.Println("OK — 到达用户代理")

	fmt.Println("\n== Step 3: CONNECT 目标", target, "to user proxy ==")
	step3 := "CONNECT " + target + " HTTP/1.1\r\n" +
		"Host: " + target + "\r\n" +
		"Proxy-Authorization: " + userAuth + "\r\n\r\n"
	fmt.Printf("SEND [%d bytes]:\n%s\n", len(step3), step3)
	if _, err := conn.Write([]byte(step3)); err != nil {
		fmt.Println("FAIL step3 write:", err)
		return
	}
	resp3, err := http.ReadResponse(br, nil)
	if err != nil {
		fmt.Println("FAIL step3 read:", err)
		return
	}
	fmt.Printf("RECV status=%d\n", resp3.StatusCode)
	resp3.Body.Close()
	if resp3.StatusCode != 200 {
		buf, _ := io.ReadAll(io.LimitReader(br, 512))
		fmt.Printf("extra body (len=%d): %q\n", len(buf), string(buf))
		if resp3.StatusCode == 407 {
			// 试一下明文认证？
			fmt.Println("\n== Step 3B: 重试 Plain user:pass 认证 在新连接上 ===")
			conn2, err := d.DialContext(ctx, "tcp", localTunnel)
			if err != nil {
				fmt.Println("FAIL new dial:", err)
				return
			}
			s2b := "CONNECT " + userProxy + " HTTP/1.1\r\nHost: " + userProxy + "\r\n\r\n"
			conn2.Write([]byte(s2b))
			br2 := bufio.NewReader(conn2)
			r2b, _ := http.ReadResponse(br2, nil)
			fmt.Printf("  tunnel CONNECT -> status=%d\n", r2b.StatusCode)
			r2b.Body.Close()
			if r2b.StatusCode == 200 {
				plainAuth := user + ":" + pass
				s3b := "CONNECT " + target + " HTTP/1.1\r\n" +
					"Host: " + target + "\r\n" +
					"Proxy-Authorization: " + plainAuth + "\r\n\r\n"
				fmt.Printf("  SEND Plain CONNECT (auth=%q)\n", plainAuth)
				conn2.Write([]byte(s3b))
				r3b, err := http.ReadResponse(br2, nil)
				if err != nil {
					fmt.Println("  FAIL Plain read:", err)
				} else {
					fmt.Printf("  Plain status=%d\n", r3b.StatusCode)
					buf2, _ := io.ReadAll(io.LimitReader(br2, 512))
					if len(buf2) > 0 {
						fmt.Println("  body:", string(buf2))
					}
				}
			}
			conn2.Close()
		}
		return
	}
	fmt.Println("OK — 到达目标站点隧道")

	fmt.Println("\n== Step 4: TLS 握手 ==")
	sn := strings.Split(target, ":")[0]
	tc := tls.Client(conn, &tls.Config{ServerName: sn})
	if err := tc.HandshakeContext(ctx); err != nil {
		fmt.Println("FAIL TLS:", err)
		return
	}
	fmt.Println("OK TLS handshake done")

	fmt.Println("\n== Step 5: 发送 HTTPS GET https://httpbin.org/ip ==")
	httpReq := "GET /ip HTTP/1.1\r\nHost: httpbin.org\r\nUser-Agent: curl/8.5.0\r\nAccept: */*\r\nConnection: close\r\n\r\n"
	fmt.Printf("SEND [%d bytes]\n", len(httpReq))
	if _, err := tc.Write([]byte(httpReq)); err != nil {
		fmt.Println("FAIL write:", err)
		return
	}
	resp5, err := http.ReadResponse(bufio.NewReader(tc), nil)
	if err != nil {
		fmt.Println("FAIL read:", err)
		return
	}
	defer resp5.Body.Close()
	body, _ := io.ReadAll(resp5.Body)
	fmt.Printf("status=%d, body len=%d:\n%s\n", resp5.StatusCode, len(body), string(body))
	tc.Close()
}
