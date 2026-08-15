// Package ecode 定义业务错误码:携带业务 code + 对外消息。
// service 层返回 ecode.Error,server/http 层用 FromError 提取 code,统一包进
// { code, message, data } 响应信封(HTTP 状态恒 200)。用 errors.As 提取。
package ecode

import "errors"

// 业务错误码(粗粒度,7 类 + 成功)。分段规则 <HTTP前缀><序号>:人读即知原
// HTTP 语义,又不等于 HTTP status 本身。成功码 0;非 0 均为各类业务错。
const (
	CodeOK              = 0     // 成功
	CodeBadRequest      = 40001 // 参数 / schema 格式错(原 400)
	CodeUnauthorized    = 40101 // 未授权 / 会话无效或过期(原 401)
	CodeForbidden       = 40301 // 平台能力级越权:已登录但角色无权做这类动作(原 403,RBAC 第一层)
	CodeNotFound        = 40401 // 不存在 / 非本人 / 未发布(原 404,含资源级 Forbidden 防枚举)
	CodeValidation      = 42201 // 提交校验失败(逐题明细走 data.errors)
	CodeTooManyRequests = 42901 // 限频(原 429)
	CodeConflict        = 40901 // 状态机非法跳转(原 409)
	CodeInternal        = 50001 // 未预期内部错误(原 500)
)

// Error 是带业务 code 的错误。消息对外可见,code 进响应信封。
// data 是可选的不透明附加负载(如校验失败的逐题明细),由 render 原样渲染进信封 data;
// 绝大多数业务错 data=nil(信封 data:null)。render 不解析 data 结构,故其形状归 api 层定义。
type Error struct {
	code int
	msg  string
	data any
}

func (e *Error) Error() string { return e.msg }

// Code 返回业务错误码。
func (e *Error) Code() int { return e.code }

// Data 返回附加负载(无则 nil)。供 render 取出渲染进信封 data。
func (e *Error) Data() any { return e.data }

// 构造器:每个对应一类业务错误。消息由调用方给,保持与现有对外文案一致。data 默认 nil。
func NotFound(msg string) *Error        { return &Error{code: CodeNotFound, msg: msg} }
func BadRequest(msg string) *Error      { return &Error{code: CodeBadRequest, msg: msg} }
func Unauthorized(msg string) *Error    { return &Error{code: CodeUnauthorized, msg: msg} }
func Conflict(msg string) *Error        { return &Error{code: CodeConflict, msg: msg} }
func TooManyRequests(msg string) *Error { return &Error{code: CodeTooManyRequests, msg: msg} }
func Internal(msg string) *Error        { return &Error{code: CodeInternal, msg: msg} }

// Validation 提交校验失败:code=CodeValidation,携带逐题明细 payload 作 data。
// payload 的 wire 形状(如 api.ValidationErrorsPayload{errors:[...]})由调用方给,
// render 原样渲染 → 信封 data:{errors:[...]}。
func Validation(payload any) *Error {
	return &Error{code: CodeValidation, msg: "校验未通过", data: payload}
}

// Forbidden 资源级归属校验失败:对外一律回「不存在」不泄露存在性(与现有 ownedSurvey 语义一致)。
// 用于「跨用户访问他人资源」——非 owner 且非 admin 时,防资源枚举。
func Forbidden() *Error { return &Error{code: CodeNotFound, msg: "不存在"} }

// Forbidden403 平台能力级越权:已登录但角色无权做这类动作(RBAC 第一层 can() 不通过)。
// 与 Forbidden() 分用 —— 该动作本无「资源归属」可言(如非 admin 调账号管理、respondent 调创作端、
// creator 调作答提交),直白告知无权更清晰,且无枚举风险,故用真 403 而非伪装 404。
func Forbidden403(msg string) *Error { return &Error{code: CodeForbidden, msg: msg} }

// FromError 从任意 error 提取 (业务code, 消息)。
// 非 ecode.Error(未预期的内部错误)归 CodeInternal + 通用文案,ok=false 供调用方决定是否记日志。
func FromError(err error) (code int, msg string, ok bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.code, e.msg, true
	}
	return CodeInternal, "内部错误", false
}

// DataFromError 提取 ecode.Error 携带的附加负载(如校验明细);非 ecode.Error 或无负载返回 nil。
// 供 render 渲染进信封 data,render 无需 import 具体错误类型。
func DataFromError(err error) any {
	var e *Error
	if errors.As(err, &e) {
		return e.data
	}
	return nil
}
