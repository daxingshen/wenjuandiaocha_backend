// public 端点(匿名公开,runtime 用):取发布快照 + 提交答卷。
//
// 安全:无鉴权(决策 7 匿名作答)。提交端点做基础 IP 限频防滥用;
// 「同 IP/微信限答次数、密码访问、时长下限」是 PRD §4.3 发布回收范围,本轮埋 meta 不实现。
//
// 权威流程(决策 6 安全底线):用后端载入的发布版 schema 完整重跑
// Evaluate → Validate → Normalize,永不信任客户端;客户端多传的隐藏题答案被剔除。
package http

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/internal/domain"
	"wenjuandiaocha_backend/internal/store"
)

// getPublicSurvey GET /api/public/surveys/:id —— 返回已发布快照 SurveySchema。
func (s *Server) getPublicSurvey(c *gin.Context) {
	id := c.Param("id")
	schemaJSON, err := s.store.GetPublishedSchema(c.Request.Context(), id)
	if err != nil {
		if err == store.ErrNotFound {
			fail(c, http.StatusNotFound, "问卷不存在或未发布")
			return
		}
		s.storeError(c, err)
		return
	}
	// 快照本身就是 SurveySchema JSON,原样吐(前端无适配层)。
	c.Data(http.StatusOK, "application/json; charset=utf-8", schemaJSON)
}

type submitReq struct {
	Answers domain.Answers `json:"answers"`
}

// submitAnswers POST /api/public/surveys/:id/answers —— 权威校验 + 双写落库。
func (s *Server) submitAnswers(c *gin.Context) {
	id := c.Param("id")

	if !submitLimiter.allow(c.ClientIP()) {
		fail(c, http.StatusTooManyRequests, "提交过于频繁,请稍后再试")
		return
	}

	var req submitReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}

	// 载入发布版快照 —— 校验/规范化以它为准,不信任客户端传的 schema。
	schemaJSON, err := s.store.GetPublishedSchema(c.Request.Context(), id)
	if err != nil {
		if err == store.ErrNotFound {
			fail(c, http.StatusNotFound, "问卷不存在或未发布")
			return
		}
		s.storeError(c, err)
		return
	}
	var schema domain.SurveySchema
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		s.storeError(c, err)
		return
	}

	// 权威重跑:校验(隐藏题跳过)。
	if errs := domain.ValidateSurvey(schema, req.Answers); len(errs) > 0 {
		failValidation(c, errs)
		return
	}
	// 规范化:隐藏题不产行 —— 客户端多传的隐藏题答案在此被剔除。
	rows := domain.NormalizeSurvey(schema, req.Answers)

	// raw 存后端认定的答案(原样存客户端提交的 answers 亦可,但落库规范化行以后端为准)。
	rawJSON, _ := json.Marshal(req.Answers)
	meta := clientMeta(c)

	respID := newID()
	if err := s.store.SaveSubmission(c.Request.Context(), respID, id, schema.Version, rawJSON, meta, rows); err != nil {
		s.storeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "rows": len(rows)})
}

// clientMeta 采集防刷预留信息(ip/ua),存 responses.meta。
func clientMeta(c *gin.Context) []byte {
	m := map[string]any{"ip": c.ClientIP(), "ua": c.Request.UserAgent()}
	b, _ := json.Marshal(m)
	return b
}
