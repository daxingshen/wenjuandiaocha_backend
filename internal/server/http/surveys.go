// studio 端点(需登录):问卷 CRUD + 发布。业务逻辑在 service/survey,handler 只做 bind→调→render。
package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/api"
	"wenjuandiaocha_backend/internal/server/http/render"
)

// surveyListItem 是 HTTP 响应形状(带 json tag),由 service.SurveyListItem 翻译而来。
// 对齐前端 SurveyListItem { id, title, type, status, updatedAt }。
type surveyListItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt"`
}

func (s *Server) listSurveys(c *gin.Context) {
	resp, err := s.surveys.List(c.Request.Context())
	if err != nil {
		render.Error(c, err)
		return
	}
	out := make([]surveyListItem, 0, len(resp.Items))
	for _, it := range resp.Items {
		out = append(out, surveyListItem{
			ID: it.ID, Title: it.Title, Type: it.Type, Status: it.Status,
			UpdatedAt: it.UpdatedAt.Format(time.RFC3339),
		})
	}
	render.Success(c, out)
}

// createSurvey POST /api/surveys —— 首存落库,返回 { id }(后端分配 id)。
func (s *Server) createSurvey(c *gin.Context) {
	body, _ := c.GetRawData()
	resp, err := s.surveys.Create(c.Request.Context(), api.SurveyCreateReq{Body: body})
	if err != nil {
		render.Error(c, err)
		return
	}
	render.Success(c, gin.H{"id": resp.ID})
}

// getSurvey GET /api/surveys/:id —— 返回草稿 SurveySchema(供编辑)。归属校验。
func (s *Server) getSurvey(c *gin.Context) {
	resp, err := s.surveys.Get(c.Request.Context(), api.SurveyGetReq{ID: c.Param("id")})
	if err != nil {
		render.Error(c, err)
		return
	}
	// 草稿 schema JSON,用 RawMessage 原样嵌入信封 data(不二次转义)。
	render.Success(c, json.RawMessage(resp.Schema))
}

// updateSurvey PUT /api/surveys/:id —— 存草稿。body 是整份 SurveySchema。归属校验。
func (s *Server) updateSurvey(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		render.Fail(c, http.StatusBadRequest, "读请求体失败")
		return
	}
	if _, err := s.surveys.Update(c.Request.Context(), api.SurveyUpdateReq{ID: c.Param("id"), Body: body}); err != nil {
		render.Error(c, err)
		return
	}
	// 纯占位成功:code=0 已表成功,data 置 nil。
	render.Success(c, nil)
}

// publishReq 发布请求可选体:answerAccess 指定作答访问模式(D6)。
// 缺省(空体/旧客户端)→ service 回落 anonymous,维持现状。
type publishReq struct {
	AnswerAccess string `json:"answerAccess"`
}

// publishSurvey POST /api/surveys/:id/publish —— 冻结草稿为新版本快照 + status=live + 设作答模式。
func (s *Server) publishSurvey(c *gin.Context) {
	// 体可选:忽略绑定错误(空体/非法体一律回落 service 默认 anonymous),不因缺体而 400。
	var body publishReq
	_ = c.ShouldBindJSON(&body)
	resp, err := s.surveys.Publish(c.Request.Context(), api.SurveyPublishReq{ID: c.Param("id"), AnswerAccess: body.AnswerAccess})
	if err != nil {
		render.Error(c, err)
		return
	}
	// unchanged=true:草稿与当前对外版本一致,未造新版本(重发免空版)。前端据此提示「内容未变」。
	render.Success(c, gin.H{"version": resp.Version, "unchanged": resp.Unchanged})
}

// closeSurvey POST /api/surveys/:id/close —— 结束回收(live → closed)。状态机守卫在 service。
func (s *Server) closeSurvey(c *gin.Context) {
	if _, err := s.surveys.Close(c.Request.Context(), api.SurveyCloseReq{ID: c.Param("id")}); err != nil {
		render.Error(c, err)
		return
	}
	render.Success(c, nil)
}

// reopenSurvey POST /api/surveys/:id/reopen —— 重新打开(closed → live)。守卫在 service。
func (s *Server) reopenSurvey(c *gin.Context) {
	if _, err := s.surveys.Reopen(c.Request.Context(), api.SurveyReopenReq{ID: c.Param("id")}); err != nil {
		render.Error(c, err)
		return
	}
	render.Success(c, nil)
}

// submitAnswersAuthed POST /api/surveys/:id/answers —— 需登录作答(login_required 问卷)。
// 已过 requireAuth(会话有效 + ctx 带 role);作答能力位(respondent/admin,creator 被拒 403)
// 与作答模式匹配(仅 login_required)由 submission service 判定。权威校验/落库同匿名路径。
func (s *Server) submitAnswersAuthed(c *gin.Context) {
	var req submitReq
	if err := c.ShouldBindJSON(&req); err != nil {
		render.Fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	res, err := s.submissions.Submit(c.Request.Context(), api.SubmitReq{
		SurveyID:      c.Param("id"),
		Answers:       req.Answers,
		Version:       req.Version,
		Authenticated: true, // 鉴权路径:submission 据此走 login_required 分支 + 校作答能力位
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

// surveyStats 对齐前端问卷概览:状态 + 已发布版本 + 答卷数。
type surveyStats struct {
	Status           string `json:"status"`
	PublishedVersion *int32 `json:"publishedVersion"`
	ResponseCount    int32  `json:"responseCount"`
}

// surveyStats GET /api/surveys/:id/stats —— 问卷概览统计。归属校验。
func (s *Server) surveyStats(c *gin.Context) {
	st, err := s.surveys.Stats(c.Request.Context(), api.SurveyStatsReq{ID: c.Param("id")})
	if err != nil {
		render.Error(c, err)
		return
	}
	render.Success(c, surveyStats{
		Status:           st.Status,
		PublishedVersion: st.PublishedVersion,
		ResponseCount:    st.ResponseCount,
	})
}
