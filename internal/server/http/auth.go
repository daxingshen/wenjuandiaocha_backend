// 鉴权端点:login / logout / me。业务在 service/auth;cookie 下发是传输关切,留在本层。
package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/api"
	authmw "wenjuandiaocha_backend/internal/server/http/middleware/auth"
	"wenjuandiaocha_backend/internal/server/http/render"
)

func (s *Server) login(c *gin.Context) {
	// api.AuthLoginReq 带 json tag、即为登录请求线格式,直接 bind(不再另立 http 私有 struct)。
	var req api.AuthLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		render.Fail(c, http.StatusBadRequest, "账号或密码缺失")
		return
	}
	resp, err := s.auth.Login(c.Request.Context(), req)
	if err != nil {
		render.JSON(c, nil, err)
		return
	}
	// 成功:先下发会话 cookie(传输关切),再以 AuthUser 作信封 data → data:{id,name,role}。
	s.setSessionCookie(c, resp.Token)
	render.JSON(c, resp.User, nil)
}

func (s *Server) logout(c *gin.Context) {
	// logout 无 requireAuth 中间件,token 在此从 cookie 取并补进 ctx metadata(保留全局 ip/ua)。
	if token, err := c.Cookie(authmw.SessionCookie); err == nil && token != "" {
		md := api.MetadataFrom(c.Request.Context())
		md.Token = token
		_ = s.auth.Logout(api.WithMetadata(c.Request.Context(), md))
	}
	s.clearSessionCookie(c)
	render.JSON(c, nil, nil)
}

func (s *Server) me(c *gin.Context) {
	// userID 已由 requireAuth 注入 ctx metadata。
	resp, err := s.auth.Me(c.Request.Context())
	if err != nil {
		render.JSON(c, nil, err)
		return
	}
	render.JSON(c, resp.User, nil)
}

func (s *Server) setSessionCookie(c *gin.Context, token string) {
	// MaxAge 秒;HttpOnly 防 XSS 读取;SameSite=Lax 配合同源 proxy;prod 加 Secure。
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(authmw.SessionCookie, token, int(s.cfg.SessionTTL.Seconds()), "/", "", s.cfg.CookieSecure, true)
}

func (s *Server) clearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(authmw.SessionCookie, "", -1, "/", "", s.cfg.CookieSecure, true)
}
