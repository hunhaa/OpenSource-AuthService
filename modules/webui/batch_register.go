package webui

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	account4399 "github.com/Yeah114/g79client/account/com4399"
	"github.com/gin-gonic/gin"
)

// ---------- 身份证库（SFZ）----------
// 内存里维护一份 sfz 列表，格式兼容 MCQTSS (姓名----身份证号) 和 boluoreg (姓名:身份证号)

type sfzEntry struct {
	Name   string
	Number string
}

var (
	sfzMu       sync.RWMutex
	sfzPool     []sfzEntry
	sfzPoolPos  int
	sfzPoolOnce sync.Once
)

func loadSfzDefaultPool() {
	sfzMu.Lock()
	defer sfzMu.Unlock()
	if len(sfzPool) == 0 {
		sfzPool = getBuiltinSFZPool()
	}
}

// getBuiltinSFZPool 如果用户未上传 sfz 文件，给出一份兜底内置列表。
// 注意：如果身份证被限制/超出了，请用户上传 sfz.txt
func getBuiltinSFZPool() []sfzEntry {
	return []sfzEntry{
		{Name: "刘智毅", Number: "430521198709122618"},
		{Name: "邓敏", Number: "512529197608296270"},
		{Name: "张健", Number: "152105197908280035"},
		{Name: "杜春风", Number: "330106196910072116"},
		{Name: "陈津", Number: "330621198902107411"},
		{Name: "李石聚", Number: "410203196212062013"},
		{Name: "杨刘兵", Number: "450203198011211058"},
		{Name: "周华杰", Number: "310230196309146470"},
		{Name: "陈增辉", Number: "440181198612033914"},
		{Name: "杨春", Number: "41070319750628051X"},
		{Name: "孔祥海", Number: "211022198202103935"},
		{Name: "冯彦龙", Number: "150103196704201076"},
		{Name: "顾珺", Number: "310102198405191640"},
		{Name: "王丽君", Number: "230105198907123049"},
		{Name: "田文杰", Number: "210104196010162319"},
		{Name: "刘振华", Number: "130430198109122239"},
		{Name: "孙海霞", Number: "370481199003072285"},
		{Name: "赵军平", Number: "341621198512087814"},
		{Name: "黄丽娟", Number: "450102198305091020"},
		{Name: "徐伟", Number: "320107197810094517"},
		{Name: "朱少华", Number: "420111197305063023"},
		{Name: "马晓阳", Number: "610112199308176178"},
		{Name: "胡斌", Number: "500108198407263443"},
		{Name: "郭佳佳", Number: "140106199503051822"},
		{Name: "何亮", Number: "620102198212176816"},
		{Name: "高淑芬", Number: "220103197203203027"},
		{Name: "罗伟", Number: "53011119870908231X"},
		{Name: "郑欣怡", Number: "350102199806151741"},
		{Name: "梁建华", Number: "440106197103076193"},
		{Name: "谢文宇", Number: "360103198909221015"},
	}
}

func popRandomSFZ() sfzEntry {
	sfzPoolOnce.Do(loadSfzDefaultPool)
	sfzMu.Lock()
	defer sfzMu.Unlock()
	n := len(sfzPool)
	if n == 0 {
		return sfzEntry{Name: "张三", Number: "110101199003071234"}
	}
	// 顺序取（避免同一个 sfz 在短时间内被反复使用 → 触发「实名过于频繁」）
	entry := sfzPool[sfzPoolPos%n]
	sfzPoolPos++
	return entry
}

// HandleUploadSFZ 上传身份证库。
// 每行格式：「姓名----身份证号」 或 「姓名:身份证号」 或 「姓名,身份证号」
func HandleUploadSFZ(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		fail(c, 400, "表单解析失败: %v", err)
		return
	}
	files := form.File["sfz_file"]
	if len(files) == 0 {
		fail(c, 400, "未上传 sfz_file")
		return
	}
	f, err := files[0].Open()
	if err != nil {
		fail(c, 400, "打开文件失败: %v", err)
		return
	}
	defer f.Close()

	var list []sfzEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var name, num string
		switch {
		case strings.Contains(line, "----"):
			parts := strings.SplitN(line, "----", 2)
			name, num = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		case strings.Contains(line, ":"):
			parts := strings.SplitN(line, ":", 2)
			name, num = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		case strings.Contains(line, ","):
			parts := strings.SplitN(line, ",", 2)
			name, num = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		case strings.Contains(line, "\t"):
			parts := strings.SplitN(line, "\t", 2)
			name, num = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		default:
			continue
		}
		if len(num) == 15 || len(num) == 18 {
			list = append(list, sfzEntry{Name: name, Number: num})
		}
	}
	if len(list) == 0 {
		fail(c, 400, "SFZ 文件内容为空或格式无法识别（支持 姓名----身份证号 / 姓名:身份证号 / CSV）")
		return
	}
	sfzMu.Lock()
	sfzPool = list
	sfzPoolPos = 0
	sfzMu.Unlock()
	ok(c, gin.H{"count": len(list)})
}

