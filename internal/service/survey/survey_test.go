package survey

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"

	"wenjuandiaocha_backend/api"
	"wenjuandiaocha_backend/internal/dao"
	"wenjuandiaocha_backend/internal/domain"
	"wenjuandiaocha_backend/internal/ecode"
	"wenjuandiaocha_backend/internal/lib/metadata"
)

// fakeStore 只实现被测路径需要的方法;其余返回零值。
type fakeStore struct {
	meta             dao.SurveyMeta
	getErr           error
	setStatus        string // 记录 SetStatus 实际写入的状态
	setCalled        bool
	publishVer       int
	createdAccess    string               // 记录 CreateSurvey 实际写入的作答模式(新建默认)
	createdDisplay   string               // 记录 CreateSurvey 实际写入的展示模式(新建默认)
	createdOwner     string               // 记录 CreateSurvey 实际写入的 ownerID(复制归属断言)
	createdTitle     string               // 记录 CreateSurvey 实际写入的标题(复制副本后缀断言)
	createdSchema    []byte               // 记录 CreateSurvey 实际写入的 schema jsonb(复制内容断言)
	setAccess        string               // 记录 SetAnswerAccess 实际写入的作答模式
	setAccessCalled  bool                 // 记录 SetAnswerAccess 是否被调用(守卫拦下时不应写)
	setDisplay       string               // 记录 SetDisplayMode 实际写入的展示模式
	setDisplayCalled bool                 // 记录 SetDisplayMode 是否被调用(守卫拦下时不应写)
	listOwner        string               // 记录 ListSurveysByOwner 收到的 ownerID
	listAllCalled    bool                 // 记录是否走了全站列表(admin)
	listKeyword      *string              // 记录两条列表分支收到的 keyword(nil=不过滤)
	listParams       dao.SurveyListParams // 记录列表分支收到的完整参数(limit/offset/status/type 断言用)
	listTotal        int64                // fake 返回的总行数(COUNT(*) OVER())
	ownerItems       []dao.SurveyListItem
	allItems         []dao.SurveyListItem
}

func (f *fakeStore) GetSurvey(_ context.Context, _ string) (dao.SurveyMeta, error) {
	return f.meta, f.getErr
}
func (f *fakeStore) ListSurveysByOwner(_ context.Context, ownerID string, p dao.SurveyListParams) ([]dao.SurveyListItem, int64, error) {
	f.listOwner = ownerID
	f.listKeyword = p.Keyword
	f.listParams = p
	return f.ownerItems, f.listTotal, nil
}
func (f *fakeStore) ListAllSurveys(_ context.Context, p dao.SurveyListParams) ([]dao.SurveyListItem, int64, error) {
	f.listAllCalled = true
	f.listKeyword = p.Keyword
	f.listParams = p
	return f.allItems, f.listTotal, nil
}
func (f *fakeStore) CreateSurvey(_ context.Context, _, ownerID, _, title string, draftSchema []byte, answerAccess, displayMode string) error {
	f.createdOwner = ownerID
	f.createdTitle = title
	f.createdSchema = draftSchema
	f.createdAccess = answerAccess
	f.createdDisplay = displayMode
	return nil
}
func (f *fakeStore) UpdateDraft(_ context.Context, _, _, _ string, _ []byte) error { return nil }
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
func (f *fakeStore) SetDisplayMode(_ context.Context, _, mode string) error {
	f.setDisplayCalled = true
	f.setDisplay = mode
	return nil
}
func (f *fakeStore) CountResponses(_ context.Context, _ string) (int32, error) { return 0, nil }

// ctxUser 造一个带指定用户身份的 ctx(默认 creator 角色:既有归属/状态机用例都是「创作者管自己的卷」场景)。
func ctxUser(uid string) context.Context {
	return ctxRole(uid, "creator")
}

