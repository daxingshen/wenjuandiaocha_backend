// Package survey 是问卷 studio 业务层:CRUD + 发布 + 生命周期状态机守卫 + 归属校验。
// 从原 http handler 下沉的业务逻辑。传输层(server/http)只做 bind→调本层→render。
package survey

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/google/wire"

	"wenjuandiaocha_backend/api"
	"wenjuandiaocha_backend/internal/dao"
	"wenjuandiaocha_backend/internal/domain"
	"wenjuandiaocha_backend/internal/ecode"
	"wenjuandiaocha_backend/internal/lib/id"
	"wenjuandiaocha_backend/internal/lib/metadata"
	"wenjuandiaocha_backend/internal/rbac"
)

// Store 是本层依赖的 dao 子集(消费方定义接口,便于单测注入 fake)。*dao.Store 实现它。
type Store interface {
	GetSurvey(ctx context.Context, id string) (dao.SurveyMeta, error)
	ListSurveysByOwner(ctx context.Context, ownerID string, p dao.SurveyListParams) ([]dao.SurveyListItem, int64, error)
	ListAllSurveys(ctx context.Context, p dao.SurveyListParams) ([]dao.SurveyListItem, int64, error)
	CreateSurvey(ctx context.Context, id, ownerID, typ, title string, draftSchema []byte, answerAccess, displayMode string) error
	UpdateDraft(ctx context.Context, id, title, typ string, draftSchema []byte) error
	SetStatus(ctx context.Context, id, status string) error
	SetAnswerAccess(ctx context.Context, id, access string) error
	SetDisplayMode(ctx context.Context, id, mode string) error
	Publish(ctx context.Context, surveyID string, draftSchema []byte) (int, bool, error)
	CountResponses(ctx context.Context, id string) (int32, error)
}

// I/O 契约集中在 api 包(一处定义,http/gRPC 两端共用)。本层方法形态
// Method(ctx, api.SurveyXReq) (api.SurveyXResp, error);OwnerID 由传输层填入 Req。

// Service 是问卷 studio 业务契约。*Manager 实现它;传输层持本接口。
// ownerID 来自 ctx 的 metadata.Metadata(传输层注入),不进 Req。
type Service interface {
	List(ctx context.Context, req api.SurveyListReq) (api.SurveyListResp, error)
	Create(ctx context.Context, req api.SurveyCreateReq) (api.SurveyCreateResp, error)
	Copy(ctx context.Context, req api.SurveyCopyReq) (api.SurveyCopyResp, error)
	Get(ctx context.Context, req api.SurveyGetReq) (api.SurveyGetResp, error)
	Update(ctx context.Context, req api.SurveyUpdateReq) (api.SurveyUpdateResp, error)
	SetAnswerAccess(ctx context.Context, req api.SurveySetAnswerAccessReq) (api.SurveySetAnswerAccessResp, error)
	SetDisplayMode(ctx context.Context, req api.SurveySetDisplayModeReq) (api.SurveySetDisplayModeResp, error)
	Publish(ctx context.Context, req api.SurveyPublishReq) (api.SurveyPublishResp, error)
	Close(ctx context.Context, req api.SurveyCloseReq) (api.SurveyCloseResp, error)
	Reopen(ctx context.Context, req api.SurveyReopenReq) (api.SurveyReopenResp, error)
	Stats(ctx context.Context, req api.SurveyStatsReq) (api.SurveyStatsResp, error)
}

// Manager 持有 dao 门面。业务方法返回 ecode.Error,传输层据此映射状态码。
type Manager struct {
	store Store
}

func New(store Store) *Manager { return &Manager{store: store} }

// 编译期确认 *Manager 实现 Service。
var _ Service = (*Manager)(nil)

// ProviderSet 供 wire 组装:提供 *Manager,并绑定到 Service 接口(传输层持接口)。
// *dao.Store→Store 接口的绑定放在 di.wire.Build(与 dao.ProviderSet 同一作用域)。
var ProviderSet = wire.NewSet(New, wire.Bind(new(Service), new(*Manager)))

