package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func main() {
	fmt.Println("========== 单个代理调试（对比成功 vs ChainedTransport）==========\n")

	localProxy := "127.0.0.1:18080"
	userProxy := "182.247.255.69:43405"
	userAuth := "ydl84074816:CiuEhvwj"

	// ============== 方式A: 完全复刻 proxy_auth/main.go 变体4 (之前成功) ==============
	fmt.Println("=== [方式A] 复刻之前成功代码 ===")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var d net.Dialer
	connA, err := d.DialContext(ctx, "tcp", localProxy)
	if err != nil {
		fmt.Printf("连隧道失败: %v\n", err)
		return
	}
	defer connA.Close()
	// 1. CONNECT 用户代理
	fmt.Fprintf(connA, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", userProxy, userProxy)
	respA1, _ := http.ReadResponse(bufio.NewReader(connA), nil)
	respA1.Body.Close()
	if respA1.StatusCode != 200 {
		fmt.Printf("隧道 CONNECT 失败: %d\n", respA1.StatusCode)
		return
	}
	fmt.Println("隧道->用户代理: 已建立")

	// 2. 发送 GET http://httpbin.org/ip
	getBase := "GET http://httpbin.org/ip HTTP/1.1\r\n" +
		"Host: httpbin.org\r\n" +
		"User-Agent: curl/7.81\r\n" +
		"Accept: */*\r\n"
	reqA := getBase + "Proxy-Authorization: " + userAuth + "\r\nProxy-Connection: close\r\n\r\n"
	fmt.Printf("--- 发送请求 ---\n%q\n", reqA)
	fmt.Println("--- hex 前120字节 ---")
	for i := 0; i < len(reqA) && i < 120; i++ {
		fmt.Printf("%02x ", reqA[i])
		if (i+1)%16 == 0 {
			fmt.Println()
		}
	}
	fmt.Println()

	fmt.Fprint(connA, reqA)
	respA2, err := http.ReadResponse(bufio.NewReader(connA), nil)
	if err != nil {
		fmt.Printf("❌ 响应失败: %v\n", err)
		return
	}
	bA, _ := io.ReadAll(respA2.Body)
	respA2.Body.Close()
	fmt.Printf("[方式A] ✅ status=%d body=%s\n\n", respA2.StatusCode, preview(string(bA), 200))

	// ============== 方式B: ChainedTransport.RoundTrip 的代码 ==============
	fmt.Println("=== [方式B] 模拟 ChainedTransport HTTP 逻辑 ===")
	connB, err := d.DialContext(ctx, "tcp", localProxy)
	if err != nil {
		fmt.Printf("连隧道失败: %v\n", err)
		return
	}
	defer connB.Close()
	fmt.Fprintf(connB, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", userProxy, userProxy)
	respB1, _ := http.ReadResponse(bufio.NewReader(connB), nil)
	respB1.Body.Close()
	if respB1.StatusCode != 200 {
		fmt.Printf("隧道 CONNECT 失败: %d\n", respB1.StatusCode)
		return
	}
	fmt.Println("隧道->用户代理: 已建立")

	// 构造一个 http.Request
	reqB, _ := http.NewRequest("GET", "http://httpbin.org/ip", nil)
	reqB.Header.Set("User-Agent", "curl/7.81")
	reqB.Header.Set("Accept", "*/*")

	abs := reqB.URL.String()
	fmt.Printf("req.URL.Host = %q\n", reqB.URL.Host)
	fmt.Printf("abs = %q\n", abs)

	hdr := ""
	for k, vs := range reqB.Header {
		if strings.EqualFold(k, "Host") || strings.EqualFold(k, "Proxy-Authorization") || strings.EqualFold(k, "Proxy-Connection") {
			fmt.Printf("  跳过 header: %s\n", k)
			continue
		}
		for _, v := range vs {
			hdr += fmt.Sprintf("%s: %s\r\n", k, v)
			fmt.Printf("  拼接 header: %s: %s\n", k, v)
		}
	}
	fmt.Printf("中间 hdr = %q\n", hdr)

	full := fmt.Sprintf("%s %s HTTP/1.1\r\nHost: %s\r\n%sProxy-Authorization: %s\r\nProxy-Connection: close\r\n\r\n",
		reqB.Method, abs, reqB.URL.Host, hdr, userAuth)
	fmt.Printf("--- 发送请求 ---\n%q\n", full)
	fmt.Println("--- hex 前120字节 ---")
	for i := 0; i < len(full) && i < 120; i++ {
		fmt.Printf("%02x ", full[i])
		if (i+1)%16 == 0 {
			fmt.Println()
		}
	}
	fmt.Println()

	fmt.Print(connB, full) // 不对，用 Fprint
	fmt.Fprint(connB, full)
	respB2, err := http.ReadResponse(bufio.NewReader(connB), nil)
	if err != nil {
		fmt.Printf("❌ 响应失败: %v\n", err)
		return
	}
	bB, _ := io.ReadAll(respB2.Body)
	respB2.Body.Close()
	fmt.Printf("[方式B] status=%d body=%s\n", respB2.StatusCode, preview(string(bB), 200))

	if respA2.StatusCode == respB2.StatusCode {
		fmt.Println("\n👉 A 和 B 结果一致")
	} else {
		fmt.Println("\n👉 ❌ A 成功 但 B 失败！需要对比请求字符串")
		fmt.Printf("A 请求长度: %d\nB 请求长度: %d\n", len(reqA), len(full))
		// 差异对比
		minLen := len(reqA)
		if len(full) < minLen {
			minLen = len(full)
		}
		for i := 0; i < minLen; i++ {
			if reqA[i] != full[i] {
				fmt.Printf("  第%d字节不同: A=%02x(%c) B=%02x(%c)\n", i, reqA[i], printable(reqA[i]), full[i], printable(full[i]))
				if i > 50 {
					break
				}
			}
		}
	}
}

func printable(b byte) string {
	if 32 <= b && b < 127 {
		return string(rune(b))
	}
	return "."
}

func preview(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