// ctxRole 造带指定用户 id + 角色的 ctx(RBAC 判定用)。
func ctxRole(uid, role string) context.Context {
	return metadata.With(context.Background(), metadata.Metadata{UserID: uid, Role: role})
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
	f := &fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "live"}}
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
	f := &fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "draft"}}
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
	f := &fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "live"}}
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
	f := &fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "closed", PublishedVersion: nil}}
	m := New(f)
	_, err := m.Reopen(ctxUser("alice"), api.SurveyReopenReq{ID: "s1"})
	if got := codeOf(t, err); got != ecode.CodeConflict {
		t.Fatalf("未发布 Reopen code = %d, want CodeConflict", got)
	}
}

// 重开守卫:closed 且曾发布 → 写 live。
func TestReopen_Published_SetsLive(t *testing.T) {
	v := int32(2)
	f := &fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "closed", PublishedVersion: &v}}
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
	f := &updateFake{fakeStore: fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "draft"}}}
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
	f := &updateFake{fakeStore: fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "live"}}}
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
	f := &updateFake{fakeStore: fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "closed"}}}
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
	// 新建默认作答模式由代码显式写入 login_required(不依赖列 DEFAULT)。
	if f.createdAccess != "login_required" {
		t.Fatalf("Create 应写入 login_required 默认,得到 %q", f.createdAccess)
	}
	// 新建默认展示模式由代码显式写入 single(不依赖列 DEFAULT)。
	if f.createdDisplay != "single" {
		t.Fatalf("Create 应写入 single 默认,得到 %q", f.createdDisplay)
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

// Copy:发布态问卷复制成 draft —— 新 id、标题加副本后缀、沿用源作答/展示配置、归操作者。
func TestCopy_LiveSurvey_DerivesDraft(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{
		SurveyID: "s1", OwnerID: "alice", Type: "survey", Status: "live",
		DraftSchema:  []byte(`{"id":"s1","type":"survey","title":"客户满意度","version":3,"questions":[{"id":"q1"}],"rules":[]}`),
		AnswerAccess: "anonymous", DisplayMode: "paged",
	}}
	m := New(f)
	resp, err := m.Copy(ctxUser("alice"), api.SurveyCopyReq{ID: "s1"})
	if err != nil {
		t.Fatalf("Copy 应成功,得到 %v", err)
	}
	if resp.ID == "" || resp.ID == "s1" {
		t.Fatalf("Copy 应分配全新非空 id,得到 %q", resp.ID)
	}
	if f.createdOwner != "alice" {
		t.Fatalf("副本 owner 应为操作者 alice,得到 %q", f.createdOwner)
	}
	if f.createdTitle != "客户满意度（副本）" {
		t.Fatalf("副本标题应加「(副本)」后缀,得到 %q", f.createdTitle)
	}
	// 沿用源作答/展示配置,而非回落新建默认。
	if f.createdAccess != "anonymous" || f.createdDisplay != "paged" {
		t.Fatalf("副本应沿用源配置 anonymous/paged,得到 %q/%q", f.createdAccess, f.createdDisplay)
	}
	// 落库 schema:id 换新、version 归 1、题目结构保留。
	var got domain.SurveySchema
	if err := json.Unmarshal(f.createdSchema, &got); err != nil {
		t.Fatalf("副本 schema 应为合法 JSON,得到 %v", err)
	}
	if got.ID != resp.ID {
		t.Fatalf("副本 schema.id 应为新 id %q,得到 %q", resp.ID, got.ID)
	}
	if got.Version != 1 {
		t.Fatalf("副本 version 应归 1,得到 %d", got.Version)
	}
	if len(got.Questions) != 1 {
		t.Fatalf("副本应保留源题目结构(1 题),得到 %d 题", len(got.Questions))
	}
}

// Copy:非本人复制 → 404(第二层归属,不泄露存在性)。
func TestCopy_CrossUser_ReturnsNotFound(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "live", DraftSchema: []byte(`{"id":"s1"}`)}}
	m := New(f)
	_, err := m.Copy(ctxRole("bob", "creator"), api.SurveyCopyReq{ID: "s1"})
	if got := codeOf(t, err); got != ecode.CodeNotFound {
		t.Fatalf("非本人 Copy code = %d, want CodeNotFound", got)
	}
}

