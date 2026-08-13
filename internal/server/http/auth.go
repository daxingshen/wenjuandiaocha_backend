// 鉴权端点:login / logout / me。业务在 service/auth;cookie 下发是传输关切,留在本层。
package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/api"
	authmw "wenjuandiaocha_backend/internal/server/http/middleware/auth"
	"wenjuandiaocha_backend/internal/server/http/render"
)

// loginReq 是登录请求的传输形状(带 json tag);bind 后映射到 api.AuthLoginReq。
// I/O 契约类型(api 包)不带 json tag,传输格式由本层负责。
type loginReq struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

// userResp 对齐前端 AuthUser { id, name, role }。
// 响应体手写(不直接序列化 I/O 类型):显式 json tag,保「对外逐字节不变」。
// role 为 RBAC 角色轴(前端体验层门控用)。level(套餐轴)已移除,前端同步不在本轮范围。
type userResp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

func (s *Server) login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		render.Fail(c, http.StatusBadRequest, "账号或密码缺失")
		return
	}
	resp, err := s.auth.Login(c.Request.Context(), api.AuthLoginReq{Account: req.Account, Password: req.Password})
	if err != nil {
		render.Error(c, err)
		return
	}
	s.setSessionCookie(c, resp.Token)
	render.Success(c, userResp{ID: resp.User.ID, Name: resp.User.Name, Role: resp.User.Role})
}

func (s *Server) logout(c *gin.Context) {
	// logout 无 requireAuth 中间件,token 在此从 cookie 取并补进 ctx metadata(保留全局 ip/ua)。
	if token, err := c.Cookie(authmw.SessionCookie); err == nil && token != "" {
		md := api.MetadataFrom(c.Request.Context())
		md.Token = token
		_ = s.auth.Logout(api.WithMetadata(c.Request.Context(), md))
	}
	s.clearSessionCookie(c)
	render.Success(c, nil)
}

func (s *Server) me(c *gin.Context) {
	// userID 已由 requireAuth 注入 ctx metadata。
	resp, err := s.auth.Me(c.Request.Context())
	if err != nil {
		render.Error(c, err)
		return
	}
	render.Success(c, userResp{ID: resp.User.ID, Name: resp.User.Name, Role: resp.User.Role})
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
