package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PaidHTTPProxies obtains a fresh batch from Qingguo's short-lived HTTP proxy pool.
// It never reads or mutates the free public proxy pool.
func PaidHTTPProxies(ctx context.Context) ([]string, error) {
	infos, err := PaidHTTPProxyInfos(ctx)
	if err != nil {
		return nil, err
	}
	return paidProxyAddresses(infos), nil
}

type PaidProxyInfo struct {
	Address   string
	ExpiresAt time.Time
}

func PaidHTTPProxyInfos(ctx context.Context) ([]PaidProxyInfo, error) {
	return paidHTTPProxyInfos(ctx, qingguoProxyEndpoint, &http.Client{Timeout: 15 * time.Second})
}

func paidHTTPProxies(ctx context.Context, endpoint string, client *http.Client) ([]string, error) {
	infos, err := paidHTTPProxyInfos(ctx, endpoint, client)
	if err != nil {
		return nil, err
	}
	return paidProxyAddresses(infos), nil
}

func paidHTTPProxyInfos(ctx context.Context, endpoint string, client *http.Client) ([]PaidProxyInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	requestURL, err := qingguoProxyRequestURL(endpoint)
	if err != nil {
		return nil, fmt.Errorf("build Qingguo proxy request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build Qingguo proxy request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request Qingguo proxy: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Qingguo proxy response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Qingguo proxy returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	proxies, err := parsePaidProxyInfoResponse(body)
	if err != nil {
		return nil, err
	}
	return proxies, nil
}

func qingguoProxyRequestURL(endpoint string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid Qingguo endpoint %q", endpoint)
	}
	if strings.TrimSpace(qingguoProxyKey) == "" {
		return "", fmt.Errorf("Qingguo proxy key is empty")
	}
	query := parsed.Query()
	query.Set("key", qingguoProxyKey)
	query.Set("num", fmt.Sprintf("%d", qingguoProxyBatchSize))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func paidProxyAddresses(infos []PaidProxyInfo) []string {
	items := make([]string, 0, len(infos))
	for _, info := range infos {
		if strings.TrimSpace(info.Address) != "" {
			items = append(items, info.Address)
		}
	}
	return cleanProxies(items)
}

func parsePaidProxyResponse(body []byte) ([]string, error) {
	infos, err := parsePaidProxyInfoResponse(body)
	if err != nil {
		return nil, err
	}
	return paidProxyAddresses(infos), nil
}

func parsePaidProxyInfoResponse(body []byte) ([]PaidProxyInfo, error) {
	var response qingguoProxyResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("parse Qingguo proxy response: %w", err)
	}
	if response.Code != "SUCCESS" {
		return nil, fmt.Errorf("Qingguo proxy request failed: code=%q request_id=%q", response.Code, response.RequestID)
	}
	items := make([]PaidProxyInfo, 0, len(response.Data))
	for _, entry := range response.Data {
		if !isValidHTTPProxyAddress(entry.Server) {
			continue
		}
		proxyURL, _ := ParseHTTPProxy(entry.Server)
		if qingguoProxyAuthKey != "" || qingguoProxyAuthPwd != "" {
			proxyURL.User = url.UserPassword(qingguoProxyAuthKey, qingguoProxyAuthPwd)
		}
		items = append(items, PaidProxyInfo{Address: proxyURL.String(), ExpiresAt: qingguoProxyDeadline(entry.Deadline)})
	}
	items = cleanPaidProxyInfos(items)
	if len(items) == 0 {
		return nil, fmt.Errorf("Qingguo proxy returned no usable addresses: request_id=%q", response.RequestID)
	}
	return items, nil
}

type qingguoProxyResponse struct {
	Code string `json:"code"`
	Data []struct {
		Server   string `json:"server"`
		Deadline string `json:"deadline"`
	} `json:"data"`
	RequestID string `json:"request_id"`
}

func qingguoProxyDeadline(deadline string) time.Time {
	expiresAt, err := time.ParseInLocation("2006-01-02 15:04:05", deadline, time.Local)
	if err == nil {
		return expiresAt
	}
	return time.Now().Add(time.Duration(qingguoProxyFallbackLifetime) * time.Second)
}

func isValidHTTPProxyAddress(raw string) bool {
	proxyURL, err := ParseHTTPProxy(raw)
	return err == nil && proxyURL != nil && proxyURL.Hostname() != "" && proxyURL.Port() != ""
}

func cleanPaidProxyInfos(items []PaidProxyInfo) []PaidProxyInfo {
	seen := make(map[string]struct{}, len(items))
	cleaned := make([]PaidProxyInfo, 0, len(items))
	for _, item := range items {
		address := strings.TrimSpace(item.Address)
		if address == "" {
			continue
		}
		if _, exists := seen[address]; exists {
			continue
		}
		seen[address] = struct{}{}
		item.Address = address
		cleaned = append(cleaned, item)
	}
	return cleaned
}

// NewHTTPClientWithProxy creates a direct client when proxyURL is nil, or an
// HTTP-proxy client otherwise.
func NewHTTPClientWithProxy(proxyURL *url.URL, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL != nil {
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}

func ParseHTTPProxy(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("invalid http proxy %q", raw)
	}
	return parsed, nil
}
