package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Yeah114/FunAuth/internal/db"
	com4399tools "github.com/Yeah114/FunAuth/utils/com4399"
	httpproxy "github.com/Yeah114/FunAuth/utils/proxy"
	account4399 "github.com/Yeah114/g79client/account/com4399"
)

const (
	proxyFetchInterval          = 500 * time.Millisecond
	registerRetryDelay          = 5 * time.Second
	successfulRegistrationDelay = 5 * time.Second
	registerHTTPTimeout         = 15 * time.Second
	minimumIDCodeCount          = 100
	proxyExpireGrace            = 1 * time.Second
)

type statistics struct {
	successes       atomic.Uint64
	failures        atomic.Uint64
	attempts        atomic.Int64
	realNameWaiters atomic.Int64
	webRegisters    atomic.Int64
}

type controller struct {
	ctx              context.Context
	ipStore          *ipHistoryStore
	workers          map[string]struct{}
	mu               sync.Mutex
	wg               sync.WaitGroup
	stats            statistics
	draining         bool
	workersDone      chan struct{}
	announcedWorkers bool
	proxyFetchActive atomic.Bool
	proxyBatches     chan []httpproxy.PaidProxyInfo
	checkingIDCode   bool
	failureMu        sync.Mutex
	failureReasons   map[string]uint64
	failureDetails   map[string]map[string]uint64
}

func main() {
	if err := db.InitDBWithOptions(db.InitOptions{}); err != nil {
		log.Fatalf("init database: %v", err)
	}
	var ipStore *ipHistoryStore
	if httpproxy.Com4399RegisterDeduplicateProxy {
		var err error
		ipStore, err = newIPHistoryStore(context.Background())
		if err != nil {
			log.Fatalf("init temporary ip database: %v", err)
		}
		defer ipStore.Close()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	newController(ctx, ipStore).run()
}

func newController(ctx context.Context, ipStore *ipHistoryStore) *controller {
	return &controller{
		ctx:            ctx,
		ipStore:        ipStore,
		workers:        make(map[string]struct{}),
		proxyBatches:   make(chan []httpproxy.PaidProxyInfo, 32),
		failureReasons: make(map[string]uint64),
		failureDetails: make(map[string]map[string]uint64),
	}
}

func (c *controller) run() {
	log.Printf("正在请求首批付费代理")
	if httpproxy.Com4399DirectSingleThread {
		c.start("direct", time.Time{})
	} else {
		c.startProxyFetcher()
	}
	statsTicker := time.NewTicker(time.Second)
	defer statsTicker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			c.printStats("interrupted")
			c.printFailureSummary()
			return
			flushCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := db.WaitAccountWrites(flushCtx); err != nil {
				log.Printf("等待账号缓存入库失败: %v", err)
			}
			cancel()
			c.printStats("stopped")
			return
		case proxies := <-c.proxyBatches:
			c.assignProxies(proxies)
		case <-statsTicker.C:
			c.printStats("running")
			c.checkIDCodeAsync()
		case <-c.done():
			flushCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := db.WaitAccountWrites(flushCtx); err != nil {
				log.Printf("等待账号缓存入库失败: %v", err)
			}
			cancel()
			c.printStats("idcode threshold reached")
			return
		}
	}
}

func (c *controller) checkIDCodeAsync() {
	c.mu.Lock()
	if c.checkingIDCode || c.draining {
		c.mu.Unlock()
		return
	}
	c.checkingIDCode = true
	c.mu.Unlock()
	go func() {
		c.beginDrainIfLowIDCodeCount()
		c.mu.Lock()
		c.checkingIDCode = false
		c.mu.Unlock()
	}()
}

func (c *controller) startProxyFetcher() {
	go func() {
		ticker := time.NewTicker(proxyFetchInterval)
		defer ticker.Stop()
		go c.requestProxyBatch()
		for {
			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
				go c.requestProxyBatch()
			}
		}
	}()
}

