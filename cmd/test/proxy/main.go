package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/Yeah114/FunAuth/utils/proxy"
)

func main() {
	threads := flag.Int("threads", 1000, "代理验证并发数量")
	timeout := flag.Duration("timeout", 5*time.Second, "单个代理验证超时时间")
	cacheDir := flag.String("cache", "", "代理缓存目录，留空则不使用缓存")
	flag.Parse()

	startedAt := time.Now()
	mgr := proxy.NewProxyManager().
		WithContext(context.Background()).
		WithCheckThreads(*threads).
		WithCheckTimeout(*timeout).
		WithFetchingSourceCallback(func(source string) {
			fmt.Printf("正在拉取代理源: %s\n", source)
		}).
		WithFetchedSourceCallback(func(source string, count int) {
			fmt.Printf("代理源拉取完成: %s added=%d\n", source, count)
		}).
		WithFetchingScraperCallback(func(source string) {
			fmt.Printf("正在抓取代理页面: %s\n", source)
		}).
		WithFetchedScraperCallback(func(source string, count int) {
			fmt.Printf("代理页面抓取完成: %s added=%d\n", source, count)
		}).
		WithCheckingProxiesCallback(func(total, workers int) {
			fmt.Printf("正在验证代理: total=%d workers=%d\n", total, workers)
		}).
		WithCheckedProxiesCallback(func(checked, ready, failed int) {
			fmt.Printf("代理验证进度: checked=%d ready=%d failed=%d\n", checked, ready, failed)
		}).
		WithErrorCallback(func(stage string, err error) {
			fmt.Printf("代理池阶段失败: stage=%s err=%v\n", stage, err)
		})

	if *cacheDir != "" {
		mgr = mgr.WithStoragePath(*cacheDir)
	}

	proxies, err := mgr.Fetch()
	if err != nil {
		panic(err)
	}

	fmt.Printf("代理池刷新完成: count=%d elapsed=%s\n", len(proxies), time.Since(startedAt).Round(time.Millisecond))
	if len(proxies) > 0 {
		fmt.Printf("示例代理: %s\n", proxies[0])
	}
}