// Copy:admin 复制他人问卷 → 放行,但副本归 admin 本人(与 Create 一致,不做跨 owner 归属)。
func TestCopy_Admin_CrossOwner_OwnedByOperator(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{
		SurveyID: "s1", OwnerID: "alice", Type: "survey", Status: "closed",
		DraftSchema: []byte(`{"id":"s1","title":"旧卷","version":2}`), AnswerAccess: "login_required", DisplayMode: "single",
	}}
	m := New(f)
	if _, err := m.Copy(ctxRole("admin-user", "admin"), api.SurveyCopyReq{ID: "s1"}); err != nil {
		t.Fatalf("admin 跨 owner 复制应放行,得到 %v", err)
	}
	if f.createdOwner != "admin-user" {
		t.Fatalf("admin 复制的副本应归操作者 admin-user,得到 %q", f.createdOwner)
	}
}

// --- RBAC 第二层归属(admin 短路)---
// 注:第一层能力位(respondent 调创作端 → 403)已上移到 RequireAuth 中间件,
// 其回归测试在 internal/server/http 端点级(能力位真正生效的位置);此处只测归属。

// creator A 访问 creator B 的卷 → 404(第二层归属,防枚举;能力位已过)。
func TestGet_CreatorCrossUser_ReturnsNotFound(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "draft"}}
	m := New(f)
	_, err := m.Get(ctxRole("bob", "creator"), api.SurveyGetReq{ID: "s1"})
	if got := codeOf(t, err); got != ecode.CodeNotFound {
		t.Fatalf("creator 跨用户 code = %d, want CodeNotFound(404)", got)
	}
}

// admin 短路归属:读他人的卷 → 放行(跨 owner)。
func TestGet_Admin_CrossOwner_Allowed(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "draft", DraftSchema: []byte(`{"id":"s1"}`)}}
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
	f := &updateFake{fakeStore: fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "draft"}}}
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
	fa := &fakeStore{allItems: []dao.SurveyListItem{{SurveyID: "x"}}}
	if _, err := New(fa).List(ctxRole("admin-user", "admin"), api.SurveyListReq{}); err != nil {
		t.Fatalf("admin List 应成功,得到 %v", err)
	}
	if !fa.listAllCalled {
		t.Fatal("admin List 应走全站 ListAllSurveys")
	}

	fc := &fakeStore{ownerItems: []dao.SurveyListItem{{SurveyID: "y"}}}
	if _, err := New(fc).List(ctxRole("alice", "creator"), api.SurveyListReq{}); err != nil {
		t.Fatalf("creator List 应成功,得到 %v", err)
	}
	if fc.listAllCalled || fc.listOwner != "alice" {
		t.Fatalf("creator List 应按本人过滤(listOwner=%q, listAll=%v)", fc.listOwner, fc.listAllCalled)
	}
}

// List 搜索:空 Q 不过滤(keyword=nil);非空 Q 去空格后透传两条分支;纯空格视同空。
func TestList_KeywordPassthrough(t *testing.T) {
	// 空 Q → keyword 应为 nil(不过滤),admin/creator 两条分支都验。
	for _, role := range []string{"admin", "creator"} {
		f := &fakeStore{}
		if _, err := New(f).List(ctxRole("u", role), api.SurveyListReq{Q: "  "}); err != nil {
			t.Fatalf("%s List 空 Q 应成功,得到 %v", role, err)
		}
		if f.listKeyword != nil {
			t.Fatalf("%s List 空/空格 Q 应 keyword=nil(不过滤),得到 %q", role, *f.listKeyword)
		}
	}

	// 非空 Q → 去首尾空格后透传。creator 分支。
	fc := &fakeStore{}
	if _, err := New(fc).List(ctxRole("alice", "creator"), api.SurveyListReq{Q: "  满意度  "}); err != nil {
		t.Fatalf("creator List 应成功,得到 %v", err)
	}
	if fc.listKeyword == nil || *fc.listKeyword != "满意度" {
		t.Fatalf("creator List keyword 应为去空格后的 %q,得到 %v", "满意度", fc.listKeyword)
	}

	// 非空 Q → admin 全站分支也应收到 keyword。
	fa := &fakeStore{}
	if _, err := New(fa).List(ctxRole("admin-user", "admin"), api.SurveyListReq{Q: "问卷"}); err != nil {
		t.Fatalf("admin List 应成功,得到 %v", err)
	}
	if !fa.listAllCalled || fa.listKeyword == nil || *fa.listKeyword != "问卷" {
		t.Fatalf("admin List 应走全站且 keyword=%q,得到 listAll=%v keyword=%v", "问卷", fa.listAllCalled, fa.listKeyword)
	}
}

