// http 层装配:Server 持有各 service manager,挂路由。传输层只做 bind→调 service→render。
package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/wire"

	"wenjuandiaocha_backend/internal/config"
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

	// requireAuth 构造一次复用(依赖注入 auth service)。
	requireAuth := authmw.RequireAuth(s.auth)

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
		a.GET("/me", requireAuth, s.me)
	}

	// studio:需登录。问卷 CRUD + 发布。
	sv := root.Group("/surveys", requireAuth)
	{
		sv.GET("", s.listSurveys)
		sv.POST("", s.createSurvey)
		sv.GET("/:id", s.getSurvey)
		sv.PUT("/:id", s.updateSurvey)
		sv.POST("/:id/publish", s.publishSurvey)
		sv.POST("/:id/close", s.closeSurvey)
		sv.POST("/:id/reopen", s.reopenSurvey)
		sv.GET("/:id/stats", s.surveyStats)
		// 需登录作答(login_required 问卷):过 requireAuth + 作答能力位(respondent/admin)。
		// 与匿名 /public 提交并存;anonymous 问卷仍走 /public。
		sv.POST("/:id/answers", s.submitAnswersAuthed)
	}

	return r
}
