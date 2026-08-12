// Package render 是传输层统一响应/错误封装。
//
// 业务响应走统一信封:HTTP 状态恒 200,body = { code, message, data }。
//   - 成功:Success → { code:0, message:"", data:<负载> }
//   - service 抛的业务错(ecode):Error → { code:<业务码>, message:<文案>, data:null }
//   - 提交校验失败:Validation → { code:CodeValidation, message, data:{errors:[...]} }
//
// 传输层自身失败(bind 失败、限频、未登录、读体失败)走 Fail,保留原生非-200 HTTP
// 状态,不进信封——这类是框架层拒绝,前端按 HTTP status 分级。
//
// 抽成独立包供 http handler 与各中间件共用,避免「中间件 → http 包」的 import 环。
package render

import (
	"net/http"

	"log/slog"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/internal/domain"
	"wenjuandiaocha_backend/internal/ecode"
)

// envelope 是统一响应信封。data 用 any:成功负载 / nil / {errors:[...]}。
// 裸 schema 端点传 json.RawMessage,原样嵌入不二次转义。
type envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// Success 成功响应:200 + { code:0, message:"", data }。占位成功负载传 nil → data:null。
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, envelope{Code: ecode.CodeOK, Message: "", Data: data})
}

// Fail 传输层自身失败(bind、限频、未登录等):保留原生 HTTP 状态,{ "error": msg }。
// 不进信封——前端按 HTTP status 分级(429 限频 / 401 未登录 / 400 请求错)。
func Fail(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": msg})
}

// Error 把 service 层返回的 error 映射为业务信封:200 + { code, message, data:null }。
// ecode.Error → 其携带的业务 code + 消息;非预期错误 → CodeInternal + 通用文案并记日志。
func Error(c *gin.Context, err error) {
	code, msg, ok := ecode.FromError(err)
	if !ok {
		slog.Error("service error", "err", err, "path", c.Request.URL.Path)
	}
	// 中间件里 Error 需终止后续 handler,用 AbortWithStatusJSON 保持恒 200 信封语义。
	c.AbortWithStatusJSON(http.StatusOK, envelope{Code: code, Message: msg, Data: nil})
}

// Validation 提交校验失败:200 + { code:CodeValidation, data:{errors:[{qid,message}...]} }。
func Validation(c *gin.Context, errs []domain.ValidationError) {
	c.JSON(http.StatusOK, envelope{
		Code:    ecode.CodeValidation,
		Message: "校验未通过",
		Data:    gin.H{"errors": errs},
	})
}
