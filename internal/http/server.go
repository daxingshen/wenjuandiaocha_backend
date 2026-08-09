// http 层装配:Server 持有依赖(store + config),挂路由。
package http

import (
	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/internal/config"
	"wenjuandiaocha_backend/internal/store"
)

// Server 承载 HTTP 依赖。
type Server struct {
	store *store.Store
	cfg   config.Config
}

// NewServer 建 Server。
func NewServer(st *store.Store, cfg config.Config) *Server {
	return &Server{store: st, cfg: cfg}
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
	}

	return r
}
