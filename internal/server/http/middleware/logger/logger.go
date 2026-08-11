// Package logger 提供结构化访问日志中间件:记录 method/path/status/耗时/reqid。
package logger

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/internal/server/http/middleware/requestid"
)

// New 返回访问日志中间件。reqid 经 requestid.Get 读取(需 requestid 中间件在前)。
func New() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("req",
			"method", c.Request.Method, "path", c.Request.URL.Path,
			"status", c.Writer.Status(), "dur", time.Since(start).String(),
			"reqid", requestid.Get(c))
	}
}
