// Package metadata 承载传输层派生的调用上下文,经 ctx 隐式流转,不进 service 的业务 Req。
//
// 由各传输层(http 从 session/cookie/请求、gRPC 从拦截器/metadata)填入 ctx,service 从 ctx 读。
// 独立成 lib 包(不放 api):它是「调用上下文/传输派生值」而非 I/O 契约,api 只承载对外线格式。
package metadata

import "context"

// Metadata 承载会话身份、会话 token、客户端网络信息。
type Metadata struct {
	UserID    string // 已认证用户 id(studio 端点归属校验用)
	Role      string // 已认证用户角色 admin|creator|respondent(RBAC 能力判定用;RequireAuth 注入)
	Token     string // 会话 token(logout/validateSession 用)
	ClientIP  string // 客户端 IP(防刷 meta)
	UserAgent string // 客户端 UA(防刷 meta)
}

type ctxKey struct{}

// With 把 Metadata 挂到 ctx。传输层在调 service 前调用。
func With(ctx context.Context, md Metadata) context.Context {
	return context.WithValue(ctx, ctxKey{}, md)
}

// From 从 ctx 取 Metadata;未设置返回零值。service 层调用。
func From(ctx context.Context) Metadata {
	md, _ := ctx.Value(ctxKey{}).(Metadata)
	return md
}
