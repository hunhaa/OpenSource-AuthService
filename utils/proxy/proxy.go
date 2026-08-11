package proxy

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	xnetproxy "golang.org/x/net/proxy"
)

const (
	defaultCheckThreads = 1000
	defaultCheckTimeout = 2 * time.Second
	defaultCheckURL     = "https://ptlogin.4399.com/ptlogin/captcha.do?captchaId=test"
	proxyPoolFileName   = "http_proxies.json"
	blacklistFileName   = "ips.json"
	githubMirrorTimeout = 5 * time.Second
	defaultMinReady     = 3
)

var sourceURLs = []string{
	"https://raw.githubusercontent.com/iplocate/free-proxy-list/main/protocols/http.txt",
	"https://raw.githubusercontent.com/komutan234/Proxy-List-Free/main/proxies/http.txt",
	"https://cdn.jsdelivr.net/gh/proxifly/free-proxy-list@main/proxies/protocols/http/data.txt",
	"https://raw.githubusercontent.com/r00tee/Proxy-List/main/Https.txt",
	"https://raw.githubusercontent.com/ABoredCat/Free-Proxy/main/proxies/http.txt",
	"https://raw.githubusercontent.com/mmpx12/proxy-list/master/http.txt",
	"https://raw.githubusercontent.com/ShiftyTR/Proxy-List/master/http.txt",
	"https://raw.githubusercontent.com/monosans/proxy-list/main/proxies/http.txt",
	"https://raw.githubusercontent.com/TheSpeedX/PROXY-List/master/http.txt",
	"https://proxy.scdn.io/text.php",
}

var defaultGitHubMirrors = []string{
	"https://github.yuansi.xyz/",
}

var scraperURLs = []string{
	"https://www.kuaidaili.com/free/inha/1/",
	"https://www.kuaidaili.com/free/inha/2/",
	"https://www.kuaidaili.com/free/inha/3/",
}

var pool = struct {
	sync.Mutex
	proxies []string
}{}

var blacklistedProxies = struct {
	sync.RWMutex
	hosts map[string]struct{}
}{hosts: make(map[string]struct{})}

var blacklistLoadOnce sync.Once

type Manager struct {
	ctx           context.Context
	storagePath   string
	sources       []string
	scrapers      []string
	gitHubMirrors []string
	checkThreads  int
	checkTimeout  time.Duration
	checkURL      string
	minReady      int
	collectAll    bool
	httpClient    *http.Client

	loadingSavedCallback    func(path string)
	loadedSavedCallback     func(path string, count int)
	fetchingSourceCallback  func(source string)
	fetchedSourceCallback   func(source string, count int)
	fetchingScraperCallback func(source string)
	fetchedScraperCallback  func(source string, count int)
	checkingProxiesCallback func(total, workers int)
	checkedProxiesCallback  func(checked, ready, failed int)
	savingPoolCallback      func(path string, count int)
	savedPoolCallback       func(path string, count int)
	initializedCallback     func(count int, fromCache bool)
	errorCallback           func(stage string, err error)
}

type savedPool struct {
	UpdatedAt int64    `json:"updated_at"`
	Proxies   []string `json:"proxies"`
}

