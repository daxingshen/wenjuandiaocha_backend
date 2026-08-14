package submission

import (
	"context"
	"testing"

	"wenjuandiaocha_backend/api"
	"wenjuandiaocha_backend/internal/dao"
	"wenjuandiaocha_backend/internal/domain"
	"wenjuandiaocha_backend/internal/ecode"
	"wenjuandiaocha_backend/internal/lib/metadata"
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
	_, err := m.Submit(context.Background(), api.SubmitReq{SurveyID: "s1", Answers: api.Answers{}})
	if got := codeOf(t, err); got != ecode.CodeNotFound {
		t.Fatalf("closed 提交 code = %d, want CodeNotFound", got)
	}
}

// 收答守卫:问卷查无 → NotFound。
func TestSubmit_SurveyNotFound_ReturnsNotFound(t *testing.T) {
	f := &fakeStore{metaErr: dao.ErrNotFound}
	m := New(f)
	_, err := m.Submit(context.Background(), api.SubmitReq{SurveyID: "s1", Answers: api.Answers{}})
	if got := codeOf(t, err); got != ecode.CodeNotFound {
		t.Fatalf("查无提交 code = %d, want CodeNotFound", got)
	}
}

// 版本锚定:version>0 走 GetVersionSchema(按作答者所见版校验)。
func TestSubmit_VersionPinned_UsesVersionSchema(t *testing.T) {
	f := &fakeStore{meta: liveMeta(), versionJSON: []byte(emptySchema)}
	m := New(f)
	_, err := m.Submit(context.Background(), api.SubmitReq{SurveyID: "s1", Answers: api.Answers{}, Version: 3})
	if err != nil {
		t.Fatalf("空 schema 提交应成功: err=%v", err)
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
	_, err := m.Submit(context.Background(), api.SubmitReq{SurveyID: "s1", Answers: api.Answers{}, Version: 9})
	if got := codeOf(t, err); got != ecode.CodeBadRequest {
		t.Fatalf("失效版本提交 code = %d, want CodeBadRequest", got)
	}
}

// 向后兼容:version==0 回落当前发布版(GetPublishedSchema)。
func TestSubmit_NoVersion_FallsBackToPublished(t *testing.T) {
	f := &fakeStore{meta: liveMeta(), publishedJSON: []byte(emptySchema)}
	m := New(f)
	_, err := m.Submit(context.Background(), api.SubmitReq{SurveyID: "s1", Answers: api.Answers{}})
	if err != nil {
		t.Fatalf("回落发布版提交应成功: err=%v", err)
	}
	if f.versionCalled {
		t.Fatal("version==0 不应调用 GetVersionSchema")
	}
	if !f.saveCalled {
		t.Fatal("应落库")
	}
}

// ctxRole 造一个「已登录」ctx:带真实会话身份(UserID)+ 角色。
// UserID 非空 = submission 据此判定为已登录路径(等价 requireAuth 注入后的 ctx)。
func ctxRole(role string) context.Context {
	return metadata.With(context.Background(), metadata.Metadata{UserID: "u_" + role, Role: role})
}

// 作答模式闸门:login_required 问卷经匿名路径提交(无会话)→ 401 需登录。
// 公开 GET 已回显 answerAccess,需登录本非秘密,故直白 401 而非伪装 404。
func TestSubmit_LoginRequired_AnonPath_ReturnsUnauthorized(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", Status: "live", AnswerAccess: "login_required"}, publishedJSON: []byte(emptySchema)}
	m := New(f)
	_, err := m.Submit(context.Background(), api.SubmitReq{SurveyID: "s1", Answers: api.Answers{}}) // 无 UserID = 匿名
	if got := codeOf(t, err); got != ecode.CodeUnauthorized {
		t.Fatalf("login_required 匿名提交 code = %d, want CodeUnauthorized(401)", got)
	}
	if f.saveCalled {
		t.Fatal("闸门拦下不应落库")
	}
}

// 作答模式闸门:anonymous 问卷经鉴权路径提交 → BadRequest(引导走 /public)。
func TestSubmit_Anonymous_AuthedPath_ReturnsBadRequest(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", Status: "live", AnswerAccess: "anonymous"}, publishedJSON: []byte(emptySchema)}
	m := New(f)
	_, err := m.Submit(ctxRole("respondent"), api.SubmitReq{SurveyID: "s1", Answers: api.Answers{}})
	if got := codeOf(t, err); got != ecode.CodeBadRequest {
		t.Fatalf("anonymous 鉴权提交 code = %d, want CodeBadRequest", got)
	}
}

// 能力位:creator 经鉴权路径提交 login_required 问卷 → 403(creator 不能作答)。
func TestSubmit_LoginRequired_Creator_ReturnsForbidden(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", Status: "live", AnswerAccess: "login_required"}, publishedJSON: []byte(emptySchema)}
	m := New(f)
	_, err := m.Submit(ctxRole("creator"), api.SubmitReq{SurveyID: "s1", Answers: api.Answers{}})
	if got := codeOf(t, err); got != ecode.CodeForbidden {
		t.Fatalf("creator 鉴权作答 code = %d, want CodeForbidden(403)", got)
	}
	if f.saveCalled {
		t.Fatal("能力位拦下不应落库")
	}
}

// respondent 经鉴权路径提交 login_required 问卷 → 成功落库。
func TestSubmit_LoginRequired_Respondent_Succeeds(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", Status: "live", AnswerAccess: "login_required"}, publishedJSON: []byte(emptySchema)}
	m := New(f)
	_, err := m.Submit(ctxRole("respondent"), api.SubmitReq{SurveyID: "s1", Answers: api.Answers{}})
	if err != nil {
		t.Fatalf("respondent 作答 login_required 应成功: err=%v", err)
	}
	if !f.saveCalled {
		t.Fatal("应落库")
	}
}

// admin 经鉴权路径提交 login_required 问卷 → 成功(超级权限保留作答)。
func TestSubmit_LoginRequired_Admin_Succeeds(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", Status: "live", AnswerAccess: "login_required"}, publishedJSON: []byte(emptySchema)}
	m := New(f)
	_, err := m.Submit(ctxRole("admin"), api.SubmitReq{SurveyID: "s1", Answers: api.Answers{}})
	if err != nil {
		t.Fatalf("admin 作答 login_required 应成功: err=%v", err)
	}
	if !f.saveCalled {
		t.Fatal("应落库")
	}
}

// 匿名问卷经匿名路径提交(现状)→ 成功,行为不变。
func TestSubmit_Anonymous_AnonPath_Succeeds(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", Status: "live", AnswerAccess: "anonymous"}, publishedJSON: []byte(emptySchema)}
	m := New(f)
	_, err := m.Submit(context.Background(), api.SubmitReq{SurveyID: "s1", Answers: api.Answers{}})
	if err != nil {
		t.Fatalf("匿名问卷匿名提交应成功: err=%v", err)
	}
	if !f.saveCalled {
		t.Fatal("应落库")
	}
}

// 校验失败:必答题缺答 → 返回 ecode.Validation(code=CodeValidation),携逐题明细 payload,不落库。
// 收敛后校验错以 error 形式返回(原 SubmitResp.ValidationErrors 字段已移除)。
func TestSubmit_ValidationFails_ReturnsValidationError(t *testing.T) {
	// schema 含一道必答单选题;提交空答案 → 必答校验不过。
	const requiredSchema = `{"id":"s1","type":"survey","title":"t","version":3,"questions":[{"id":"q1","type":"single","title":"Q1","required":true,"options":[{"id":"o1","label":"A"}]}],"rules":[]}`
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", Status: "live", AnswerAccess: "anonymous"}, publishedJSON: []byte(requiredSchema)}
	m := New(f)
	_, err := m.Submit(context.Background(), api.SubmitReq{SurveyID: "s1", Answers: api.Answers{}})
	if got := codeOf(t, err); got != ecode.CodeValidation {
		t.Fatalf("校验失败 code = %d, want CodeValidation", got)
	}
	payload, ok := ecode.DataFromError(err).(api.ValidationErrorsPayload)
	if !ok || len(payload.Errors) == 0 {
		t.Fatalf("应携带 ValidationErrorsPayload 明细, got %#v", ecode.DataFromError(err))
	}
	if f.saveCalled {
		t.Fatal("校验失败不应落库")
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

// GetPublished 带出 answer_access:login_required 问卷返回该模式,供前端选登录作答路径。
func TestGetPublished_ReturnsAnswerAccess(t *testing.T) {
	f := &fakeStore{
		meta:          dao.SurveyMeta{ID: "s1", Status: "live", AnswerAccess: "login_required"},
		publishedJSON: []byte(emptySchema),
	}
	resp, err := New(f).GetPublished(context.Background(), api.GetPublishedReq{ID: "s1"})
	if err != nil {
		t.Fatalf("GetPublished 应成功: %v", err)
	}
	if resp.AnswerAccess != "login_required" {
		t.Fatalf("AnswerAccess = %q, want login_required", resp.AnswerAccess)
	}
}

// GetPublished 空 answer_access(历史数据)按 anonymous 回落,不返回空串误导前端。
func TestGetPublished_EmptyAccess_FallsBackAnonymous(t *testing.T) {
	f := &fakeStore{
		meta:          dao.SurveyMeta{ID: "s1", Status: "live", AnswerAccess: ""},
		publishedJSON: []byte(emptySchema),
	}
	resp, err := New(f).GetPublished(context.Background(), api.GetPublishedReq{ID: "s1"})
	if err != nil {
		t.Fatalf("GetPublished 应成功: %v", err)
	}
	if resp.AnswerAccess != "anonymous" {
		t.Fatalf("空 access 回落 = %q, want anonymous", resp.AnswerAccess)
	}
}

var _ Store = (*dao.Store)(nil)
var _ Store = (*fakeStore)(nil)
