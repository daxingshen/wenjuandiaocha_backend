package auth

import (
	"context"
	"testing"
	"time"

	"wenjuandiaocha_backend/api"
	authlib "wenjuandiaocha_backend/internal/lib/auth"
	"wenjuandiaocha_backend/internal/dao"
	"wenjuandiaocha_backend/internal/ecode"
	"wenjuandiaocha_backend/internal/lib/metadata"
)

type fakeStore struct {
	user        dao.User
	userErr     error
	sessUserID  string
	sessRole    string
	sessExpires time.Time
	sessErr     error
	deleted     bool
	created     bool
}

func (f *fakeStore) GetUserByAccount(_ context.Context, _ string) (dao.User, error) {
	return f.user, f.userErr
}
func (f *fakeStore) GetUserByID(_ context.Context, _ string) (dao.User, error) {
	return f.user, f.userErr
}
func (f *fakeStore) CreateSession(_ context.Context, _, _ string, _ time.Time) error {
	f.created = true
	return nil
}
func (f *fakeStore) GetSession(_ context.Context, _ string) (string, string, time.Time, error) {
	return f.sessUserID, f.sessRole, f.sessExpires, f.sessErr
}
func (f *fakeStore) DeleteSession(_ context.Context, _ string) error {
	f.deleted = true
	return nil
}

// codeOf 提取业务错误码(信封化后 FromError 返回 ecode.Code*,不再是 HTTP status)。
func codeOf(t *testing.T, err error) int {
	t.Helper()
	c, _, ok := ecode.FromError(err)
	if !ok {
		t.Fatalf("期望 ecode.Error,得到 %v", err)
	}
	return c
}

// 防枚举:账号不存在 → Unauthorized（与密码错同文案同码）。
func TestLogin_AccountNotFound_Unauthorized(t *testing.T) {
	f := &fakeStore{userErr: dao.ErrNotFound}
	m := New(f, time.Hour)
	_, err := m.Login(context.Background(), api.AuthLoginReq{Account: "ghost", Password: "pw"})
	if got := codeOf(t, err); got != ecode.CodeUnauthorized {
		t.Fatalf("账号不存在 code = %d, want CodeUnauthorized", got)
	}
}

// 防枚举:密码错 → Unauthorized，消息与账号不存在一致（不可区分）。
func TestLogin_WrongPassword_SameAsUnknownAccount(t *testing.T) {
	hash, _ := authlib.HashPassword("correct-horse")
	f := &fakeStore{user: dao.User{ID: "u1", PasswordHash: hash, Name: "n", Role: "creator"}}
	m := New(f, time.Hour)

	_, errWrong := m.Login(context.Background(), api.AuthLoginReq{Account: "alice", Password: "wrong"})
	cWrong, mWrong, _ := ecode.FromError(errWrong)

	f2 := &fakeStore{userErr: dao.ErrNotFound}
	_, errGhost := New(f2, time.Hour).Login(context.Background(), api.AuthLoginReq{Account: "ghost", Password: "wrong"})
	cGhost, mGhost, _ := ecode.FromError(errGhost)

	if cWrong != ecode.CodeUnauthorized || cWrong != cGhost || mWrong != mGhost {
		t.Fatalf("密码错(%d,%q)与账号不存在(%d,%q)应不可区分", cWrong, mWrong, cGhost, mGhost)
	}
	if f.created {
		t.Fatal("密码错不应建会话")
	}
}

// 缺账号 → 400。
func TestLogin_EmptyAccount_BadRequest(t *testing.T) {
	m := New(&fakeStore{}, time.Hour)
	_, err := m.Login(context.Background(), api.AuthLoginReq{Account: "", Password: "pw"})
	if got := codeOf(t, err); got != ecode.CodeBadRequest {
		t.Fatalf("空账号 code = %d, want CodeBadRequest", got)
	}
}

// 正确账密 → 成功建会话,返回用户与 token。
func TestLogin_Success_CreatesSession(t *testing.T) {
	hash, _ := authlib.HashPassword("pw")
	f := &fakeStore{user: dao.User{ID: "u1", PasswordHash: hash, Name: "Alice", Role: "creator"}}
	m := New(f, time.Hour)
	resp, err := m.Login(context.Background(), api.AuthLoginReq{Account: "alice", Password: "pw"})
	if err != nil {
		t.Fatalf("正确账密应成功,得到 %v", err)
	}
	if resp.User.ID != "u1" || resp.User.Name != "Alice" || resp.Token == "" || !f.created {
		t.Fatalf("登录结果异常: u=%+v token=%q created=%v", resp.User, resp.Token, f.created)
	}
}

// ValidateSession:过期 → 删除会话并 Unauthorized。
func TestValidateSession_Expired_DeletesAndUnauthorized(t *testing.T) {
	f := &fakeStore{sessUserID: "u1", sessExpires: time.Now().Add(-time.Minute)}
	m := New(f, time.Hour)
	_, err := m.ValidateSession(metadata.With(context.Background(), metadata.Metadata{Token: "tok"}))
	if got := codeOf(t, err); got != ecode.CodeUnauthorized {
		t.Fatalf("过期会话 code = %d, want CodeUnauthorized", got)
	}
	if !f.deleted {
		t.Fatal("过期会话应被删除")
	}
}

// ValidateSession:查无 → Unauthorized。
func TestValidateSession_NotFound_Unauthorized(t *testing.T) {
	f := &fakeStore{sessErr: dao.ErrNotFound}
	m := New(f, time.Hour)
	_, err := m.ValidateSession(metadata.With(context.Background(), metadata.Metadata{Token: "tok"}))
	if got := codeOf(t, err); got != ecode.CodeUnauthorized {
		t.Fatalf("无效会话 code = %d, want CodeUnauthorized", got)
	}
}

// ValidateSession:有效 → 返回 userID + role(供中间件注入 ctx 供 RBAC 判定)。
func TestValidateSession_Valid_ReturnsUIDAndRole(t *testing.T) {
	f := &fakeStore{sessUserID: "u1", sessRole: "admin", sessExpires: time.Now().Add(time.Hour)}
	m := New(f, time.Hour)
	resp, err := m.ValidateSession(metadata.With(context.Background(), metadata.Metadata{Token: "tok"}))
	if err != nil || resp.UserID != "u1" || resp.Role != "admin" {
		t.Fatalf("有效会话应返回 u1/admin,得到 uid=%q role=%q err=%v", resp.UserID, resp.Role, err)
	}
}

// Login/Me 透出 role(对外 AuthUser.Role),供前端体验层门控。
func TestLoginAndMe_PropagateRole(t *testing.T) {
	hash, _ := authlib.HashPassword("pw")
	f := &fakeStore{user: dao.User{ID: "u1", PasswordHash: hash, Name: "Alice", Role: "creator"}}
	m := New(f, time.Hour)
	login, err := m.Login(context.Background(), api.AuthLoginReq{Account: "alice", Password: "pw"})
	if err != nil || login.User.Role != "creator" {
		t.Fatalf("Login 应透出 role=creator,得到 %q err=%v", login.User.Role, err)
	}
	me, err := m.Me(metadata.With(context.Background(), metadata.Metadata{UserID: "u1"}))
	if err != nil || me.User.Role != "creator" {
		t.Fatalf("Me 应透出 role=creator,得到 %q err=%v", me.User.Role, err)
	}
}

var _ Store = (*dao.Store)(nil)
var _ Store = (*fakeStore)(nil)