// makeItems 造 n 条列表项(id 唯一),供分页断言。
func makeItems(n int) []dao.SurveyListItem {
	out := make([]dao.SurveyListItem, n)
	for i := 0; i < n; i++ {
		out[i] = dao.SurveyListItem{SurveyID: string(rune('a' + i)), Title: "t", Type: "survey", Status: "draft"}
	}
	return out
}

// List 默认页大小 10:store 收到 limit=10 offset=0;status/type 透传。
func TestList_DefaultPageSizeAndFilters(t *testing.T) {
	fc := &fakeStore{ownerItems: makeItems(3), listTotal: 3}
	_, err := New(fc).List(ctxRole("alice", "creator"), api.SurveyListReq{Status: "live", Type: "survey"})
	if err != nil {
		t.Fatalf("List 应成功,得到 %v", err)
	}
	if fc.listParams.Limit != defaultPageSize || fc.listParams.Offset != 0 {
		t.Fatalf("默认应 limit=%d offset=0,得到 limit=%d offset=%d", defaultPageSize, fc.listParams.Limit, fc.listParams.Offset)
	}
	if fc.listParams.Status == nil || *fc.listParams.Status != "live" {
		t.Fatalf("status 应透传 live,得到 %v", fc.listParams.Status)
	}
	if fc.listParams.Type == nil || *fc.listParams.Type != "survey" {
		t.Fatalf("type 应透传 survey,得到 %v", fc.listParams.Type)
	}
}

// List limit clamp:超上限落 maxPageSize;非正数落默认。
func TestList_LimitClamp(t *testing.T) {
	fc := &fakeStore{}
	_, _ = New(fc).List(ctxRole("alice", "creator"), api.SurveyListReq{Limit: 99999})
	if fc.listParams.Limit != maxPageSize {
		t.Fatalf("超上限 limit 应 clamp 到 %d,得到 %d", maxPageSize, fc.listParams.Limit)
	}

	fc2 := &fakeStore{}
	_, _ = New(fc2).List(ctxRole("alice", "creator"), api.SurveyListReq{Limit: -5})
	if fc2.listParams.Limit != defaultPageSize {
		t.Fatalf("非正 limit 应落默认 %d,得到 %d", defaultPageSize, fc2.listParams.Limit)
	}
}

// List page→offset:offset=(page-1)*limit;page<1 落 1(offset=0)。
func TestList_PageOffset(t *testing.T) {
	fc := &fakeStore{}
	_, _ = New(fc).List(ctxRole("alice", "creator"), api.SurveyListReq{Limit: 20, Page: 3})
	if fc.listParams.Offset != 40 || fc.listParams.Limit != 20 {
		t.Fatalf("page3 size20 应 offset=40 limit=20,得到 offset=%d limit=%d", fc.listParams.Offset, fc.listParams.Limit)
	}

	fc2 := &fakeStore{}
	_, _ = New(fc2).List(ctxRole("alice", "creator"), api.SurveyListReq{Limit: 20, Page: 0})
	if fc2.listParams.Offset != 0 {
		t.Fatalf("page<1 应落第一页 offset=0,得到 %d", fc2.listParams.Offset)
	}

	// 超大 page:(page-1)*size 用 int64 算再 clamp 到 int32 上限,绝不回绕成负 offset。
	// page=1e8、size=100 → 乘积 ~1e10 远超 int32,直接 int32 转换会回绕成负数。
	fc3 := &fakeStore{}
	_, _ = New(fc3).List(ctxRole("alice", "creator"), api.SurveyListReq{Limit: 100, Page: 100000000})
	if fc3.listParams.Offset < 0 {
		t.Fatalf("超大 page 的 offset 不得为负(int32 回绕),得到 %d", fc3.listParams.Offset)
	}
	if fc3.listParams.Offset != math.MaxInt32 {
		t.Fatalf("超大 page 的 offset 应 clamp 到 %d,得到 %d", int32(math.MaxInt32), fc3.listParams.Offset)
	}
}

