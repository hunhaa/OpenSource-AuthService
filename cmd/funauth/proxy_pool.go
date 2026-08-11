package main

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"github.com/Yeah114/FunAuth/internal/db"
	httpproxy "github.com/Yeah114/FunAuth/utils/proxy"
)

const (
	// Keep a small verified cache available before the account pool begins
	// registering. The larger target is filled in the background afterwards.
	funauthProxyStartMinReady = 6
	funauthProxyTargetReady   = 40
	funauthProxyRefillAt      = 20
	funauthProxyInitWait      = 90 * time.Second
)

var proxyPoolRefreshInFlight atomic.Bool

// initProxyPoolThenCom4399 runs off the request-serving path. It verifies an
// in-memory proxy pool before starting Com4399Pool.
func initProxyPoolThenCom4399() {
	if !httpproxy.Auth4399Proxy && !httpproxy.AuthG79clientProxy {
		log.Printf("[ProxyCachePool] Auth4399Proxy 和 AuthG79clientProxy 均已关闭，跳过代理池初始化")
		return
	}

	log.Printf("[ProxyCachePool] 已启动后台初始化任务")
	go func() {
		proxies, err := initProxyPool()
		if err != nil {
			log.Printf("[Com4399Pool] 代理缓存初始化失败，不启动 4399 账号池: %v", err)
			return
		}
		if len(proxies) < funauthProxyStartMinReady {
			log.Printf("[Com4399Pool] 可用代理缓存不足 (%d/%d)，不启动 4399 账号池", len(proxies), funauthProxyStartMinReady)
			return
		}
		log.Printf("[Com4399Pool] 代理缓存初始化结束，开始初始化 4399 账号池")
		db.InitCom4399AccountPool()

		if len(proxies) < funauthProxyTargetReady {
			go refreshProxyPool()
		}
		go monitorProxyPool()
	}()
}

func monitorProxyPool() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		count := len(httpproxy.HTTPProxies())
		if count < funauthProxyRefillAt && !proxyPoolRefreshInFlight.Load() {
			log.Printf("[ProxyCachePool] 可用代理不足 (%d/%d)，开始后台补充", count, funauthProxyRefillAt)
			go refreshProxyPool()
		}
	}
}

func initProxyPool() ([]string, error) {
	ctx := context.Background()

	log.Printf("[ProxyCachePool] 初始化代理缓存：全量拉取并验证所有候选代理，不设数量上限")
	proxies, err := httpproxy.NewProxyManager().
		WithContext(ctx).
		WithMinReady(funauthProxyTargetReady).
		WithCollectAll().
		WithCheckingProxiesCallback(func(total, workers int) {
			log.Printf("[ProxyCachePool] 开始全量验证代理：候选=%d，并发=%d", total, workers)
		}).
		WithCheckedProxiesCallback(func(checked, ready, failed int) {
			log.Printf("[ProxyCachePool] 全量验证进度：已检查=%d，可用=%d，失败=%d", checked, ready, failed)
		}).
		Init()
	if err != nil {
		log.Printf("[ProxyCachePool] 初始化失败: %v", err)
		return nil, err
	}
	log.Printf("[ProxyCachePool] 全量验证完成，已缓存可用代理: %d", len(proxies))
	return proxies, nil
}

// refreshProxyPool refreshes the in-memory pool up to the target.
func refreshProxyPool() {
	if !proxyPoolRefreshInFlight.CompareAndSwap(false, true) {
		return
	}
	defer proxyPoolRefreshInFlight.Store(false)

	ctx, cancel := context.WithTimeout(context.Background(), funauthProxyInitWait)
	defer cancel()

	log.Printf("[ProxyCachePool] 后台补充代理缓存，目标可用数: %d", funauthProxyTargetReady)
	proxies, err := httpproxy.NewProxyManager().
		WithContext(ctx).
		WithMinReady(funauthProxyTargetReady).
		Fetch()
	if err != nil {
		log.Printf("[ProxyCachePool] 后台补充失败，保留当前代理缓存: %v", err)
		return
	}
	if len(proxies) < funauthProxyTargetReady {
		log.Printf("[ProxyCachePool] 本轮补充未达目标，当前可用代理: %d/%d，将继续补充", len(proxies), funauthProxyTargetReady)
		time.AfterFunc(time.Second, refreshProxyPool)
		return
	}
	log.Printf("[ProxyCachePool] 后台补充完成，已缓存可用代理: %d/%d", len(proxies), funauthProxyTargetReady)
}
