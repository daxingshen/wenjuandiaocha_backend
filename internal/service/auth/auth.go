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

// Manager 持有 dao + 会话 TTL(建会话过期时间用)。
type Manager struct {
	store      Store
	sessionTTL time.Duration
}

func New(store Store, sessionTTL time.Duration) *Manager {
	return &Manager{store: store, sessionTTL: sessionTTL}
}

// ProviderSet 供 wire 组装(绑定放 di.wire.Build)。sessionTTL 由 di.provideSessionTTL 提供。
var ProviderSet = wire.NewSet(New)

// User 对外用户信息(对齐前端 AuthUser)。
type User struct {
	ID    string
	Name  string
	Level string
}

// Login 验账密 → 建会话 → 返回 (用户, sessionToken, 过期时刻)。
// 不区分「账号不存在」与「密码错」,避免账号枚举(统一 Unauthorized)。
func (m *Manager) Login(ctx context.Context, account, password string) (User, string, time.Time, error) {
	if account == "" {
		return User{}, "", time.Time{}, ecode.BadRequest("账号或密码缺失")
	}
	u, err := m.store.GetUserByAccount(ctx, account)
	if err != nil || !authlib.CheckPassword(u.PasswordHash, password) {
		return User{}, "", time.Time{}, ecode.Unauthorized("账号或密码错误")
	}
	token, err := authlib.NewSessionToken()
	if err != nil {
		return User{}, "", time.Time{}, ecode.Internal("内部错误")
	}
	expires := time.Now().Add(m.sessionTTL)
	if err := m.store.CreateSession(ctx, token, u.ID, expires); err != nil {
		return User{}, "", time.Time{}, err
	}
	return User{ID: u.ID, Name: u.Name, Level: u.Level}, token, expires, nil
}

// Logout 删会话(token 为空则无操作)。
func (m *Manager) Logout(ctx context.Context, token string) {
	if token != "" {
		_ = m.store.DeleteSession(ctx, token)
	}
}

// Me 取当前用户信息。
func (m *Manager) Me(ctx context.Context, userID string) (User, error) {
	u, err := m.store.GetUserByID(ctx, userID)
	if err != nil {
		return User{}, err
	}
	return User{ID: u.ID, Name: u.Name, Level: u.Level}, nil
}

// ValidateSession 校验会话 token:查无/无效 → Unauthorized("会话无效");
// 已过期 → 删除并 Unauthorized("会话已过期");有效返回 userID。空 token 由传输层先行拦截。
func (m *Manager) ValidateSession(ctx context.Context, token string) (string, error) {
	uid, expires, err := m.store.GetSession(ctx, token)
	if err != nil {
		return "", ecode.Unauthorized("会话无效")
	}
	if time.Now().After(expires) {
		_ = m.store.DeleteSession(ctx, token)
		return "", ecode.Unauthorized("会话已过期")
	}
	return uid, nil
}