func (c *controller) requestProxyBatch() {
	if !c.proxyFetchActive.CompareAndSwap(false, true) {
		return
	}
	defer c.proxyFetchActive.Store(false)

	if !c.canFetchMoreWorkers() {
		return
	}
	proxies, err := httpproxy.PaidHTTPProxyInfos(c.ctx)
	if err != nil {
		c.stats.failures.Add(1)
		log.Printf("付费代理提取失败: %v", err)
		return
	}
	log.Printf("付费代理批次已获取: count=%d", len(proxies))
	proxies = c.filterNewProxies(proxies)
	if len(proxies) == 0 {
		return
	}
	select {
	case c.proxyBatches <- proxies:
	case <-c.ctx.Done():
	}
}

func (c *controller) filterNewProxies(proxies []httpproxy.PaidProxyInfo) []httpproxy.PaidProxyInfo {
	if !httpproxy.Com4399RegisterDeduplicateProxy || c.ipStore == nil {
		return proxies
	}
	fetched := len(proxies)
	proxies = c.ipStore.filterNewProxies(c.ctx, proxies)
	log.Printf("付费代理去重完成: fetched=%d new=%d duplicate=%d", fetched, len(proxies), fetched-len(proxies))
	return proxies
}

func (c *controller) assignProxies(proxies []httpproxy.PaidProxyInfo) {
	started := 0
	for _, proxy := range proxies {
		if c.start(proxy.Address, proxy.ExpiresAt) {
			started++
		}
	}
	if started > 0 {
		c.mu.Lock()
		if !c.announcedWorkers {
			c.announcedWorkers = true
			log.Printf("付费代理工作线程已启动: count=%d", started)
		}
		c.mu.Unlock()
	}
}

func (c *controller) fetchAndAssign() {
	if httpproxy.Com4399DirectSingleThread {
		c.start("direct", time.Time{})
		return
	}
	if !c.canFetchMoreWorkers() {
		return
	}
	proxies, err := httpproxy.PaidHTTPProxyInfos(c.ctx)
	if err != nil {
		c.stats.failures.Add(1)
		log.Printf("付费代理提取失败: %v", err)
		return
	}
	proxies = c.filterNewProxies(proxies)
	started := 0
	for _, proxy := range proxies {
		if c.start(proxy.Address, proxy.ExpiresAt) {
			started++
		}
	}
	if started > 0 {
		c.mu.Lock()
		if !c.announcedWorkers {
			c.announcedWorkers = true
			log.Printf("付费代理工作线程已启动: count=%d", started)
		}
		c.mu.Unlock()
	}
}

func (c *controller) start(proxy string, expiresAt time.Time) bool {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return false
	}
	deadline := proxyDeadline(expiresAt)
	if !deadline.IsZero() && time.Now().After(deadline) {
		return false
	}
	c.mu.Lock()
	if c.draining {
		c.mu.Unlock()
		return false
	}
	limit := httpproxy.Com4399RegisterMaxWorkers
	if httpproxy.Com4399DirectSingleThread {
		limit = 1
	}
	if limit >= 0 && len(c.workers) >= limit {
		c.mu.Unlock()
		return false
	}
	if _, exists := c.workers[proxy]; exists {
		c.mu.Unlock()
		return false
	}
	c.workers[proxy] = struct{}{}
	active := len(c.workers)
	c.mu.Unlock()
	db.SetIDCodeCacheTarget(active)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer c.remove(proxy)
		workerCtx := c.ctx
		cancel := func() {}
		if !deadline.IsZero() {
			workerCtx, cancel = context.WithDeadline(c.ctx, deadline)
		}
		defer cancel()
		c.worker(workerCtx, proxy, expiresAt)
	}()
	return true
}

func proxyDeadline(expiresAt time.Time) time.Time {
	if expiresAt.IsZero() {
		return time.Time{}
	}
	return expiresAt.Add(-proxyExpireGrace)
}

