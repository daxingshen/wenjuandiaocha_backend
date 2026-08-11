package submission

import (
	"context"
	"net/http"
	"testing"

	"wenjuandiaocha_backend/internal/dao"
	"wenjuandiaocha_backend/internal/domain"
	"wenjuandiaocha_backend/internal/ecode"
)

// fakeStore 记录调用,按字段返回预设结果。
type fakeStore struct {
	meta          dao.SurveyMeta
	metaErr       error
	publishedJSON []byte
	publishedErr  error
	versionJSON   []byte
	versionErr    error
	versionCalled bool
	savedVersion  int
	saveCalled    bool
}

func (f *fakeStore) GetSurvey(_ context.Context, _ string) (dao.SurveyMeta, error) {
	return f.meta, f.metaErr
}
func (f *fakeStore) GetPublishedSchema(_ context.Context, _ string) ([]byte, error) {
	return f.publishedJSON, f.publishedErr
}
func (f *fakeStore) GetVersionSchema(_ context.Context, _ string, _ int32) ([]byte, error) {
	f.versionCalled = true
	return f.versionJSON, f.versionErr
}
func (f *fakeStore) SaveSubmission(_ context.Context, _, _ string, version int, _, _ []byte, _ []domain.NormalizedRow) error {
	f.saveCalled = true
	f.savedVersion = version
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

func liveMeta() dao.SurveyMeta { return dao.SurveyMeta{ID: "s1", Status: "live"} }

// 空 schema(无题无规则)的最小合法 JSON。
const emptySchema = `{"id":"s1","type":"survey","title":"t","version":3,"questions":[],"rules":[]}`

// 收答守卫:非 live → 404。
func TestSubmit_NotLive_Returns404(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", Status: "closed"}}
	m := New(f)
	_, err := m.Submit(context.Background(), SubmitReq{SurveyID: "s1", Answers: domain.Answers{}})
	if got := statusOf(t, err); got != http.StatusNotFound {
		t.Fatalf("closed 提交状态 = %d, want 404", got)
	}
}

// 收答守卫:问卷查无 → 404。
func TestSubmit_SurveyNotFound_Returns404(t *testing.T) {
	f := &fakeStore{metaErr: dao.ErrNotFound}
	m := New(f)
	_, err := m.Submit(context.Background(), SubmitReq{SurveyID: "s1", Answers: domain.Answers{}})
	if got := statusOf(t, err); got != http.StatusNotFound {
		t.Fatalf("查无提交状态 = %d, want 404", got)
	}
}

// 版本锚定:version>0 走 GetVersionSchema(按作答者所见版校验)。
func TestSubmit_VersionPinned_UsesVersionSchema(t *testing.T) {
	f := &fakeStore{meta: liveMeta(), versionJSON: []byte(emptySchema)}
	m := New(f)
	res, err := m.Submit(context.Background(), SubmitReq{SurveyID: "s1", Answers: domain.Answers{}, Version: 3})
	if err != nil || len(res.ValidationErrors) > 0 {
		t.Fatalf("空 schema 提交应成功: err=%v verrs=%v", err, res.ValidationErrors)
	}
	if !f.versionCalled {
		t.Fatal("version>0 应调用 GetVersionSchema")
	}
	if !f.saveCalled || f.savedVersion != 3 {
		t.Fatalf("应落库且版本锚定为快照 version=3,得到 saved=%v ver=%d", f.saveCalled, f.savedVersion)
	}
}

// 版本锚定:version>0 但该版失效 → 400 引导刷新。
func TestSubmit_VersionStale_Returns400(t *testing.T) {
	f := &fakeStore{meta: liveMeta(), versionErr: dao.ErrNotFound}
	m := New(f)
	_, err := m.Submit(context.Background(), SubmitReq{SurveyID: "s1", Answers: domain.Answers{}, Version: 9})
	if got := statusOf(t, err); got != http.StatusBadRequest {
		t.Fatalf("失效版本提交状态 = %d, want 400", got)
	}
}

// 向后兼容:version==0 回落当前发布版(GetPublishedSchema)。
func TestSubmit_NoVersion_FallsBackToPublished(t *testing.T) {
	f := &fakeStore{meta: liveMeta(), publishedJSON: []byte(emptySchema)}
	m := New(f)
	res, err := m.Submit(context.Background(), SubmitReq{SurveyID: "s1", Answers: domain.Answers{}})
	if err != nil || len(res.ValidationErrors) > 0 {
		t.Fatalf("回落发布版提交应成功: err=%v verrs=%v", err, res.ValidationErrors)
	}
	if f.versionCalled {
		t.Fatal("version==0 不应调用 GetVersionSchema")
	}
	if !f.saveCalled {
		t.Fatal("应落库")
	}
}

// GetPublished:未发布 → 404。
func TestGetPublished_NotFound_Returns404(t *testing.T) {
	f := &fakeStore{publishedErr: dao.ErrNotFound}
	m := New(f)
	_, err := m.GetPublished(context.Background(), GetPublishedReq{ID: "s1"})
	if got := statusOf(t, err); got != http.StatusNotFound {
		t.Fatalf("未发布 GetPublished 状态 = %d, want 404", got)
	}
}

var _ Store = (*dao.Store)(nil)
var _ Store = (*fakeStore)(nil)
