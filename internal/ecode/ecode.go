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
	CodeNotFound        = 40401 // 不存在 / 非本人 / 未发布(原 404,含 Forbidden)
	CodeValidation      = 42201 // 提交校验失败(逐题明细走 data.errors)
	CodeTooManyRequests = 42901 // 限频(原 429)
	CodeConflict        = 40901 // 状态机非法跳转(原 409)
	CodeInternal        = 50001 // 未预期内部错误(原 500)
)

// Error 是带业务 code 的错误。消息对外可见,code 进响应信封。
type Error struct {
	code int
	msg  string
}

func (e *Error) Error() string { return e.msg }

// Code 返回业务错误码。
func (e *Error) Code() int { return e.code }

// 构造器:每个对应一类业务错误。消息由调用方给,保持与现有对外文案一致。
func NotFound(msg string) *Error        { return &Error{CodeNotFound, msg} }
func BadRequest(msg string) *Error      { return &Error{CodeBadRequest, msg} }
func Unauthorized(msg string) *Error    { return &Error{CodeUnauthorized, msg} }
func Conflict(msg string) *Error        { return &Error{CodeConflict, msg} }
func TooManyRequests(msg string) *Error { return &Error{CodeTooManyRequests, msg} }
func Internal(msg string) *Error        { return &Error{CodeInternal, msg} }

// Forbidden 归属校验失败:对外一律回「不存在」不泄露存在性(与现有 ownedSurvey 语义一致)。
func Forbidden() *Error { return &Error{CodeNotFound, "不存在"} }

// FromError 从任意 error 提取 (业务code, 消息)。
// 非 ecode.Error(未预期的内部错误)归 CodeInternal + 通用文案,ok=false 供调用方决定是否记日志。
func FromError(err error) (code int, msg string, ok bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.code, e.msg, true
	}
	return CodeInternal, "内部错误", false
}