// roleOf 从 ctx metadata 取当前用户角色(RequireAuth 注入)。
func roleOf(ctx context.Context) rbac.Role {
	return rbac.Role(metadata.From(ctx).Role)
}

// 第一层平台能力位(role 能不能做该类动作)已上移到 HTTP 传输层 RequireAuth(svc, action) 中间件,
// service 不再判能力位;此处只保留第二层归属(owned)。roleOf 仍用于 owned 的 admin 短路。

// owned 取问卷并校验归属(第二层):查无 → NotFound("不存在");非本人 → Forbidden(对外同样 404 不泄露存在性)。
// admin 短路归属:平台超级权限可跨 owner,仅校存在性,不比对 OwnerID。
// ownerID/role 从 ctx metadata 取(传输层注入)。第一层能力位已由 RequireAuth 中间件先行判定。
func (m *Manager) owned(ctx context.Context, id string) (dao.SurveyMeta, error) {
	meta, err := m.store.GetSurvey(ctx, id)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return dao.SurveyMeta{}, ecode.NotFound("不存在")
		}
		return dao.SurveyMeta{}, err
	}
	if rbac.IsAdmin(roleOf(ctx)) {
		return meta, nil // admin 短路归属
	}
	if meta.OwnerID != metadata.From(ctx).UserID {
		return dao.SurveyMeta{}, ecode.Forbidden()
	}
	return meta, nil
}

// 列表分页参数:默认页大小 10,上限 100;搜索态固定前 searchLimit 条。
const (
	defaultPageSize = 10
	maxPageSize     = 100
	searchLimit     = 10
)

// List 问卷列表:admin 全站,creator 仅本人,respondent 无权(第一层能力位挡下)。
// offset 分页(ORDER BY created_at DESC, id DESC);keyword/status/type 透传两条分支。
// 搜索态(Q 非空):锁 page=1、limit=10、只返回前 10 条(搜索不翻页,前端隐藏页码器)。
// 返回 Total 为筛选后总行数(独立 COUNT 查询),供前端算总页数。ownerID/role 从 ctx metadata 取。
func (m *Manager) List(ctx context.Context, req api.SurveyListReq) (api.SurveyListResp, error) {
	keyword := trimToPtr(req.Q)
	searching := keyword != nil
	if keyword != nil {
		// 转义 LIKE 元字符:SQL 侧走 ILIKE ... ESCAPE '\',否则用户输入的 % _ 会被当通配符,
		// 搜 "50%" 变成前缀匹配、"a_b" 匹配 "axb"。先转义 \ 自身,再转义 % 和 _。
		esc := escapeLike(*keyword)
		keyword = &esc
	}

	p := dao.SurveyListParams{
		Keyword: keyword,
		Status:  trimToPtr(req.Status),
		Type:    trimToPtr(req.Type),
	}
	if searching {
		// 搜索态:锁前 10 条第一页,不翻页。
		p.Limit = searchLimit
		p.Offset = 0
	} else {
		size := req.Limit
		if size <= 0 {
			size = defaultPageSize
		}
		if size > maxPageSize {
			size = maxPageSize
		}
		page := req.Page
		if page < 1 {
			page = 1
		}
		// int64 计算 + clamp 到 int32 上限:page 用户可控且只做了下限钳制,
		// 直接 int32((page-1)*size) 会在大 page 下回绕成负 OFFSET(Postgres 报错 / 返回错误页)。
		offset := int64(page-1) * int64(size)
		if offset > math.MaxInt32 {
			offset = math.MaxInt32
		}
		p.Limit = int32(size)
		p.Offset = int32(offset)
	}

	var rows []dao.SurveyListItem
	var total int64
	var err error
	if rbac.IsAdmin(roleOf(ctx)) {
		rows, total, err = m.store.ListAllSurveys(ctx, p) // admin 全站视角
	} else {
		rows, total, err = m.store.ListSurveysByOwner(ctx, metadata.From(ctx).UserID, p) // creator 仅本人
	}
	if err != nil {
		return api.SurveyListResp{}, err
	}

	items := make([]api.SurveyListItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, api.SurveyListItem{
			ID: r.SurveyID, Title: r.Title, Type: r.Type, Status: r.Status,
			UpdatedAt: r.UpdatedAt.Format(time.RFC3339), // 对外 RFC3339 字符串(前端契约)
		})
	}
	return api.SurveyListResp{Items: items, Total: int(total)}, nil
}

