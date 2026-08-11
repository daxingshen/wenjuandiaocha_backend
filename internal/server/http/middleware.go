// gin 中间件:请求 id、recover、结构化日志、CORS、鉴权。
package http

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	svcauth "wenjuandiaocha_backend/internal/service/auth"
)

const (
	ctxUserID     = "uid"
	sessionCookie = "sid"
)

// dev 允许的前端源(studio 5173 / runtime 5174)。带 credentials 不能用 *。
var allowedOrigins = map[string]bool{
	"http://localhost:5173": true,
	"http://localhost:5174": true,
	"http://127.0.0.1:5173": true,
	"http://127.0.0.1:5174": true,
}

func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		c.Set("reqid", hex.EncodeToString(b))
		c.Next()
	}
}

// recovery panic → 500,不崩进程。
func recovery() gin.HandlerFunc {
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

func logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("req",
			"method", c.Request.Method, "path", c.Request.URL.Path,
			"status", c.Writer.Status(), "dur", time.Since(start).String(),
			"reqid", c.GetString("reqid"))
	}
}

// cors dev 跨源:放行 5173/5174,允许 credentials(cookie)。
func cors() gin.HandlerFunc {
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

// requireAuth 读 session cookie → 校验 → 注入 userID;失败 401。
// 空 token 是传输层判定(未登录);token 有效性交 auth manager。
func (s *Server) requireAuth(c *gin.Context) {
	token, err := c.Cookie(sessionCookie)
	if err != nil || token == "" {
		fail(c, http.StatusUnauthorized, "未登录")
		return
	}
	resp, verr := s.auth.ValidateSession(c.Request.Context(), svcauth.ValidateSessionReq{Token: token})
	if verr != nil {
		renderError(c, verr)
		return
	}
	c.Set(ctxUserID, resp.UserID)
	c.Next()
}

// currentUserID 取中间件注入的用户 id。
func currentUserID(c *gin.Context) string {
	return c.GetString(ctxUserID)
}
