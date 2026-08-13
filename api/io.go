// Package api 是星卷后端的统一 I/O 契约层:所有 service 方法的输入/输出数据结构集中于此,
// 一处定义、http(及未来 gRPC)两端共用,谁都不另立业务输入输出。
//
// 约定:
//   - 类型按域前缀命名(Auth* / Survey* / Submit* / GetPublished*),同层内永不重名。
//   - 字段导出、不带 json tag —— 传输格式由各传输层负责(http 私有 struct 定 tag)。
//   - 本包零框架依赖:只 import context/time + domain,不碰 gin/pgx/dao/protobuf。
//   - schema/answers 主体作为 []byte / domain.Answers 透传(jsonb 整存 + 前端零适配)。
//   - **Req 只装客户端真实发送的东西**(请求体 + URL 路径参数)。会话身份、token、
//     服务端采集的 ip/ua 等传输派生值走 Metadata + ctx 隐式注入,不进 Req
//     (对齐未来 gRPC 的 metadata/interceptor 语义)。
package api

import (
	"context"
	"time"

	"wenjuandiaocha_backend/internal/domain"
)

// ---------- 传输派生元数据(不属于客户端业务入参,走 ctx 流转)----------

// Metadata 承载传输层派生的调用上下文:会话身份、会话 token、客户端网络信息。
// 由各传输层(http 从 session/cookie/请求、gRPC 从拦截器/metadata)填入 ctx,service 从 ctx 读。
type Metadata struct {
	UserID    string // 已认证用户 id(studio 端点归属校验用)
	Role      string // 已认证用户角色 admin|creator|respondent(RBAC 能力判定用;RequireAuth 注入)
	Token     string // 会话 token(logout/validateSession 用)
	ClientIP  string // 客户端 IP(防刷 meta)
	UserAgent string // 客户端 UA(防刷 meta)
}

type metadataKey struct{}

// WithMetadata 把 Metadata 挂到 ctx。传输层在调 service 前调用。
func WithMetadata(ctx context.Context, md Metadata) context.Context {
	return context.WithValue(ctx, metadataKey{}, md)
}

// MetadataFrom 从 ctx 取 Metadata;未设置返回零值。service 层调用。
func MetadataFrom(ctx context.Context) Metadata {
	md, _ := ctx.Value(metadataKey{}).(Metadata)
	return md
}

// ---------- auth 域 ----------

// AuthUser 对外用户信息(对齐前端 AuthUser)。
// Role 为 RBAC 角色轴;前端可据此做体验层门控(前端同步不在本轮范围)。
type AuthUser struct {
	ID   string
	Name string
	Role string
}

type AuthLoginReq struct{ Account, Password string }
type AuthLoginResp struct {
	User    AuthUser
	Token   string    // session token,传输层负责下发(HTTP set-cookie / gRPC metadata)
	Expires time.Time // 会话过期时刻,供传输层设 cookie MaxAge
}

// Me/Logout/ValidateSession 无客户端入参:userID/token 走 Metadata。
type AuthMeResp struct{ User AuthUser }
type AuthValidateSessionResp struct {
	UserID string
	Role   string // 会话用户角色,供中间件注入 ctx metadata
}

// ---------- survey 域 ----------

// SurveyListItem 列表项(对齐前端 SurveyListItem)。
type SurveyListItem struct {
	ID        string
	Title     string
	Type      string
	Status    string
	UpdatedAt time.Time
}

// List 无客户端入参(ownerID 走 Metadata),故无 Req。
type SurveyListResp struct{ Items []SurveyListItem }

type SurveyCreateReq struct {
	Body []byte // 前端内存草稿的整份 SurveySchema 原始字节;空则回落最小 schema
}
type SurveyCreateResp struct{ ID string }

type SurveyGetReq struct{ ID string }      // ID 为 URL 路径参数
type SurveyGetResp struct{ Schema []byte } // 草稿 SurveySchema 原始 jsonb

type SurveyUpdateReq struct {
	ID   string // URL 路径参数
	Body []byte
}
type SurveyUpdateResp struct{}

type SurveyPublishReq struct {
	ID           string
	AnswerAccess string // anonymous|login_required(发布配置,D6);空/未知回落 anonymous
}
type SurveyPublishResp struct {
	Version   int
	Unchanged bool
}

type SurveyCloseReq struct{ ID string }
type SurveyCloseResp struct{}

type SurveyReopenReq struct{ ID string }
type SurveyReopenResp struct{}

type SurveyStatsReq struct{ ID string }
type SurveyStatsResp struct {
	Status           string
	PublishedVersion *int32
	ResponseCount    int32
}

// ---------- submission 域 ----------

type GetPublishedReq struct{ ID string }
type GetPublishedResp struct{ Schema []byte } // 已发布快照 SurveySchema 原始 jsonb

type SubmitReq struct {
	SurveyID string // URL 路径参数
	Answers  domain.Answers
	Version  int32 // >0 版本锚定按该历史版校验;0 回落当前发布版
	// 是否已登录不进 Req —— 由 submission service 从 ctx 的会话身份(Metadata.UserID)
	// 自行确认,不采信调用方声明:UserID 仅由 requireAuth 校验 session 后注入,
	// 匿名 /public 路由无此中间件、拿不到 UserID,故后端据真实会话状态区分匿名/登录。
}
type SubmitResp struct {
	Rows             int
	ValidationErrors []domain.ValidationError // 非空 → 传输层回 400 + {errors},业务无错
}
