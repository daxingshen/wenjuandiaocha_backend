// studio 端点(需登录):问卷 CRUD + 发布。业务逻辑在 service/survey,handler 只做 bind→调→render。
package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/api"
	"wenjuandiaocha_backend/internal/server/http/render"
)

func (s *Server) listSurveys(c *gin.Context) {
	// 全部可选 query 参数:q(标题搜索)/status/type(过滤)/limit(页大小)/page(页码,1-based)。
	// limit/page 非数字按 0 处理(service 落默认);空参数即旧全量首页行为。
	limit, _ := strconv.Atoi(c.Query("limit"))
	page, _ := strconv.Atoi(c.Query("page"))
	resp, err := s.surveys.List(c.Request.Context(), api.SurveyListReq{
		Q:      c.Query("q"),
		Status: c.Query("status"),
		Type:   c.Query("type"),
		Limit:  limit,
		Page:   page,
	})
	if err != nil {
		render.JSON(c, nil, err)
		return
	}
	// 整个 resp 作信封 data → data:{items,total};UpdatedAt 已在 service 格式化为 RFC3339。
	render.JSON(c, resp, nil)
}

// createSurvey POST /api/surveys —— 首存落库,返回 { id }(后端分配 id)。
func (s *Server) createSurvey(c *gin.Context) {
	body, _ := c.GetRawData()
	resp, err := s.surveys.Create(c.Request.Context(), api.SurveyCreateReq{Body: body})
	if err != nil {
		render.JSON(c, nil, err)
		return
	}
	// SurveyCreateResp{ID json:"id"} 作 data → data:{id}。
	render.JSON(c, resp, nil)
}

// getSurvey GET /api/surveys/:id —— 返回草稿 SurveySchema(供编辑)。归属校验。
func (s *Server) getSurvey(c *gin.Context) {
	resp, err := s.surveys.Get(c.Request.Context(), api.SurveyGetReq{ID: c.Param("id")})
	if err != nil {
		render.JSON(c, nil, err)
		return
	}
	// data 即裸草稿 schema 对象本体(不包 {schema} 层):resp.Schema 已是 json.RawMessage,原样嵌入不转义。
	render.JSON(c, resp.Schema, nil)
}

// updateSurvey PUT /api/surveys/:id —— 存草稿。body 是整份 SurveySchema。归属校验。
func (s *Server) updateSurvey(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		render.Fail(c, http.StatusBadRequest, "读请求体失败")
		return
	}
	if _, err := s.surveys.Update(c.Request.Context(), api.SurveyUpdateReq{ID: c.Param("id"), Body: body}); err != nil {
		render.JSON(c, nil, err)
		return
	}
	// 纯占位成功:code=0 已表成功,data 置 nil。
	render.JSON(c, nil, nil)
}

// publishSurvey POST /api/surveys/:id/publish —— 冻结草稿为新版本快照 + status=live。
// 不接受作答模式:它由 draft 阶段的 PATCH /answer-access 设定(单一真相源),发布不碰。
func (s *Server) publishSurvey(c *gin.Context) {
	resp, err := s.surveys.Publish(c.Request.Context(), api.SurveyPublishReq{ID: c.Param("id")})
	if err != nil {
		render.JSON(c, nil, err)
		return
	}
	// SurveyPublishResp{Version,Unchanged} 作 data → data:{version,unchanged}。
	// unchanged=true:草稿与当前对外版本一致,未造新版本(重发免空版),前端据此提示「内容未变」。
	render.JSON(c, resp, nil)
}

// setAnswerAccess PATCH /api/surveys/:id/answer-access —— 设作答访问模式(仅 draft 可改,守卫在 service)。
func (s *Server) setAnswerAccess(c *gin.Context) {
	// api.SurveySetAnswerAccessReq 带 json tag(answerAccess),bind body 后从 c.Param 覆盖 ID。
	var req api.SurveySetAnswerAccessReq
	if err := c.ShouldBindJSON(&req); err != nil {
		render.Fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	req.ID = c.Param("id")
	if _, err := s.surveys.SetAnswerAccess(c.Request.Context(), req); err != nil {
		render.JSON(c, nil, err)
		return
	}
	render.JSON(c, nil, nil)
}

// setDisplayMode PATCH /api/surveys/:id/display-mode —— 设作答页展示模式(仅 draft 可改,守卫在 service)。
func (s *Server) setDisplayMode(c *gin.Context) {
	// api.SurveySetDisplayModeReq 带 json tag(displayMode),bind body 后从 c.Param 覆盖 ID。
	var req api.SurveySetDisplayModeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		render.Fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	req.ID = c.Param("id")
	if _, err := s.surveys.SetDisplayMode(c.Request.Context(), req); err != nil {
		render.JSON(c, nil, err)
		return
	}
	render.JSON(c, nil, nil)
}

// closeSurvey POST /api/surveys/:id/close —— 结束回收(live → closed)。状态机守卫在 service。
func (s *Server) closeSurvey(c *gin.Context) {
	if _, err := s.surveys.Close(c.Request.Context(), api.SurveyCloseReq{ID: c.Param("id")}); err != nil {
		render.JSON(c, nil, err)
		return
	}
	render.JSON(c, nil, nil)
}

// reopenSurvey POST /api/surveys/:id/reopen —— 重新打开(closed → live)。守卫在 service。
func (s *Server) reopenSurvey(c *gin.Context) {
	if _, err := s.surveys.Reopen(c.Request.Context(), api.SurveyReopenReq{ID: c.Param("id")}); err != nil {
		render.JSON(c, nil, err)
		return
	}
	render.JSON(c, nil, nil)
}

// submitAnswersAuthed POST /api/surveys/:id/answers —— 需登录作答(login_required 问卷)。
// 已过 requireAuth(会话有效 + ctx 带 role);作答能力位(respondent/admin,creator 被拒 403)
// 与作答模式匹配(仅 login_required)由 submission service 判定。权威校验/落库同匿名路径。
func (s *Server) submitAnswersAuthed(c *gin.Context) {
	// api.SubmitReq 带 json tag(answers/version),bind body 后从 c.Param 覆盖 SurveyID。
	var req api.SubmitReq
	if err := c.ShouldBindJSON(&req); err != nil {
		render.Fail(c, http.StatusBadRequest, "请求体格式错误")
		return
	}
	req.SurveyID = c.Param("id")
	// 不传登录标记 —— submission 从 ctx 的会话身份(requireAuth 注入的 UserID)自行确认已登录。
	// 校验失败由 service 返回 ecode.Validation,render.JSON 渲染进 data:{errors};成功 → data:{rows}。
	res, err := s.submissions.Submit(c.Request.Context(), req)
	render.JSON(c, res, err)
}

// surveyStats GET /api/surveys/:id/stats —— 问卷概览统计。归属校验。
func (s *Server) surveyStats(c *gin.Context) {
	st, err := s.surveys.Stats(c.Request.Context(), api.SurveyStatsReq{ID: c.Param("id")})
	if err != nil {
		render.JSON(c, nil, err)
		return
	}
	// SurveyStatsResp{status,publishedVersion,responseCount,answerAccess} 作 data。
	render.JSON(c, st, nil)
}
