// Package survey 是问卷 studio 业务层:CRUD + 发布 + 生命周期状态机守卫 + 归属校验。
// 从原 http handler 下沉的业务逻辑。传输层(server/http)只做 bind→调本层→render。
package survey

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/wire"

	"wenjuandiaocha_backend/api"
	"wenjuandiaocha_backend/internal/dao"
	"wenjuandiaocha_backend/internal/domain"
	"wenjuandiaocha_backend/internal/ecode"
	"wenjuandiaocha_backend/internal/lib/id"
	"wenjuandiaocha_backend/internal/rbac"
)

// Store 是本层依赖的 dao 子集(消费方定义接口,便于单测注入 fake)。*dao.Store 实现它。
type Store interface {
	GetSurvey(ctx context.Context, id string) (dao.SurveyMeta, error)
	ListSurveysByOwner(ctx context.Context, ownerID string) ([]dao.SurveyListItem, error)
	ListAllSurveys(ctx context.Context) ([]dao.SurveyListItem, error)
	CreateSurvey(ctx context.Context, id, ownerID, typ, title string, draftSchema []byte, answerAccess string) error
	UpdateDraft(ctx context.Context, id, title, typ string, draftSchema []byte) error
	SetStatus(ctx context.Context, id, status string) error
	SetAnswerAccess(ctx context.Context, id, access string) error
	Publish(ctx context.Context, surveyID string, draftSchema []byte) (int, bool, error)
	CountResponses(ctx context.Context, id string) (int32, error)
}

// I/O 契约集中在 api 包(一处定义,http/gRPC 两端共用)。本层方法形态
// Method(ctx, api.SurveyXReq) (api.SurveyXResp, error);OwnerID 由传输层填入 Req。

// Service 是问卷 studio 业务契约。*Manager 实现它;传输层持本接口。
// ownerID 来自 ctx 的 api.Metadata(传输层注入),不进 Req。
type Service interface {
	List(ctx context.Context) (api.SurveyListResp, error)
	Create(ctx context.Context, req api.SurveyCreateReq) (api.SurveyCreateResp, error)
	Get(ctx context.Context, req api.SurveyGetReq) (api.SurveyGetResp, error)
	Update(ctx context.Context, req api.SurveyUpdateReq) (api.SurveyUpdateResp, error)
	SetAnswerAccess(ctx context.Context, req api.SurveySetAnswerAccessReq) (api.SurveySetAnswerAccessResp, error)
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
	return rbac.Role(api.MetadataFrom(ctx).Role)
}

// authorize 是第一层平台能力位:role 不能做该类动作 → 真 403(平台能力级越权)。
// 与第二层归属(owned)分工:此处挡下「角色本就无权做这类动作」(如 respondent 调创作端),
// 归属校验挡下「能做这类动作但不是这个资源的 owner」(仍返 404 防枚举)。
func authorize(ctx context.Context, action rbac.Action) error {
	if !rbac.Can(roleOf(ctx), action) {
		return ecode.Forbidden403("无权执行此操作")
	}
	return nil
}

// owned 取问卷并校验归属(第二层):查无 → NotFound("不存在");非本人 → Forbidden(对外同样 404 不泄露存在性)。
// admin 短路归属:平台超级权限可跨 owner,仅校存在性,不比对 OwnerID。
// ownerID/role 从 ctx metadata 取(传输层注入)。调用方须先过第一层 authorize()。
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
	if meta.OwnerID != api.MetadataFrom(ctx).UserID {
		return dao.SurveyMeta{}, ecode.Forbidden()
	}
	return meta, nil
}

