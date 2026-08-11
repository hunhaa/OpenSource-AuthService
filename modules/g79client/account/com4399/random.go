package com4399

import (
	"crypto/rand"
	"encoding/hex"
)

func randomHexString(byteLen int) (string, error) {
	if byteLen <= 0 {
		return "", nil
	}
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
