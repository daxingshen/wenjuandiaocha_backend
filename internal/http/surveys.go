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

// createSurvey POST /api/surveys —— 建空草稿,返回 { id }。draft_schema 为最小 SurveySchema。
func (s *Server) createSurvey(c *gin.Context) {
	id := newID()
	schema := domain.SurveySchema{
		ID: id, Type: domain.SurveySurvey, Title: "未命名问卷", Version: 1,
		Questions: []domain.Question{}, Rules: []domain.LogicRule{},
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
	version, err := s.store.Publish(c.Request.Context(), meta.ID, meta.DraftSchema)
	if err != nil {
		s.storeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "version": version})
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