// trimToPtr 去首尾空格,空串返回 nil(用于可选过滤参数:nil=不过滤)。
func trimToPtr(s string) *string {
	t := strings.TrimSpace(s)
	if t == "" {
		return nil
	}
	return &t
}

// escapeLike 转义 SQL LIKE/ILIKE 元字符(与查询侧 ESCAPE '\' 配套)。
// 必须先转义反斜杠自身,否则会把随后为 % _ 补的转义反斜杠再转义一遍。
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// Create 首存落库:后端强制分配 id(忽略客户端传的 id,防越权/串号)、补 schema 默认值。
// Body 是前端内存草稿的整份 SurveySchema;空 Body 回落最小 schema。返回后端分配的 id。
func (m *Manager) Create(ctx context.Context, req api.SurveyCreateReq) (api.SurveyCreateResp, error) {
	ownerID := metadata.From(ctx).UserID
	newid := id.New()
	var schema domain.SurveySchema
	if len(req.Body) > 0 {
		if err := json.Unmarshal(req.Body, &schema); err != nil {
			return api.SurveyCreateResp{}, ecode.BadRequest("schema 格式错误")
		}
	}
	// 补默认 + 强制后端分配的 id。
	schema.ID = newid
	if schema.Type == "" {
		schema.Type = domain.SurveySurvey
	}
	if schema.Title == "" {
		schema.Title = "未命名问卷"
	}
	if schema.Version == 0 {
		schema.Version = 1
	}
	if schema.Questions == nil {
		schema.Questions = []domain.Question{}
	}
	if schema.Rules == nil {
		schema.Rules = []domain.LogicRule{}
	}
	schemaJSON, _ := json.Marshal(schema)
	// 新建默认作答模式/展示模式由代码显式指定(不依赖列 DEFAULT),消除「已建库未 ALTER」漂移。
	if err := m.store.CreateSurvey(ctx, newid, ownerID, string(schema.Type), schema.Title, schemaJSON, domain.AnswerLoginRequired, domain.DisplaySingle); err != nil {
		return api.SurveyCreateResp{}, err
	}
	return api.SurveyCreateResp{ID: newid}, nil
}

// Copy 复制问卷:把源问卷的草稿结构整份派生为一个新 draft 问卷,让「已发布不可编辑」的问卷
// 能以副本形式继续演化。归属校验(owned:非 owner 404,admin 短路)后,读源 draft_schema →
// 分配新 id、标题加「(副本)」、version 归 1(新草稿无发布史)→ 沿用源作答/展示配置 → 落库 draft。
// 新问卷 owner 恒为当前操作者(admin 复制他人问卷产物也归 admin 本人,与 Create 一致);
// 后端强制分配 id,不接受任何客户端传入 id(源 id 只用于定位,不进新问卷)。
func (m *Manager) Copy(ctx context.Context, req api.SurveyCopyReq) (api.SurveyCopyResp, error) {
	meta, err := m.owned(ctx, req.ID)
	if err != nil {
		return api.SurveyCopyResp{}, err
	}
	// 源 draft_schema 落库时已 marshal,恒为合法 JSON;解析失败仅在数据损坏时发生,按内部错误透传。
	var schema domain.SurveySchema
	if err := json.Unmarshal(meta.DraftSchema, &schema); err != nil {
		return api.SurveyCopyResp{}, ecode.Internal("源问卷数据损坏")
	}
	newid := id.New()
	schema.ID = newid
	schema.Title = schema.Title + "（副本）"
	schema.Version = 1 // 新草稿从头计版,与源的发布历史无关
	if schema.Questions == nil {
		schema.Questions = []domain.Question{}
	}
	if schema.Rules == nil {
		schema.Rules = []domain.LogicRule{}
	}
	// 沿用源作答配置(空值回落与 Stats 一致的默认),而非一律回落新建默认。
	access := meta.AnswerAccess
	if access == "" {
		access = domain.AnswerAnonymous
	}
	display := meta.DisplayMode
	if display == "" {
		display = domain.DisplaySingle
	}
	schemaJSON, _ := json.Marshal(schema)
	ownerID := metadata.From(ctx).UserID
	if err := m.store.CreateSurvey(ctx, newid, ownerID, string(schema.Type), schema.Title, schemaJSON, access, display); err != nil {
		return api.SurveyCopyResp{}, err
	}
	return api.SurveyCopyResp{ID: newid}, nil
}

