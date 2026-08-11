// Package requestid 提供请求 id 中间件:为每个请求生成随机 id 存入 gin ctx,
// 供日志等下游关联。ctx key 私有,外部经 Get 读取。
package requestid

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

const ctxKey = "reqid"

// New 生成 8 字节随机 reqid 存入 gin ctx。
func New() gin.HandlerFunc {
	return func(c *gin.Context) {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		c.Set(ctxKey, hex.EncodeToString(b))
		c.Next()
	}
}

// Get 取当前请求的 reqid(未设置返回空串)。
func Get(c *gin.Context) string {
	return c.GetString(ctxKey)
}
