// Package ecode 定义业务错误码:携带对外消息 + HTTP 状态。
// service 层返回 ecode.Error,server/http 层用 FromError 统一映射状态码,
// 取代散落在 handler 里的 fail(c, 4xx, msg)。用 errors.As 提取。
package ecode

import (
	"errors"
	"net/http"
)

// Error 是带 HTTP 状态的业务错误。消息对外可见,状态决定 HTTP code。
type Error struct {
	status int
	msg    string
}

func (e *Error) Error() string { return e.msg }

// Status 返回对应 HTTP 状态码。
func (e *Error) Status() int { return e.status }

// 构造器:每个对应一类业务错误。消息由调用方给,保持与现有对外文案一致。
func NotFound(msg string) *Error        { return &Error{http.StatusNotFound, msg} }
func BadRequest(msg string) *Error      { return &Error{http.StatusBadRequest, msg} }
func Unauthorized(msg string) *Error    { return &Error{http.StatusUnauthorized, msg} }
func Conflict(msg string) *Error        { return &Error{http.StatusConflict, msg} }
func TooManyRequests(msg string) *Error { return &Error{http.StatusTooManyRequests, msg} }
func Internal(msg string) *Error        { return &Error{http.StatusInternalServerError, msg} }

// Forbidden 归属校验失败:对外一律回 404 不泄露存在性(与现有 ownedSurvey 语义一致)。
func Forbidden() *Error { return &Error{http.StatusNotFound, "不存在"} }

// FromError 从任意 error 提取 (HTTP状态, 消息)。
// 非 ecode.Error(未预期的内部错误)归 500 + 通用文案,ok=false 供调用方决定是否记日志。
func FromError(err error) (status int, msg string, ok bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.status, e.msg, true
	}
	return http.StatusInternalServerError, "内部错误", false
}