// Get 返回草稿 SurveySchema 原始 jsonb(供编辑)。归属校验。
func (m *Manager) Get(ctx context.Context, req api.SurveyGetReq) (api.SurveyGetResp, error) {
	meta, err := m.owned(ctx, req.ID)
	if err != nil {
		return api.SurveyGetResp{}, err
	}
	return api.SurveyGetResp{Schema: meta.DraftSchema}, nil
}

// Update 存草稿:Body 为整份 SurveySchema 原始 bytes(保留前端原样落库),title/type 从解析出的 schema 取。
func (m *Manager) Update(ctx context.Context, req api.SurveyUpdateReq) (api.SurveyUpdateResp, error) {
	meta, err := m.owned(ctx, req.ID)
	if err != nil {
		return api.SurveyUpdateResp{}, err
	}
	// 状态守卫:只有草稿可编辑。已发布(live/closed)问卷内容已冻结,拒绝写库
	// —— 前端弹窗拦截只是体验层,此处才是防绕过接口直改的真正防线。复用 owned() 已取的 meta.Status,零额外查询。
	if meta.Status != domain.StatusDraft {
		return api.SurveyUpdateResp{}, ecode.Conflict(msgEditForbidden)
	}
	var schema domain.SurveySchema
	if err := json.Unmarshal(req.Body, &schema); err != nil {
		return api.SurveyUpdateResp{}, ecode.BadRequest("schema 格式错误")
	}
	if err := m.store.UpdateDraft(ctx, meta.SurveyID, schema.Title, string(schema.Type), req.Body); err != nil {
		return api.SurveyUpdateResp{}, err
	}
	return api.SurveyUpdateResp{}, nil
}

// Publish 冻结草稿为新版本快照 + status=live。Unchanged=true 表示草稿与当前对外版一致(重发免空版)。
// 作答访问模式不在此设定:它是 draft 阶段经 SetAnswerAccess 定的独立列,发布不碰(单一真相源)。
func (m *Manager) Publish(ctx context.Context, req api.SurveyPublishReq) (api.SurveyPublishResp, error) {
	meta, err := m.owned(ctx, req.ID)
	if err != nil {
		return api.SurveyPublishResp{}, err
	}
	version, unchanged, err := m.store.Publish(ctx, meta.SurveyID, meta.DraftSchema)
	if err != nil {
		return api.SurveyPublishResp{}, err
	}
	return api.SurveyPublishResp{Version: version, Unchanged: unchanged}, nil
}

// SetAnswerAccess 设作答访问模式(anonymous|login_required)。任意状态(draft/live/closed)均可改:
// 作答配置是发布后仍可调的运营开关,不随内容一同冻结(与 Update「仅草稿可编辑」不同,不设状态守卫)。
// 复用 owned() 归属校验(非 owner → 404 防枚举);值域白名单(非法 → BadRequest,不只靠列 CHECK)。
func (m *Manager) SetAnswerAccess(ctx context.Context, req api.SurveySetAnswerAccessReq) (api.SurveySetAnswerAccessResp, error) {
	if req.AnswerAccess != domain.AnswerAnonymous && req.AnswerAccess != domain.AnswerLoginRequired {
		return api.SurveySetAnswerAccessResp{}, ecode.BadRequest("作答访问模式非法")
	}
	meta, err := m.owned(ctx, req.ID)
	if err != nil {
		return api.SurveySetAnswerAccessResp{}, err
	}
	if err := m.store.SetAnswerAccess(ctx, meta.SurveyID, req.AnswerAccess); err != nil {
		return api.SurveySetAnswerAccessResp{}, err
	}
	return api.SurveySetAnswerAccessResp{}, nil
}

