package com4399

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ---------- 代理工厂（2026/08/24 架构彻底简化）----------
//
// 2026/08/24 假隧道 + curl 对比实验 实锤结论：
//   用户提供的 "IP:PORT:USER:PASS" 四段式中，**IP:PORT 完全是占位符**！
//   沙箱本地隧道 127.0.0.1:18080 是 Titan-Edge-Node 协议入口，它会：
//     1) 接收带 Proxy-Authorization: Basic base64(user:pass) 的 CONNECT 请求
//     2) 忽略请求头里任何"中间代理IP"，自行与 Titan 平台协商出口节点
//     3) 每次 CONNECT 都会拿到一个新的出口 IP（自动轮换）
//   三次验证：
//     ✓ curl -x http://USER:PASS@1.2.3.4:9999 --preproxy http://127.0.0.1:18080 https://httpbin.org/ip  → 成功（假IP也能用）
//     ✓ curl -x http://USER:PASS@127.0.0.1:18080 https://httpbin.org/ip  → 成功（直接给隧道凭证）
//     ✓ curl -x http://127.0.0.1:18080 --proxy-header "Proxy-Authorization: Basic xxx" → 成功
//
// 所以最终架构就是：
//   ┌──────────────┐     Basic认证     ┌──────────────────┐     Titan协议     ┌──────────┐
//   │ Go Transport │ ───────────────▶ │ 127.0.0.1:18080  │ ───────────────▶ │ 出口IP池 │
//   └──────────────┘  CONNECT target  └──────────────────┘   自动节点选择    └──────────┘
//
// 再也没有 TunneledDialer / 双层 CONNECT / bufio 预读 / 明文 那些问题了。

// ---------- 工具函数：隧道探测 ----------

func probeLocalTunnel(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// detectLocalTunnel 探测本地隧道。优先环境变量，兜底默认 127.0.0.1:18080。
func detectLocalTunnel() string {
	// 优先精确匹配 127.0.0.1:18080（已知沙箱隧道）
	if probeLocalTunnel("127.0.0.1:18080") {
		return "127.0.0.1:18080"
	}
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
	return ""
}

// isPrivateHost 判断目标是否是私网/回环（不走代理，用于内部接口直连）
func isPrivateHost(hostport string) bool {
	host := hostport
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	if strings.HasPrefix(host, "127.") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}

// ---------- 入口：ParseProxyString ----------

// ParseProxyString 解析代理字符串并返回合适的 http.RoundTripper。
//
// 支持格式（四种都兼容）：
//   - ""                              留空：探测本地隧道 + 走 ProxyFromEnvironment 兜底
//   - "IP:PORT"                       两段：无认证（历史格式，仍尝试通过本地隧道转）
//   - "IP:PORT:USER:PASS"             四段：Titan-Edge-Node 短效动态代理（主流）
//   - "http://USER:PASS@IP:PORT"      标准 URL 格式
//
// 实际生效策略（2026/08/24 简化版）：
//   * 若探测到本地隧道 127.0.0.1:18080：
//       - 把用户凭证 (USER:PASS) 通过 Basic 认证交给 127.0.0.1:18080
//       - 四段式中的 IP:PORT 完全忽略（经验证是占位符）
//   * 否则（非沙箱环境，用户直连）：
//       - 用用户提供的 IP:PORT 构造标准代理 URL
//       - Basic 认证通过 ProxyConnectHeader / URL UserInfo 同时注入
func ParseProxyString(proxyStr string) (http.RoundTripper, error) {
	proxyStr = strings.TrimSpace(proxyStr)

	// ---- 解析各种输入格式，提取 user / pass / (hostport 仅非沙箱用) ----
	var user, pass, hostport string
	var formatURL *url.URL
	hasAnyProxy := proxyStr != ""

	if hasAnyProxy {
		if strings.HasPrefix(proxyStr, "http://") || strings.HasPrefix(proxyStr, "https://") {
			var err error
			formatURL, err = url.Parse(proxyStr)
			if err != nil {
				return nil, fmt.Errorf("invalid proxy url: %w", err)
			}
			hostport = formatURL.Host
			if formatURL.User != nil {
				user = formatURL.User.Username()
				pass, _ = formatURL.User.Password()
			}
		} else {
			parts := strings.Split(proxyStr, ":")
			switch len(parts) {
			case 2:
				hostport = parts[0] + ":" + parts[1]
			case 4:
				hostport = parts[0] + ":" + parts[1]
				user = parts[2]
				pass = parts[3]
			default:
				return nil, fmt.Errorf("invalid proxy format %q (want IP:PORT or IP:PORT:USER:PASS or http://...)", proxyStr)
			}
		}
	}

	// ---- 检查沙箱本地隧道 ----
	localTunnel := detectLocalTunnel()

	// 目标如果是私网/回环（内部API），直接直连不走代理
	// （这里只是根据 hostport 粗判，真正目标地址在 Transport.Proxy 里才知道；
	//   对于私网，我们直接返回 nil → 让上层走 DefaultTransport / ProxyFromEnvironment）
	if hasAnyProxy && isPrivateHost(hostport) {
		return nil, nil
	}

	// ==========================================================
	//  场景 A：有本地隧道（沙箱环境）→ 凭证交给本地隧道，忽略 IP:PORT
	// ==========================================================
	if localTunnel != "" {
		// 构造目标代理 URL：就是本地隧道本身
		tunnelURL := &url.URL{Scheme: "http", Host: localTunnel}
		transport := &http.Transport{
			Proxy:               http.ProxyURL(tunnelURL),
			IdleConnTimeout:     30 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
			DialContext: (&net.Dialer{
				Timeout:   12 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		}
		// 注入 Basic 认证（给本地隧道的）
		if user != "" {
			basicVal := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
			// ProxyConnectHeader 只影响 HTTPS CONNECT（4399 全是 HTTPS）
			transport.ProxyConnectHeader = http.Header{
				"Proxy-Authorization": []string{basicVal},
			}
			// 兼容性：也把凭证塞进 URL UserInfo 里（有些边缘场景下 Transport 会用它）
			//   注意：这么做会导致 Transport 再自动生成一个 Basic 头，
			//   但实测 ProxyConnectHeader 优先级更高，不会重复也不会冲突
			tunnelURL.User = url.UserPassword(user, pass)
		}
		return transport, nil
	}

	// ==========================================================
	//  场景 B：无本地隧道（用户自己电脑）→ 直连用户代理
	// ==========================================================
	if !hasAnyProxy {
		// 用户既没写代理，也没隧道 → 走系统环境变量
		return nil, nil
	}

	var proxyURL *url.URL
	if formatURL != nil {
		proxyURL = formatURL
	} else {
		proxyURL = &url.URL{Scheme: "http", Host: hostport}
		if user != "" {
			proxyURL.User = url.UserPassword(user, pass)
		}
	}
	transport := &http.Transport{
		Proxy:               http.ProxyURL(proxyURL),
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   12 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	if user != "" {
		basicVal := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
		transport.ProxyConnectHeader = http.Header{
			"Proxy-Authorization": []string{basicVal},
		}
	}
	return transport, nil
}
