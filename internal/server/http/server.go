// http 层装配:Server 持有各 service manager,挂路由。传输层只做 bind→调 service→render。
package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/wire"

	"wenjuandiaocha_backend/internal/config"
	"wenjuandiaocha_backend/internal/rbac"
	authmw "wenjuandiaocha_backend/internal/server/http/middleware/auth"
	"wenjuandiaocha_backend/internal/server/http/middleware/clientinfo"
	"wenjuandiaocha_backend/internal/server/http/middleware/cors"
	"wenjuandiaocha_backend/internal/server/http/middleware/logger"
	"wenjuandiaocha_backend/internal/server/http/middleware/recovery"
	"wenjuandiaocha_backend/internal/server/http/middleware/requestid"
	svcauth "wenjuandiaocha_backend/internal/service/auth"
	"wenjuandiaocha_backend/internal/service/submission"
	"wenjuandiaocha_backend/internal/service/survey"
)

// ProviderSet 供 wire 组装:提供 *Server。
var ProviderSet = wire.NewSet(NewServer)

// Server 承载 HTTP 依赖:各业务 service 接口 + config。传输层持接口,不认识具体 Manager。
type Server struct {
	surveys     survey.Service
	submissions submission.Service
	auth        svcauth.Service
	cfg         config.Config
}

// NewServer 建 Server,注入各 service。
func NewServer(surveys survey.Service, submissions submission.Service, auth svcauth.Service, cfg config.Config) *Server {
	return &Server{surveys: surveys, submissions: submissions, auth: auth, cfg: cfg}
}

// Router 构建 gin 引擎:全局中间件 + 路由分组。
func (s *Server) Router() *gin.Engine {
	r := gin.New()
	// 本地单机部署,不信任任何转发代理头(ClientIP 取真实 RemoteAddr)。
	// 未来置于反代后,改为设置反代 IP。
	_ = r.SetTrustedProxies(nil)
	r.Use(requestid.New(), recovery.New(), logger.New(), cors.New(), clientinfo.New())

	// auth 中间件按端点所需能力位逐路由构造:RequireAuth(svc, action)。
	// action=="" 只鉴权不判能力位(/auth/me、需登录但作答能力由 service 按问卷模式另判的 submit)。
	// 逐路由显式传 action = 路由表即「端点 → 所需能力」清单(可审计);不设组级保底,漏挂由端点级测试兜。
	reqAuth := func(action rbac.Action) gin.HandlerFunc { return authmw.RequireAuth(s.auth, action) }

	root := r.Group("/api")

	// public:匿名公开。取发布快照 + 提交答卷。无鉴权(runtime 决策 7)。
	pub := root.Group("/public")
	{
		pub.GET("/surveys/:id", s.getPublicSurvey)
		pub.POST("/surveys/:id/answers", s.submitAnswers)
	}

	// auth:登录/登出/取当前用户。
	a := root.Group("/auth")
	{
		a.POST("/login", s.login)
		a.POST("/logout", s.logout)
		a.GET("/me", reqAuth(""), s.me) // 只需登录,能力位无关
	}

	// studio:需登录 + RBAC 第一层能力位(第二层归属在 service)。
	sv := root.Group("/surveys")
	{
		sv.GET("", reqAuth(rbac.ActionSurveyList), s.listSurveys)                          // admin 全站/creator 本人范围在 service
		sv.POST("", reqAuth(rbac.ActionSurveyCreate), s.createSurvey)                      //
		sv.GET("/:id", reqAuth(rbac.ActionSurveyRead), s.getSurvey)                        //
		sv.PUT("/:id", reqAuth(rbac.ActionSurveyUpdate), s.updateSurvey)                   //
		sv.PATCH("/:id/answer-access", reqAuth(rbac.ActionSurveyUpdate), s.setAnswerAccess) // 设作答模式 = 改草稿,复用 Update 能力位
		sv.PATCH("/:id/display-mode", reqAuth(rbac.ActionSurveyUpdate), s.setDisplayMode)   // 设展示模式 = 改草稿,复用 Update 能力位
		sv.POST("/:id/publish", reqAuth(rbac.ActionSurveyPublish), s.publishSurvey)        //
		sv.POST("/:id/close", reqAuth(rbac.ActionSurveyClose), s.closeSurvey)              //
		sv.POST("/:id/reopen", reqAuth(rbac.ActionSurveyReopen), s.reopenSurvey)           //
		sv.GET("/:id/stats", reqAuth(rbac.ActionSurveyStats), s.surveyStats)               //
		// 需登录作答(login_required 问卷):第一层能力位 answer:submit 在中间件判(creator 被拒 403);
		// 作答模式匹配(问卷是不是 login_required)+ 状态 live 等数据相关闸门需查问卷,留 submission service。
		sv.POST("/:id/answers", reqAuth(rbac.ActionSubmitAnswer), s.submitAnswersAuthed)
	}

	return r
}
