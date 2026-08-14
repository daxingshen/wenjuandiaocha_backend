// Package submission 是匿名作答提交的业务层:收答守卫 + 版本锚定分流 + 权威重跑 + 双写落库。
// 安全底线(决策6):用后端载入的发布版 schema 完整重跑 Evaluate→Validate→Normalize,永不信任客户端。
// 限频是传输关切,留在 server/http 层。
package submission

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/wire"

	"wenjuandiaocha_backend/api"
	"wenjuandiaocha_backend/internal/dao"
	"wenjuandiaocha_backend/internal/domain"
	"wenjuandiaocha_backend/internal/ecode"
	"wenjuandiaocha_backend/internal/lib/id"
	"wenjuandiaocha_backend/internal/rbac"
)

// Store 是本层依赖的 dao 子集(消费方定义接口,便于单测)。*dao.Store 实现它。
type Store interface {
	GetSurvey(ctx context.Context, id string) (dao.SurveyMeta, error)
	GetPublishedSchema(ctx context.Context, id string) ([]byte, error)
	GetVersionSchema(ctx context.Context, id string, version int32) ([]byte, error)
	SaveSubmission(ctx context.Context, respID, surveyID string, version int, raw, meta []byte, rows []domain.NormalizedRow) error
}

// I/O 契约集中在 api 包(一处定义,http/gRPC 两端共用)。

// Service 是匿名作答提交业务契约。*Manager 实现它;传输层持本接口。
// Submit 的防刷 meta(ip/ua)来自 ctx 的 api.Metadata,不进 Req。
type Service interface {
	GetPublished(ctx context.Context, req api.GetPublishedReq) (api.GetPublishedResp, error)
	Submit(ctx context.Context, req api.SubmitReq) (api.SubmitResp, error)
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
func (m *Manager) GetPublished(ctx context.Context, req api.GetPublishedReq) (api.GetPublishedResp, error) {
	b, err := m.store.GetPublishedSchema(ctx, req.ID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return api.GetPublishedResp{}, ecode.NotFound("问卷不存在或未发布")
		}
		return api.GetPublishedResp{}, err
	}
	// 取问卷 meta 带出 answer_access:前端据此选匿名/登录作答路径。空/历史值按 anonymous。
	meta, err := m.store.GetSurvey(ctx, req.ID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return api.GetPublishedResp{}, ecode.NotFound("问卷不存在或未发布")
		}
		return api.GetPublishedResp{}, err
	}
	access := meta.AnswerAccess
	if access == "" {
		access = domain.AnswerAnonymous
	}
	return api.GetPublishedResp{Schema: b, AnswerAccess: access}, nil
}

// Submit 提交答卷。Version>0 按该历史版快照校验(版本锚定);0 回落当前发布版。
// 校验失败返回 ecode.Validation(携逐题明细 payload),由 render.JSON 渲染进信封 data:{errors:[...]}(恒200)。
func (m *Manager) Submit(ctx context.Context, req api.SubmitReq) (api.SubmitResp, error) {
	// 登录态由后端从 ctx 会话身份确认,不采信调用方声明:UserID 仅由 requireAuth 校验
	// session 后注入(匿名 /public 路由无此中间件,UserID 恒空),故 UserID != "" ⟺ 已登录。
	md := api.MetadataFrom(ctx)
	authenticated := md.UserID != ""

	// 已登录路径:第一层能力位 —— 仅有作答能力的角色(respondent/admin)可提交;creator 被挡下(真 403)。
	// 匿名路径无账号、不查能力位,走 anonymous 问卷分支。
	if authenticated && !rbac.Can(rbac.Role(md.Role), rbac.ActionSubmitAnswer) {
		return api.SubmitResp{}, ecode.Forbidden403("当前账号无作答权限")
	}

	// 收答前置:问卷必须存在且 status=live(close 后停收)。
	survey, err := m.store.GetSurvey(ctx, req.SurveyID)
	if err != nil {
		if errors.Is(err, dao.ErrNotFound) {
			return api.SubmitResp{}, ecode.NotFound("问卷不存在或未发布")
		}
		return api.SubmitResp{}, err
	}
	if survey.Status != domain.StatusLive {
		return api.SubmitResp{}, ecode.NotFound("问卷不存在或未发布")
	}

	// 作答访问模式闸门(D5):问卷的 answer_access 与提交路径必须匹配。
	//   - login_required 问卷:仅鉴权路径可提交;匿名 /public 路径提交(无会话)→ 401 需登录。
	//     (公开 GET 已回显 answerAccess,该问卷需登录本就非秘密,故直白 401 而非伪装 404。)
	//   - anonymous 问卷:仅匿名路径提交;鉴权路径不服务它 → BadRequest 引导走 /public(职责单一)。
	// 空/历史值(迁移默认 anonymous)按 anonymous 处理。
	loginRequired := survey.AnswerAccess == domain.AnswerLoginRequired
	if loginRequired && !authenticated {
		return api.SubmitResp{}, ecode.Unauthorized("该问卷需登录后作答")
	}
	if !loginRequired && authenticated {
		return api.SubmitResp{}, ecode.BadRequest("该问卷为匿名作答,请通过公开链接提交")
	}

	// 载入快照 —— 校验/规范化以它为准,不信任客户端传的 schema。
	schemaJSON, err := m.loadSchema(ctx, req.SurveyID, req.Version)
	if err != nil {
		return api.SubmitResp{}, err
	}
	var schema domain.SurveySchema
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return api.SubmitResp{}, err
	}

	// api.Answers 与 domain.Answers 同底层 map[string]any,边界处显式转换(api 不依赖 domain)。
	answers := domain.Answers(req.Answers)
	// 权威重跑:校验(隐藏题跳过)。失败 → ecode.Validation 携逐题明细,render 渲染进 data:{errors}。
	if errs := domain.ValidateSurvey(schema, answers); len(errs) > 0 {
		// domain.ValidationError → api.ValidationError 逐条转换(slice-of-struct 不能整体转)。
		payload := api.ValidationErrorsPayload{Errors: make([]api.ValidationError, len(errs))}
		for i, e := range errs {
			payload.Errors[i] = api.ValidationError{QID: e.QID, Message: e.Message}
		}
		return api.SubmitResp{}, ecode.Validation(payload)
	}
	// 规范化:隐藏题不产行 —— 客户端多传的隐藏题答案在此被剔除。
	rows := domain.NormalizeSurvey(schema, answers)

	// raw 存客户端提交的 answers;落库规范化行以后端为准。
	rawJSON, _ := json.Marshal(req.Answers)
	// 防刷 meta(ip/ua)来自 ctx metadata(md 已在方法开头取得);字段形状与原 http clientMeta 一致(map 键序 ip<ua)。
	metaJSON, _ := json.Marshal(map[string]any{"ip": md.ClientIP, "ua": md.UserAgent})
	respID := id.New()
	if err := m.store.SaveSubmission(ctx, respID, req.SurveyID, schema.Version, rawJSON, metaJSON, rows); err != nil {
		return api.SubmitResp{}, err
	}
	return api.SubmitResp{Rows: len(rows)}, nil
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