// SetDisplayMode 设作答页展示模式(paged|single)。任意状态(draft/live/closed)均可改:
// 展示配置是发布后仍可调的运营开关,不随内容一同冻结(与 Update「仅草稿可编辑」不同,不设状态守卫)。
// 复用 owned() 归属校验(非 owner → 404 防枚举);值域白名单(非法 → BadRequest,不只靠列 CHECK)。
func (m *Manager) SetDisplayMode(ctx context.Context, req api.SurveySetDisplayModeReq) (api.SurveySetDisplayModeResp, error) {
	if req.DisplayMode != domain.DisplayPaged && req.DisplayMode != domain.DisplaySingle {
		return api.SurveySetDisplayModeResp{}, ecode.BadRequest("作答页展示模式非法")
	}
	meta, err := m.owned(ctx, req.ID)
	if err != nil {
		return api.SurveySetDisplayModeResp{}, err
	}
	if err := m.store.SetDisplayMode(ctx, meta.SurveyID, req.DisplayMode); err != nil {
		return api.SurveySetDisplayModeResp{}, err
	}
	return api.SurveySetDisplayModeResp{}, nil
}

// Close 结束回收(live → closed)。状态机守卫:仅 live 可结束。
func (m *Manager) Close(ctx context.Context, req api.SurveyCloseReq) (api.SurveyCloseResp, error) {
	meta, err := m.owned(ctx, req.ID)
	if err != nil {
		return api.SurveyCloseResp{}, err
	}
	if meta.Status != domain.StatusLive {
		return api.SurveyCloseResp{}, ecode.Conflict(msgCloseNotLive)
	}
	if err := m.store.SetStatus(ctx, meta.SurveyID, domain.StatusClosed); err != nil {
		return api.SurveyCloseResp{}, err
	}
	return api.SurveyCloseResp{}, nil
}

// Reopen 重新打开(closed → live)。守卫:仅 closed 且曾发布过可重开(复用现有快照)。
func (m *Manager) Reopen(ctx context.Context, req api.SurveyReopenReq) (api.SurveyReopenResp, error) {
	meta, err := m.owned(ctx, req.ID)
	if err != nil {
		return api.SurveyReopenResp{}, err
	}
	if meta.Status != domain.StatusClosed || meta.PublishedVersion == nil {
		return api.SurveyReopenResp{}, ecode.Conflict(msgReopenInvalid)
	}
	if err := m.store.SetStatus(ctx, meta.SurveyID, domain.StatusLive); err != nil {
		return api.SurveyReopenResp{}, err
	}
	return api.SurveyReopenResp{}, nil
}

// Stats 问卷概览:状态 + 已发布版本 + 答卷数。归属校验。
func (m *Manager) Stats(ctx context.Context, req api.SurveyStatsReq) (api.SurveyStatsResp, error) {
	meta, err := m.owned(ctx, req.ID)
	if err != nil {
		return api.SurveyStatsResp{}, err
	}
	count, err := m.store.CountResponses(ctx, meta.SurveyID)
	if err != nil {
		return api.SurveyStatsResp{}, err
	}
	access := meta.AnswerAccess
	if access == "" {
		access = domain.AnswerAnonymous
	}
	display := meta.DisplayMode
	if display == "" {
		display = domain.DisplaySingle
	}
	return api.SurveyStatsResp{Status: meta.Status, PublishedVersion: meta.PublishedVersion, ResponseCount: count, AnswerAccess: access, DisplayMode: display}, nil
}
