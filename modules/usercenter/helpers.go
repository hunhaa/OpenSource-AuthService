package usercenter

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

// forUpdate 返回 SELECT ... FOR UPDATE 的 GORM clause
func forUpdate() clause.Locking {
	return clause.Locking{
		Strength: "UPDATE",
	}
}

// sha256Hex 计算 SHA256 十六进制
func sha256Hex(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// randomCode 生成大写字母+数字的兑换码（形如 XXXX-XXXX-XXXX-XXXX）
func randomCode(prefix string, segLen, segCount int) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	if segLen <= 0 {
		segLen = 4
	}
	if segCount <= 0 {
		segCount = 4
	}
	parts := make([]string, 0, segCount)
	maxN := big.NewInt(int64(len(alphabet)))
	for i := 0; i < segCount; i++ {
		seg := make([]byte, segLen)
		for j := 0; j < segLen; j++ {
			n, _ := rand.Int(rand.Reader, maxN)
			seg[j] = alphabet[int(n.Int64())]
		}
		parts = append(parts, string(seg))
	}
	code := strings.Join(parts, "-")
	if prefix != "" {
		code = strings.ToUpper(prefix) + "-" + code
	}
	return code
}

// newUUIDString 生成 UUID v4 字符串
func newUUIDString() string {
	return uuid.NewString()
}