func (c *controller) canFetchMoreWorkers() bool {
	limit := httpproxy.Com4399RegisterMaxWorkers
	if limit < 0 {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.draining && len(c.workers) < limit
}

// beginDrainIfLowIDCodeCount stops only new proxy work. Existing workers keep
// their current IP until it fails or the process is cancelled.
func (c *controller) beginDrainIfLowIDCodeCount() bool {
	c.mu.Lock()
	if c.draining {
		c.mu.Unlock()
		return true
	}
	c.mu.Unlock()

	count, err := db.CountIDCodes()
	if err != nil {
		log.Printf("count remaining idcode records failed: %v", err)
		return false
	}
	if count > minimumIDCodeCount {
		return false
	}

	c.mu.Lock()
	if c.draining {
		c.mu.Unlock()
		return true
	}
	c.draining = true
	c.workersDone = make(chan struct{})
	done := c.workersDone
	c.mu.Unlock()
	log.Printf("remaining idcode=%d, stopping paid-proxy fetches", count)
	go func() {
		c.wg.Wait()
		close(done)
	}()
	return true
}

func (c *controller) done() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.workersDone
}

func (c *controller) remove(proxy string) {
	c.mu.Lock()
	delete(c.workers, proxy)
	active := len(c.workers)
	c.mu.Unlock()
	db.SetIDCodeCacheTarget(active)
}

func (c *controller) worker(ctx context.Context, proxy string, expiresAt time.Time) {
	var proxyURL *url.URL
	if !httpproxy.Com4399DirectSingleThread {
		var err error
		proxyURL, err = httpproxy.ParseHTTPProxy(proxy)
		if err != nil {
			c.stats.failures.Add(1)
			c.recordFailure("无效代理", "proxy_parse_error")
			log.Printf("无效付费代理 %s: %v", proxy, err)
			return
		}
	}
	for {
		if ctx.Err() != nil {
			deadline := proxyDeadline(expiresAt)
			if !deadline.IsZero() && time.Now().After(deadline) {
				log.Printf("代理即将到期，提前丢弃 IP: %s expire=%s", proxy, expiresAt.Format("2006-01-02 15:04:05"))
			}
			return
		}
		if err := c.registerOne(ctx, proxyURL); err != nil {
			if isExpectedDeadlineExit(err) {
				deadline := proxyDeadline(expiresAt)
				if !deadline.IsZero() && time.Now().After(deadline) {
					log.Printf("代理即将到期，正常结束线程: %s expire=%s", proxy, expiresAt.Format("2006-01-02 15:04:05"))
				}
				return
			}
			c.stats.failures.Add(1)
			reason, detail := classifyFailureDetail(err)
			c.recordFailure(reason, detail)
			if proxy != "direct" {
				log.Printf("discarding proxy IP after registration failure: %s: %v", proxy, err)
				return
			}
			if isRegisterPleaseWait(err) {
				delay := registerRetryDelay
				log.Printf("代理=%s 收到服务端限流，保留当前 IP 并等待 %s: %v", proxy, delay, err)
				if !waitContext(ctx, delay) {
					return
				}
				continue
			}
			log.Printf("代理=%s 注册失败: %v", proxy, err)
			if !waitContext(ctx, registerRetryDelay) {
				return
			}
			continue
		}
		c.stats.successes.Add(1)
		if proxy != "direct" {
			log.Printf("discarding proxy IP after successful registration: %s", proxy)
			return
		}
		if !waitContext(ctx, successfulRegistrationDelay) {
			return
		}
	}
}

type registerStage int

const (
	stageNone registerStage = iota
	stageRegister
	stageRealName    = stageRegister
	stageWebRegister = stageRegister
)

