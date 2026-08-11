// Package recovery 提供 panic 恢复中间件:panic → 500,不崩进程。
// 作为安全网,不依赖 render 包(保持自包含,任何情况都能兜底)。
package recovery

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// New 返回 panic 恢复中间件。
func New() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic 恢复", "err", err, "path", c.Request.URL.Path)
				if !c.Writer.Written() {
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
				}
			}
		}()
		c.Next()
	}
}