// ---------- 代理池 ----------

type proxyEntry struct {
	URL     *url.URL
	Raw     string
	Dead    bool
	DeadCnt int
}

var (
	proxyMu   sync.RWMutex
	proxyPool []proxyEntry
	proxyPos  int

	proxyIpPortRegex = regexp.MustCompile(`^(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}):(\d{1,5})$`)
)

// parseProxyString 解析 "127.0.0.1:8080" / "user:pass@127.0.0.1:8080" / "http(s)://..."
func parseProxyString(s string) (*proxyEntry, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	hasScheme := strings.Contains(s, "://")
	if !hasScheme {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" {
		return nil, fmt.Errorf("不支持的代理协议: %s", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("代理缺少 host:port")
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		return nil, fmt.Errorf("代理 host:port 格式不合法: %w", err)
	}
	portNum, _ := strconv.Atoi(port)
	if portNum <= 0 || portNum > 65535 {
		return nil, fmt.Errorf("非法端口: %s", port)
	}
	// 对裸 IP:PORT 做基本校验（带域名的放行）
	if ip := net.ParseIP(host); ip != nil {
		if !proxyIpPortRegex.MatchString(ip.String() + ":" + port) {
			return nil, fmt.Errorf("IP:PORT 不匹配正则")
		}
	}
	return &proxyEntry{URL: u, Raw: s}, nil
}

func HandleUploadProxies(c *gin.Context) {
	parseList := func(items []string) ([]proxyEntry, []string) {
		var list []proxyEntry
		var errs []string
		for i, s := range items {
			s = strings.TrimSpace(s)
			if s == "" || strings.HasPrefix(s, "#") {
				continue
			}
			p, e := parseProxyString(s)
			if e != nil {
				errs = append(errs, fmt.Sprintf("第%d行[%s]: %v", i+1, s, e))
				continue
			}
			if p != nil {
				list = append(list, *p)
			}
		}
		return list, errs
	}

	var list []proxyEntry
	var errs []string

	form, err := c.MultipartForm()
	if err != nil {
		// 兼容 JSON body
		var body struct {
			Proxies []string `json:"proxies"`
		}
		if e2 := c.ShouldBindJSON(&body); e2 != nil {
			fail(c, 400, "解析失败: %v 或 %v", err, e2)
			return
		}
		list, errs = parseList(body.Proxies)
	} else {
		files := form.File["proxy_file"]
		if len(files) > 0 {
			f, err := files[0].Open()
			if err != nil {
				fail(c, 400, "打开文件失败: %v", err)
				return
			}
			defer f.Close()
			var items []string
			scanner := bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for scanner.Scan() {
				items = append(items, scanner.Text())
			}
			list, errs = parseList(items)
		} else if vals, got := form.Value["proxies"]; got && len(vals) > 0 {
			raw := vals[0]
			lines := strings.FieldsFunc(raw, func(r rune) bool { return r == '\n' || r == ',' || r == ';' })
			list, errs = parseList(lines)
		} else {
			fail(c, 400, "未找到 proxy_file 或 proxies 字段")
			return
		}
	}
	proxyMu.Lock()
	proxyPool = list
	proxyPos = 0
	proxyMu.Unlock()
	ok(c, gin.H{"count": len(list), "invalid": errs})
}

// nextProxyTransport 取下一个活代理，最多跳过 n*2 个已死代理
func nextProxyTransport() (*http.Transport, bool) {
	proxyMu.Lock()
	n := len(proxyPool)
	if n == 0 {
		proxyMu.Unlock()
		return nil, false
	}
	start := proxyPos
	var chosen *proxyEntry
	for i := 0; i < n*2; i++ {
		idx := proxyPos % n
		entry := &proxyPool[idx]
		proxyPos++
		if entry.Dead {
			continue
		}
		chosen = entry
		break
	}
	// 没有活的：重置 dead 标记，再取第一个
	if chosen == nil {
		for i := range proxyPool {
			proxyPool[i].Dead = false
		}
		idx := start % n
		chosen = &proxyPool[idx]
		proxyPos = start + 1
	}
	u := chosen.URL
	proxyMu.Unlock()
	return &http.Transport{
		Proxy:               http.ProxyURL(u),
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}, true
}

func currentProxyFromTransport(tr http.RoundTripper) string {
	tp, ok := tr.(*http.Transport)
	if !ok || tp.Proxy == nil {
		return ""
	}
	u, e := tp.Proxy(nil)
	if e != nil || u == nil {
		return ""
	}
	return u.Host
}

func markProxyDead(rawHost string) {
	if rawHost == "" {
		return
	}
	proxyMu.Lock()
	defer proxyMu.Unlock()
	for i := range proxyPool {
		if proxyPool[i].URL.Host == rawHost {
			proxyPool[i].DeadCnt++
			if proxyPool[i].DeadCnt >= 3 {
				proxyPool[i].Dead = true
			}
			return
		}
	}
}

func refreshProxy(tr http.RoundTripper) (*http.Transport, bool) {
	if tr == nil {
		// 本来就没代理，下次还是没代理
		return nil, false
	}
	return nextProxyTransport()
}


// ---------- 批量注册任务 ----------

type BatchRegisterReq struct {
	Count           int    `json:"count"`
	Concurrency     int    `json:"concurrency"`
	UsernamePrefix  string `json:"username_prefix"`
	NicknamePrefix  string `json:"nickname_prefix"`
	Password        string `json:"password"` // 留空自动生成
	DelaySec        int    `json:"delay_sec"`  // 每个成功/失败后的间隔（秒）
	UseDirect       bool   `json:"use_direct"` // true=轻量ptlogin.register.do，false=走原 OAuth 流程
	GenSauth        bool   `json:"gen_sauth"`  // 是否生成 sauth cookie
	UseProxyPerTask bool   `json:"use_proxy_per_task"`
}

type BatchRegisterResultItem struct {
	Index         int    `json:"index"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	Nickname      string `json:"nickname"`
	Status        string `json:"status"` // success / captcha_fail / sfz_limit / sfz_freq / realname_error / username_exist / risk_control / error
	DisplayMsg    string `json:"msg"`
	SauthCookie   string `json:"sauth_cookie,omitempty"`
	SauthCookieLen int   `json:"sauth_len,omitempty"`
	SFZName       string `json:"sfz_name"`
	SFZNumber     string `json:"sfz_number"`
	ProxyUsed     string `json:"proxy_used,omitempty"`
}

func generateUsername(prefix string, i int) string {
	suffix := account4399RandomString(5)
	u := fmt.Sprintf("%s%s%03d", prefix, suffix, i)
	if len(u) > 20 {
		u = u[:20]
	}
	return u
}

func account4399RandomString(n int) string {
	alphabet := "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[i%len(alphabet)]
	}
	// 真随机化
	rnd := make([]byte, n)
	httpGetRandom(rnd)
	for i := range b {
		b[i] = alphabet[int(rnd[i])%len(alphabet)]
	}
	return string(b)
}

func httpGetRandom(dst []byte) {
	// 简单填充: 用 time.Now 做 seed，避免直接引 crypto/rand（顶层已引）
	now := uint64(time.Now().UnixNano())
	for i := range dst {
		now = now*1103515245 + 12345
		dst[i] = byte(now)
	}
}

func doOneRegister(ctx context.Context, idx int, req *BatchRegisterReq) *BatchRegisterResultItem {
	sfz := popRandomSFZ()
	user := generateUsername(strings.TrimSpace(req.UsernamePrefix), idx)
	pwd := req.Password
	if pwd == "" {
		pwd = account4399RandomString(10)
	}
	nick := req.NicknamePrefix + user
	if len(nick) > 16 {
		nick = nick[:16]
	}

	item := &BatchRegisterResultItem{
		Index:     idx,
		Username:  user,
		Password:  pwd,
		Nickname:  nick,
		SFZName:   sfz.Name,
		SFZNumber: sfz.Number,
		Status:    "error",
	}

	var transport *http.Transport
	var proxyOK bool
	if req.UseProxyPerTask {
		transport, proxyOK = nextProxyTransport()
		if proxyOK {
			item.ProxyUsed = currentProxyFromTransport(transport)
		}
	}

	maxRetries := 5
	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			item.DisplayMsg = "任务已取消"
			return item
		default:
		}

		// 每次重试都换新代理（如果启用了代理池）
		if attempt > 0 && req.UseProxyPerTask {
			newTr, ok2 := refreshProxy(transport)
			if ok2 {
				transport = newTr
				item.ProxyUsed = currentProxyFromTransport(transport)
			}
		}

		if req.UseDirect {
			dreq := &account4399.DirectRegisterRequest{
				Username:  user,
				Password:  pwd,
				RealName:  sfz.Name,
				IDCard:    sfz.Number,
			}
			// 防止 typed nil (*http.Transport)(nil) 被塞到 http.RoundTripper 接口里
			// —— 否则接口值 != nil 但底层指针为空，会在 net/http 内 SIGSEGV
			if transport != nil {
				dreq.Transport = transport
			}
			if req.GenSauth {
				cookie, res, err := account4399.DirectRegisterAndLoginCookie(ctx, dreq)
				if err == nil && res.Success {
					item.Status = "success"
					item.DisplayMsg = "注册+SAuth成功"
					item.SauthCookie = cookie
					item.SauthCookieLen = len(cookie)
					return item
				}
				item.DisplayMsg = fmt.Sprintf("直连模式失败: %v", err)
				if res != nil && res.DisplayMessage != "" {
					item.DisplayMsg = res.DisplayMessage
				}
				switch e := err; {
				case e == account4399.ErrUsernameExists:
					item.Status = "username_exist"
					user = generateUsername(strings.TrimSpace(req.UsernamePrefix), idx*100+attempt)
					item.Username = user
					continue
				case e == account4399.ErrRealNameRejected:
					item.Status = "sfz_limit"
					sfz = popRandomSFZ()
					item.SFZName = sfz.Name
					item.SFZNumber = sfz.Number
					continue
				case e == account4399.ErrRiskControlTriggered:
					item.Status = "risk_control"
					markProxyDead(item.ProxyUsed)
					if attempt < maxRetries-1 {
						time.Sleep(5 * time.Second)
					}
					continue
				case errors.Is(e, account4399.ErrCaptchaEngineNotReady):
					item.Status = "ocr_missing"
					return item
				case e == account4399.ErrCaptchaFailed:
					item.Status = "captcha_fail"
					if attempt < maxRetries-1 {
						time.Sleep(2 * time.Second)
					}
					continue
				case e == account4399.ErrInvalidRegisterInput:
					item.Status = "error"
					return item
				case e == account4399.ErrRegisterRejected:
					item.Status = "error"
					markProxyDead(item.ProxyUsed)
					continue
				default:
					// 网络错误 / 代理超时 / 未知错误一律视为代理失效，换下一个
					if e != nil {
						markProxyDead(item.ProxyUsed)
					}
				}
				if attempt < maxRetries-1 {
					time.Sleep(2 * time.Second)
				}
				continue
			}
			// 不生成 sauth
			res, err := account4399.DirectRegister(ctx, dreq)
			if err == nil && res.Success {
				item.Status = "success"
				item.DisplayMsg = "注册成功"
				return item
			}
			item.DisplayMsg = fmt.Sprintf("%v", err)
			if res != nil && res.DisplayMessage != "" {
				item.DisplayMsg = res.DisplayMessage
			}
			switch {
			case err == account4399.ErrUsernameExists:
				item.Status = "username_exist"
				user = generateUsername(strings.TrimSpace(req.UsernamePrefix), idx*100+attempt)
				item.Username = user
				continue
			case err == account4399.ErrRealNameRejected:
				item.Status = "sfz_limit"
				sfz = popRandomSFZ()
				item.SFZName = sfz.Name
				item.SFZNumber = sfz.Number
				continue
			case errors.Is(err, account4399.ErrCaptchaEngineNotReady):
				item.Status = "ocr_missing"
				return item
			case err == account4399.ErrCaptchaFailed:
				item.Status = "captcha_fail"
				if attempt < maxRetries-1 {
					time.Sleep(2 * time.Second)
				}
				continue
			case err == account4399.ErrRiskControlTriggered:
				item.Status = "risk_control"
				markProxyDead(item.ProxyUsed)
				if attempt < maxRetries-1 {
					time.Sleep(5 * time.Second)
				}
				continue
			default:
				if err != nil {
					markProxyDead(item.ProxyUsed)
				}
			}
			if attempt < maxRetries-1 {
				time.Sleep(2 * time.Second)
			}
			continue
		}

		// 旧 OAuth 模式（兜底）
		wreq := account4399.WebRegisterRequest{
			Username: user,
			Password: pwd,
			RealName: sfz.Name,
			IDCard:   sfz.Number,
		}
		if transport != nil {
			wreq.Transport = transport
		}
		if req.GenSauth {
			wRes, err := account4399.RegisterWebCookie(ctx, wreq)
			if err == nil && wRes != nil {
				item.Status = "success"
				item.DisplayMsg = "OAuth注册+SAuth成功"
				cookie := ""
				if wRes.Cookie != "" {
					cookie = wRes.Cookie
				}
				item.SauthCookie = cookie
				item.SauthCookieLen = len(cookie)
				return item
			}
			item.DisplayMsg = fmt.Sprintf("%v", err)
		} else {
			wRes, err := account4399.RegisterWeb(ctx, wreq)
			if err == nil && wRes != nil {
				item.Status = "success"
				item.DisplayMsg = "OAuth注册成功"
				return item
			}
			item.DisplayMsg = fmt.Sprintf("%v", err)
		}
		em := strings.ToLower(item.DisplayMsg)
		switch {
		case strings.Contains(em, "用户名已被注册"), strings.Contains(em, "username already"):
			item.Status = "username_exist"
			user = generateUsername(strings.TrimSpace(req.UsernamePrefix), idx*100+attempt)
			item.Username = user
			continue
		case strings.Contains(em, "身份证"), strings.Contains(em, "实名"), strings.Contains(em, "sfz"):
			item.Status = "sfz_limit"
			sfz = popRandomSFZ()
			item.SFZName = sfz.Name
			item.SFZNumber = sfz.Number
			continue
		case strings.Contains(em, "风控"), strings.Contains(em, "稍后再试"):
			item.Status = "risk_control"
			markProxyDead(item.ProxyUsed)
			if attempt < maxRetries-1 {
				time.Sleep(5 * time.Second)
			}
			continue
		default:
			markProxyDead(item.ProxyUsed)
		}
		if attempt < maxRetries-1 {
			time.Sleep(2 * time.Second)
		}
	}
	return item
}

// HandleBatchRegister 批量注册
// 支持 SSE（HTTP stream）逐步推送进度，也支持普通 JSON 返回（等全部完成）。
// 本实现采用普通 JSON 返回 + 结果集，配合前端轮询或直接等待。
func HandleBatchRegister(c *gin.Context) {
	var req BatchRegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "参数错误: %v", err)
		return
	}
	if req.Count <= 0 {
		req.Count = 1
	}
	if req.Count > 500 {
		req.Count = 500
	}
	if req.Concurrency <= 0 {
		req.Concurrency = 1
	}
	if req.Concurrency > 32 {
		req.Concurrency = 32
	}
	if req.DelaySec < 0 {
		req.DelaySec = 2
	}

	sfzPoolOnce.Do(loadSfzDefaultPool)

	totalCtx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Minute)
	defer cancel()

	sem := make(chan struct{}, req.Concurrency)
	var wg sync.WaitGroup
	var okCnt, failCnt int32
	var sfzLimitCnt, sfzFreqCnt, captchaCnt, ocrMissingCnt, userExistCnt, riskCnt int32

	results := make([]*BatchRegisterResultItem, req.Count)
	for i := 0; i < req.Count; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if req.DelaySec > 0 {
					time.Sleep(time.Duration(req.DelaySec) * time.Second)
				}
			}()
			item := doOneRegister(totalCtx, idx, &req)
			results[idx] = item
			switch item.Status {
			case "success":
				atomic.AddInt32(&okCnt, 1)
			case "sfz_limit":
				atomic.AddInt32(&sfzLimitCnt, 1)
				atomic.AddInt32(&failCnt, 1)
			case "sfz_freq":
				atomic.AddInt32(&sfzFreqCnt, 1)
				atomic.AddInt32(&failCnt, 1)
			case "captcha_fail":
				atomic.AddInt32(&captchaCnt, 1)
				atomic.AddInt32(&failCnt, 1)
			case "ocr_missing":
				atomic.AddInt32(&ocrMissingCnt, 1)
				atomic.AddInt32(&failCnt, 1)
			case "username_exist":
				atomic.AddInt32(&userExistCnt, 1)
				atomic.AddInt32(&failCnt, 1)
			case "risk_control":
				atomic.AddInt32(&riskCnt, 1)
				atomic.AddInt32(&failCnt, 1)
			default:
				atomic.AddInt32(&failCnt, 1)
			}
			log.Printf("[批量注册] #%d/%d status=%s user=%s msg=%s", idx+1, req.Count, item.Status, item.Username, item.DisplayMsg)
		}(i)
	}
	wg.Wait()

	ok(c, gin.H{
		"results": results,
		"total":   req.Count,
		"success": okCnt,
		"failed":  failCnt,
		"stats": gin.H{
			"sfz_limit":       sfzLimitCnt,
			"sfz_freq":        sfzFreqCnt,
			"captcha_fail":    captchaCnt,
			"ocr_missing":     ocrMissingCnt,
			"username_exist":  userExistCnt,
			"risk_control":    riskCnt,
		},
		"concurrency":  req.Concurrency,
		"use_direct":   req.UseDirect,
		"gen_sauth":    req.GenSauth,
		"proxy_loaded": func() int { proxyMu.RLock(); defer proxyMu.RUnlock(); return len(proxyPool) }(),
		"sfz_loaded":   func() int { sfzMu.RLock(); defer sfzMu.RUnlock(); return len(sfzPool) }(),
	})
}

// HandleExportBatchRegisterCSV 导出结果为 CSV / TXT / JSON
func HandleExportBatchRegisterCSV(c *gin.Context) {
	var body struct {
		Results []BatchRegisterResultItem `json:"results"`
		Format  string                    `json:"format"` // csv / txt_accounts / txt_sauth / json
		SuccessOnly bool                  `json:"success_only"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, 400, "参数错误: %v", err)
		return
	}
	var list []BatchRegisterResultItem
	for _, r := range body.Results {
		if body.SuccessOnly && r.Status != "success" {
			continue
		}
		list = append(list, r)
	}

	switch strings.ToLower(body.Format) {
	case "txt_accounts", "txt":
		var sb strings.Builder
		for _, r := range list {
			fmt.Fprintf(&sb, "%s----%s\n", r.Username, r.Password)
		}
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(sb.String()))
		return
	case "txt_sauth":
		var sb strings.Builder
		for _, r := range list {
			if r.SauthCookie != "" {
				sb.WriteString(r.SauthCookie)
				sb.WriteString("\n")
			}
		}
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(sb.String()))
		return
	case "json":
		c.JSON(http.StatusOK, gin.H{"results": list})
		return
	default: // csv
		pr, pw := io.Pipe()
		go func() {
			w := csv.NewWriter(pw)
			_ = w.Write([]string{"序号", "状态", "用户名", "密码", "昵称", "身份证姓名", "身份证号", "SAuth长度", "SAuth", "代理", "错误信息"})
			for _, r := range list {
				_ = w.Write([]string{
					fmt.Sprintf("%d", r.Index+1), r.Status, r.Username, r.Password, r.Nickname,
					r.SFZName, r.SFZNumber, fmt.Sprintf("%d", r.SauthCookieLen), r.SauthCookie,
					r.ProxyUsed, r.DisplayMsg,
				})
			}
			w.Flush()
			pw.Close()
		}()
		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Header("Content-Disposition", `attachment; filename="funauth_batch_register.csv"`)
		c.Status(http.StatusOK)
		io.Copy(c.Writer, pr)
	}
}

