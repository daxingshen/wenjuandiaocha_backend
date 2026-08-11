package ecode

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestFromError_ExtractsStatusAndMsg(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		msg    string
		ok     bool
	}{
		{"notfound", NotFound("问卷不存在或未发布"), http.StatusNotFound, "问卷不存在或未发布", true},
		{"badrequest", BadRequest("schema 格式错误"), http.StatusBadRequest, "schema 格式错误", true},
		{"unauthorized", Unauthorized("未登录"), http.StatusUnauthorized, "未登录", true},
		{"conflict", Conflict("仅进行中的问卷可结束"), http.StatusConflict, "仅进行中的问卷可结束", true},
		{"toomany", TooManyRequests("提交过于频繁,请稍后再试"), http.StatusTooManyRequests, "提交过于频繁,请稍后再试", true},
		{"forbidden", Forbidden(), http.StatusNotFound, "不存在", true},
		{"plain", errors.New("boom"), http.StatusInternalServerError, "内部错误", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, msg, ok := FromError(tc.err)
			if status != tc.status || msg != tc.msg || ok != tc.ok {
				t.Fatalf("FromError(%v) = (%d,%q,%v), want (%d,%q,%v)",
					tc.err, status, msg, ok, tc.status, tc.msg, tc.ok)
			}
		})
	}
}

// FromError 须能穿透 fmt.Errorf 的 %w 包裹(errors.As 语义)。
func TestFromError_Wrapped(t *testing.T) {
	wrapped := fmt.Errorf("发布失败: %w", Conflict("仅 live 可结束"))
	status, msg, ok := FromError(wrapped)
	if !ok || status != http.StatusConflict || msg != "仅 live 可结束" {
		t.Fatalf("wrapped FromError = (%d,%q,%v), want (409,\"仅 live 可结束\",true)", status, msg, ok)
	}
}