func (c *controller) registerOne(ctx context.Context, proxyURL *url.URL) error {
	c.stats.attempts.Add(1)
	defer c.stats.attempts.Add(-1)

	var stage registerStage
	setStage := func(next registerStage) {
		switch stage {
		case stageRegister:
			c.stats.webRegisters.Add(-1)
		}
		stage = next
		switch stage {
		case stageRegister:
			c.stats.webRegisters.Add(1)
		}
	}
	defer setStage(stageNone)

	username, err := randomWebUsername()
	if err != nil {
		return err
	}
	password, err := randomPassword(14)
	if err != nil {
		return err
	}
	var registrationProxy *url.URL
	if httpproxy.Com4399Proxy && !httpproxy.Com4399DirectSingleThread {
		registrationProxy = proxyURL
	}
	httpClient := httpproxy.NewHTTPClientWithProxy(registrationProxy, registerHTTPTimeout)
	registerClient := account4399.NewWebRegisterClient(httpClient)
	setStage(stageRegister)
	for idAttempt := 0; idAttempt < 2; idAttempt++ {
		idCode, err := db.GetRandomIDCode()
		if err != nil {
			return fmt.Errorf("获取实名信息: %w", err)
		}
		log.Printf("4399 单请求注册开始: username=%s", username)
		request := account4399.WebRegisterRequest{
			Username: username, Password: password, RealName: idCode.Name, IDCard: idCode.IDCard,
		}
		if idAttempt == 0 {
			_, err = registerClient.Register(ctx, request)
		} else {
			callbackURL, submitErr := registerClient.SubmitRealName(ctx, request)
			err = submitErr
			if err == nil {
				err = registerClient.FollowCallback(ctx, callbackURL)
			}
		}
		if err != nil {
			if idAttempt == 0 && shouldDiscardIDCode(err) && idCode.ID > 0 {
				if deleteErr := db.DeleteIDCode(idCode.ID); deleteErr != nil {
					return fmt.Errorf("delete rejected id code: %w", deleteErr)
				}
				continue
			}
			if shouldDiscardIDCode(err) && idCode.ID > 0 {
				if deleteErr := db.DeleteIDCode(idCode.ID); deleteErr != nil {
					log.Printf("删除已消耗实名信息失败: id=%d err=%v", idCode.ID, deleteErr)
				}
			}
			return fmt.Errorf("register 4399: %w", err)
		}
		if err := db.QueueAccount(&db.Account{Type: "4399", Username: username, Password: password}); err != nil {
			return fmt.Errorf("写入 account 失败: %w", err)
		}
		return nil
	}
	return errors.New("registration exhausted identity retries")
	_, err = com4399tools.NewCom4399CookieRegister().WithContext(ctx).WithHTTPClient(httpClient).
		WithAccount(username, password).WithAutoProxy(false).WithAutoCaptcha(true).
		WithOCRHTTPClient(http.DefaultClient).WithOCRLogging(true).WithRealNameAttempts(2).WithSkipLogin(true).
		WithGeneratedAccountCallback(func(username, _ string) {
			log.Printf("已生成 4399 账号: username=%s", username)
		}).
		WithGettingRealNameCallback(func() {
			setStage(stageRealName)
			log.Printf("正在获取实名信息: username=%s", username)
		}).
		WithGotRealNameCallback(func(_, _ string) {
			setStage(stageNone)
			log.Printf("已获取实名信息: username=%s", username)
		}).
		WithRegisteringCallback(func(username string) {
			setStage(stageWebRegister)
			log.Printf("4399 注册开始: username=%s", username)
		}).
		WithRegisteredCallback(func(uid, username string) {
			log.Printf("4399 注册成功: username=%s uid=%s", username, uid)
		}).
		WithLoggingInCallback(func(username string) {
			log.Printf("4399 登录开始: username=%s", username)
		}).
		WithLoggedInCallback(func() {
			log.Printf("4399 登录成功: username=%s", username)
		}).
		WithGotCookieCallback(func(cookieLength int) {
			log.Printf("已获取 4399 Sauth: username=%s length=%d", username, cookieLength)
		}).
		WithErrorCallback(func(stage string, err error) {
			if isProxyFailure(err) {
				return
			}
			log.Printf("4399 %s failed: username=%s err=%v", stage, username, err)
		}).
		RegisterCookie()
	if err != nil {
		return fmt.Errorf("register 4399: %w", err)
	}
	log.Printf("4399 注册成功，开始写入 account: username=%s", username)
	if err := db.QueueAccount(&db.Account{Type: "4399", Username: username, Password: password}); err != nil {
		return fmt.Errorf("写入 account 失败: %w", err)
	}
	log.Printf("账号已提交入库缓存: type=4399 username=%s", username)
	return nil
}

