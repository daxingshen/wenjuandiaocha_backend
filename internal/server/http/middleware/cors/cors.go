// Package cors 提供 dev 跨源中间件:放行 studio(5173)/runtime(5174),允许 credentials(cookie)。
package cors

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// dev 允许的前端源(studio 5173 / runtime 5174)。带 credentials 不能用 *。
var allowedOrigins = map[string]bool{
	"http://localhost:5173": true,
	"http://localhost:5174": true,
	"http://127.0.0.1:5173": true,
	"http://127.0.0.1:5174": true,
}

// New 返回 CORS 中间件。
func New() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if allowedOrigins[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
