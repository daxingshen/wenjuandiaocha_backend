// Package survey 是问卷 studio 业务层:CRUD + 发布 + 生命周期状态机守卫 + 归属校验。
// 从原 http handler 下沉的业务逻辑。传输层(server/http)只做 bind→调本层→render。
package survey

import (
	"context"
	"encoding/json"
	"errors"

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

// Manager 持有 dao 门面。业务方法返回 ecode.Error,传输层据此映射状态码。
type Manager struct {
	store Store
}

func New(store Store) *Manager { return &Manager{store: store} }

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
func (m *Manager) List(ctx context.Context, ownerID string) ([]dao.SurveyListItem, error) {
	return m.store.ListSurveysByOwner(ctx, ownerID)
}

// Create 首存落库:后端强制分配 id(忽略客户端传的 id,防越权/串号)、补 schema 默认值。
// body 是前端内存草稿的整份 SurveySchema;空 body 回落最小 schema。返回后端分配的 id。
func (m *Manager) Create(ctx context.Context, ownerID string, body []byte) (string, error) {
	newid := id.New()
	var schema domain.SurveySchema
	if len(body) > 0 {
		if err := json.Unmarshal(body, &schema); err != nil {
			return "", ecode.BadRequest("schema 格式错误")
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
	if err := m.store.CreateSurvey(ctx, newid, ownerID, string(schema.Type), schema.Title, schemaJSON); err != nil {
		return "", err
	}
	return newid, nil
}

// Get 返回草稿 SurveySchema 原始 jsonb(供编辑)。归属校验。
func (m *Manager) Get(ctx context.Context, id, ownerID string) ([]byte, error) {
	meta, err := m.owned(ctx, id, ownerID)
	if err != nil {
		return nil, err
	}
	return meta.DraftSchema, nil
}

// Update 存草稿:body 为整份 SurveySchema 原始 bytes(保留前端原样落库),title/type 从解析出的 schema 取。
func (m *Manager) Update(ctx context.Context, id, ownerID string, body []byte) error {
	meta, err := m.owned(ctx, id, ownerID)
	if err != nil {
		return err
	}
	var schema domain.SurveySchema
	if err := json.Unmarshal(body, &schema); err != nil {
		return ecode.BadRequest("schema 格式错误")
	}
	return m.store.UpdateDraft(ctx, meta.ID, schema.Title, string(schema.Type), body)
}

// Publish 冻结草稿为新版本快照 + status=live。unchanged=true 表示草稿与当前对外版一致(重发免空版)。
func (m *Manager) Publish(ctx context.Context, id, ownerID string) (version int, unchanged bool, err error) {
	meta, err := m.owned(ctx, id, ownerID)
	if err != nil {
		return 0, false, err
	}
	return m.store.Publish(ctx, meta.ID, meta.DraftSchema)
}

// Close 结束回收(live → closed)。状态机守卫:仅 live 可结束。
func (m *Manager) Close(ctx context.Context, id, ownerID string) error {
	meta, err := m.owned(ctx, id, ownerID)
	if err != nil {
		return err
	}
	if meta.Status != "live" {
		return ecode.Conflict("仅进行中的问卷可结束")
	}
	return m.store.SetStatus(ctx, meta.ID, "closed")
}

// Reopen 重新打开(closed → live)。守卫:仅 closed 且曾发布过可重开(复用现有快照)。
func (m *Manager) Reopen(ctx context.Context, id, ownerID string) error {
	meta, err := m.owned(ctx, id, ownerID)
	if err != nil {
		return err
	}
	if meta.Status != "closed" || meta.PublishedVersion == nil {
		return ecode.Conflict("仅已结束且曾发布过的问卷可重新打开")
	}
	return m.store.SetStatus(ctx, meta.ID, "live")
}

// Stats 问卷概览:状态 + 已发布版本 + 答卷数。归属校验。
type Stats struct {
	Status           string
	PublishedVersion *int32
	ResponseCount    int32
}

func (m *Manager) Stats(ctx context.Context, id, ownerID string) (Stats, error) {
	meta, err := m.owned(ctx, id, ownerID)
	if err != nil {
		return Stats{}, err
	}
	count, err := m.store.CountResponses(ctx, meta.ID)
	if err != nil {
		return Stats{}, err
	}
	return Stats{Status: meta.Status, PublishedVersion: meta.PublishedVersion, ResponseCount: count}, nil
}
