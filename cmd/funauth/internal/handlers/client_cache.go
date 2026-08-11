package handlers

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/Yeah114/g79client"
)

const (
	TokenValidDuration    = 3200
	TokenRefreshInterval  = 1500
	VitalityCheckInterval = 15
)

type CachedClient struct {
	Client           *g79client.Client
	UserUUID         string
	LastAuthTime     time.Time
	LastRefreshTime  time.Time
	RefreshDuration  time.Duration
	LastVitalityTime time.Time
	mu               sync.RWMutex
}

func (c *CachedClient) IsTokenValid() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Since(c.LastAuthTime) < time.Duration(TokenValidDuration)*time.Second
}

func (c *CachedClient) ShouldRefresh() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if time.Since(c.LastAuthTime) >= c.RefreshDuration {
		return false
	}
	return time.Since(c.LastRefreshTime) >= time.Duration(TokenRefreshInterval)*time.Second
}

type ClientCacheManager struct {
	clients sync.Map
}

var globalClientCache *ClientCacheManager
var once sync.Once

func GetClientCache() *ClientCacheManager {
	once.Do(func() {
		globalClientCache = &ClientCacheManager{}
		go globalClientCache.startRefreshTask()
	})
	return globalClientCache
}

func (m *ClientCacheManager) GetClient(userUUID string) (*CachedClient, bool) {
	value, ok := m.clients.Load(userUUID)
	if !ok {
		return nil, false
	}
	cached := value.(*CachedClient)
	if !cached.IsTokenValid() || !isAuthenticatedG79Client(cached.Client) {
		m.clients.Delete(userUUID)
		return nil, false
	}
	return cached, true
}

func (m *ClientCacheManager) SetClient(userUUID string, client *g79client.Client) *CachedClient {
	if !isAuthenticatedG79Client(client) {
		m.clients.Delete(userUUID)
		return nil
	}

	refreshMinutes := 120 + rand.Intn(61)
	refreshDuration := time.Duration(refreshMinutes) * time.Minute

	cached := &CachedClient{
		Client:           client,
		UserUUID:         userUUID,
		LastAuthTime:     time.Now(),
		LastRefreshTime:  time.Now(),
		RefreshDuration:  refreshDuration,
		LastVitalityTime: time.Now(),
	}
	m.clients.Store(userUUID, cached)
	return cached
}

func (m *ClientCacheManager) RemoveClient(userUUID string) {
	m.clients.Delete(userUUID)
}

func isAuthenticatedG79Client(client *g79client.Client) bool {
	return client != nil &&
		strings.TrimSpace(client.UserID) != "" &&
		strings.TrimSpace(client.UserToken) != ""
}

func (m *ClientCacheManager) startRefreshTask() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		m.clients.Range(func(key, value interface{}) bool {
			userUUID := key.(string)
			cached := value.(*CachedClient)

			if !cached.IsTokenValid() {
				m.clients.Delete(userUUID)
				return true
			}

			// 刷新 token
			if cached.ShouldRefresh() {
				go func(uuid string, c *CachedClient) {
					c.mu.Lock()
					defer c.mu.Unlock()

					timeSinceAuth := time.Since(c.LastAuthTime)
					isValid := timeSinceAuth < time.Duration(TokenValidDuration)*time.Second

					if !isValid || timeSinceAuth >= c.RefreshDuration {
						return
					}

					cli := c.Client
					if !isAuthenticatedG79Client(cli) {
						m.clients.Delete(uuid)
						return
					}
					_, err := cli.UpdateToken()
					if err != nil {
						fmt.Printf("后台刷新token失败，用户 %s: %v\n", uuid, err)
						return
					}
					c.LastRefreshTime = time.Now()
					c.LastAuthTime = time.Now()
				}(userUUID, cached)
			}

			// 获取活力值（每 15 分钟）
			cached.mu.RLock()
			lastVitality := cached.LastVitalityTime
			cached.mu.RUnlock()
			if time.Since(lastVitality) >= time.Duration(VitalityCheckInterval)*time.Minute {
				go func(uuid string, c *CachedClient) {
					c.mu.Lock()
					defer c.mu.Unlock()

					// 再次检查时间，避免并发重复执行
					if time.Since(c.LastVitalityTime) < time.Duration(VitalityCheckInterval)*time.Minute {
						return
					}

					cli := c.Client
					if !isAuthenticatedG79Client(cli) {
						return
					}

					// 获取活力在线时长
					resp, err := cli.GetCurrencyOnline()
					if err != nil {
						fmt.Printf("获取活力值失败，用户 %s: %v\n", uuid, err)
						return
					}
					c.LastVitalityTime = time.Now()
					fmt.Printf("获取活力值成功，用户 %s: 剩余活力时间=%d分钟\n", uuid, resp.Entity.RestCurrencyTime/60)
				}(userUUID, cached)
			}

			return true
		})
	}
}

func (m *ClientCacheManager) GetStats() map[string]interface{} {
	count := 0
	m.clients.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return map[string]interface{}{
		"cached_clients":       count,
		"token_valid_duration": TokenValidDuration,
	}
}
