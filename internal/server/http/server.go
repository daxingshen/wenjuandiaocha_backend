// http 层装配:Server 持有各 service manager,挂路由。传输层只做 bind→调 service→render。
package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/wire"

	"wenjuandiaocha_backend/internal/config"
	svcauth "wenjuandiaocha_backend/internal/service/auth"
	"wenjuandiaocha_backend/internal/service/submission"
	"wenjuandiaocha_backend/internal/service/survey"
)

// ProviderSet 供 wire 组装:提供 *Server。
var ProviderSet = wire.NewSet(NewServer)

// Server 承载 HTTP 依赖:各业务 service 接口 + config。传输层持接口,不认识具体 Manager。
type Server struct {
	surveys     survey.Service
	submissions *submission.Manager
	auth        *svcauth.Manager
	cfg         config.Config
}

// NewServer 建 Server,注入各 service。
func NewServer(surveys survey.Service, submissions *submission.Manager, auth *svcauth.Manager, cfg config.Config) *Server {
	return &Server{surveys: surveys, submissions: submissions, auth: auth, cfg: cfg}
}

// Router 构建 gin 引擎:全局中间件 + 路由分组。
func (s *Server) Router() *gin.Engine {
	r := gin.New()
	// 本地单机部署,不信任任何转发代理头(ClientIP 取真实 RemoteAddr)。
	// 未来置于反代后,改为设置反代 IP。
	_ = r.SetTrustedProxies(nil)
	r.Use(requestID(), recovery(), logger(), cors())

	api := r.Group("/api")

	// public:匿名公开。取发布快照 + 提交答卷。无鉴权(runtime 决策 7)。
	pub := api.Group("/public")
	{
		pub.GET("/surveys/:id", s.getPublicSurvey)
		pub.POST("/surveys/:id/answers", s.submitAnswers)
	}

	// auth:登录/登出/取当前用户。
	a := api.Group("/auth")
	{
		a.POST("/login", s.login)
		a.POST("/logout", s.logout)
		a.GET("/me", s.requireAuth, s.me)
	}

	// studio:需登录。问卷 CRUD + 发布。
	sv := api.Group("/surveys", s.requireAuth)
	{
		sv.GET("", s.listSurveys)
		sv.POST("", s.createSurvey)
		sv.GET("/:id", s.getSurvey)
		sv.PUT("/:id", s.updateSurvey)
		sv.POST("/:id/publish", s.publishSurvey)
		sv.POST("/:id/close", s.closeSurvey)
		sv.POST("/:id/reopen", s.reopenSurvey)
		sv.GET("/:id/stats", s.surveyStats)
	}

	return r
}