// List keyword 转义:含 LIKE 元字符的搜索词进 store 前应被转义(配合 SQL 侧 ESCAPE '\'),
// 否则用户输入的 % _ 会被当通配符,污染匹配语义。
func TestList_KeywordEscapesLikeMetachars(t *testing.T) {
	cases := []struct{ in, want string }{
		{"50%", `50\%`},
		{"a_b", `a\_b`},
		{`x\y`, `x\\y`},
		{"满意度", "满意度"}, // 无元字符原样透传
	}
	for _, c := range cases {
		fc := &fakeStore{}
		_, _ = New(fc).List(ctxRole("alice", "creator"), api.SurveyListReq{Q: c.in})
		if fc.listKeyword == nil || *fc.listKeyword != c.want {
			t.Fatalf("keyword %q 应转义为 %q,得到 %v", c.in, c.want, fc.listKeyword)
		}
	}
}

// List total 透传:store 返回的总数(独立 COUNT 查询)原样出现在响应,供前端算总页数。
func TestList_TotalPassthrough(t *testing.T) {
	fc := &fakeStore{ownerItems: makeItems(2), listTotal: 57}
	resp, err := New(fc).List(ctxRole("alice", "creator"), api.SurveyListReq{Limit: 2, Page: 1})
	if err != nil {
		t.Fatalf("List 应成功,得到 %v", err)
	}
	if resp.Total != 57 {
		t.Fatalf("Total 应透传 57,得到 %d", resp.Total)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("应返回当前页 2 条,得到 %d", len(resp.Items))
	}
}

// List 搜索态:Q 非空时锁 page1 + limit10 + offset0(搜索不翻页)。
func TestList_SearchModeNoPaging(t *testing.T) {
	fc := &fakeStore{ownerItems: makeItems(searchLimit), listTotal: 99}
	resp, err := New(fc).List(ctxRole("alice", "creator"), api.SurveyListReq{
		Q:     "问卷",
		Limit: 50, // 搜索态应被忽略
		Page:  5,  // 搜索态应被忽略
	})
	if err != nil {
		t.Fatalf("搜索 List 应成功,得到 %v", err)
	}
	if fc.listParams.Limit != searchLimit || fc.listParams.Offset != 0 {
		t.Fatalf("搜索态应锁 limit=%d offset=0,得到 limit=%d offset=%d", searchLimit, fc.listParams.Limit, fc.listParams.Offset)
	}
	if len(resp.Items) != searchLimit {
		t.Fatalf("搜索态应返回前 %d 条,得到 %d", searchLimit, len(resp.Items))
	}
}

// 注:List 的 respondent→403 属第一层能力位,已上移中间件,回归测试见端点级。

// SetAnswerAccess:draft 问卷设 login_required → 写列成功。
func TestSetAnswerAccess_Draft_Succeeds(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "draft"}}
	if _, err := New(f).SetAnswerAccess(ctxUser("alice"), api.SurveySetAnswerAccessReq{ID: "s1", AnswerAccess: "login_required"}); err != nil {
		t.Fatalf("draft 设作答模式应成功,得到 %v", err)
	}
	if f.setAccess != "login_required" {
		t.Fatalf("SetAnswerAccess 应写入 login_required,得到 %q", f.setAccess)
	}
}

