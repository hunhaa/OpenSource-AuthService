package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	httpproxy "github.com/Yeah114/FunAuth/utils/proxy"
	"github.com/go-sql-driver/mysql"
)

const (
	ipHistoryDSN       = "ipproxy:密码@tcp(127.0.0.1:3306)/ipproxy?charset=utf8mb4&parseTime=True&loc=Local"
	ipHistoryMaxChecks = 16
)

type ipHistoryStore struct {
	db *sql.DB
	mu sync.Mutex
}

func newIPHistoryStore(ctx context.Context) (*ipHistoryStore, error) {
	db, err := sql.Open("mysql", ipHistoryDSN)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(ipHistoryMaxChecks)
	db.SetMaxIdleConns(ipHistoryMaxChecks)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := &ipHistoryStore{db: db}
	if err := store.ensureSchema(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *ipHistoryStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *ipHistoryStore) ensureSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS ips (
	id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
	ip VARCHAR(64) NOT NULL,
	proxy VARCHAR(255) NOT NULL,
	used_date DATE NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE KEY uniq_ip_used_date (ip, used_date),
	KEY idx_used_date (used_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	return err
}

func (s *ipHistoryStore) filterNewProxies(ctx context.Context, proxies []httpproxy.PaidProxyInfo) []httpproxy.PaidProxyInfo {
	if s == nil || len(proxies) == 0 {
		return proxies
	}

	// Keep the lookup and insert together so overlapping fetches cannot both
	// admit the same IP before the unique database index is updated.
	s.mu.Lock()
	defer s.mu.Unlock()

	candidates, ips := uniqueProxyCandidates(proxies)
	if len(candidates) == 0 {
		return nil
	}
	today := time.Now().Format("2006-01-02")
	existing, err := s.existingIPs(ctx, today, ips)
	if err != nil {
		log.Printf("batch proxy dedup lookup failed: count=%d err=%v", len(candidates), err)
		return nil
	}

	newProxies := make([]httpproxy.PaidProxyInfo, 0, len(candidates))
	for _, proxy := range candidates {
		if _, exists := existing[proxyIP(proxy.Address)]; !exists {
			newProxies = append(newProxies, proxy)
		}
	}
	if len(newProxies) == 0 {
		return nil
	}
	if err := s.insertNewProxies(ctx, today, newProxies); err != nil {
		log.Printf("batch proxy dedup insert failed: count=%d err=%v", len(newProxies), err)
		return nil
	}
	return newProxies
}

func uniqueProxyCandidates(proxies []httpproxy.PaidProxyInfo) ([]httpproxy.PaidProxyInfo, []string) {
	seen := make(map[string]struct{}, len(proxies))
	candidates := make([]httpproxy.PaidProxyInfo, 0, len(proxies))
	ips := make([]string, 0, len(proxies))
	for _, proxy := range proxies {
		ip := proxyIP(proxy.Address)
		if ip == "" {
			continue
		}
		if _, exists := seen[ip]; exists {
			continue
		}
		seen[ip] = struct{}{}
		candidates = append(candidates, proxy)
		ips = append(ips, ip)
	}
	return candidates, ips
}

func (s *ipHistoryStore) existingIPs(ctx context.Context, date string, ips []string) (map[string]struct{}, error) {
	args := make([]any, 0, len(ips)+1)
	args = append(args, date)
	for _, ip := range ips {
		args = append(args, ip)
	}
	query := "SELECT ip FROM ips WHERE used_date = ? AND ip IN (" + strings.TrimSuffix(strings.Repeat("?,", len(ips)), ",") + ")"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	existing := make(map[string]struct{}, len(ips))
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return nil, err
		}
		existing[ip] = struct{}{}
	}
	return existing, rows.Err()
}

func (s *ipHistoryStore) insertNewProxies(ctx context.Context, date string, proxies []httpproxy.PaidProxyInfo) error {
	args := make([]any, 0, len(proxies)*3)
	values := make([]string, 0, len(proxies))
	for _, proxy := range proxies {
		args = append(args, proxyIP(proxy.Address), proxy.Address, date)
		values = append(values, "(?, ?, ?)")
	}
	_, err := s.db.ExecContext(ctx, "INSERT IGNORE INTO ips (ip, proxy, used_date) VALUES "+strings.Join(values, ","), args...)
	return err
}

func (s *ipHistoryStore) filterNewProxiesSlow(ctx context.Context, proxies []httpproxy.PaidProxyInfo) []httpproxy.PaidProxyInfo {
	if s == nil || len(proxies) == 0 {
		return proxies
	}
	type result struct {
		index int
		keep  bool
	}
	sem := make(chan struct{}, ipHistoryMaxChecks)
	results := make(chan result, len(proxies))
	var wg sync.WaitGroup
	for index, proxy := range proxies {
		index, proxy := index, proxy
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			keep, err := s.markProxy(ctx, proxy)
			if err != nil {
				log.Printf("临时 IP 库检查失败，丢弃代理: proxy=%s err=%v", proxy.Address, err)
			}
			results <- result{index: index, keep: keep && err == nil}
		}()
	}
	wg.Wait()
	close(results)

	keep := make([]bool, len(proxies))
	for item := range results {
		keep[item.index] = item.keep
	}
	filtered := make([]httpproxy.PaidProxyInfo, 0, len(proxies))
	for index, proxy := range proxies {
		if keep[index] {
			filtered = append(filtered, proxy)
		}
	}
	return filtered
}

func (s *ipHistoryStore) markProxy(ctx context.Context, proxy httpproxy.PaidProxyInfo) (bool, error) {
	ip := proxyIP(proxy.Address)
	if ip == "" {
		return false, fmt.Errorf("empty proxy ip")
	}
	today := time.Now().Format("2006-01-02")
	_, err := s.db.ExecContext(ctx, `INSERT INTO ips (ip, proxy, used_date) VALUES (?, ?, ?)`, ip, proxy.Address, today)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return false, nil
		}
		return false, err
	}
	if err := s.appendText(proxy, ip); err != nil {
		log.Printf("写入 ips.txt 失败: ip=%s proxy=%s err=%v", ip, proxy.Address, err)
	}
	return true, nil
}

func (s *ipHistoryStore) appendText(proxy httpproxy.PaidProxyInfo, ip string) error {
	return nil
}

func proxyIP(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	parsed, err := url.Parse(address)
	if err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	if at := strings.LastIndex(address, "@"); at >= 0 {
		address = address[at+1:]
	}
	if colon := strings.LastIndex(address, ":"); colon > 0 {
		return strings.Trim(address[:colon], "[]")
	}
	return strings.Trim(address, "[]")
}