// HandleStatsBatch 查看当前 sfz 和 proxy 池数量
func HandleStatsBatch(c *gin.Context) {
	sfzPoolOnce.Do(loadSfzDefaultPool)
	sfzMu.RLock()
	sfzCnt := len(sfzPool)
	sfzPos := sfzPoolPos
	sfzMu.RUnlock()
	proxyMu.RLock()
	prxCnt := len(proxyPool)
	prxPos := proxyPos
	var alive, dead int
	for _, p := range proxyPool {
		if p.Dead {
			dead++
		} else {
			alive++
		}
	}
	proxyMu.RUnlock()
	ok(c, gin.H{
		"sfz_count":   sfzCnt,
		"sfz_pos":     sfzPos,
		"proxy_count": prxCnt,
		"proxy_pos":   prxPos,
		"proxy_alive": alive,
		"proxy_dead":  dead,
	})
}

type proxyCheckResult struct {
	Proxy       string  `json:"proxy"`
	OK          bool    `json:"ok"`
	HTTPStatus  int     `json:"http_status,omitempty"`
	Size        int     `json:"size,omitempty"`
	TimeMs      int64   `json:"time_ms"`
	Error       string  `json:"error,omitempty"`
}

// HandleCheckProxies 对已上传代理池或入参代理列表做连通性预检
// 目标：访问 https://ptlogin.4399.com/ptlogin/captcha.do 验证能否拉到验证码（HTTP 200 且 body>300B）
func HandleCheckProxies(c *gin.Context) {
	var body struct {
		Proxies []string `json:"proxies"` // 空=检查当前 proxyPool
		Target  string   `json:"target"`  // 自定义探测 URL
		Timeout int      `json:"timeout"` // 秒，默认 6
	}
	var errmsg string
	if err := c.ShouldBindJSON(&body); err != nil {
		errmsg = err.Error()
	}
	if body.Timeout <= 0 {
		body.Timeout = 6
	}
	if body.Target == "" {
		body.Target = "https://ptlogin.4399.com/ptlogin/captcha.do?captchaId=captchaReq01"
	}

	// 构造待检测列表
	var list []string
	if len(body.Proxies) > 0 {
		for _, s := range body.Proxies {
			s = strings.TrimSpace(s)
			if s == "" || strings.HasPrefix(s, "#") {
				continue
			}
			list = append(list, s)
		}
	} else {
		proxyMu.RLock()
		for _, p := range proxyPool {
			list = append(list, p.URL.Host)
		}
		proxyMu.RUnlock()
	}
	if len(list) == 0 {
		ok(c, gin.H{"total": 0, "alive": 0, "dead": 0, "results": nil,
			"msg": "proxy_pool_empty", "parse_err": errmsg})
		return
	}
	if len(list) > 500 {
		list = list[:500]
	}

	const conc = 20
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	results := make([]proxyCheckResult, len(list))
	for i, raw := range list {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, raw string) {
			defer wg.Done()
			defer func() { <-sem }()
			r := proxyCheckResult{Proxy: raw}
			t0 := time.Now()
			// 构造 proxy url
			uStr := raw
			if !strings.Contains(uStr, "://") {
				uStr = "http://" + uStr
			}
			u, perr := url.Parse(uStr)
			if perr != nil {
				r.Error = "parse: " + perr.Error()
				results[i] = r
				return
			}
			client := &http.Client{
				Timeout: time.Duration(body.Timeout) * time.Second,
				Transport: &http.Transport{
					Proxy:               http.ProxyURL(u),
					TLSHandshakeTimeout: time.Duration(body.Timeout) * time.Second,
				},
			}
			req, herr := http.NewRequestWithContext(c.Request.Context(), "GET", body.Target, nil)
			if herr != nil {
				r.Error = herr.Error()
				r.TimeMs = time.Since(t0).Milliseconds()
				results[i] = r
				return
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125 Safari/537.36")
			resp, xerr := client.Do(req)
			r.TimeMs = time.Since(t0).Milliseconds()
			if xerr != nil {
				r.Error = xerr.Error()
				results[i] = r
				return
			}
			defer resp.Body.Close()
			r.HTTPStatus = resp.StatusCode
			b, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
			r.Size = int(b)
			r.OK = resp.StatusCode == 200 && r.Size > 300
			results[i] = r
		}(i, raw)
	}
	wg.Wait()

	alive := 0
	for _, r := range results {
		if r.OK {
			alive++
		}
	}
	ok(c, gin.H{
		"target":    body.Target,
		"timeout_s": body.Timeout,
		"total":     len(results),
		"alive":     alive,
		"dead":      len(results) - alive,
		"results":   results,
		"parse_err": errmsg,
	})
}