func NewProxyManager() *Manager {
	return &Manager{
		ctx:           context.Background(),
		sources:       append([]string(nil), sourceURLs...),
		scrapers:      append([]string(nil), scraperURLs...),
		gitHubMirrors: append([]string(nil), defaultGitHubMirrors...),
		checkThreads:  defaultCheckThreads,
		checkTimeout:  defaultCheckTimeout,
		checkURL:      defaultCheckURL,
		minReady:      defaultMinReady,
		httpClient:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (m *Manager) WithContext(ctx context.Context) *Manager {
	if m == nil {
		return m
	}
	if ctx != nil {
		m.ctx = ctx
	}
	return m
}

func (m *Manager) WithStoragePath(path string) *Manager {
	if m == nil {
		return m
	}
	m.storagePath = strings.TrimSpace(path)
	return m
}

func (m *Manager) WithSources(sources []string) *Manager {
	if m == nil {
		return m
	}
	m.sources = cleanStrings(sources)
	return m
}

func (m *Manager) WithScrapers(scrapers []string) *Manager {
	if m == nil {
		return m
	}
	m.scrapers = cleanStrings(scrapers)
	return m
}

func (m *Manager) WithGitHubMirrors(mirrors ...string) *Manager {
	if m == nil {
		return m
	}
	m.gitHubMirrors = cleanStrings(mirrors)
	return m
}

func (m *Manager) WithCheckThreads(threads int) *Manager {
	if m == nil {
		return m
	}
	if threads > 0 {
		m.checkThreads = threads
	}
	return m
}

func (m *Manager) WithCheckTimeout(timeout time.Duration) *Manager {
	if m == nil {
		return m
	}
	if timeout > 0 {
		m.checkTimeout = timeout
	}
	return m
}

func (m *Manager) WithCheckURL(checkURL string) *Manager {
	if m == nil {
		return m
	}
	if strings.TrimSpace(checkURL) != "" {
		m.checkURL = strings.TrimSpace(checkURL)
	}
	return m
}

func (m *Manager) WithMinReady(min int) *Manager {
	if m == nil {
		return m
	}
	if min > 0 {
		m.minReady = min
	}
	return m
}

// WithCollectAll keeps checking every fetched candidate and stores all valid
// proxies instead of stopping when the minimum ready count is reached.
func (m *Manager) WithCollectAll() *Manager {
	if m != nil {
		m.collectAll = true
	}
	return m
}

func (m *Manager) WithHTTPClient(client *http.Client) *Manager {
	if m == nil {
		return m
	}
	if client != nil {
		m.httpClient = client
	}
	return m
}

func (m *Manager) WithLoadingSavedCallback(callback func(path string)) *Manager {
	if m == nil {
		return m
	}
	m.loadingSavedCallback = callback
	return m
}

func (m *Manager) WithLoadedSavedCallback(callback func(path string, count int)) *Manager {
	if m == nil {
		return m
	}
	m.loadedSavedCallback = callback
	return m
}

func (m *Manager) WithFetchingSourceCallback(callback func(source string)) *Manager {
	if m == nil {
		return m
	}
	m.fetchingSourceCallback = callback
	return m
}

func (m *Manager) WithFetchedSourceCallback(callback func(source string, count int)) *Manager {
	if m == nil {
		return m
	}
	m.fetchedSourceCallback = callback
	return m
}

func (m *Manager) WithFetchingScraperCallback(callback func(source string)) *Manager {
	if m == nil {
		return m
	}
	m.fetchingScraperCallback = callback
	return m
}

func (m *Manager) WithFetchedScraperCallback(callback func(source string, count int)) *Manager {
	if m == nil {
		return m
	}
	m.fetchedScraperCallback = callback
	return m
}

func (m *Manager) WithCheckingProxiesCallback(callback func(total, workers int)) *Manager {
	if m == nil {
		return m
	}
	m.checkingProxiesCallback = callback
	return m
}

func (m *Manager) WithCheckedProxiesCallback(callback func(checked, ready, failed int)) *Manager {
	if m == nil {
		return m
	}
	m.checkedProxiesCallback = callback
	return m
}

func (m *Manager) WithSavingPoolCallback(callback func(path string, count int)) *Manager {
	if m == nil {
		return m
	}
	m.savingPoolCallback = callback
	return m
}

func (m *Manager) WithSavedPoolCallback(callback func(path string, count int)) *Manager {
	if m == nil {
		return m
	}
	m.savedPoolCallback = callback
	return m
}

func (m *Manager) WithInitializedCallback(callback func(count int, fromCache bool)) *Manager {
	if m == nil {
		return m
	}
	m.initializedCallback = callback
	return m
}

func (m *Manager) WithErrorCallback(callback func(stage string, err error)) *Manager {
	if m == nil {
		return m
	}
	m.errorCallback = callback
	return m
}

func (m *Manager) Init() ([]string, error) {
	if m == nil {
		return nil, errors.New("proxy manager is nil")
	}
	ctx := m.context()
	path := m.poolPath()

	if path != "" {
		m.notifyLoadingSaved(path)
		proxies, err := loadSavedPool(path)
		if err == nil && len(proxies) > 0 {
			SetHTTPProxies(proxies)
			out := HTTPProxies()
			m.notifyLoadedSaved(path, len(out))
			m.notifyInitialized(len(out), true)
			return out, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			m.notifyError("load saved proxy pool", err)
		}
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	return m.Fetch()
}

func (m *Manager) Fetch() ([]string, error) {
	if m == nil {
		return nil, errors.New("proxy manager is nil")
	}
	ctx := m.context()
	path := m.poolPath()

	raw, err := m.fetchRaw(ctx)
	if err != nil {
		m.notifyError("fetch proxy sources", err)
		return nil, err
	}
	ready := m.checkProxies(ctx, raw)
	if len(ready) == 0 {
		err := errors.New("no verified http proxies")
		m.notifyError("check proxy pool", err)
		return nil, err
	}

	if path != "" {
		m.notifySavingPool(path, len(ready))
		if err := savePool(path, ready); err != nil {
			m.notifyError("save proxy pool", err)
		} else {
			m.notifySavedPool(path, len(ready))
		}
	}

	SetHTTPProxies(ready)
	out := HTTPProxies()
	m.notifyInitialized(len(out), false)
	return out, nil
}

func HTTPProxies() []string {
	pool.Lock()
	defer pool.Unlock()
	return append([]string(nil), pool.proxies...)
}

func SetHTTPProxies(proxies []string) {
	pool.Lock()
	pool.proxies = cleanProxies(proxies)
	pool.Unlock()
}

// RemoveHTTPProxy removes a failed proxy from the in-memory pool. The saved
// cache is refreshed by the service's pool manager when the ready count drops.
func RemoveHTTPProxy(proxy string) {
	targetHost := proxyHost(proxy)
	if targetHost == "" {
		return
	}
	pool.Lock()
	defer pool.Unlock()
	filtered := pool.proxies[:0]
	for _, candidate := range pool.proxies {
		if proxyHost(candidate) != targetHost {
			filtered = append(filtered, candidate)
		}
	}
	pool.proxies = filtered
}

// BlacklistHTTPProxy permanently excludes a failed proxy for this process.
// Future cache loads and source refreshes filter it before adding it back.
func BlacklistHTTPProxy(proxy string) {
	host := proxyHost(proxy)
	if host == "" {
		return
	}
	loadBlacklistedProxies()
	blacklistedProxies.Lock()
	blacklistedProxies.hosts[host] = struct{}{}
	if err := saveBlacklistedProxiesLocked(); err != nil {
		log.Printf("[proxy] 保存代理黑名单失败: %v", err)
	}
	blacklistedProxies.Unlock()
	RemoveHTTPProxy(proxy)
}

func IsHTTPProxyBlacklisted(proxy string) bool {
	host := proxyHost(proxy)
	if host == "" {
		return true
	}
	loadBlacklistedProxies()
	blacklistedProxies.RLock()
	_, blacklisted := blacklistedProxies.hosts[host]
	blacklistedProxies.RUnlock()
	return blacklisted
}

func loadBlacklistedProxies() {
	blacklistLoadOnce.Do(func() {
		data, err := os.ReadFile(blacklistFileName)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				log.Printf("[proxy] 读取代理黑名单失败: %v", err)
			}
			return
		}
		var hosts []string
		if err := json.Unmarshal(data, &hosts); err != nil {
			log.Printf("[proxy] 解析代理黑名单失败: %v", err)
			return
		}
		blacklistedProxies.Lock()
		for _, host := range hosts {
			if normalized := proxyHost(host); normalized != "" {
				blacklistedProxies.hosts[normalized] = struct{}{}
			}
		}
		blacklistedProxies.Unlock()
	})
}

func saveBlacklistedProxiesLocked() error {
	hosts := make([]string, 0, len(blacklistedProxies.hosts))
	for host := range blacklistedProxies.hosts {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	data, err := json.MarshalIndent(hosts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(blacklistFileName, data, 0644)
}

func RandomProxy(ctx context.Context) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	proxies, err := ensureProxies(ctx)
	if err != nil {
		return "", err
	}
	if len(proxies) == 0 {
		return "", errors.New("http proxy list is empty")
	}
	index, err := cryptoRandInt(len(proxies))
	if err != nil {
		return "", err
	}
	return proxies[index], nil
}

func RandomProxyURL(ctx context.Context) (*url.URL, error) {
	proxy, err := RandomProxy(ctx)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(proxy, "://") {
		proxy = "http://" + proxy
	}
	return url.Parse(proxy)
}

func RandomLoginProxyURL(ctx context.Context) (*url.URL, error) {
	return RandomProxyURL(ctx)
}

func NewLoginHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL := environmentProxyURL(); proxyURL != nil {
		if isSocksProxy(proxyURL) {
			dialer, err := xnetproxy.FromURL(proxyURL, xnetproxy.Direct)
			if err != nil {
				return &http.Client{Timeout: timeout, Transport: transport}
			}
			transport.Proxy = nil
			if contextDialer, ok := dialer.(xnetproxy.ContextDialer); ok {
				transport.DialContext = contextDialer.DialContext
			} else {
				transport.DialContext = func(_ context.Context, network, address string) (net.Conn, error) {
					return dialer.Dial(network, address)
				}
			}
		} else {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}

func environmentProxyURL() *url.URL {
	for _, key := range []string{"ALL_PROXY", "all_proxy", "HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		raw := strings.TrimSpace(os.Getenv(key))
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err == nil && parsed.Scheme != "" && parsed.Host != "" {
			return parsed
		}
	}
	return nil
}

func isSocksProxy(proxyURL *url.URL) bool {
	if proxyURL == nil {
		return false
	}
	scheme := strings.ToLower(proxyURL.Scheme)
	return scheme == "socks5" || scheme == "socks5h"
}

func ensureProxies(ctx context.Context) ([]string, error) {
	pool.Lock()
	if len(pool.proxies) > 0 {
		proxies := append([]string(nil), pool.proxies...)
		pool.Unlock()
		return proxies, nil
	}
	pool.Unlock()

	verbose := proxyVerbose()
	if verbose {
		log.Println("[proxy] 正在初始化代理池...")
	}
	manager := NewProxyManager().WithContext(ctx)
	if verbose {
		manager.
			WithLoadingSavedCallback(func(path string) {
				log.Printf("[proxy] 正在读取缓存: %s", path)
			}).
			WithLoadedSavedCallback(func(path string, count int) {
				log.Printf("[proxy] 缓存加载成功: %s count=%d", path, count)
			}).
			WithFetchingSourceCallback(func(source string) {
				log.Printf("[proxy] 正在拉取: %s", source)
			}).
			WithFetchedSourceCallback(func(source string, count int) {
				log.Printf("[proxy] 拉取完成: %s added=%d", source, count)
			}).
			WithFetchingScraperCallback(func(source string) {
				log.Printf("[proxy] 正在抓取: %s", source)
			}).
			WithFetchedScraperCallback(func(source string, count int) {
				log.Printf("[proxy] 抓取完成: %s added=%d", source, count)
			}).
			WithCheckingProxiesCallback(func(total, workers int) {
				log.Printf("[proxy] 正在验证代理: total=%d workers=%d", total, workers)
			}).
			WithCheckedProxiesCallback(func(checked, ready, failed int) {
				log.Printf("[proxy] 验证进度: checked=%d ready=%d failed=%d", checked, ready, failed)
			}).
			WithSavingPoolCallback(func(path string, count int) {
				log.Printf("[proxy] 正在保存缓存: %s count=%d", path, count)
			}).
			WithSavedPoolCallback(func(path string, count int) {
				log.Printf("[proxy] 缓存保存完成: %s count=%d", path, count)
			}).
			WithErrorCallback(func(stage string, err error) {
				log.Printf("[proxy] %s 失败: %v", stage, err)
			})
	}
	proxies, err := manager.Init()
	if err != nil {
		if verbose {
			log.Printf("[proxy] 初始化失败: %v", err)
		}
		return nil, err
	}
	if verbose {
		log.Printf("[proxy] 代理池就绪: count=%d", len(proxies))
	}
	return proxies, nil
}

func proxyVerbose() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PROXY_VERBOSE"))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		mainPath := filepath.ToSlash(info.Main.Path)
		return strings.Contains(mainPath, "/cmd/test/")
	}
	return false
}

func (m *Manager) fetchRaw(ctx context.Context) ([]string, error) {
	seen := map[string]struct{}{}
	var raw []string
	var lastErr error

	for _, source := range m.sources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		m.notifyFetchingSource(source)
		proxies, err := m.fetchSource(ctx, source)
		if err != nil {
			lastErr = err
			m.notifyError("fetch proxy source", err)
			continue
		}
		added := appendUniqueProxy(&raw, seen, proxies)
		m.notifyFetchedSource(source, added)
	}

	for _, source := range m.scrapers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		m.notifyFetchingScraper(source)
		proxies, err := m.fetchScraper(ctx, source)
		if err != nil {
			lastErr = err
			m.notifyError("scrape proxy source", err)
			continue
		}
		added := appendUniqueProxy(&raw, seen, proxies)
		m.notifyFetchedScraper(source, added)
	}

	if len(raw) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, errors.New("no proxies fetched")
	}
	return raw, nil
}

