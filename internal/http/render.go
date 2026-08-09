// 统一响应/错误封装。成功走 gin JSON;错误走一致的 { error } 或 { errors } 形状。
package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/internal/domain"
)

// errForbidden 内部哨兵:归属校验失败(响应已写),供调用方判断是否已终止。
var errForbidden = errString("forbidden")

type errString string

func (e errString) Error() string { return string(e) }

// fail 统一错误响应:{ "error": "..." }。
func fail(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": msg})
}

// failValidation 校验失败:400 + { "errors": [{qid,message}...] }(对齐前端消费形状)。
func failValidation(c *gin.Context, errs []domain.ValidationError) {
	c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"errors": errs})
}
