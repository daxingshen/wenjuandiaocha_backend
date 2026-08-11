// Package auth 是鉴权业务层:登录(验密+建会话)、登出、取当前用户。
// 纯密码学原语在 internal/auth;会话存取在 dao;cookie 下发是传输关切,留在 server/http。
package auth

import (
	"context"
	"time"

	"github.com/google/wire"

	authlib "wenjuandiaocha_backend/internal/auth"
	"wenjuandiaocha_backend/internal/dao"
	"wenjuandiaocha_backend/internal/ecode"
)

// Store 是本层依赖的 dao 子集(消费方定义接口,便于单测)。*dao.Store 实现它。
type Store interface {
	GetUserByAccount(ctx context.Context, account string) (dao.User, error)
	GetUserByID(ctx context.Context, id string) (dao.User, error)
	CreateSession(ctx context.Context, token, userID string, expires time.Time) error
	GetSession(ctx context.Context, token string) (userID string, expires time.Time, err error)
	DeleteSession(ctx context.Context, token string) error
}

// User 对外用户信息(对齐前端 AuthUser)。
type User struct {
	ID    string
	Name  string
	Level string
}

// ---------- 统一 I/O 契约(一处定义,http/gRPC 两端共用)----------

type LoginReq struct{ Account, Password string }
type LoginResp struct {
	User    User
	Token   string    // session token,传输层负责下发(HTTP set-cookie / gRPC metadata)
	Expires time.Time // 会话过期时刻,供传输层设 cookie MaxAge
}

type MeReq struct{ UserID string }
type MeResp struct{ User User }

type LogoutReq struct{ Token string }
type LogoutResp struct{}

type ValidateSessionReq struct{ Token string }
type ValidateSessionResp struct{ UserID string }

// Service 是鉴权业务契约。*Manager 实现它;传输层持本接口。
type Service interface {
	Login(ctx context.Context, req LoginReq) (LoginResp, error)
	Me(ctx context.Context, req MeReq) (MeResp, error)
	Logout(ctx context.Context, req LogoutReq) (LogoutResp, error)
	ValidateSession(ctx context.Context, req ValidateSessionReq) (ValidateSessionResp, error)
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
func (m *Manager) Login(ctx context.Context, req LoginReq) (LoginResp, error) {
	if req.Account == "" {
		return LoginResp{}, ecode.BadRequest("账号或密码缺失")
	}
	u, err := m.store.GetUserByAccount(ctx, req.Account)
	if err != nil || !authlib.CheckPassword(u.PasswordHash, req.Password) {
		return LoginResp{}, ecode.Unauthorized("账号或密码错误")
	}
	token, err := authlib.NewSessionToken()
	if err != nil {
		return LoginResp{}, ecode.Internal("内部错误")
	}
	expires := time.Now().Add(m.sessionTTL)
	if err := m.store.CreateSession(ctx, token, u.ID, expires); err != nil {
		return LoginResp{}, err
	}
	return LoginResp{User: User{ID: u.ID, Name: u.Name, Level: u.Level}, Token: token, Expires: expires}, nil
}

// Logout 删会话(token 为空则无操作)。
func (m *Manager) Logout(ctx context.Context, req LogoutReq) (LogoutResp, error) {
	if req.Token != "" {
		_ = m.store.DeleteSession(ctx, req.Token)
	}
	return LogoutResp{}, nil
}

// Me 取当前用户信息。
func (m *Manager) Me(ctx context.Context, req MeReq) (MeResp, error) {
	u, err := m.store.GetUserByID(ctx, req.UserID)
	if err != nil {
		return MeResp{}, err
	}
	return MeResp{User: User{ID: u.ID, Name: u.Name, Level: u.Level}}, nil
}

// ValidateSession 校验会话 token:查无/无效 → Unauthorized("会话无效");
// 已过期 → 删除并 Unauthorized("会话已过期");有效返回 userID。空 token 由传输层先行拦截。
func (m *Manager) ValidateSession(ctx context.Context, req ValidateSessionReq) (ValidateSessionResp, error) {
	uid, expires, err := m.store.GetSession(ctx, req.Token)
	if err != nil {
		return ValidateSessionResp{}, ecode.Unauthorized("会话无效")
	}
	if time.Now().After(expires) {
		_ = m.store.DeleteSession(ctx, req.Token)
		return ValidateSessionResp{}, ecode.Unauthorized("会话已过期")
	}
	return ValidateSessionResp{UserID: uid}, nil
}
