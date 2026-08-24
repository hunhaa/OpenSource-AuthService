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

func main() {
	fmt.Println("========== 检查用户代理 407 响应细节 ==========")

	proxyIP := "182.247.255.69"
	proxyPort := "43405"
	proxyUser := "ydl84074816"
	proxyPass := "CiuEhvwj"
	proxyAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(proxyUser+":"+proxyPass))
	localProxy := "127.0.0.1:18080"

	// 验证认证字符串
	decoded, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(proxyAuth, "Basic "))
	fmt.Printf("验证: Basic base64 解码后 = %q (期望 %q)\n\n", string(decoded), proxyUser+":"+proxyPass)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", localProxy)
	if err != nil {
		fmt.Printf("❌ 本地隧道失败: %v\n", err)
		return
	}
	defer conn.Close()

	// Step1: 本地 CONNECT 用户代理
	target := proxyIP + ":" + proxyPort
	fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	br := bufio.NewReader(conn)
	resp1, _ := http.ReadResponse(br, nil)
	resp1.Body.Close()
	if resp1.StatusCode != 200 {
		fmt.Printf("❌ 本地隧道 CONNECT 失败: %d\n", resp1.StatusCode)
		return
	}
	fmt.Printf("✅ 本地隧道->用户代理 TCP 隧道建立\n\n")

	// Step2: 在隧道上发送 HTTP 请求，先不发送认证，看看 407 的头
	fmt.Println("--- 不发送认证，观察 407 响应头 ---")
	fmt.Fprint(conn, "GET http://httpbin.org/ip HTTP/1.1\r\nHost: httpbin.org\r\nUser-Agent: curl/7.81\r\nAccept: */*\r\nProxy-Connection: close\r\n\r\n")
	resp2, err := http.ReadResponse(br, nil)
	if err != nil {
		fmt.Printf("❌ 读失败: %v\n", err)
		return
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	fmt.Printf("status=%d\n", resp2.StatusCode)
	for k, vs := range resp2.Header {
		for _, v := range vs {
			fmt.Printf("  %-30s: %s\n", k, v)
		}
	}
	fmt.Printf("  Body: %s\n\n", string(body2))

	// Step3: 重新建立连接，这次带认证
	fmt.Println("--- 带 Proxy-Authorization 发送请求 ---")
	conn3, _ := dialer.DialContext(ctx, "tcp", localProxy)
	defer conn3.Close()
	fmt.Fprintf(conn3, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	br3 := bufio.NewReader(conn3)
	resp3a, _ := http.ReadResponse(br3, nil)
	resp3a.Body.Close()
	fmt.Printf("✅ 隧道重建\n")

	// 逐字输出认证头，检查是否有空格
	fmt.Printf("发送的 Proxy-Authorization: 字节=%v\n\n", []byte(proxyAuth))

	// 方式：用 Proxy 前缀变体（有些代理需要区分大小写或格式）
	variants := []struct {
		Name  string
		Value string
	}{
		{"标准小写+换行", "proxy-authorization: " + proxyAuth + "\r\n"},
		{"大写标准", "Proxy-Authorization: " + proxyAuth + "\r\n"},
		{"带空格后", "Proxy-Authorization:  " + proxyAuth + "\r\n"},
		{"不用Basic前缀直接传", "Proxy-Authorization: " + proxyUser + ":" + proxyPass + "\r\n"},
	}

	getBase := "GET http://httpbin.org/ip HTTP/1.1\r\n" +
		"Host: httpbin.org\r\n" +
		"User-Agent: curl/7.81\r\n" +
		"Accept: */*\r\n"

	for i, v := range variants {
		fmt.Printf("[变体%d] %s\n", i+1, v.Name)
		// 发送变体请求
		if i > 0 {
			// 重建连接，因为之前的可能被关闭了
			connX, _ := dialer.DialContext(ctx, "tcp", localProxy)
			fmt.Fprintf(connX, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
			brX := bufio.NewReader(connX)
			rx, _ := http.ReadResponse(brX, nil)
			rx.Body.Close()
			reqX := getBase + v.Value + "Proxy-Connection: close\r\n\r\n"
			fmt.Fprint(connX, reqX)
			respX, err := http.ReadResponse(brX, nil)
			if err != nil {
				fmt.Printf("  ❌ 失败: %v\n", err)
				connX.Close()
				continue
			}
			bx, _ := io.ReadAll(respX.Body)
			respX.Body.Close()
			fmt.Printf("  status=%d body=%s\n", respX.StatusCode, preview(string(bx), 200))
			connX.Close()
			if respX.StatusCode == 200 {
				fmt.Printf("  🎉 变体%d成功！\n\n", i+1)
				break
			}
		} else {
			reqX := getBase + v.Value + "Proxy-Connection: close\r\n\r\n"
			fmt.Fprint(conn3, reqX)
			respX, err := http.ReadResponse(br3, nil)
			if err != nil {
				fmt.Printf("  ❌ 失败: %v\n", err)
				continue
			}
			bx, _ := io.ReadAll(respX.Body)
			respX.Body.Close()
			fmt.Printf("  status=%d body=%s\n", respX.StatusCode, preview(string(bx), 200))
			if respX.StatusCode == 200 {
				fmt.Printf("  🎉 变体%d成功！\n\n", i+1)
				break
			}
		}
	}

	// 最后验证：不用本地隧道（直接连），看看能不能通（确认用户密码正确）
	fmt.Println("\n--- 最后：跳过本地隧道，直连用户代理（验证用户名密码是否正确）---")
	fmt.Println("   预期: 沙箱直连可能被拦截（超时或失败），但如果用户名密码正确，本地网络能通")
	directCtx, directCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer directCancel()
	directConn, err := dialer.DialContext(directCtx, "tcp", proxyIP+":"+proxyPort)
	if err != nil {
		fmt.Printf("   直连失败（预期沙箱出口拦截）: %v\n", err)
	} else {
		defer directConn.Close()
		fmt.Println("   直连成功！（说明不用本地隧道也能出）现在测试用户名密码...")
		fmt.Fprint(directConn, getBase+"Proxy-Authorization: "+proxyAuth+"\r\nProxy-Connection: close\r\n\r\n")
		dr, err := http.ReadResponse(bufio.NewReader(directConn), nil)
		if err != nil {
			fmt.Printf("   直连读失败: %v\n", err)
		} else {
			db, _ := io.ReadAll(dr.Body)
			dr.Body.Close()
			fmt.Printf("   直连 status=%d body=%s\n", dr.StatusCode, preview(string(db), 200))
			if dr.StatusCode == 200 {
				fmt.Println("   ✅ 直连+认证成功！用户名密码正确！")
			} else if dr.StatusCode == 407 {
				fmt.Println("   ❌ 直连仍407，用户名密码本身错误或代理已过期！")
			}
		}
	}

	fmt.Println("\n=== TLS 测试结束标记 ===")
	_ = tls.Client // avoid unused
}

func preview(s string, n int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
