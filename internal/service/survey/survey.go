// Package survey 是问卷 studio 业务层:CRUD + 发布 + 生命周期状态机守卫 + 归属校验。
// 从原 http handler 下沉的业务逻辑。传输层(server/http)只做 bind→调本层→render。
package survey

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/wire"

	"wenjuandiaocha_backend/internal/dao"
	"wenjuandiaocha_backend/internal/domain"
	"wenjuandiaocha_backend/internal/ecode"
	"wenjuandiaocha_backend/internal/lib/id"
)

// Store 是本层依赖的 dao 子集(消费方定义接口,便于单测注入 fake)。*dao.Store 实现它。
type Store interface {
	GetSurvey(ctx context.Context, id string) (dao.SurveyMeta, error)
	ListSurveysByOwner(ctx context.Context, ownerID string) ([]dao.SurveyListItem, error)
	CreateSurvey(ctx context.Context, id, ownerID, typ, title string, draftSchema []byte) error
	UpdateDraft(ctx context.Context, id, title, typ string, draftSchema []byte) error
	SetStatus(ctx context.Context, id, status string) error
	Publish(ctx context.Context, surveyID string, draftSchema []byte) (int, bool, error)
	CountResponses(ctx context.Context, id string) (int32, error)
}

// ---------- 统一 I/O 契约(一处定义,http/gRPC 两端共用,谁都不另立业务输入输出)----------
//
// 每个方法形态 Method(ctx, XReq) (XResp, error)。OwnerID 是调用方身份,由各传输层
// (http 从 session、gRPC 从拦截器)取得后填入 Req,不走 context 隐式注入。
// schema 主体作为 []byte 透传(jsonb 整存 + 前端零适配)。

// SurveyListItem 列表项(对齐前端 SurveyListItem)。从 http 包搬入,消除 dao 类型泄漏。
type SurveyListItem struct {
	ID        string
	Title     string
	Type      string
	Status    string
	UpdatedAt time.Time
}

type ListReq struct{ OwnerID string }
type ListResp struct{ Items []SurveyListItem }

type CreateReq struct {
	OwnerID string
	Body    []byte // 前端内存草稿的整份 SurveySchema 原始字节;空则回落最小 schema
}
type CreateResp struct{ ID string }

type GetReq struct{ ID, OwnerID string }
type GetResp struct{ Schema []byte } // 草稿 SurveySchema 原始 jsonb

type UpdateReq struct {
	ID, OwnerID string
	Body        []byte
}
type UpdateResp struct{}

type PublishReq struct{ ID, OwnerID string }
type PublishResp struct {
	Version   int
	Unchanged bool
}

type CloseReq struct{ ID, OwnerID string }
type CloseResp struct{}

type ReopenReq struct{ ID, OwnerID string }
type ReopenResp struct{}

type StatsReq struct{ ID, OwnerID string }
type StatsResp struct {
	Status           string
	PublishedVersion *int32
	ResponseCount    int32
}

// Service 是问卷 studio 业务契约。*Manager 实现它;传输层持本接口。
type Service interface {
	List(ctx context.Context, req ListReq) (ListResp, error)
	Create(ctx context.Context, req CreateReq) (CreateResp, error)
	Get(ctx context.Context, req GetReq) (GetResp, error)
	Update(ctx context.Context, req UpdateReq) (UpdateResp, error)
	Publish(ctx context.Context, req PublishReq) (PublishResp, error)
	Close(ctx context.Context, req CloseReq) (CloseResp, error)
	Reopen(ctx context.Context, req ReopenReq) (ReopenResp, error)
	Stats(ctx context.Context, req StatsReq) (StatsResp, error)
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

// owned 取问卷并校验归属:查无 → NotFound("不存在");非本人 → Forbidden(对外同样 404 不泄露存在性)。
func (m *Manager) owned(ctx context.Context, id, ownerID string) (dao.SurveyMeta, error) {
	meta, err := m.store.GetSurvey(ctx, id)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return dao.SurveyMeta{}, ecode.NotFound("不存在")
		}
		return dao.SurveyMeta{}, err
	}
	if meta.OwnerID != ownerID {
		return dao.SurveyMeta{}, ecode.Forbidden()
	}
	return meta, nil
}

// List 本人问卷列表。
func (m *Manager) List(ctx context.Context, req ListReq) (ListResp, error) {
	rows, err := m.store.ListSurveysByOwner(ctx, req.OwnerID)
	if err != nil {
		return ListResp{}, err
	}
	items := make([]SurveyListItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, SurveyListItem{
			ID: r.ID, Title: r.Title, Type: r.Type, Status: r.Status, UpdatedAt: r.UpdatedAt,
		})
	}
	return ListResp{Items: items}, nil
}

