package survey

import (
	"context"
	"errors"
	"testing"

	"wenjuandiaocha_backend/api"
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

// ctxUser 造一个带指定用户身份的 ctx(替代原 Req.OwnerID)。
func ctxUser(uid string) context.Context {
	return api.WithMetadata(context.Background(), api.Metadata{UserID: uid})
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

// 归属校验:非本人 → NotFound(不泄露存在性)。
func TestOwned_NotOwner_ReturnsNotFound(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "live"}}
	m := New(f)
	_, err := m.Get(ctxUser("bob"), api.SurveyGetReq{ID: "s1"})
	if got := codeOf(t, err); got != ecode.CodeNotFound {
		t.Fatalf("非本人 Get code = %d, want CodeNotFound", got)
	}
}

// 归属校验:查无 → NotFound。
func TestOwned_NotFound_ReturnsNotFound(t *testing.T) {
	f := &fakeStore{getErr: dao.ErrNotFound}
	m := New(f)
	_, err := m.Close(ctxUser("alice"), api.SurveyCloseReq{ID: "s1"})
	if got := codeOf(t, err); got != ecode.CodeNotFound {
		t.Fatalf("查无 Close code = %d, want CodeNotFound", got)
	}
}

// 状态机守卫:非 live 结束 → Conflict,且不写状态。
func TestClose_NotLive_ReturnsConflict(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "draft"}}
	m := New(f)
	_, err := m.Close(ctxUser("alice"), api.SurveyCloseReq{ID: "s1"})
	if got := codeOf(t, err); got != ecode.CodeConflict {
		t.Fatalf("draft Close code = %d, want CodeConflict", got)
	}
	if f.setCalled {
		t.Fatalf("非法跳转不应调用 SetStatus")
	}
}

// 状态机守卫:live 正常结束 → 写 closed。
func TestClose_Live_SetsClosed(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "live"}}
	m := New(f)
	if _, err := m.Close(ctxUser("alice"), api.SurveyCloseReq{ID: "s1"}); err != nil {
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
	_, err := m.Reopen(ctxUser("alice"), api.SurveyReopenReq{ID: "s1"})
	if got := codeOf(t, err); got != ecode.CodeConflict {
		t.Fatalf("未发布 Reopen code = %d, want CodeConflict", got)
	}
}

// 重开守卫:closed 且曾发布 → 写 live。
func TestReopen_Published_SetsLive(t *testing.T) {
	v := int32(2)
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "closed", PublishedVersion: &v}}
	m := New(f)
	if _, err := m.Reopen(ctxUser("alice"), api.SurveyReopenReq{ID: "s1"}); err != nil {
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
	resp, err := m.Create(ctxUser("alice"), api.SurveyCreateReq{Body: nil})
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
	_, err := m.Create(ctxUser("alice"), api.SurveyCreateReq{Body: []byte("{not json")})
	if got := codeOf(t, err); got != ecode.CodeBadRequest {
		t.Fatalf("非法 JSON Create code = %d, want CodeBadRequest", got)
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
