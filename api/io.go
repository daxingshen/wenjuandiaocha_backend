// Package api 是星卷后端的统一 I/O 契约层:所有 service 方法的输入/输出数据结构集中于此,
// 一处定义、http(及未来 gRPC)两端共用,谁都不另立业务输入输出。
//
// 约定:
//   - 类型按域前缀命名(Auth* / Survey* / Submit* / GetPublished*),同层内永不重名。
//   - **字段带 json tag —— api 类型即对外线格式**,传输层直接序列化,不再定义私有 struct 翻译。
//     Req 的 URL 路径参数字段打 `json:"-"`(不从 body 收,由 handler 从 c.Param 覆盖)。
//   - **本包自包含**:只 import context/time/encoding/json(均标准库),不引用业务核心 domain,
//     不碰 gin/pgx/dao/protobuf。api 是最顶层协议层(手写 proto 的等价物),契约类型自定义。
//   - schema/answers 主体作为 json.RawMessage / api.Answers 透传(jsonb 整存 + 前端零适配,
//     裸 schema 用 RawMessage 原样嵌入信封 data,不二次转义成 base64)。
//   - **Req 只装客户端真实发送的东西**(请求体 + URL 路径参数)。会话身份、token、
//     服务端采集的 ip/ua 等传输派生值走 Metadata + ctx 隐式注入,不进 Req
//     (对齐未来 gRPC 的 metadata/interceptor 语义)。
package api

import (
	"context"
	"encoding/json"
	"time"
)

// ---------- 协议自有基础类型(不引用 domain,api 自包含)----------

// Answers 作答载荷:题 ID → 答案值。与 domain.Answers 同底层(map[string]any),
// 但 api 不依赖 domain,故在协议层自定义;service 边界按需转换。对齐前端提交体 answers。
type Answers map[string]any

// ValidationError 单条校验错误(对齐前端 { qid, message })。
// 与 domain.ValidationError 结构一致但独立定义,保 api 层零业务依赖。
type ValidationError struct {
	QID     string `json:"qid"`
	Message string `json:"message"`
}

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
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type AuthLoginReq struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

// AuthLoginResp 的 User 是对外响应负载(login 直接返回 AuthUser 作 data);
// Token/Expires 是传输派生值(cookie 下发用),不出现在响应 body,故打 json:"-"。
type AuthLoginResp struct {
	User    AuthUser  `json:"user"`
	Token   string    `json:"-"` // session token,传输层负责下发(HTTP set-cookie / gRPC metadata)
	Expires time.Time `json:"-"` // 会话过期时刻,供传输层设 cookie MaxAge
}

// Me/Logout/ValidateSession 无客户端入参:userID/token 走 Metadata。
type AuthMeResp struct {
	User AuthUser `json:"user"`
}
type AuthValidateSessionResp struct {
	UserID string `json:"-"` // 中间件注入 ctx 用,不出响应
	Role   string `json:"-"` // 会话用户角色,供中间件注入 ctx metadata
}

// ---------- survey 域 ----------

// SurveyListItem 列表项(对齐前端 SurveyListItem)。
// UpdatedAt 对外为 RFC3339 字符串(前端契约),故用 string 而非 time.Time,由 service 赋值时格式化。
type SurveyListItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt"`
}

// List 无客户端入参(ownerID 走 Metadata),故无 Req。
// 响应直接返回 Items 切片作 data(data:[...]),故 SurveyListResp 本身不出信封,Items 打 json:"-"。
type SurveyListResp struct {
	Items []SurveyListItem `json:"-"`
}

type SurveyCreateReq struct {
	Body json.RawMessage `json:"-"` // 前端内存草稿整份 SurveySchema 原始字节(裸 body,非 JSON 字段);空则回落最小 schema
}
type SurveyCreateResp struct {
	ID string `json:"id"`
}

