// studio 端点(需登录):问卷 CRUD + 发布。业务逻辑在 service/survey,handler 只做 bind→调→render。
package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// surveyListItem 对齐前端 SurveyListItem { id, title, type, status, updatedAt }。
type surveyListItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt"`
}

func (s *Server) listSurveys(c *gin.Context) {
	items, err := s.surveys.List(c.Request.Context(), currentUserID(c))
	if err != nil {
		renderError(c, err)
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
func (s *Server) createSurvey(c *gin.Context) {
	body, _ := c.GetRawData()
	id, err := s.surveys.Create(c.Request.Context(), currentUserID(c), body)
	if err != nil {
		renderError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

// getSurvey GET /api/surveys/:id —— 返回草稿 SurveySchema(供编辑)。归属校验。
func (s *Server) getSurvey(c *gin.Context) {
	schema, err := s.surveys.Get(c.Request.Context(), c.Param("id"), currentUserID(c))
	if err != nil {
		renderError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", schema)
}

// updateSurvey PUT /api/surveys/:id —— 存草稿。body 是整份 SurveySchema。归属校验。
func (s *Server) updateSurvey(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		fail(c, http.StatusBadRequest, "读请求体失败")
		return
	}
	if err := s.surveys.Update(c.Request.Context(), c.Param("id"), currentUserID(c), body); err != nil {
		renderError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// publishSurvey POST /api/surveys/:id/publish —— 冻结草稿为新版本快照 + status=live。
func (s *Server) publishSurvey(c *gin.Context) {
	version, unchanged, err := s.surveys.Publish(c.Request.Context(), c.Param("id"), currentUserID(c))
	if err != nil {
		renderError(c, err)
		return
	}
	// unchanged=true:草稿与当前对外版本一致,未造新版本(重发免空版)。前端据此提示「内容未变」。
	c.JSON(http.StatusOK, gin.H{"ok": true, "version": version, "unchanged": unchanged})
}

// closeSurvey POST /api/surveys/:id/close —— 结束回收(live → closed)。状态机守卫在 service。
func (s *Server) closeSurvey(c *gin.Context) {
	if err := s.surveys.Close(c.Request.Context(), c.Param("id"), currentUserID(c)); err != nil {
		renderError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// reopenSurvey POST /api/surveys/:id/reopen —— 重新打开(closed → live)。守卫在 service。
func (s *Server) reopenSurvey(c *gin.Context) {
	if err := s.surveys.Reopen(c.Request.Context(), c.Param("id"), currentUserID(c)); err != nil {
		renderError(c, err)
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
	st, err := s.surveys.Stats(c.Request.Context(), c.Param("id"), currentUserID(c))
	if err != nil {
		renderError(c, err)
		return
	}
	c.JSON(http.StatusOK, surveyStats{
		Status:           st.Status,
		PublishedVersion: st.PublishedVersion,
		ResponseCount:    st.ResponseCount,
	})
}
