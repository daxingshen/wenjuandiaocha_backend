package auth

import (
	"context"
	"net/http"
	"testing"
	"time"

	authlib "wenjuandiaocha_backend/internal/auth"
	"wenjuandiaocha_backend/internal/dao"
	"wenjuandiaocha_backend/internal/ecode"
)

type fakeStore struct {
	user        dao.User
	userErr     error
	sessUserID  string
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
func (f *fakeStore) GetSession(_ context.Context, _ string) (string, time.Time, error) {
	return f.sessUserID, f.sessExpires, f.sessErr
}
func (f *fakeStore) DeleteSession(_ context.Context, _ string) error {
	f.deleted = true
	return nil
}

func statusOf(t *testing.T, err error) int {
	t.Helper()
	s, _, ok := ecode.FromError(err)
	if !ok {
		t.Fatalf("期望 ecode.Error,得到 %v", err)
	}
	return s
}

// 防枚举:账号不存在 → Unauthorized（与密码错同文案同码）。
func TestLogin_AccountNotFound_Unauthorized(t *testing.T) {
	f := &fakeStore{userErr: dao.ErrNotFound}
	m := New(f, time.Hour)
	_, err := m.Login(context.Background(), LoginReq{Account: "ghost", Password: "pw"})
	if got := statusOf(t, err); got != http.StatusUnauthorized {
		t.Fatalf("账号不存在状态 = %d, want 401", got)
	}
}

// 防枚举:密码错 → Unauthorized，消息与账号不存在一致（不可区分）。
func TestLogin_WrongPassword_SameAsUnknownAccount(t *testing.T) {
	hash, _ := authlib.HashPassword("correct-horse")
	f := &fakeStore{user: dao.User{ID: "u1", PasswordHash: hash, Name: "n", Level: "admin"}}
	m := New(f, time.Hour)

	_, errWrong := m.Login(context.Background(), LoginReq{Account: "alice", Password: "wrong"})
	sWrong, mWrong, _ := ecode.FromError(errWrong)

	f2 := &fakeStore{userErr: dao.ErrNotFound}
	_, errGhost := New(f2, time.Hour).Login(context.Background(), LoginReq{Account: "ghost", Password: "wrong"})
	sGhost, mGhost, _ := ecode.FromError(errGhost)

	if sWrong != http.StatusUnauthorized || sWrong != sGhost || mWrong != mGhost {
		t.Fatalf("密码错(%d,%q)与账号不存在(%d,%q)应不可区分", sWrong, mWrong, sGhost, mGhost)
	}
	if f.created {
		t.Fatal("密码错不应建会话")
	}
}

// 缺账号 → 400。
func TestLogin_EmptyAccount_BadRequest(t *testing.T) {
	m := New(&fakeStore{}, time.Hour)
	_, err := m.Login(context.Background(), LoginReq{Account: "", Password: "pw"})
	if got := statusOf(t, err); got != http.StatusBadRequest {
		t.Fatalf("空账号状态 = %d, want 400", got)
	}
}

// 正确账密 → 成功建会话,返回用户与 token。
func TestLogin_Success_CreatesSession(t *testing.T) {
	hash, _ := authlib.HashPassword("pw")
	f := &fakeStore{user: dao.User{ID: "u1", PasswordHash: hash, Name: "Alice", Level: "admin"}}
	m := New(f, time.Hour)
	resp, err := m.Login(context.Background(), LoginReq{Account: "alice", Password: "pw"})
	if err != nil {
		t.Fatalf("正确账密应成功,得到 %v", err)
	}
	if resp.User.ID != "u1" || resp.User.Name != "Alice" || resp.Token == "" || !f.created {
		t.Fatalf("登录结果异常: u=%+v token=%q created=%v", resp.User, resp.Token, f.created)
	}
}

// ValidateSession:过期 → 删除会话并 401。
func TestValidateSession_Expired_DeletesAnd401(t *testing.T) {
	f := &fakeStore{sessUserID: "u1", sessExpires: time.Now().Add(-time.Minute)}
	m := New(f, time.Hour)
	_, err := m.ValidateSession(context.Background(), ValidateSessionReq{Token: "tok"})
	if got := statusOf(t, err); got != http.StatusUnauthorized {
		t.Fatalf("过期会话状态 = %d, want 401", got)
	}
	if !f.deleted {
		t.Fatal("过期会话应被删除")
	}
}

// ValidateSession:查无 → 401。
func TestValidateSession_NotFound_401(t *testing.T) {
	f := &fakeStore{sessErr: dao.ErrNotFound}
	m := New(f, time.Hour)
	_, err := m.ValidateSession(context.Background(), ValidateSessionReq{Token: "tok"})
	if got := statusOf(t, err); got != http.StatusUnauthorized {
		t.Fatalf("无效会话状态 = %d, want 401", got)
	}
}

// ValidateSession:有效 → 返回 userID。
func TestValidateSession_Valid_ReturnsUID(t *testing.T) {
	f := &fakeStore{sessUserID: "u1", sessExpires: time.Now().Add(time.Hour)}
	m := New(f, time.Hour)
	resp, err := m.ValidateSession(context.Background(), ValidateSessionReq{Token: "tok"})
	if err != nil || resp.UserID != "u1" {
		t.Fatalf("有效会话应返回 u1,得到 uid=%q err=%v", resp.UserID, err)
	}
}

var _ Store = (*dao.Store)(nil)
var _ Store = (*fakeStore)(nil)
