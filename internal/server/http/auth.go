// 鉴权端点:login / logout / me。业务在 service/auth;cookie 下发是传输关切,留在本层。
package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/api"
)

// userResp 对齐前端 AuthUser { id, name, level }。
// 响应体手写(不用 pb 生成物):pb 字段带 omitempty 会丢零值,破坏「对外逐字节不变」。
type userResp struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Level string `json:"level"`
}

func (s *Server) login(c *gin.Context) {
	var req api.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "账号或密码缺失")
		return
	}
	u, token, _, err := s.auth.Login(c.Request.Context(), req.GetAccount(), req.GetPassword())
	if err != nil {
		renderError(c, err)
		return
	}
	s.setSessionCookie(c, token)
	c.JSON(http.StatusOK, userResp{ID: u.ID, Name: u.Name, Level: u.Level})
}

func (s *Server) logout(c *gin.Context) {
	if token, err := c.Cookie(sessionCookie); err == nil && token != "" {
		s.auth.Logout(c.Request.Context(), token)
	}
	s.clearSessionCookie(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) me(c *gin.Context) {
	u, err := s.auth.Me(c.Request.Context(), currentUserID(c))
	if err != nil {
		renderError(c, err)
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
