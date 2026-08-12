// public 端点(匿名公开,runtime 用):取发布快照 + 提交答卷。业务在 service/submission。
//
// 安全:无鉴权(决策 7 匿名作答)。提交端点做基础 IP 限频防滥用(传输关切,留在本层);
// 「同 IP/微信限答次数、密码访问、时长下限」是 PRD §4.3 发布回收范围,本轮埋 meta 不实现。
//
// 权威流程(决策 6 安全底线)由 service/submission 承担:用后端载入的发布版 schema 完整重跑
// Evaluate → Validate → Normalize,永不信任客户端;客户端多传的隐藏题答案被剔除。
package http

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/api"
	"wenjuandiaocha_backend/internal/domain"
	"wenjuandiaocha_backend/internal/lib/ratelimit"
	"wenjuandiaocha_backend/internal/server/http/render"
)

// submitLimiter:提交答卷限频。每 IP 平均 1 次/秒,突发 10。
var submitLimiter = ratelimit.New(1, 10)

// getPublicSurvey GET /api/public/surveys/:id —— 返回已发布快照 SurveySchema。
func (s *Server) getPublicSurvey(c *gin.Context) {
	resp, err := s.submissions.GetPublished(c.Request.Context(), api.GetPublishedReq{ID: c.Param("id")})
	if err != nil {
		render.Error(c, err)
		return
	}
	// 快照本身就是 SurveySchema JSON,用 RawMessage 原样嵌入信封 data(不二次转义)。
	render.Success(c, json.RawMessage(resp.Schema))
}

type submitReq struct {
	Answers domain.Answers `json:"answers"`
	// Version 作答者实际看到的问卷版本(版本锚定提交)。>0 时后端按该版快照校验落库,
	// 消除「作答中所有者重发新版 → 拿没见过的题报必答」死局。0/缺省 = 旧客户端,回落当前发布版。
	Version int32 `json:"version"`
}

// submitAnswers POST /api/public/surveys/:id/answers —— 限频 + 权威校验 + 双写落库。
func (s *Server) submitAnswers(c *gin.Context) {
	if !submitLimiter.Allow(c.ClientIP()) {
		render.Fail(c, http.StatusTooManyRequests, "提交过于频繁,请稍后再试")
		return
	}

	var req submitReq
	if err := c.ShouldBindJSON(&req); err != nil {
		render.Fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}

	// ip/ua 已由全局 clientInfo 中间件注入 ctx metadata,submission service 从中采集存 meta。
	res, err := s.submissions.Submit(c.Request.Context(), api.SubmitReq{
		SurveyID: c.Param("id"),
		Answers:  req.Answers,
		Version:  req.Version,
	})
	if err != nil {
		render.Error(c, err)
		return
	}
	if len(res.ValidationErrors) > 0 {
		render.Validation(c, res.ValidationErrors)
		return
	}
	render.Success(c, gin.H{"rows": res.Rows})
}
