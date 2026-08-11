package survey

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"wenjuandiaocha_backend/internal/dao"
	"wenjuandiaocha_backend/internal/ecode"
)

// fakeStore 只实现被测路径需要的方法;其余返回零值。
type fakeStore struct {
	meta       dao.SurveyMeta
	getErr     error
	setStatus  string // 记录 SetStatus 实际写入的状态
	setCalled  bool
	publishVer int
}

func (f *fakeStore) GetSurvey(_ context.Context, _ string) (dao.SurveyMeta, error) {
	return f.meta, f.getErr
}
func (f *fakeStore) ListSurveysByOwner(_ context.Context, _ string) ([]dao.SurveyListItem, error) {
	return nil, nil
}
func (f *fakeStore) CreateSurvey(_ context.Context, _, _, _, _ string, _ []byte) error { return nil }
func (f *fakeStore) UpdateDraft(_ context.Context, _, _, _ string, _ []byte) error     { return nil }
func (f *fakeStore) SetStatus(_ context.Context, _, status string) error {
	f.setCalled = true
	f.setStatus = status
	return nil
}
func (f *fakeStore) Publish(_ context.Context, _ string, _ []byte) (int, bool, error) {
	return f.publishVer, false, nil
}
func (f *fakeStore) CountResponses(_ context.Context, _ string) (int32, error) { return 0, nil }

func statusOf(t *testing.T, err error) int {
	t.Helper()
	s, _, ok := ecode.FromError(err)
	if !ok {
		t.Fatalf("期望 ecode.Error,得到 %v", err)
	}
	return s
}

// 归属校验:非本人 → 404(不泄露存在性)。
func TestOwned_NotOwner_Returns404(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "live"}}
	m := New(f)
	_, err := m.Get(context.Background(), GetReq{ID: "s1", OwnerID: "bob"})
	if got := statusOf(t, err); got != http.StatusNotFound {
		t.Fatalf("非本人 Get 状态 = %d, want 404", got)
	}
}

// 归属校验:查无 → 404。
func TestOwned_NotFound_Returns404(t *testing.T) {
	f := &fakeStore{getErr: dao.ErrNotFound}
	m := New(f)
	_, err := m.Close(context.Background(), CloseReq{ID: "s1", OwnerID: "alice"})
	if got := statusOf(t, err); got != http.StatusNotFound {
		t.Fatalf("查无 Close 状态 = %d, want 404", got)
	}
}

// 状态机守卫:非 live 结束 → 409,且不写状态。
func TestClose_NotLive_Returns409(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "draft"}}
	m := New(f)
	_, err := m.Close(context.Background(), CloseReq{ID: "s1", OwnerID: "alice"})
	if got := statusOf(t, err); got != http.StatusConflict {
		t.Fatalf("draft Close 状态 = %d, want 409", got)
	}
	if f.setCalled {
		t.Fatalf("非法跳转不应调用 SetStatus")
	}
}

// 状态机守卫:live 正常结束 → 写 closed。
func TestClose_Live_SetsClosed(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "live"}}
	m := New(f)
	if _, err := m.Close(context.Background(), CloseReq{ID: "s1", OwnerID: "alice"}); err != nil {
		t.Fatalf("live Close 应成功,得到 %v", err)
	}
	if f.setStatus != "closed" {
		t.Fatalf("SetStatus = %q, want closed", f.setStatus)
	}
}

// 重开守卫:closed 但从未发布 → 409。
func TestReopen_NeverPublished_Returns409(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "closed", PublishedVersion: nil}}
	m := New(f)
	_, err := m.Reopen(context.Background(), ReopenReq{ID: "s1", OwnerID: "alice"})
	if got := statusOf(t, err); got != http.StatusConflict {
		t.Fatalf("未发布 Reopen 状态 = %d, want 409", got)
	}
}

// 重开守卫:closed 且曾发布 → 写 live。
func TestReopen_Published_SetsLive(t *testing.T) {
	v := int32(2)
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "closed", PublishedVersion: &v}}
	m := New(f)
	if _, err := m.Reopen(context.Background(), ReopenReq{ID: "s1", OwnerID: "alice"}); err != nil {
		t.Fatalf("已发布 Reopen 应成功,得到 %v", err)
	}
	if f.setStatus != "live" {
		t.Fatalf("SetStatus = %q, want live", f.setStatus)
	}
}

// Create:空 body 也应补默认并落库,返回后端分配的 id(忽略客户端 id)。
func TestCreate_EmptyBody_AssignsID(t *testing.T) {
	f := &fakeStore{}
	m := New(f)
	resp, err := m.Create(context.Background(), CreateReq{OwnerID: "alice", Body: nil})
	if err != nil {
		t.Fatalf("Create 空 body 应成功,得到 %v", err)
	}
	if resp.ID == "" {
		t.Fatalf("Create 应返回后端分配的非空 id")
	}
}

// Create:非法 JSON → 400。
func TestCreate_BadJSON_Returns400(t *testing.T) {
	f := &fakeStore{}
	m := New(f)
	_, err := m.Create(context.Background(), CreateReq{OwnerID: "alice", Body: []byte("{not json")})
	if got := statusOf(t, err); got != http.StatusBadRequest {
		t.Fatalf("非法 JSON Create 状态 = %d, want 400", got)
	}
}

// 确认 fakeStore 满足 Store 接口(编译期)。
var _ Store = (*fakeStore)(nil)

// 确认 *dao.Store 满足 Store 接口(编译期,防接口漂移)。
var _ Store = (*dao.Store)(nil)

// sanity:ErrNotFound 是可 Is 的哨兵。
func TestErrNotFoundIsSentinel(t *testing.T) {
	if !errors.Is(dao.ErrNotFound, dao.ErrNotFound) {
		t.Fatal("ErrNotFound 应可 errors.Is")
	}
}