// Create 首存落库:后端强制分配 id(忽略客户端传的 id,防越权/串号)、补 schema 默认值。
// Body 是前端内存草稿的整份 SurveySchema;空 Body 回落最小 schema。返回后端分配的 id。
func (m *Manager) Create(ctx context.Context, req CreateReq) (CreateResp, error) {
	newid := id.New()
	var schema domain.SurveySchema
	if len(req.Body) > 0 {
		if err := json.Unmarshal(req.Body, &schema); err != nil {
			return CreateResp{}, ecode.BadRequest("schema 格式错误")
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
	if err := m.store.CreateSurvey(ctx, newid, req.OwnerID, string(schema.Type), schema.Title, schemaJSON); err != nil {
		return CreateResp{}, err
	}
	return CreateResp{ID: newid}, nil
}

// Get 返回草稿 SurveySchema 原始 jsonb(供编辑)。归属校验。
func (m *Manager) Get(ctx context.Context, req GetReq) (GetResp, error) {
	meta, err := m.owned(ctx, req.ID, req.OwnerID)
	if err != nil {
		return GetResp{}, err
	}
	return GetResp{Schema: meta.DraftSchema}, nil
}

// Update 存草稿:Body 为整份 SurveySchema 原始 bytes(保留前端原样落库),title/type 从解析出的 schema 取。
func (m *Manager) Update(ctx context.Context, req UpdateReq) (UpdateResp, error) {
	meta, err := m.owned(ctx, req.ID, req.OwnerID)
	if err != nil {
		return UpdateResp{}, err
	}
	var schema domain.SurveySchema
	if err := json.Unmarshal(req.Body, &schema); err != nil {
		return UpdateResp{}, ecode.BadRequest("schema 格式错误")
	}
	if err := m.store.UpdateDraft(ctx, meta.ID, schema.Title, string(schema.Type), req.Body); err != nil {
		return UpdateResp{}, err
	}
	return UpdateResp{}, nil
}

// Publish 冻结草稿为新版本快照 + status=live。Unchanged=true 表示草稿与当前对外版一致(重发免空版)。
func (m *Manager) Publish(ctx context.Context, req PublishReq) (PublishResp, error) {
	meta, err := m.owned(ctx, req.ID, req.OwnerID)
	if err != nil {
		return PublishResp{}, err
	}
	version, unchanged, err := m.store.Publish(ctx, meta.ID, meta.DraftSchema)
	if err != nil {
		return PublishResp{}, err
	}
	return PublishResp{Version: version, Unchanged: unchanged}, nil
}

// Close 结束回收(live → closed)。状态机守卫:仅 live 可结束。
func (m *Manager) Close(ctx context.Context, req CloseReq) (CloseResp, error) {
	meta, err := m.owned(ctx, req.ID, req.OwnerID)
	if err != nil {
		return CloseResp{}, err
	}
	if meta.Status != "live" {
		return CloseResp{}, ecode.Conflict("仅进行中的问卷可结束")
	}
	if err := m.store.SetStatus(ctx, meta.ID, "closed"); err != nil {
		return CloseResp{}, err
	}
	return CloseResp{}, nil
}

// Reopen 重新打开(closed → live)。守卫:仅 closed 且曾发布过可重开(复用现有快照)。
func (m *Manager) Reopen(ctx context.Context, req ReopenReq) (ReopenResp, error) {
	meta, err := m.owned(ctx, req.ID, req.OwnerID)
	if err != nil {
		return ReopenResp{}, err
	}
	if meta.Status != "closed" || meta.PublishedVersion == nil {
		return ReopenResp{}, ecode.Conflict("仅已结束且曾发布过的问卷可重新打开")
	}
	if err := m.store.SetStatus(ctx, meta.ID, "live"); err != nil {
		return ReopenResp{}, err
	}
	return ReopenResp{}, nil
}

// Stats 问卷概览:状态 + 已发布版本 + 答卷数。归属校验。
func (m *Manager) Stats(ctx context.Context, req StatsReq) (StatsResp, error) {
	meta, err := m.owned(ctx, req.ID, req.OwnerID)
	if err != nil {
		return StatsResp{}, err
	}
	count, err := m.store.CountResponses(ctx, meta.ID)
	if err != nil {
		return StatsResp{}, err
	}
	return StatsResp{Status: meta.Status, PublishedVersion: meta.PublishedVersion, ResponseCount: count}, nil
}