// List 问卷列表:admin 全站,creator 仅本人,respondent 无权(第一层能力位挡下)。
// ownerID/role 从 ctx metadata 取。
func (m *Manager) List(ctx context.Context) (api.SurveyListResp, error) {
	if err := authorize(ctx, rbac.ActionSurveyList); err != nil {
		return api.SurveyListResp{}, err
	}
	var rows []dao.SurveyListItem
	var err error
	if rbac.IsAdmin(roleOf(ctx)) {
		rows, err = m.store.ListAllSurveys(ctx) // admin 全站视角
	} else {
		rows, err = m.store.ListSurveysByOwner(ctx, api.MetadataFrom(ctx).UserID) // creator 仅本人
	}
	if err != nil {
		return api.SurveyListResp{}, err
	}
	items := make([]api.SurveyListItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, api.SurveyListItem{
			ID: r.ID, Title: r.Title, Type: r.Type, Status: r.Status,
			UpdatedAt: r.UpdatedAt.Format(time.RFC3339), // 对外 RFC3339 字符串(前端契约),原在 http handler 格式化,收敛后移入此处
		})
	}
	return api.SurveyListResp{Items: items}, nil
}

// Create 首存落库:后端强制分配 id(忽略客户端传的 id,防越权/串号)、补 schema 默认值。
// Body 是前端内存草稿的整份 SurveySchema;空 Body 回落最小 schema。返回后端分配的 id。
func (m *Manager) Create(ctx context.Context, req api.SurveyCreateReq) (api.SurveyCreateResp, error) {
	if err := authorize(ctx, rbac.ActionSurveyCreate); err != nil {
		return api.SurveyCreateResp{}, err
	}
	ownerID := api.MetadataFrom(ctx).UserID
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
	// 新建默认作答模式由代码显式指定(不依赖列 DEFAULT),消除「已建库未 ALTER」漂移。
	if err := m.store.CreateSurvey(ctx, newid, ownerID, string(schema.Type), schema.Title, schemaJSON, domain.AnswerLoginRequired); err != nil {
		return api.SurveyCreateResp{}, err
	}
	return api.SurveyCreateResp{ID: newid}, nil
}

// Get 返回草稿 SurveySchema 原始 jsonb(供编辑)。归属校验。
func (m *Manager) Get(ctx context.Context, req api.SurveyGetReq) (api.SurveyGetResp, error) {
	if err := authorize(ctx, rbac.ActionSurveyRead); err != nil {
		return api.SurveyGetResp{}, err
	}
	meta, err := m.owned(ctx, req.ID)
	if err != nil {
		return api.SurveyGetResp{}, err
	}
	return api.SurveyGetResp{Schema: meta.DraftSchema}, nil
}

// Update 存草稿:Body 为整份 SurveySchema 原始 bytes(保留前端原样落库),title/type 从解析出的 schema 取。
func (m *Manager) Update(ctx context.Context, req api.SurveyUpdateReq) (api.SurveyUpdateResp, error) {
	if err := authorize(ctx, rbac.ActionSurveyUpdate); err != nil {
		return api.SurveyUpdateResp{}, err
	}
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
	if err := m.store.UpdateDraft(ctx, meta.ID, schema.Title, string(schema.Type), req.Body); err != nil {
		return api.SurveyUpdateResp{}, err
	}
	return api.SurveyUpdateResp{}, nil
}

// Publish 冻结草稿为新版本快照 + status=live。Unchanged=true 表示草稿与当前对外版一致(重发免空版)。
// 作答访问模式不在此设定:它是 draft 阶段经 SetAnswerAccess 定的独立列,发布不碰(单一真相源)。
func (m *Manager) Publish(ctx context.Context, req api.SurveyPublishReq) (api.SurveyPublishResp, error) {
	if err := authorize(ctx, rbac.ActionSurveyPublish); err != nil {
		return api.SurveyPublishResp{}, err
	}
	meta, err := m.owned(ctx, req.ID)
	if err != nil {
		return api.SurveyPublishResp{}, err
	}
	version, unchanged, err := m.store.Publish(ctx, meta.ID, meta.DraftSchema)
	if err != nil {
		return api.SurveyPublishResp{}, err
	}
	return api.SurveyPublishResp{Version: version, Unchanged: unchanged}, nil
}

