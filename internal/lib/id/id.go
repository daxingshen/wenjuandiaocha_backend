// Package id 生成短 id:crypto/rand + base32(去易混字符),对齐前端 a3f9 风格。
package id

import (
	"crypto/rand"
	"strings"
)

// 去掉易混字符(0/O/1/I/L)的 base32 字母表。
const alphabet = "23456789abcdefghjkmnpqrstuvwxyz"

// New 生成 8 位短 id。用于 survey / response。
func New() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	var sb strings.Builder
	for _, x := range b {
		sb.WriteByte(alphabet[int(x)%len(alphabet)])
	}
	return sb.String()
}
