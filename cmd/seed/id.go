package main

import (
	"crypto/rand"
	"encoding/hex"
)

// randSuffix 6 字节随机 hex,拼进用户 id。
func randSuffix() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