// SetAnswerAccess 设作答访问模式(anonymous|login_required)。仅 draft 可改:
// 已发布(live/closed)问卷作答模式锁定(与 Update「仅草稿可编辑」同一约束,防绕接口直改)。
// 复用 owned() 归属校验(非 owner → 404 防枚举);值域白名单(非法 → BadRequest,不只靠列 CHECK)。
func (m *Manager) SetAnswerAccess(ctx context.Context, req api.SurveySetAnswerAccessReq) (api.SurveySetAnswerAccessResp, error) {
	if err := authorize(ctx, rbac.ActionSurveyUpdate); err != nil {
		return api.SurveySetAnswerAccessResp{}, err
	}
	if req.AnswerAccess != domain.AnswerAnonymous && req.AnswerAccess != domain.AnswerLoginRequired {
		return api.SurveySetAnswerAccessResp{}, ecode.BadRequest("作答访问模式非法")
	}
	meta, err := m.owned(ctx, req.ID)
	if err != nil {
		return api.SurveySetAnswerAccessResp{}, err
	}
	if meta.Status != domain.StatusDraft {
		return api.SurveySetAnswerAccessResp{}, ecode.Conflict(msgEditForbidden)
	}
	if err := m.store.SetAnswerAccess(ctx, meta.ID, req.AnswerAccess); err != nil {
		return api.SurveySetAnswerAccessResp{}, err
	}
	return api.SurveySetAnswerAccessResp{}, nil
}

// Close 结束回收(live → closed)。状态机守卫:仅 live 可结束。
func (m *Manager) Close(ctx context.Context, req api.SurveyCloseReq) (api.SurveyCloseResp, error) {
	if err := authorize(ctx, rbac.ActionSurveyClose); err != nil {
		return api.SurveyCloseResp{}, err
	}
	meta, err := m.owned(ctx, req.ID)
	if err != nil {
		return api.SurveyCloseResp{}, err
	}
	if meta.Status != domain.StatusLive {
		return api.SurveyCloseResp{}, ecode.Conflict(msgCloseNotLive)
	}
	if err := m.store.SetStatus(ctx, meta.ID, domain.StatusClosed); err != nil {
		return api.SurveyCloseResp{}, err
	}
	return api.SurveyCloseResp{}, nil
}

// Reopen 重新打开(closed → live)。守卫:仅 closed 且曾发布过可重开(复用现有快照)。
func (m *Manager) Reopen(ctx context.Context, req api.SurveyReopenReq) (api.SurveyReopenResp, error) {
	if err := authorize(ctx, rbac.ActionSurveyReopen); err != nil {
		return api.SurveyReopenResp{}, err
	}
	meta, err := m.owned(ctx, req.ID)
	if err != nil {
		return api.SurveyReopenResp{}, err
	}
	if meta.Status != domain.StatusClosed || meta.PublishedVersion == nil {
		return api.SurveyReopenResp{}, ecode.Conflict(msgReopenInvalid)
	}
	if err := m.store.SetStatus(ctx, meta.ID, domain.StatusLive); err != nil {
		return api.SurveyReopenResp{}, err
	}
	return api.SurveyReopenResp{}, nil
}

// Stats 问卷概览:状态 + 已发布版本 + 答卷数。归属校验。
func (m *Manager) Stats(ctx context.Context, req api.SurveyStatsReq) (api.SurveyStatsResp, error) {
	if err := authorize(ctx, rbac.ActionSurveyStats); err != nil {
		return api.SurveyStatsResp{}, err
	}
	meta, err := m.owned(ctx, req.ID)
	if err != nil {
		return api.SurveyStatsResp{}, err
	}
	count, err := m.store.CountResponses(ctx, meta.ID)
	if err != nil {
		return api.SurveyStatsResp{}, err
	}
	access := meta.AnswerAccess
	if access == "" {
		access = domain.AnswerAnonymous
	}
	return api.SurveyStatsResp{Status: meta.Status, PublishedVersion: meta.PublishedVersion, ResponseCount: count, AnswerAccess: access}, nil
}