// SetAnswerAccess:live 问卷 → Conflict(仅 draft 可改),且不写库。
func TestSetAnswerAccess_Live_ReturnsConflict(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "live"}}
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
	f := &fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "bob", Status: "draft"}}
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
	f := &fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "draft"}}
	_, err := New(f).SetAnswerAccess(ctxUser("alice"), api.SurveySetAnswerAccessReq{ID: "s1", AnswerAccess: "bogus"})
	if got := codeOf(t, err); got != ecode.CodeBadRequest {
		t.Fatalf("非法值 code = %d, want CodeBadRequest", got)
	}
	if f.setAccessCalled {
		t.Fatal("非法值不应写库")
	}
}

// SetDisplayMode:draft 问卷设 paged → 写列成功。
func TestSetDisplayMode_Draft_Succeeds(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "draft"}}
	if _, err := New(f).SetDisplayMode(ctxUser("alice"), api.SurveySetDisplayModeReq{ID: "s1", DisplayMode: "paged"}); err != nil {
		t.Fatalf("draft 设展示模式应成功,得到 %v", err)
	}
	if f.setDisplay != "paged" {
		t.Fatalf("SetDisplayMode 应写入 paged,得到 %q", f.setDisplay)
	}
}

// SetDisplayMode:live 问卷 → Conflict(仅 draft 可改),且不写库。
func TestSetDisplayMode_Live_ReturnsConflict(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "live"}}
	_, err := New(f).SetDisplayMode(ctxUser("alice"), api.SurveySetDisplayModeReq{ID: "s1", DisplayMode: "single"})
	if got := codeOf(t, err); got != ecode.CodeConflict {
		t.Fatalf("live 设展示模式 code = %d, want CodeConflict", got)
	}
	if f.setDisplayCalled {
		t.Fatal("守卫拦下不应写库")
	}
}

// SetDisplayMode:非 owner → NotFound(归属防枚举),不写库。
func TestSetDisplayMode_NotOwner_ReturnsNotFound(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "bob", Status: "draft"}}
	_, err := New(f).SetDisplayMode(ctxUser("alice"), api.SurveySetDisplayModeReq{ID: "s1", DisplayMode: "single"})
	if got := codeOf(t, err); got != ecode.CodeNotFound {
		t.Fatalf("非 owner code = %d, want CodeNotFound", got)
	}
	if f.setDisplayCalled {
		t.Fatal("归属拦下不应写库")
	}
}

// SetDisplayMode:非法值 → BadRequest,不写库。
func TestSetDisplayMode_InvalidValue_ReturnsBadRequest(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "draft"}}
	_, err := New(f).SetDisplayMode(ctxUser("alice"), api.SurveySetDisplayModeReq{ID: "s1", DisplayMode: "bogus"})
	if got := codeOf(t, err); got != ecode.CodeBadRequest {
		t.Fatalf("非法值 code = %d, want CodeBadRequest", got)
	}
	if f.setDisplayCalled {
		t.Fatal("非法值不应写库")
	}
}

// Stats 回显展示模式:空/历史值回落 single(不返回空串误导前端)。
func TestStats_EmptyDisplayMode_FallsBackSingle(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "live", DisplayMode: ""}}
	resp, err := New(f).Stats(ctxUser("alice"), api.SurveyStatsReq{ID: "s1"})
	if err != nil {
		t.Fatalf("Stats 应成功,得到 %v", err)
	}
	if resp.DisplayMode != "single" {
		t.Fatalf("空 displayMode 回落 = %q, want single", resp.DisplayMode)
	}
}

// Publish 不再改 answer_access:发布只冻结版本 + 转 live(作答模式由 SetAnswerAccess 定,单一真相源)。
// fakeStore.Publish 已无 answerAccess 形参,此处仅确认发布成功、不触碰作答模式写入路径。
func TestPublish_DoesNotTouchAnswerAccess(t *testing.T) {
	f := &fakeStore{meta: dao.SurveyMeta{SurveyID: "s1", OwnerID: "alice", Status: "draft"}, publishVer: 1}
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
