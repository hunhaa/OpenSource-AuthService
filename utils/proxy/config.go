package proxy

const (
	// Auth4399Proxy and AuthG79clientProxy keep funauth on the free proxy pool.
	Auth4399Proxy      = false
	AuthG79clientProxy = false

	// Com4399Proxy and G79clientProxy control paid-proxy use by com4399register.
	Com4399Proxy   = true
	G79clientProxy = true

	// Qingguo short-lived proxy settings. Fill these with credentials from the
	// Qingguo console before enabling paid proxy registration.
	// 这个代理是专门用来提供给 cmd\com4399register 批量注册账号的 ，验证服务需要使用免费的公共代理池，不能用这个
	qingguoProxyEndpoint = "https://overseas.proxy.qg.net/get"

	// The provider currently allows at most 100 IPs per extraction.
	qingguoProxyBatchSize        = 1
	qingguoProxyFallbackLifetime = 30
)

var (
	// Fill these with the product key and proxy authentication credentials.
	qingguoProxyKey     = ""
	qingguoProxyAuthKey = ""
	qingguoProxyAuthPwd = ""
)

// Com4399RegisterMaxWorkers limits active paid-proxy registration workers.
// Set it to -1 to allow unlimited workers.
var Com4399RegisterMaxWorkers = -1

// Com4399DirectSingleThread enables a direct, single-worker registration mode
// for connectivity testing. Set false to return to paid-proxy scheduling.
var Com4399DirectSingleThread = false

// Com4399RegisterDeduplicateProxy controls daily paid-proxy IP deduplication.
// Disable it to reuse previously seen proxy IPs without opening the IP history store.
var Com4399RegisterDeduplicateProxy = false
