// 短 id 生成:crypto/rand + base32(去易混字符),对齐前端 a3f9 风格。
package http

import (
	"crypto/rand"
	"strings"
)

// 去掉易混字符(0/O/1/I/L)的 base32 字母表。
const idAlphabet = "23456789abcdefghjkmnpqrstuvwxyz"

// newID 生成 8 位短 id。用于 survey / response。
func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	var sb strings.Builder
	for _, x := range b {
		sb.WriteByte(idAlphabet[int(x)%len(idAlphabet)])
	}
	return sb.String()
}
