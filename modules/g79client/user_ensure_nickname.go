package g79client

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

var (
	nklmRng   *rand.Rand
	nklmRngMu sync.Mutex
)

func init() {
	nklmRng = rand.New(rand.NewSource(time.Now().UnixNano()))
}

// GenerateNKLMName 生成 "nklm_XXXXX" 格式的 10 位昵称（X 为 0-9 随机数字）
// 注意：网易昵称 2-8 中文字符/2-16 字符 —— "nklm_12345" = 10 ASCII 字符。
// 超长风险：某些服或场景（例如大厅/登录后）可能需要严格长度，必要时缩短后缀或截尾。
func GenerateNKLMName() string {
	nklmRngMu.Lock()
	defer nklmRngMu.Unlock()
	n := nklmRng.Intn(100000) // [0, 99999]
	return fmt.Sprintf("nklm_%05d", n)
}

// EnsureNicknameNKLMIfEmpty 在客户端未设置昵称（UserDetail.Name 为空）时自动调用 UpdateNickname。
//   - 优先使用 GenerateNKLMName() 生成的 nklm_XXXXX
//   - 若失败且传入的 legacyPrefix != ""，再退回 legacyPrefix+0-99999 兜底，最多重试 6 次
//   - 成功后会自动刷新 UserDetail.Name，避免下游重复触发
func EnsureNicknameNKLMIfEmpty(c *Client, legacyPrefix string) error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}
	if c.UserDetail == nil {
		d, err := c.GetUserDetail()
		if err != nil {
			return fmt.Errorf("GetUserDetail: %w", err)
		}
		c.UserDetail = &d.Entity
	}
	if c.UserDetail != nil && strings.TrimSpace(c.UserDetail.Name) != "" {
		return nil
	}

	var lastErr error
	const maxAttempts = 6
	for i := 0; i < maxAttempts; i++ {
		var name string
		if i == 0 {
			name = GenerateNKLMName()
		} else {
			// 冲突/失败兜底：nklm_ 仍优先，legacyPrefix 只有显式传入才用
			if legacyPrefix != "" && i >= 4 {
				name = fmt.Sprintf("%s%05d", legacyPrefix, randIntN(100000))
			} else {
				name = GenerateNKLMName()
			}
		}
		err := c.UpdateNickname(name)
		if err == nil {
			if c.UserDetail != nil {
				c.UserDetail.Name = name
			}
			return nil
		}
		lastErr = err
		time.Sleep(time.Duration(150+i*80) * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("unknown error")
	}
	return fmt.Errorf("EnsureNicknameNKLMIfEmpty: %w", lastErr)
}

func randIntN(n int) int {
	nklmRngMu.Lock()
	defer nklmRngMu.Unlock()
	return nklmRng.Intn(n)
}
