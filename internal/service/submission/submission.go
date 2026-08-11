// Package submission 是匿名作答提交的业务层:收答守卫 + 版本锚定分流 + 权威重跑 + 双写落库。
// 安全底线(决策6):用后端载入的发布版 schema 完整重跑 Evaluate→Validate→Normalize,永不信任客户端。
// 限频是传输关切,留在 server/http 层。
package submission

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/wire"

	"wenjuandiaocha_backend/internal/dao"
	"wenjuandiaocha_backend/internal/domain"
	"wenjuandiaocha_backend/internal/ecode"
	"wenjuandiaocha_backend/internal/lib/id"
)

// Store 是本层依赖的 dao 子集(消费方定义接口,便于单测)。*dao.Store 实现它。
type Store interface {
	GetSurvey(ctx context.Context, id string) (dao.SurveyMeta, error)
	GetPublishedSchema(ctx context.Context, id string) ([]byte, error)
	GetVersionSchema(ctx context.Context, id string, version int32) ([]byte, error)
	SaveSubmission(ctx context.Context, respID, surveyID string, version int, raw, meta []byte, rows []domain.NormalizedRow) error
}

// ---------- 统一 I/O 契约(一处定义,http/gRPC 两端共用)----------

type GetPublishedReq struct{ ID string }
type GetPublishedResp struct{ Schema []byte } // 已发布快照 SurveySchema 原始 jsonb

type SubmitReq struct {
	SurveyID string
	Answers  domain.Answers
	Version  int32  // >0 版本锚定按该历史版校验;0 回落当前发布版
	Meta     []byte // 传输层采集的防刷信息(ip/ua),存 responses.meta
}
type SubmitResp struct {
	Rows             int
	ValidationErrors []domain.ValidationError // 非空 → 传输层回 400 + {errors},业务无错
}

// Service 是匿名作答提交业务契约。*Manager 实现它;传输层持本接口。
type Service interface {
	GetPublished(ctx context.Context, req GetPublishedReq) (GetPublishedResp, error)
	Submit(ctx context.Context, req SubmitReq) (SubmitResp, error)
}

type Manager struct {
	store Store
}

func New(store Store) *Manager { return &Manager{store: store} }

// 编译期确认 *Manager 实现 Service。
var _ Service = (*Manager)(nil)

// ProviderSet 供 wire 组装:提供 *Manager,并绑定到 Service 接口。
var ProviderSet = wire.NewSet(New, wire.Bind(new(Service), new(*Manager)))

// GetPublished 取已发布快照 SurveySchema 原始 jsonb(原样吐前端,无适配层);未发布/不存在 → NotFound。
func (m *Manager) GetPublished(ctx context.Context, req GetPublishedReq) (GetPublishedResp, error) {
	b, err := m.store.GetPublishedSchema(ctx, req.ID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return GetPublishedResp{}, ecode.NotFound("问卷不存在或未发布")
		}
		return GetPublishedResp{}, err
	}
	return GetPublishedResp{Schema: b}, nil
}

// Submit 提交答卷。Version>0 按该历史版快照校验(版本锚定);0 回落当前发布版。
// 校验失败通过 SubmitResp.ValidationErrors 返回(传输层据此回 400 + {errors}),error 仍为 nil。
func (m *Manager) Submit(ctx context.Context, req SubmitReq) (SubmitResp, error) {
	// 收答前置:问卷必须存在且 status=live(close 后停收)。
	survey, err := m.store.GetSurvey(ctx, req.SurveyID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return SubmitResp{}, ecode.NotFound("问卷不存在或未发布")
		}
		return SubmitResp{}, err
	}
	if survey.Status != "live" {
		return SubmitResp{}, ecode.NotFound("问卷不存在或未发布")
	}

	// 载入快照 —— 校验/规范化以它为准,不信任客户端传的 schema。
	schemaJSON, err := m.loadSchema(ctx, req.SurveyID, req.Version)
	if err != nil {
		return SubmitResp{}, err
	}
	var schema domain.SurveySchema
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return SubmitResp{}, err
	}

	// 权威重跑:校验(隐藏题跳过)。
	if errs := domain.ValidateSurvey(schema, req.Answers); len(errs) > 0 {
		return SubmitResp{ValidationErrors: errs}, nil
	}
	// 规范化:隐藏题不产行 —— 客户端多传的隐藏题答案在此被剔除。
	rows := domain.NormalizeSurvey(schema, req.Answers)

	// raw 存客户端提交的 answers;落库规范化行以后端为准。
	rawJSON, _ := json.Marshal(req.Answers)
	respID := id.New()
	if err := m.store.SaveSubmission(ctx, respID, req.SurveyID, schema.Version, rawJSON, req.Meta, rows); err != nil {
		return SubmitResp{}, err
	}
	return SubmitResp{Rows: len(rows)}, nil
}

// loadSchema 版本锚定分流:version>0 取该历史版(失效→BadRequest 引导刷新);version==0 回落当前发布版(向后兼容)。
func (m *Manager) loadSchema(ctx context.Context, surveyID string, version int32) ([]byte, error) {
	if version > 0 {
		b, err := m.store.GetVersionSchema(ctx, surveyID, version)
		if err != nil {
			if errors.Is(err, dao.ErrNotFound) {
				return nil, ecode.BadRequest("问卷版本已失效,请刷新后重新作答")
			}
			return nil, err
		}
		return b, nil
	}
	b, err := m.store.GetPublishedSchema(ctx, surveyID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return nil, ecode.NotFound("问卷不存在或未发布")
		}
		return nil, err
	}
	return b, nil
}
