// gin 中间件:请求 id、recover、结构化日志、CORS、鉴权。
package http

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/internal/store"
)

const (
	ctxUserID    = "uid"
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
func (s *Server) requireAuth(c *gin.Context) {
	token, err := c.Cookie(sessionCookie)
	if err != nil || token == "" {
		fail(c, http.StatusUnauthorized, "未登录")
		return
	}
	uid, expires, err := s.store.GetSession(c.Request.Context(), token)
	if err != nil {
		fail(c, http.StatusUnauthorized, "会话无效")
		return
	}
	if time.Now().After(expires) {
		_ = s.store.DeleteSession(c.Request.Context(), token)
		fail(c, http.StatusUnauthorized, "会话已过期")
		return
	}
	c.Set(ctxUserID, uid)
	c.Next()
}

// currentUserID 取中间件注入的用户 id。
func currentUserID(c *gin.Context) string {
	return c.GetString(ctxUserID)
}

// ensureNotFound 把 store.ErrNotFound 映射成 404,其余 500。
func (s *Server) storeError(c *gin.Context, err error) {
	if err == store.ErrNotFound {
		fail(c, http.StatusNotFound, "不存在")
		return
	}
	slog.Error("store error", "err", err, "path", c.Request.URL.Path)
	fail(c, http.StatusInternalServerError, "内部错误")
}
