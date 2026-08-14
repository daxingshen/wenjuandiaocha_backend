// Package auth 提供鉴权中间件:校验 session cookie,把已认证 userID 补进 ctx metadata。
// 依赖显式注入的 auth.Service(不挂 Server),错误经 render 包渲染。
package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/api"
	"wenjuandiaocha_backend/internal/server/http/render"
	svcauth "wenjuandiaocha_backend/internal/service/auth"
)

// SessionCookie 是会话 cookie 名。login/logout 下发、requireAuth 读取共用。
const SessionCookie = "sid"

// RequireAuth 读 session cookie → 校验 → 把 userID 补进 ctx metadata 供下游;失败 401。
// 空 token 是传输层判定(未登录);token 有效性交 auth service。
// 在全局 clientinfo 已填 ip/ua 的基础上「补充」身份字段,不覆盖已有 metadata。
func RequireAuth(svc svcauth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(SessionCookie)
		if err != nil || token == "" {
			render.Fail(c, http.StatusUnauthorized, "未登录")
			return
		}
		// 先把 token 补进 ctx metadata 供 ValidateSession 读(保留 ip/ua)。
		md := api.MetadataFrom(c.Request.Context())
		md.Token = token
		ctx := api.WithMetadata(c.Request.Context(), md)
		resp, verr := svc.ValidateSession(ctx)
		if verr != nil {
			render.JSON(c, nil, verr)
			return
		}
		// 校验通过:补 userID + role(RBAC 能力判定用),替换 request ctx 供下游 handler。
		md.UserID = resp.UserID
		md.Role = resp.Role
		c.Request = c.Request.WithContext(api.WithMetadata(ctx, md))
		c.Next()
	}
}
