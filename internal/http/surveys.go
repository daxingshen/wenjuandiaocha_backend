// studio 端点(需登录):问卷 CRUD + 发布。归属校验:只能读写自己的问卷。
package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/internal/domain"
	"wenjuandiaocha_backend/internal/store"
)

// surveyListItem 对齐前端 SurveyListItem { id, title, type, updatedAt }。
type surveyListItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt"`
}

func (s *Server) listSurveys(c *gin.Context) {
	items, err := s.store.ListSurveysByOwner(c.Request.Context(), currentUserID(c))
	if err != nil {
		s.storeError(c, err)
		return
	}
	out := make([]surveyListItem, 0, len(items))
	for _, it := range items {
		out = append(out, surveyListItem{
			ID: it.ID, Title: it.Title, Type: it.Type, Status: it.Status,
			UpdatedAt: it.UpdatedAt.Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, out)
}

// createSurvey POST /api/surveys —— 首存落库,返回 { id }(后端分配 id)。
// body 是前端内存草稿的整份 SurveySchema(不保存则前端不发此请求 → 后端零写入);
// 空 body 回落最小 schema(仍支持直接建空)。id 一律由后端分配并覆盖进 schema。
// 与 updateSurvey 一致:首存只落草稿、不做逻辑求值,发布时才严格校验。
func (s *Server) createSurvey(c *gin.Context) {
	id := newID()
	body, _ := c.GetRawData()

	var schema domain.SurveySchema
	if len(body) > 0 {
		if err := json.Unmarshal(body, &schema); err != nil {
			fail(c, http.StatusBadRequest, "schema 格式错误")
			return
		}
	}
	// 补默认 + 强制后端分配的 id(客户端传的 id 一律忽略,防越权/串号)。
	schema.ID = id
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
	if err := s.store.CreateSurvey(c.Request.Context(), id, currentUserID(c), string(schema.Type), schema.Title, schemaJSON); err != nil {
		s.storeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

// getSurvey GET /api/surveys/:id —— 返回草稿 SurveySchema(供编辑)。归属校验。
func (s *Server) getSurvey(c *gin.Context) {
	meta, err := s.ownedSurvey(c)
	if err != nil {
		return // ownedSurvey 已写响应
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", meta.DraftSchema)
}

// updateSurvey PUT /api/surveys/:id —— 存草稿。body 是整份 SurveySchema。归属校验。
func (s *Server) updateSurvey(c *gin.Context) {
	meta, err := s.ownedSurvey(c)
	if err != nil {
		return
	}
	body, err := c.GetRawData()
	if err != nil {
		fail(c, http.StatusBadRequest, "读请求体失败")
		return
	}
	var schema domain.SurveySchema
	if err := json.Unmarshal(body, &schema); err != nil {
		fail(c, http.StatusBadRequest, "schema 格式错误")
		return
	}
	// 落库存 body(保留前端原样),但 title/type 从解析出的 schema 取,便于列表展示。
	if err := s.store.UpdateDraft(c.Request.Context(), meta.ID, schema.Title, string(schema.Type), body); err != nil {
		s.storeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// publishSurvey POST /api/surveys/:id/publish —— 冻结草稿为新版本快照 + status=live。
func (s *Server) publishSurvey(c *gin.Context) {
	meta, err := s.ownedSurvey(c)
	if err != nil {
		return
	}
	version, unchanged, err := s.store.Publish(c.Request.Context(), meta.ID, meta.DraftSchema)
	if err != nil {
		s.storeError(c, err)
		return
	}
	// unchanged=true:草稿与当前对外版本一致,未造新版本(重发免空版)。前端据此提示「内容未变」。
	c.JSON(http.StatusOK, gin.H{"ok": true, "version": version, "unchanged": unchanged})
}

// closeSurvey POST /api/surveys/:id/close —— 结束回收(live → closed)。
// 状态机守卫:仅 live 可结束;非法跳转返回 409。归属校验。
func (s *Server) closeSurvey(c *gin.Context) {
	meta, err := s.ownedSurvey(c)
	if err != nil {
		return
	}
	if meta.Status != "live" {
		fail(c, http.StatusConflict, "仅进行中的问卷可结束")
		return
	}
	if err := s.store.SetStatus(c.Request.Context(), meta.ID, "closed"); err != nil {
		s.storeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// reopenSurvey POST /api/surveys/:id/reopen —— 重新打开(closed → live)。
// 复用现有已发布快照(不重新冻结);守卫:仅 closed 且曾发布过可重开;非法跳转返回 409。
func (s *Server) reopenSurvey(c *gin.Context) {
	meta, err := s.ownedSurvey(c)
	if err != nil {
		return
	}
	if meta.Status != "closed" || meta.PublishedVersion == nil {
		fail(c, http.StatusConflict, "仅已结束且曾发布过的问卷可重新打开")
		return
	}
	if err := s.store.SetStatus(c.Request.Context(), meta.ID, "live"); err != nil {
		s.storeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// surveyStats 对齐前端问卷概览:状态 + 已发布版本 + 答卷数。
type surveyStats struct {
	Status           string `json:"status"`
	PublishedVersion *int32 `json:"publishedVersion"`
	ResponseCount    int32  `json:"responseCount"`
}

// surveyStats GET /api/surveys/:id/stats —— 问卷概览统计。归属校验。
func (s *Server) surveyStats(c *gin.Context) {
	meta, err := s.ownedSurvey(c)
	if err != nil {
		return // ownedSurvey 已写响应
	}
	count, err := s.store.CountResponses(c.Request.Context(), meta.ID)
	if err != nil {
		s.storeError(c, err)
		return
	}
	c.JSON(http.StatusOK, surveyStats{
		Status:           meta.Status,
		PublishedVersion: meta.PublishedVersion,
		ResponseCount:    count,
	})
}

// ownedSurvey 取 :id 问卷并校验归属;非本人 → 404(不泄露存在性),已写响应时返回 err。
func (s *Server) ownedSurvey(c *gin.Context) (store.SurveyMeta, error) {
	meta, err := s.store.GetSurvey(c.Request.Context(), c.Param("id"))
	if err != nil {
		s.storeError(c, err)
		return store.SurveyMeta{}, err
	}
	if meta.OwnerID != currentUserID(c) {
		fail(c, http.StatusNotFound, "不存在")
		return store.SurveyMeta{}, errForbidden
	}
	return meta, nil
}
