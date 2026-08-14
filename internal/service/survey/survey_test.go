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
	meta            dao.SurveyMeta
	getErr          error
	setStatus       string // 记录 SetStatus 实际写入的状态
	setCalled       bool
	publishVer      int
	setAccess       string // 记录 SetAnswerAccess 实际写入的作答模式
	setAccessCalled bool   // 记录 SetAnswerAccess 是否被调用(守卫拦下时不应写)
	listOwner       string // 记录 ListSurveysByOwner 收到的 ownerID
	listAllCalled   bool   // 记录是否走了全站列表(admin)
	ownerItems      []dao.SurveyListItem
	allItems        []dao.SurveyListItem
}

func (f *fakeStore) GetSurvey(_ context.Context, _ string) (dao.SurveyMeta, error) {
	return f.meta, f.getErr
}
func (f *fakeStore) ListSurveysByOwner(_ context.Context, ownerID string) ([]dao.SurveyListItem, error) {
	f.listOwner = ownerID
	return f.ownerItems, nil
}
func (f *fakeStore) ListAllSurveys(_ context.Context) ([]dao.SurveyListItem, error) {
	f.listAllCalled = true
	return f.allItems, nil
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
func (f *fakeStore) SetAnswerAccess(_ context.Context, _, access string) error {
	f.setAccessCalled = true
	f.setAccess = access
	return nil
}
func (f *fakeStore) CountResponses(_ context.Context, _ string) (int32, error) { return 0, nil }

// ctxUser 造一个带指定用户身份的 ctx(默认 creator 角色:既有归属/状态机用例都是「创作者管自己的卷」场景)。
func ctxUser(uid string) context.Context {
	return ctxRole(uid, "creator")
}

// ctxRole 造带指定用户 id + 角色的 ctx(RBAC 判定用)。
func ctxRole(uid, role string) context.Context {
	return api.WithMetadata(context.Background(), api.Metadata{UserID: uid, Role: role})
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

// updateFake 在 fakeStore 基础上记录 UpdateDraft 是否被调用,用于断言"守卫拦下时不写库"。
type updateFake struct {
	fakeStore
	updateCalled bool
}

func (f *updateFake) UpdateDraft(_ context.Context, _, _, _ string, _ []byte) error {
	f.updateCalled = true
	return nil
}

// 编辑守卫:draft 放行 → 写库(UpdateDraft 被调用)。
func TestUpdate_Draft_Writes(t *testing.T) {
	f := &updateFake{fakeStore: fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "draft"}}}
	m := New(f)
	if _, err := m.Update(ctxUser("alice"), api.SurveyUpdateReq{ID: "s1", Body: []byte(`{"id":"s1"}`)}); err != nil {
		t.Fatalf("draft Update 应成功,得到 %v", err)
	}
	if !f.updateCalled {
		t.Fatalf("draft Update 应调用 UpdateDraft 写库")
	}
}

// 编辑守卫:live 拒 → Conflict,且不写库。
func TestUpdate_Live_ReturnsConflict(t *testing.T) {
	f := &updateFake{fakeStore: fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "live"}}}
	m := New(f)
	_, err := m.Update(ctxUser("alice"), api.SurveyUpdateReq{ID: "s1", Body: []byte(`{"id":"s1"}`)})
	if got := codeOf(t, err); got != ecode.CodeConflict {
		t.Fatalf("live Update code = %d, want CodeConflict", got)
	}
	if f.updateCalled {
		t.Fatalf("已发布问卷不应写库")
	}
}

// 编辑守卫:closed 拒 → Conflict,且不写库。
func TestUpdate_Closed_ReturnsConflict(t *testing.T) {
	f := &updateFake{fakeStore: fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "closed"}}}
	m := New(f)
	_, err := m.Update(ctxUser("alice"), api.SurveyUpdateReq{ID: "s1", Body: []byte(`{"id":"s1"}`)})
	if got := codeOf(t, err); got != ecode.CodeConflict {
		t.Fatalf("closed Update code = %d, want CodeConflict", got)
	}
	if f.updateCalled {
		t.Fatalf("已截止问卷不应写库")
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

// --- RBAC 第一层能力位 + 第二层归属(admin 短路)---

// respondent 调创作端动作 → 403(第一层能力位;它只能作答)。
func TestGet_Respondent_ReturnsForbidden403(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "draft"}}
	m := New(f)
	_, err := m.Get(ctxRole("bob", "respondent"), api.SurveyGetReq{ID: "s1"})
	if got := codeOf(t, err); got != ecode.CodeForbidden {
		t.Fatalf("respondent 读创作端 code = %d, want CodeForbidden(403)", got)
	}
}

// respondent 建卷 → 403(能力位挡下,不落库)。
func TestCreate_Respondent_ReturnsForbidden403(t *testing.T) {
	f := &fakeStore{}
	m := New(f)
	_, err := m.Create(ctxRole("bob", "respondent"), api.SurveyCreateReq{Body: nil})
	if got := codeOf(t, err); got != ecode.CodeForbidden {
		t.Fatalf("respondent 建卷 code = %d, want CodeForbidden(403)", got)
	}
}

// creator A 访问 creator B 的卷 → 404(第二层归属,防枚举;能力位已过)。
func TestGet_CreatorCrossUser_ReturnsNotFound(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "draft"}}
	m := New(f)
	_, err := m.Get(ctxRole("bob", "creator"), api.SurveyGetReq{ID: "s1"})
	if got := codeOf(t, err); got != ecode.CodeNotFound {
		t.Fatalf("creator 跨用户 code = %d, want CodeNotFound(404)", got)
	}
}

