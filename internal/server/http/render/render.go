// Package render 是传输层统一响应/错误封装。成功走 gin JSON;错误走一致的
// { error } 或 { errors } 形状。抽成独立包,供 http handler 与各中间件共用,
// 避免「中间件 → http 包」的 import 环。
package render

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/internal/domain"
	"wenjuandiaocha_backend/internal/ecode"
)

// Fail 统一错误响应:{ "error": "..." }。用于传输层自身错误(bind 失败等)。
func Fail(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": msg})
}

// Error 把 service 层返回的 error 映射为 HTTP 响应:
// ecode.Error → 其携带的状态码 + 消息;非预期错误 → 500 + 通用文案并记日志。
func Error(c *gin.Context, err error) {
	status, msg, ok := ecode.FromError(err)
	if !ok {
		slog.Error("service error", "err", err, "path", c.Request.URL.Path)
	}
	Fail(c, status, msg)
}

// Validation 校验失败:400 + { "errors": [{qid,message}...] }(对齐前端消费形状)。
func Validation(c *gin.Context, errs []domain.ValidationError) {
	c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"errors": errs})
}
