package submission

import (
	"context"
	"testing"

	"wenjuandiaocha_backend/api"
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

// codeOf 提取业务错误码(信封化后 FromError 返回 ecode.Code*,不再是 HTTP status)。
func codeOf(t *testing.T, err error) int {
	t.Helper()
	c, _, ok := ecode.FromError(err)
	if !ok {
		t.Fatalf("期望 ecode.Error,得到 %v", err)
	}
	return c
}

func liveMeta() dao.SurveyMeta { return dao.SurveyMeta{ID: "s1", Status: "live"} }

// 空 schema(无题无规则)的最小合法 JSON。
const emptySchema = `{"id":"s1","type":"survey","title":"t","version":3,"questions":[],"rules":[]}`

// 收答守卫:非 live → NotFound。
func TestSubmit_NotLive_ReturnsNotFound(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", Status: "closed"}}
	m := New(f)
	_, err := m.Submit(context.Background(), api.SubmitReq{SurveyID: "s1", Answers: domain.Answers{}})
	if got := codeOf(t, err); got != ecode.CodeNotFound {
		t.Fatalf("closed 提交 code = %d, want CodeNotFound", got)
	}
}

// 收答守卫:问卷查无 → NotFound。
func TestSubmit_SurveyNotFound_ReturnsNotFound(t *testing.T) {
	f := &fakeStore{metaErr: dao.ErrNotFound}
	m := New(f)
	_, err := m.Submit(context.Background(), api.SubmitReq{SurveyID: "s1", Answers: domain.Answers{}})
	if got := codeOf(t, err); got != ecode.CodeNotFound {
		t.Fatalf("查无提交 code = %d, want CodeNotFound", got)
	}
}

// 版本锚定:version>0 走 GetVersionSchema(按作答者所见版校验)。
func TestSubmit_VersionPinned_UsesVersionSchema(t *testing.T) {
	f := &fakeStore{meta: liveMeta(), versionJSON: []byte(emptySchema)}
	m := New(f)
	res, err := m.Submit(context.Background(), api.SubmitReq{SurveyID: "s1", Answers: domain.Answers{}, Version: 3})
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

// 版本锚定:version>0 但该版失效 → BadRequest 引导刷新。
func TestSubmit_VersionStale_ReturnsBadRequest(t *testing.T) {
	f := &fakeStore{meta: liveMeta(), versionErr: dao.ErrNotFound}
	m := New(f)
	_, err := m.Submit(context.Background(), api.SubmitReq{SurveyID: "s1", Answers: domain.Answers{}, Version: 9})
	if got := codeOf(t, err); got != ecode.CodeBadRequest {
		t.Fatalf("失效版本提交 code = %d, want CodeBadRequest", got)
	}
}

// 向后兼容:version==0 回落当前发布版(GetPublishedSchema)。
func TestSubmit_NoVersion_FallsBackToPublished(t *testing.T) {
	f := &fakeStore{meta: liveMeta(), publishedJSON: []byte(emptySchema)}
	m := New(f)
	res, err := m.Submit(context.Background(), api.SubmitReq{SurveyID: "s1", Answers: domain.Answers{}})
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

// GetPublished:未发布 → NotFound。
func TestGetPublished_NotFound_ReturnsNotFound(t *testing.T) {
	f := &fakeStore{publishedErr: dao.ErrNotFound}
	m := New(f)
	_, err := m.GetPublished(context.Background(), api.GetPublishedReq{ID: "s1"})
	if got := codeOf(t, err); got != ecode.CodeNotFound {
		t.Fatalf("未发布 GetPublished code = %d, want CodeNotFound", got)
	}
}

var _ Store = (*dao.Store)(nil)
var _ Store = (*fakeStore)(nil)