func (c *controller) printStats(state string) {
	c.mu.Lock()
	active := len(c.workers)
	c.mu.Unlock()
	idcodeCached, idcodeTarget := db.IDCodeCacheStats()
	accountPending, accountInFlight := db.AccountWriteStats()
	log.Printf("%s: success=%d failed=%d active_ips=%d attempting=%d real_name=%d web_registering=%d idcode_cache=%d/%d account_pending=%d account_writing=%d", state, c.stats.successes.Load(), c.stats.failures.Load(), active, c.stats.attempts.Load(), c.stats.realNameWaiters.Load(), c.stats.webRegisters.Load(), idcodeCached, idcodeTarget, accountPending, accountInFlight)
}

func (c *controller) recordFailure(reason, detail string) {
	if reason == "" {
		reason = "其他"
	}
	if detail == "" {
		detail = "unknown"
	}
	c.failureMu.Lock()
	c.failureReasons[reason]++
	if c.failureDetails[reason] == nil {
		c.failureDetails[reason] = make(map[string]uint64)
	}
	c.failureDetails[reason][detail]++
	c.failureMu.Unlock()
}

func (c *controller) printFailureSummary() {
	c.failureMu.Lock()
	reasons := make([]string, 0, len(c.failureReasons))
	for reason := range c.failureReasons {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		parts = append(parts, fmt.Sprintf("%s=%d", reason, c.failureReasons[reason]))
	}
	c.failureMu.Unlock()
	if len(parts) == 0 {
		log.Printf("failure_summary: none")
		return
	}
	log.Printf("failure_summary: %s", strings.Join(parts, " "))
	c.printFailureDetails()
}

func (c *controller) printFailureDetails() {
	c.failureMu.Lock()
	reasons := make([]string, 0, len(c.failureDetails))
	for reason := range c.failureDetails {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	lines := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		details := c.failureDetails[reason]
		keys := make([]string, 0, len(details))
		for detail := range details {
			keys = append(keys, detail)
		}
		sort.Slice(keys, func(i, j int) bool {
			if details[keys[i]] == details[keys[j]] {
				return keys[i] < keys[j]
			}
			return details[keys[i]] > details[keys[j]]
		})
		if len(keys) > 12 {
			keys = keys[:12]
		}
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s=%d", key, details[key]))
		}
		lines = append(lines, fmt.Sprintf("failure_detail[%s]: %s", reason, strings.Join(parts, " ")))
	}
	c.failureMu.Unlock()
	for _, line := range lines {
		log.Print(line)
	}
}

func classifyFailureDetail(err error) (string, string) {
	if err == nil {
		return "其他", "nil"
	}
	if isProxyFailure(err) {
		return "代理网络错误", classifyProxyFailureDetail(err)
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "register rejected"):
		return "注册被拒", compactFailureMessage(err)
	case strings.Contains(message, "real-name rejected") || strings.Contains(message, "提交验证过于频繁"):
		return "实名被拒", compactFailureMessage(err)
	case strings.Contains(message, "real name is too short") || strings.Contains(message, "invalid register input"):
		return "实名格式错误", compactFailureMessage(err)
	case strings.Contains(message, "get id code") || strings.Contains(message, "idcode") || strings.Contains(message, "too many connections"):
		return "实名数据库错误", compactFailureMessage(err)
	case strings.Contains(message, "account") || strings.Contains(message, "入库") || strings.Contains(message, "写入"):
		return "账号入库错误", compactFailureMessage(err)
	default:
		return "其他", compactFailureMessage(err)
	}
}

