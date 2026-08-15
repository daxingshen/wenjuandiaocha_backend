// Package auth 提供鉴权中间件:校验 session cookie,把已认证 userID 补进 ctx metadata。
// 依赖显式注入的 auth.Service(不挂 Server),错误经 render 包渲染。
package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/internal/ecode"
	"wenjuandiaocha_backend/internal/lib/metadata"
	"wenjuandiaocha_backend/internal/rbac"
	"wenjuandiaocha_backend/internal/server/http/render"
	svcauth "wenjuandiaocha_backend/internal/service/auth"
)

// SessionCookie 是会话 cookie 名。login/logout 下发、requireAuth 读取共用。
const SessionCookie = "sid"

// RequireAuth 是「鉴权 + RBAC 第一层能力位」合并中间件:读 session cookie → 校验 → 注入
// userID+role 进 ctx metadata → 若 action 非空则判能力位。空 token → 401;能力位不通过 → 403。
//
// action 语义:传具体 rbac.Action 则该端点要求「role 能做此类动作」(平台能力级越权 → 真 403);
// 传 "" 则只鉴权不判能力位(如 /auth/me、需登录但作答能力由 service 按问卷模式另判的 submit 登录路由)。
//
// 合并成单构造器(而非鉴权、能力位两个独立中间件)是刻意的:能力位判定依赖 role,而 role 由本
// 中间件注入 —— 合并后二者先后固定、不可能因路由注册次序写反,消除顺序隐患。
// 第二层资源归属(owner 校验)仍在 service(需查库、admin 短路、与业务共享 meta),不在此。
func RequireAuth(svc svcauth.Service, action rbac.Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(SessionCookie)
		if err != nil || token == "" {
			render.Fail(c, http.StatusUnauthorized, "未登录")
			return
		}
		// 先把 token 补进 ctx metadata 供 ValidateSession 读(保留 ip/ua)。
		md := metadata.From(c.Request.Context())
		md.Token = token
		ctx := metadata.With(c.Request.Context(), md)
		resp, verr := svc.ValidateSession(ctx)
		if verr != nil {
			render.JSON(c, nil, verr)
			return
		}
		// 校验通过:补 userID + role,替换 request ctx 供下游 handler / 能力位判定。
		md.UserID = resp.UserID
		md.Role = resp.Role
		c.Request = c.Request.WithContext(metadata.With(ctx, md))

		// RBAC 第一层能力位(action 非空时):role 不能做该类动作 → 真 403(平台能力级越权)。
		if action != "" && !rbac.Can(rbac.Role(md.Role), action) {
			render.JSON(c, nil, ecode.Forbidden403("无权执行此操作"))
			return
		}
		c.Next()
	}
}
