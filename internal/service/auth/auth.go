// Package auth 是鉴权业务层:登录(验密+建会话)、登出、取当前用户。
// 纯密码学原语在 internal/auth;会话存取在 dao;cookie 下发是传输关切,留在 server/http。
package auth

import (
	"context"
	"time"

	"github.com/google/wire"

	"wenjuandiaocha_backend/api"
	"wenjuandiaocha_backend/internal/dao"
	"wenjuandiaocha_backend/internal/ecode"
	authlib "wenjuandiaocha_backend/internal/lib/auth"
	"wenjuandiaocha_backend/internal/lib/metadata"
)

// Store 是本层依赖的 dao 子集(消费方定义接口,便于单测)。*dao.Store 实现它。
type Store interface {
	GetUserByAccount(ctx context.Context, account string) (dao.User, error)
	GetUserByID(ctx context.Context, id string) (dao.User, error)
	CreateSession(ctx context.Context, token, userID string, expires time.Time) error
	GetSession(ctx context.Context, token string) (userID, role string, expires time.Time, err error)
	DeleteSession(ctx context.Context, token string) error
}

// I/O 契约集中在 api 包(一处定义,http/gRPC 两端共用)。

// Service 是鉴权业务契约。*Manager 实现它;传输层持本接口。
// Me 的 userID、Logout/ValidateSession 的 token 来自 ctx 的 metadata.Metadata,不进 Req。
type Service interface {
	Login(ctx context.Context, req api.AuthLoginReq) (api.AuthLoginResp, error)
	Me(ctx context.Context) (api.AuthMeResp, error)
	Logout(ctx context.Context) error
	ValidateSession(ctx context.Context) (api.AuthValidateSessionResp, error)
}

// Manager 持有 dao + 会话 TTL(建会话过期时间用)。
type Manager struct {
	store      Store
	sessionTTL time.Duration
}

func New(store Store, sessionTTL time.Duration) *Manager {
	return &Manager{store: store, sessionTTL: sessionTTL}
}

// 编译期确认 *Manager 实现 Service。
var _ Service = (*Manager)(nil)

// ProviderSet 供 wire 组装:提供 *Manager,并绑定到 Service 接口。sessionTTL 由 di.provideSessionTTL 提供。
var ProviderSet = wire.NewSet(New, wire.Bind(new(Service), new(*Manager)))

// Login 验账密 → 建会话 → 返回 (用户, sessionToken, 过期时刻)。
// 不区分「账号不存在」与「密码错」,避免账号枚举(统一 Unauthorized)。
func (m *Manager) Login(ctx context.Context, req api.AuthLoginReq) (api.AuthLoginResp, error) {
	if req.Account == "" {
		return api.AuthLoginResp{}, ecode.BadRequest("账号或密码缺失")
	}
	u, err := m.store.GetUserByAccount(ctx, req.Account)
	if err != nil || !authlib.CheckPassword(u.PasswordHash, req.Password) {
		return api.AuthLoginResp{}, ecode.Unauthorized("账号或密码错误")
	}
	token, err := authlib.NewSessionToken()
	if err != nil {
		return api.AuthLoginResp{}, ecode.Internal("内部错误")
	}
	expires := time.Now().Add(m.sessionTTL)
	if err := m.store.CreateSession(ctx, token, u.ID, expires); err != nil {
		return api.AuthLoginResp{}, err
	}
	return api.AuthLoginResp{User: api.AuthUser{ID: u.ID, Name: u.Name, Role: u.Role}, Token: token, Expires: expires}, nil
}

// Logout 删会话(token 从 ctx metadata 取;为空则无操作)。
func (m *Manager) Logout(ctx context.Context) error {
	if token := metadata.From(ctx).Token; token != "" {
		_ = m.store.DeleteSession(ctx, token)
	}
	return nil
}

// Me 取当前用户信息。userID 从 ctx metadata 取。
func (m *Manager) Me(ctx context.Context) (api.AuthMeResp, error) {
	u, err := m.store.GetUserByID(ctx, metadata.From(ctx).UserID)
	if err != nil {
		return api.AuthMeResp{}, err
	}
	return api.AuthMeResp{User: api.AuthUser{ID: u.ID, Name: u.Name, Role: u.Role}}, nil
}

// ValidateSession 校验会话 token:查无/无效 → Unauthorized("会话无效");
// 已过期 → 删除并 Unauthorized("会话已过期");有效返回 userID。空 token 由传输层先行拦截。
func (m *Manager) ValidateSession(ctx context.Context) (api.AuthValidateSessionResp, error) {
	token := metadata.From(ctx).Token
	uid, role, expires, err := m.store.GetSession(ctx, token)
	if err != nil {
		return api.AuthValidateSessionResp{}, ecode.Unauthorized("会话无效")
	}
	if time.Now().After(expires) {
		_ = m.store.DeleteSession(ctx, token)
		return api.AuthValidateSessionResp{}, ecode.Unauthorized("会话已过期")
	}
	return api.AuthValidateSessionResp{UserID: uid, Role: role}, nil
}
