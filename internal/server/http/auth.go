// 鉴权端点:login / logout / me。session 存 DB,token 走 HttpOnly cookie。
package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/internal/auth"
)

type loginReq struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

// userResp 对齐前端 AuthUser { id, name, level }。
type userResp struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Level string `json:"level"`
}

func (s *Server) login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Account == "" {
		fail(c, http.StatusBadRequest, "账号或密码缺失")
		return
	}
	u, err := s.store.GetUserByAccount(c.Request.Context(), req.Account)
	if err != nil || !auth.CheckPassword(u.PasswordHash, req.Password) {
		// 不区分「账号不存在」与「密码错」,避免账号枚举。
		fail(c, http.StatusUnauthorized, "账号或密码错误")
		return
	}
	token, err := auth.NewSessionToken()
	if err != nil {
		fail(c, http.StatusInternalServerError, "内部错误")
		return
	}
	expires := time.Now().Add(s.cfg.SessionTTL)
	if err := s.store.CreateSession(c.Request.Context(), token, u.ID, expires); err != nil {
		s.storeError(c, err)
		return
	}
	s.setSessionCookie(c, token)
	c.JSON(http.StatusOK, userResp{ID: u.ID, Name: u.Name, Level: u.Level})
}

func (s *Server) logout(c *gin.Context) {
	if token, err := c.Cookie(sessionCookie); err == nil && token != "" {
		_ = s.store.DeleteSession(c.Request.Context(), token)
	}
	s.clearSessionCookie(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) me(c *gin.Context) {
	u, err := s.store.GetUserByID(c.Request.Context(), currentUserID(c))
	if err != nil {
		s.storeError(c, err)
		return
	}
	c.JSON(http.StatusOK, userResp{ID: u.ID, Name: u.Name, Level: u.Level})
}

func (s *Server) setSessionCookie(c *gin.Context, token string) {
	// MaxAge 秒;HttpOnly 防 XSS 读取;SameSite=Lax 配合同源 proxy;prod 加 Secure。
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, token, int(s.cfg.SessionTTL.Seconds()), "/", "", s.cfg.CookieSecure, true)
}

func (s *Server) clearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, "", -1, "/", "", s.cfg.CookieSecure, true)
}
