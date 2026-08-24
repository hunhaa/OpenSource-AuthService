//go:build ignore

package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"
)

// 用环境变量方式 + curl 风格的直连测试
// 对比两种方式：
//   A: 环境变量 HTTP_PROXY=http://user:pass@local_tunnel ??? 不对
//   应该是：先设置本地隧道为出网，然后用标准方式构造代理URL（http://user:pass@ip:port）
//   实际上在沙箱环境里，默认所有流量要先过 127.0.0.1:18080，所以可以设置
//   HTTP_PROXY=http://127.0.0.1:18080 然后在代理里再指定用户的代理？不，不支持这种双层。
//
// 换一种思路：直接用标准库，通过 ProxyURL 指向"用户代理"但设置 DialContext 走本地隧道 CONNECT——
// 这就是 ChainedTransport 做的事情。但现在 HTTP 分支报错 unexpected EOF，说明代理服务器
// 在 Plain HTTP 模式下（非 CONNECT）不支持我们的请求格式。
//
// 让我们把所有 HTTP 请求都**强制走 HTTPS 分支的逻辑**——即对 HTTP 目标也先发 CONNECT，
// 然后再发明文 HTTP（很多代理供应商只支持 CONNECT 模式，不支持绝对URI 转发模式）。

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

type r struct {
	i    int
	p    string
	ok   bool
	ip   string
	info string
}

func testStdLibrary(idx int, proxy string, wg *sync.WaitGroup, ch chan<- r) {
	defer wg.Done()
	res := r{i: idx, p: proxy}
	parts := split4(proxy)

	// ===== 方式：本地隧道 + 标准 &http.Transport{Proxy: ...} =====
	// 先设置 HTTP_PROXY=http://127.0.0.1:18080 作为出网（因为沙箱不能直连）
	// 但是 http.Transport 的 Proxy 字段不能叠两层，所以我们改用：
	// DialContext 先连接本地隧道，再向隧道发 CONNECT 用户代理，
	// 然后在此连接上进行 HTTP(S) 操作。这其实就是 ChainedTransport，
	// 但是换一种更健壮的实现 —— 对所有请求（包括HTTP）都使用 CONNECT 模式。

	os.Setenv("HTTP_PROXY", "http://127.0.0.1:18080")
	os.Setenv("HTTPS_PROXY", "http://127.0.0.1:18080")

	// 构造标准代理 URL（带 Basic 认证）
	var proxyURL *url.URL
	if len(parts) == 4 {
		proxyURL, _ = url.Parse(fmt.Sprintf("http://%s:%s@%s:%s",
			url.QueryEscape(parts[2]), url.QueryEscape(parts[3]), parts[0], parts[1]))
	} else if len(parts) == 2 {
		proxyURL, _ = url.Parse(fmt.Sprintf("http://%s:%s", parts[0], parts[1]))
	}
	if proxyURL == nil {
		res.info = "invalid format"
		ch <- res
		return
	}

	// 这里 Transport 的 Proxy 会先被 ProxyFromEnvironment 抓到本地隧道，
	// 然后再通过隧道 CONNECT 到 proxyURL？不，http.Transport 只认一个 Proxy。
	// 所以这会失败——我们需要一个不同的办法。
	//
	// 让我们用最简单的办法：用 curl 命令行来测（curl 支持 --proxy http://user:pass@ip:port 且通过环境变量预代理）
	_ = proxyURL
	ch <- res
}

func split4(s string) []string {
	p := make([]string, 0, 4)
	cur := ""
	for _, c := range s {
		if c == ':' {
			p = append(p, cur)
			cur = ""
		} else {
			cur += string(c)
		}
	}
	p = append(p, cur)
	return p
}

func main() {
	log.Println("===== 方式A: curl 命令测试（最可靠） =====")
	var wg sync.WaitGroup
	ch := make(chan r, len(testProxies))
	sem := make(chan struct{}, 2)

	for i, p := range testProxies {
		wg.Add(1)
		sem <- struct{}{}
		go func(ii int, pp string) {
			defer func() { <-sem }()
			defer wg.Done()
			rr := r{i: ii, p: pp}
			parts := split4(pp)
			if len(parts) != 4 {
				rr.info = "not 4-part"
				ch <- rr
				return
			}
			ip, port, user, pass := parts[0], parts[1], parts[2], parts[3]

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			// curl: -x http://user:pass@ip:port (通过本地隧道出网)
			// 注意：curl 会自动应用 HTTP_PROXY 环境变量吗？不会，-x 会覆盖。
			// 我们需要的是 curl 使用 preproxy (127.0.0.1:18080) 再连到用户代理。
			// 这正是 curl --proxy http://user:pass@ip:port 的行为，但沙箱环境需要先过隧道。
            // 解决方案：利用 curl 的 --preproxy 选项
            // 或者：构造 curl 先 -x localhost:18080，然后再在请求里加 Proxy-Authorization？
			// 
			// 简化：直接走之前成功过的脚本——利用环境变量 HTTP_PROXY=127.0.0.1:18080
			// 然后指定用户的代理作为 --proxy（不，curl 不会代理代理）
			// 
			// 用 --proxy http://user:pass@ip:port 并同时设置 --noproxy ""——
			// 不，这样是直连用户代理，沙箱里直连不了。
			// 
			// 还是用我们的 Go ChainedTransport，但是修掉 unexpected EOF bug
			// 先测 HTTPS 站点，因为 CONNECT 分支更可靠

			// 让我们直接用 Go 代码 + ChainedTransport 只测 HTTPS 端点
			// (测试 4399 注册页本身就是 HTTPS，所以足够了)

			_ = ctx
			_ = ip
			_ = port
			_ = user
			_ = pass
			rr.info = "skipped, run go_test_v2.go instead"
			ch <- rr
		}(i, p)
	}
	go func() { wg.Wait(); close(ch) }()
	for range ch {
	}
	log.Println("请运行下一个脚本: go run test_proxies_v2.go")
}