func (m *Manager) fetchSource(ctx context.Context, sourceURL string) ([]string, error) {
	tryURLs := m.buildSourceURLs(sourceURL)
	var lastErr error
	for i, tryURL := range tryURLs {
		isMirror := i < len(m.gitHubMirrors)
		timeout := m.httpClient.Timeout
		if isMirror {
			timeout = githubMirrorTimeout
		}
		fetchCtx := ctx
		var cancel context.CancelFunc
		if timeout > 0 {
			fetchCtx, cancel = context.WithTimeout(ctx, timeout)
		}
		proxies, err := m.fetchSourceOnce(fetchCtx, tryURL)
		if cancel != nil {
			cancel()
		}
		if err == nil {
			if isMirror {
				if proxyVerbose() {
					log.Printf("[proxy] 加速源可用: %s", tryURL)
				}
			}
			return proxies, nil
		}
		lastErr = err
		if isMirror {
			if proxyVerbose() {
				log.Printf("[proxy] 加速源失败，回退原始地址: %s (%v)", tryURL, err)
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("fetch http proxy source %s: no urls to try", sourceURL)
	}
	return nil, lastErr
}

func (m *Manager) buildSourceURLs(sourceURL string) []string {
	if !isGitHubURL(sourceURL) || len(m.gitHubMirrors) == 0 {
		return []string{sourceURL}
	}
	result := make([]string, 0, len(m.gitHubMirrors)+1)
	for _, mirror := range m.gitHubMirrors {
		result = append(result, mirror+sourceURL)
	}
	result = append(result, sourceURL)
	return result
}

func isGitHubURL(u string) bool {
	return strings.HasPrefix(u, "https://raw.githubusercontent.com/") ||
		strings.HasPrefix(u, "https://github.com/")
}

func (m *Manager) fetchSourceOnce(ctx context.Context, sourceURL string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("fetch http proxy source %s: %s", sourceURL, resp.Status)
	}

	var proxies []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		proxy := normalizeLine(scanner.Text())
		if proxy != "" {
			proxies = append(proxies, proxy)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(proxies) == 0 {
		return nil, fmt.Errorf("http proxy source %s returned no proxies", sourceURL)
	}
	return cleanProxies(proxies), nil
}

func (m *Manager) fetchScraper(ctx context.Context, sourceURL string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("scrape http proxy source %s: %s", sourceURL, resp.Status)
	}

	var proxies []string
	scanner := bufio.NewScanner(resp.Body)
	ipPortPattern := regexp.MustCompile(`(\d{1,3}(?:\.\d{1,3}){3})[^\d](\d{2,5})`)
	for scanner.Scan() {
		for _, match := range ipPortPattern.FindAllStringSubmatch(scanner.Text(), -1) {
			proxy := normalizeLine(match[1] + ":" + match[2])
			if proxy != "" {
				proxies = append(proxies, proxy)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(proxies) == 0 {
		return nil, fmt.Errorf("scrape http proxy source %s returned no proxies", sourceURL)
	}
	return cleanProxies(proxies), nil
}

func (m *Manager) checkProxies(ctx context.Context, raw []string) []string {
	raw = cleanProxies(raw)
	if len(raw) == 0 {
		return nil
	}
	workers := m.checkThreads
	if workers <= 0 {
		workers = defaultCheckThreads
	}
	if workers > len(raw) {
		workers = len(raw)
	}
	m.notifyCheckingProxies(len(raw), workers)

	checkCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan string)
	results := make(chan checkedProxy)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for proxy := range jobs {
				if checkCtx.Err() != nil {
					results <- checkedProxy{proxy: proxy, ok: false}
					continue
				}
				results <- checkedProxy{proxy: proxy, ok: m.checkOne(checkCtx, proxy)}
			}
		}()
	}
	go func() {
		for _, proxy := range raw {
			if checkCtx.Err() != nil {
				break
			}
			jobs <- proxy
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	seen := map[string]struct{}{}
	var ready []string
	checked := 0
	failed := 0
	stoppedEarly := false
	for result := range results {
		checked++
		// Workers may finish concurrently after cancellation. Do not let those
		// late results grow the cache past the configured ready target.
		if result.ok && (m.collectAll || len(ready) < m.minReady) {
			if _, ok := seen[result.proxy]; !ok {
				seen[result.proxy] = struct{}{}
				ready = append(ready, result.proxy)
			}
		} else {
			failed++
		}
		if !m.collectAll && len(ready) >= m.minReady && !stoppedEarly {
			stoppedEarly = true
			if proxyVerbose() {
				log.Printf("[proxy] 已达到最小可用数 %d，提前停止验证", m.minReady)
			}
			cancel()
		}
		if checked%200 == 0 || checked == len(raw) {
			m.notifyCheckedProxies(checked, len(ready), failed)
		}
	}
	if stoppedEarly {
		m.notifyCheckedProxies(checked, len(ready), failed)
	}
	return ready
}

type checkedProxy struct {
	proxy string
	ok    bool
}

func (m *Manager) checkOne(ctx context.Context, proxy string) bool {
	raw := proxy
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	proxyURL, err := url.Parse(raw)
	if err != nil || proxyURL.Host == "" {
		return false
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	client := &http.Client{
		Timeout:   m.checkTimeout,
		Transport: transport,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.checkURL, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (m *Manager) context() context.Context {
	if m.ctx == nil {
		return context.Background()
	}
	return m.ctx
}

func (m *Manager) poolPath() string {
	if strings.TrimSpace(m.storagePath) != "" {
		return filepath.Join(strings.TrimSpace(m.storagePath), proxyPoolFileName)
	}
	return ""
}

func loadSavedPool(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var saved savedPool
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, err
	}
	proxies := cleanProxies(saved.Proxies)
	if len(proxies) == 0 {
		return nil, errors.New("saved proxy pool is empty")
	}
	return proxies, nil
}

func savePool(path string, proxies []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(savedPool{
		UpdatedAt: time.Now().Unix(),
		Proxies:   cleanProxies(proxies),
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func appendUniqueProxy(out *[]string, seen map[string]struct{}, proxies []string) int {
	added := 0
	for _, proxy := range proxies {
		proxy = normalizeLine(proxy)
		if proxy == "" {
			continue
		}
		if _, ok := seen[proxy]; ok {
			continue
		}
		seen[proxy] = struct{}{}
		*out = append(*out, proxy)
		added++
	}
	return added
}

func cleanProxies(proxies []string) []string {
	seen := make(map[string]struct{}, len(proxies))
	cleaned := make([]string, 0, len(proxies))
	for _, line := range proxies {
		proxy := normalizeLine(line)
		if proxy == "" {
			continue
		}
		if IsHTTPProxyBlacklisted(proxy) {
			continue
		}
		if _, ok := seen[proxy]; ok {
			continue
		}
		seen[proxy] = struct{}{}
		cleaned = append(cleaned, proxy)
	}
	return cleaned
}

func cleanStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func normalizeLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return ""
	}
	fields := strings.Fields(strings.Trim(line, "',\""))
	if len(fields) == 0 {
		return ""
	}
	proxy := strings.Trim(fields[0], "',\"")
	if proxy == "" {
		return ""
	}
	if strings.Contains(proxy, "://") &&
		!strings.HasPrefix(proxy, "http://") &&
		!strings.HasPrefix(proxy, "https://") {
		return ""
	}

	raw := proxy
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return proxy
}

func proxyHost(proxy string) string {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return ""
	}
	if !strings.Contains(proxy, "://") {
		proxy = "http://" + proxy
	}
	parsed, err := url.Parse(proxy)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Host)
}

func (m *Manager) notifyLoadingSaved(path string) {
	if m.loadingSavedCallback != nil {
		m.loadingSavedCallback(path)
	}
}

func (m *Manager) notifyLoadedSaved(path string, count int) {
	if m.loadedSavedCallback != nil {
		m.loadedSavedCallback(path, count)
	}
}

func (m *Manager) notifyFetchingSource(source string) {
	if m.fetchingSourceCallback != nil {
		m.fetchingSourceCallback(source)
	}
}

func (m *Manager) notifyFetchedSource(source string, count int) {
	if m.fetchedSourceCallback != nil {
		m.fetchedSourceCallback(source, count)
	}
}

func (m *Manager) notifyFetchingScraper(source string) {
	if m.fetchingScraperCallback != nil {
		m.fetchingScraperCallback(source)
	}
}

func (m *Manager) notifyFetchedScraper(source string, count int) {
	if m.fetchedScraperCallback != nil {
		m.fetchedScraperCallback(source, count)
	}
}

func (m *Manager) notifyCheckingProxies(total, workers int) {
	if m.checkingProxiesCallback != nil {
		m.checkingProxiesCallback(total, workers)
	}
}

func (m *Manager) notifyCheckedProxies(checked, ready, failed int) {
	if m.checkedProxiesCallback != nil {
		m.checkedProxiesCallback(checked, ready, failed)
	}
}

func (m *Manager) notifySavingPool(path string, count int) {
	if m.savingPoolCallback != nil {
		m.savingPoolCallback(path, count)
	}
}

func (m *Manager) notifySavedPool(path string, count int) {
	if m.savedPoolCallback != nil {
		m.savedPoolCallback(path, count)
	}
}

func (m *Manager) notifyInitialized(count int, fromCache bool) {
	if m.initializedCallback != nil {
		m.initializedCallback(count, fromCache)
	}
}

func (m *Manager) notifyError(stage string, err error) {
	if m.errorCallback != nil {
		m.errorCallback(strings.TrimSpace(stage), err)
	}
}

func cryptoRandInt(max int) (int, error) {
	if max <= 0 {
		return 0, errors.New("random range must be positive")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}