// admin 短路归属:读他人的卷 → 放行(跨 owner)。
func TestGet_Admin_CrossOwner_Allowed(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "draft", DraftSchema: []byte(`{"id":"s1"}`)}}
	m := New(f)
	resp, err := m.Get(ctxRole("admin-user", "admin"), api.SurveyGetReq{ID: "s1"})
	if err != nil {
		t.Fatalf("admin 跨 owner 读应放行,得到 %v", err)
	}
	if string(resp.Schema) != `{"id":"s1"}` {
		t.Fatalf("admin 应拿到他人草稿,得到 %q", resp.Schema)
	}
}

// admin 短路归属:改他人 draft 卷 → 放行写库。
func TestUpdate_Admin_CrossOwner_Writes(t *testing.T) {
	f := &updateFake{fakeStore: fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "draft"}}}
	m := New(f)
	if _, err := m.Update(ctxRole("admin-user", "admin"), api.SurveyUpdateReq{ID: "s1", Body: []byte(`{"id":"s1"}`)}); err != nil {
		t.Fatalf("admin 跨 owner 改 draft 应放行,得到 %v", err)
	}
	if !f.updateCalled {
		t.Fatal("admin 改他人 draft 应写库")
	}
}

// List:admin → 走全站列表;creator → 走本人列表。
func TestList_RoleScopes(t *testing.T) {
	fa := &fakeStore{allItems: []dao.SurveyListItem{{ID: "x"}}}
	if _, err := New(fa).List(ctxRole("admin-user", "admin")); err != nil {
		t.Fatalf("admin List 应成功,得到 %v", err)
	}
	if !fa.listAllCalled {
		t.Fatal("admin List 应走全站 ListAllSurveys")
	}

	fc := &fakeStore{ownerItems: []dao.SurveyListItem{{ID: "y"}}}
	if _, err := New(fc).List(ctxRole("alice", "creator")); err != nil {
		t.Fatalf("creator List 应成功,得到 %v", err)
	}
	if fc.listAllCalled || fc.listOwner != "alice" {
		t.Fatalf("creator List 应按本人过滤(listOwner=%q, listAll=%v)", fc.listOwner, fc.listAllCalled)
	}
}

// List:respondent → 403(无列问卷能力)。
func TestList_Respondent_ReturnsForbidden403(t *testing.T) {
	f := &fakeStore{}
	_, err := New(f).List(ctxRole("bob", "respondent"))
	if got := codeOf(t, err); got != ecode.CodeForbidden {
		t.Fatalf("respondent List code = %d, want CodeForbidden(403)", got)
	}
}

// SetAnswerAccess:draft 问卷设 login_required → 写列成功。
func TestSetAnswerAccess_Draft_Succeeds(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "draft"}}
	if _, err := New(f).SetAnswerAccess(ctxUser("alice"), api.SurveySetAnswerAccessReq{ID: "s1", AnswerAccess: "login_required"}); err != nil {
		t.Fatalf("draft 设作答模式应成功,得到 %v", err)
	}
	if f.setAccess != "login_required" {
		t.Fatalf("SetAnswerAccess 应写入 login_required,得到 %q", f.setAccess)
	}
}

// SetAnswerAccess:live 问卷 → Conflict(仅 draft 可改),且不写库。
func TestSetAnswerAccess_Live_ReturnsConflict(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "live"}}
	_, err := New(f).SetAnswerAccess(ctxUser("alice"), api.SurveySetAnswerAccessReq{ID: "s1", AnswerAccess: "anonymous"})
	if got := codeOf(t, err); got != ecode.CodeConflict {
		t.Fatalf("live 设作答模式 code = %d, want CodeConflict", got)
	}
	if f.setAccessCalled {
		t.Fatal("守卫拦下不应写库")
	}
}

// SetAnswerAccess:非 owner → NotFound(归属防枚举),不写库。
func TestSetAnswerAccess_NotOwner_ReturnsNotFound(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "bob", Status: "draft"}}
	_, err := New(f).SetAnswerAccess(ctxUser("alice"), api.SurveySetAnswerAccessReq{ID: "s1", AnswerAccess: "anonymous"})
	if got := codeOf(t, err); got != ecode.CodeNotFound {
		t.Fatalf("非 owner code = %d, want CodeNotFound", got)
	}
	if f.setAccessCalled {
		t.Fatal("归属拦下不应写库")
	}
}

// SetAnswerAccess:非法值 → BadRequest,不写库。
func TestSetAnswerAccess_InvalidValue_ReturnsBadRequest(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "draft"}}
	_, err := New(f).SetAnswerAccess(ctxUser("alice"), api.SurveySetAnswerAccessReq{ID: "s1", AnswerAccess: "bogus"})
	if got := codeOf(t, err); got != ecode.CodeBadRequest {
		t.Fatalf("非法值 code = %d, want CodeBadRequest", got)
	}
	if f.setAccessCalled {
		t.Fatal("非法值不应写库")
	}
}

// Publish 不再改 answer_access:发布只冻结版本 + 转 live(作答模式由 SetAnswerAccess 定,单一真相源)。
// fakeStore.Publish 已无 answerAccess 形参,此处仅确认发布成功、不触碰作答模式写入路径。
func TestPublish_DoesNotTouchAnswerAccess(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{ID: "s1", OwnerID: "alice", Status: "draft"}, publishVer: 1}
	if _, err := New(f).Publish(ctxUser("alice"), api.SurveyPublishReq{ID: "s1"}); err != nil {
		t.Fatalf("发布应成功,得到 %v", err)
	}
	if f.setAccessCalled {
		t.Fatal("发布不应调用 SetAnswerAccess")
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
