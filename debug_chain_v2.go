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

// 完全复刻 curl --preproxy :19090 -x http://user:pass@ip:port https://httpbin.org/ip
// 逐步打印每一步状态，确认每个字节都一致。
func main() {
	tunnel := "127.0.0.1:18080"
	userProxy := "150.139.247.191:17303"
	user := "ydl84074816"
	pass := "CiuEhvwj"
	target := "httpbin.org:443"
	basicAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))

	// 1. TCP -> tunnel
	d := net.Dialer{Timeout: 15 * time.Second}
	ctx := context.Background()
	fmt.Println(">>> 1. TCP dial", tunnel)
	c, err := d.DialContext(ctx, "tcp", tunnel)
	if err != nil {
		fmt.Println("FAIL 1:", err)
		return
	}
	defer c.Close()

	// 2. CONNECT userProxy -> tunnel (复刻 curl 不追加多余header)
	fmt.Println(">>> 2. CONNECT user proxy -> tunnel")
	cr := "CONNECT " + userProxy + " HTTP/1.1\r\n" +
		"Host: " + userProxy + "\r\n" +
		"Proxy-Connection: Keep-Alive\r\n" +
		"Connection: Keep-Alive\r\n\r\n"
	fmt.Printf("SEND (%d bytes):\n%s", len(cr), cr)
	c.Write([]byte(cr))

	br := bufio.NewReaderSize(c, 8192)
	r, err := http.ReadResponse(br, nil)
	if err != nil {
		fmt.Println("FAIL 2 read:", err)
		return
	}
	fmt.Printf("RECV: status=%d\n", r.StatusCode)
	r.Body.Close()
	if r.StatusCode != 200 {
		return
	}
	fmt.Printf("     bufio 预读了 %d 字节\n", br.Buffered())

	// 3. CONNECT target -> userProxy (追加 User-Agent/Connection/Proxy-Connection 让代理服务器友好)
	fmt.Println("\n>>> 3. CONNECT target -> user proxy")
	ct := "CONNECT " + target + " HTTP/1.1\r\n" +
		"Host: " + target + "\r\n" +
		"User-Agent: curl/8.5.0\r\n" +
		"Proxy-Authorization: " + basicAuth + "\r\n" +
		"Proxy-Connection: Keep-Alive\r\n" +
		"Connection: Keep-Alive\r\n\r\n"
	fmt.Printf("SEND (%d bytes):\n%s", len(ct), ct)
	c.Write([]byte(ct))

	r3, err := http.ReadResponse(br, nil)
	if err != nil {
		fmt.Println("FAIL 3 read:", err)
		// 打印原始可读字节
		left := make([]byte, 512)
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _ := c.Read(left)
		if n > 0 {
			fmt.Printf("    Raw bytes after error: %q\n", string(left[:n]))
		}
		return
	}
	fmt.Printf("RECV: status=%d\n", r3.StatusCode)
	r3.Body.Close()
	if r3.StatusCode != 200 {
		left := make([]byte, 1024)
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _ := c.Read(left)
		if n > 0 {
			fmt.Printf("    Body: %s\n", string(left[:n]))
		}
		return
	}

	// 4. TLS 握手
	fmt.Println("\n>>> 4. TLS handshake")
	sn := strings.Split(target, ":")[0]
	tc := tls.Client(c, &tls.Config{ServerName: sn})
	if err := tc.HandshakeContext(ctx); err != nil {
		fmt.Println("FAIL TLS:", err)
		return
	}
	fmt.Println("OK TLS")

	// 5. 发送 GET https://httpbin.org/ip
	fmt.Println("\n>>> 5. GET /ip HTTP/1.1 (通过 TLS)")
	httpStr := "GET /ip HTTP/1.1\r\n" +
		"Host: httpbin.org\r\n" +
		"User-Agent: curl/8.5.0\r\n" +
		"Accept: */*\r\n" +
		"Connection: close\r\n\r\n"
	tc.Write([]byte(httpStr))
	resp, err := http.ReadResponse(bufio.NewReader(tc), nil)
	if err != nil {
		fmt.Println("FAIL resp:", err)
		return
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	fmt.Printf("status=%d body=%s\n", resp.StatusCode, string(b))
}