func classifyProxyFailureDetail(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.Canceled) || strings.Contains(message, "context canceled"):
		return "context_canceled"
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(message, "deadline exceeded"):
		return "deadline_exceeded"
	case strings.Contains(message, "client.timeout"):
		return "client_timeout"
	case strings.Contains(message, "tls handshake"):
		return "tls_handshake_timeout"
	case strings.Contains(message, "proxyconnect"):
		return "proxyconnect_error"
	case strings.Contains(message, "connection reset"):
		return "connection_reset"
	case strings.Contains(message, "connection refused"):
		return "connection_refused"
	case strings.Contains(message, "eof"):
		return "eof"
	case strings.Contains(message, "timeout"):
		return "timeout"
	default:
		var netErr net.Error
		if errors.As(err, &netErr) {
			return "net_error"
		}
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return "url_error"
		}
		return compactFailureMessage(err)
	}
}

func compactFailureMessage(err error) string {
	message := strings.TrimSpace(err.Error())
	message = strings.ReplaceAll(message, "\r", " ")
	message = strings.ReplaceAll(message, "\n", " ")
	for strings.Contains(message, "  ") {
		message = strings.ReplaceAll(message, "  ", " ")
	}
	if len(message) > 180 {
		message = message[:180] + "..."
	}
	return message
}

func classifyFailure(err error) string {
	if err == nil {
		return "其他"
	}
	if isProxyFailure(err) {
		return "代理网络错误"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "real-name rejected") || strings.Contains(message, "提交验证过于频繁"):
		return "实名被拒"
	case strings.Contains(message, "real name is too short") || strings.Contains(message, "invalid register input"):
		return "实名格式错误"
	case strings.Contains(message, "get id code") || strings.Contains(message, "idcode") || strings.Contains(message, "too many connections"):
		return "实名数据库错误"
	case strings.Contains(message, "account") || strings.Contains(message, "入库") || strings.Contains(message, "写入"):
		return "账号入库错误"
	default:
		return "其他"
	}
}

func waitContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func isExpectedDeadlineExit(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

func shouldDiscardIDCode(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, account4399.ErrRealNameRejected) {
		return true
	}
	message := err.Error()
	return strings.Contains(message, "姓名身份证提交验证过于频繁") ||
		strings.Contains(message, "姓名身份证不匹配") ||
		strings.Contains(message, "身份证实名账号数量超过限制") ||
		strings.Contains(message, "您的身份证异常或错误")
}

func isRegisterPleaseWait(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "register rejected") &&
		(strings.Contains(message, "status=202") ||
			strings.Contains(message, "请稍后再试") ||
			strings.Contains(message, "please wait"))
}

func isProxyFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "proxyconnect") ||
		strings.Contains(message, "exceeded maximum proxy retries") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "connection reset") ||
		strings.Contains(message, "timeout") ||
		strings.Contains(message, "tls handshake") ||
		strings.Contains(message, "eof")
}

func randomLetters(length int) (string, error) {
	return randomFromAlphabet("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", length)
}

func randomWebUsername() (string, error) {
	return randomLetters(12)
}
func randomPassword(length int) (string, error) {
	if length < 3 {
		return "", errors.New("password length must be at least 3")
	}
	upper, err := randomFromAlphabet("ABCDEFGHIJKLMNOPQRSTUVWXYZ", 1)
	if err != nil {
		return "", err
	}
	lower, err := randomFromAlphabet("abcdefghijklmnopqrstuvwxyz", 1)
	if err != nil {
		return "", err
	}
	digit, err := randomFromAlphabet("0123456789", 1)
	if err != nil {
		return "", err
	}
	rest, err := randomFromAlphabet("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", length-3)
	if err != nil {
		return "", err
	}
	chars := []byte(upper + lower + digit + rest)
	for i := len(chars) - 1; i > 0; i-- {
		j, err := randomIndex(i + 1)
		if err != nil {
			return "", err
		}
		chars[i], chars[j] = chars[j], chars[i]
	}
	return string(chars), nil
}
func randomFromAlphabet(alphabet string, length int) (string, error) {
	if length <= 0 {
		return "", errors.New("random length must be positive")
	}
	bytes := make([]byte, length)
	for i := range bytes {
		index, err := randomIndex(len(alphabet))
		if err != nil {
			return "", err
		}
		bytes[i] = alphabet[index]
	}
	return string(bytes), nil
}
func randomIndex(limit int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}
