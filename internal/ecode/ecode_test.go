package ecode

import (
	"errors"
	"fmt"
	"testing"
)

func TestFromError_ExtractsCodeAndMsg(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
		msg  string
		ok   bool
	}{
		{"notfound", NotFound("问卷不存在或未发布"), CodeNotFound, "问卷不存在或未发布", true},
		{"badrequest", BadRequest("schema 格式错误"), CodeBadRequest, "schema 格式错误", true},
		{"unauthorized", Unauthorized("未登录"), CodeUnauthorized, "未登录", true},
		{"conflict", Conflict("仅进行中的问卷可结束"), CodeConflict, "仅进行中的问卷可结束", true},
		{"toomany", TooManyRequests("提交过于频繁,请稍后再试"), CodeTooManyRequests, "提交过于频繁,请稍后再试", true},
		{"forbidden", Forbidden(), CodeNotFound, "不存在", true},
		{"forbidden403", Forbidden403("无权限"), CodeForbidden, "无权限", true},
		{"plain", errors.New("boom"), CodeInternal, "内部错误", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, msg, ok := FromError(tc.err)
			if code != tc.code || msg != tc.msg || ok != tc.ok {
				t.Fatalf("FromError(%v) = (%d,%q,%v), want (%d,%q,%v)",
					tc.err, code, msg, ok, tc.code, tc.msg, tc.ok)
			}
		})
	}
}

// FromError 须能穿透 fmt.Errorf 的 %w 包裹(errors.As 语义)。
func TestFromError_Wrapped(t *testing.T) {
	wrapped := fmt.Errorf("发布失败: %w", Conflict("仅 live 可结束"))
	code, msg, ok := FromError(wrapped)
	if !ok || code != CodeConflict || msg != "仅 live 可结束" {
		t.Fatalf("wrapped FromError = (%d,%q,%v), want (%d,\"仅 live 可结束\",true)", code, msg, ok, CodeConflict)
	}
}

// Code() 暴露业务码供传输层信封使用。
func TestError_Code(t *testing.T) {
	if got := Conflict("x").Code(); got != CodeConflict {
		t.Fatalf("Conflict.Code() = %d, want %d", got, CodeConflict)
	}
}