type SurveyGetReq struct {
	ID string `json:"-"` // ID 为 URL 路径参数
}

// SurveyGetResp 的 Schema 是裸草稿 SurveySchema,handler 直接以 Schema 作信封 data
// (data 即 schema 对象本体,不包 {schema} 层);用 json.RawMessage 原样嵌入不二次转义。
type SurveyGetResp struct {
	Schema json.RawMessage `json:"-"`
}

type SurveyUpdateReq struct {
	ID   string          `json:"-"` // URL 路径参数
	Body json.RawMessage `json:"-"` // 裸 body,整份 SurveySchema
}
type SurveyUpdateResp struct{}

// SurveyPublishReq 发布只需 ID:作答模式不在发布时设定(draft 阶段经 SetAnswerAccess 定,单一真相源)。
type SurveyPublishReq struct {
	ID string `json:"-"`
}
type SurveyPublishResp struct {
	Version   int  `json:"version"`
	Unchanged bool `json:"unchanged"`
}

// SurveySetAnswerAccessReq 设作答访问模式(仅 draft 可改,守卫在 service)。
type SurveySetAnswerAccessReq struct {
	ID           string `json:"-"`
	AnswerAccess string `json:"answerAccess"` // anonymous|login_required
}
type SurveySetAnswerAccessResp struct{}

type SurveyCloseReq struct {
	ID string `json:"-"`
}
type SurveyCloseResp struct{}

type SurveyReopenReq struct {
	ID string `json:"-"`
}
type SurveyReopenResp struct{}

type SurveyStatsReq struct {
	ID string `json:"-"`
}
type SurveyStatsResp struct {
	Status           string `json:"status"`
	PublishedVersion *int32 `json:"publishedVersion"`
	ResponseCount    int32  `json:"responseCount"`
	AnswerAccess     string `json:"answerAccess"` // anonymous|login_required(发布页回显作答模式)
}

// ---------- submission 域 ----------

type GetPublishedReq struct {
	ID string `json:"-"`
}

// GetPublishedResp 已发布快照 + 作答访问模式。整体作信封 data → data:{schema,answerAccess}。
// Schema 用 json.RawMessage 原样嵌入不二次转义;
// AnswerAccess(anonymous|login_required)让前端在加载阶段即知该走匿名还是登录作答路径,
// 无需靠提交失败反推(呼应 260813-respondent-fill-page 方案A)。
type GetPublishedResp struct {
	Schema       json.RawMessage `json:"schema"`       // 已发布快照 SurveySchema 原始 jsonb
	AnswerAccess string          `json:"answerAccess"` // anonymous|login_required(问卷级配置,发布时设定)
}

type SubmitReq struct {
	SurveyID string  `json:"-"` // URL 路径参数
	Answers  Answers `json:"answers"`
	Version  int32   `json:"version"` // >0 版本锚定按该历史版校验;0 回落当前发布版
	// 是否已登录不进 Req —— 由 submission service 从 ctx 的会话身份(Metadata.UserID)
	// 自行确认,不采信调用方声明:UserID 仅由 requireAuth 校验 session 后注入,
	// 匿名 /public 路由无此中间件、拿不到 UserID,故后端据真实会话状态区分匿名/登录。
}

// SubmitResp 成功负载,整体作信封 data → data:{rows}。
// 校验失败不再走此结构的字段,而由 Submit 返回 ecode.Validation(ValidationErrorsPayload) 承载。
type SubmitResp struct {
	Rows int `json:"rows"`
}

// ValidationErrorsPayload 提交校验失败的逐题明细,作为 ecode.Validation 的不透明 data 载荷,
// 由 render.JSON 原样渲染进信封 data → data:{errors:[{qid,message}...]}。
// 定义在 api(wire 形状归此),render 不认识其结构、只当任意 data 渲染(保 render 业务无关)。
type ValidationErrorsPayload struct {
	Errors []ValidationError `json:"errors"`
}
