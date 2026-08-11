package g79client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// G79CPPVersionMap 表示 cpp-version-map 接口返回的版本映射。
type G79CPPVersionMap struct {
	PC int                    `json:"pc"`
	PE []G79CPPVersionMapItem `json:"pe"`
}

// G79CPPVersionMapItem 表示单个 PE 版本映射项。
type G79CPPVersionMapItem struct {
	Version string `json:"version"`
	Value   int    `json:"value"`
}

var (
	globalG79CPPVersionMap *G79CPPVersionMap
	g79CPPVersionMapMu     sync.RWMutex
)

// GetGlobalG79CPPVersionMap 获取全局 cpp 版本映射，首次调用时会自动拉取并缓存。
func GetGlobalG79CPPVersionMap() (*G79CPPVersionMap, error) {
	return GetGlobalG79CPPVersionMapWithHTTPClient(nil)
}

func GetGlobalG79CPPVersionMapWithHTTPClient(httpClient *http.Client) (*G79CPPVersionMap, error) {
	g79CPPVersionMapMu.RLock()
	if globalG79CPPVersionMap != nil {
		v := cloneG79CPPVersionMap(globalG79CPPVersionMap)
		g79CPPVersionMapMu.RUnlock()
		return v, nil
	}
	g79CPPVersionMapMu.RUnlock()

	g79CPPVersionMapMu.Lock()
	defer g79CPPVersionMapMu.Unlock()
	if globalG79CPPVersionMap != nil {
		return cloneG79CPPVersionMap(globalG79CPPVersionMap), nil
	}

	result, err := fetchG79CPPVersionMapWithHTTPClient(httpClient)
	if err != nil {
		return nil, err
	}
	globalG79CPPVersionMap = cloneG79CPPVersionMap(result)
	return cloneG79CPPVersionMap(result), nil
}

// RefreshG79CPPVersionMap 强制刷新 cpp 版本映射缓存。
func RefreshG79CPPVersionMap() (*G79CPPVersionMap, error) {
	return RefreshG79CPPVersionMapWithHTTPClient(nil)
}

func RefreshG79CPPVersionMapWithHTTPClient(httpClient *http.Client) (*G79CPPVersionMap, error) {
	result, err := fetchG79CPPVersionMapWithHTTPClient(httpClient)
	if err != nil {
		return nil, err
	}

	g79CPPVersionMapMu.Lock()
	globalG79CPPVersionMap = cloneG79CPPVersionMap(result)
	g79CPPVersionMapMu.Unlock()
	return cloneG79CPPVersionMap(result), nil
}

func fetchG79CPPVersionMap() (*G79CPPVersionMap, error) {
	return fetchG79CPPVersionMapWithHTTPClient(nil)
}

func fetchG79CPPVersionMapWithHTTPClient(httpClient *http.Client) (*G79CPPVersionMap, error) {
	releaseJSON, err := GetGlobalG79ReleaseJSONWithHTTPClient(httpClient)
	if err != nil {
		return nil, err
	}

	baseURL := strings.TrimRight(releaseJSON.TransferServerNewHttpUrl, "/")
	if baseURL == "" {
		return nil, fmt.Errorf("empty TransferServerNewHttpUrl")
	}

	req, err := http.NewRequest(http.MethodGet, baseURL+"/cpp-version-map", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("content-type", "text/plain")

	resp, err := resolveHTTPClient(httpClient).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result G79CPPVersionMap
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func cloneG79CPPVersionMap(src *G79CPPVersionMap) *G79CPPVersionMap {
	if src == nil {
		return nil
	}

	dst := *src
	if src.PE != nil {
		dst.PE = make([]G79CPPVersionMapItem, len(src.PE))
		copy(dst.PE, src.PE)
	}
	return &dst
}
