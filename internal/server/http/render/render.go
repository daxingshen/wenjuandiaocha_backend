// Package render 是传输层统一响应/错误封装。
//
// 对外只暴露两个出口(成功/业务错的裁决收敛在此,handler 不再自行分派):
//   - JSON(c, data, err):业务响应统一出口,HTTP 状态恒 200,body = { code, message, data }。
//       · err == nil          → { code:0, message:"", data:<负载> }
//       · err 为 ecode.Error  → { code:<业务码>, message:<文案>, data:<err 携带的 data 或 null> }
//                                (校验失败即经此:ecode.Validation 携 {errors:[...]} → data:{errors:[...]})
//       · err 非 ecode.Error  → { code:CodeInternal, message:"内部错误", data:null } 并记日志
//   - Fail(c, status, msg):传输层自身失败(bind 失败、限频、未登录、读体失败)——保留原生
//       非-200 HTTP 状态,body = { "error": msg },不进信封。前端按 HTTP status 分级。
//
// render 与业务无关:JSON 从 err 取出 ecode.Error.Data() 原样渲染,不解析其结构(如 errors 明细
// 的形状归 api 层定义)。抽成独立包供 http handler 与各中间件共用,避免「中间件 → http 包」import 环。
package render

import (
	"net/http"

	"log/slog"

	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/internal/ecode"
)

// envelope 是统一响应信封。data 用 any:成功负载 / nil / err 携带的附加负载(如 {errors:[...]})。
// 裸 schema 端点传 json.RawMessage,原样嵌入不二次转义。
type envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// JSON 业务响应统一出口:恒 200 信封。成功/业务错的裁决在此,handler 只交出 (data, err)。
//   - err == nil:{ code:0, message:"", data }。占位成功负载传 nil → data:null。
//   - err != nil:经 ecode.FromError 取 (code,msg);若为 ecode.Error 且携带 data,原样渲染进 data;
//                 非 ecode.Error(未预期内部错误)归 CodeInternal + 通用文案并记日志。
//
// 用 AbortWithStatusJSON:业务错在中间件里触发时需终止后续 handler,同时保持恒 200 信封语义。
func JSON(c *gin.Context, data any, err error) {
	if err == nil {
		c.JSON(http.StatusOK, envelope{Code: ecode.CodeOK, Message: "", Data: data})
		return
	}
	code, msg, ok := ecode.FromError(err)
	if !ok {
		slog.Error("service error", "err", err, "path", c.Request.URL.Path)
	}
	// 业务错默认 data:null;ecode.Error 携带附加负载时(如校验明细)原样渲染。
	c.AbortWithStatusJSON(http.StatusOK, envelope{Code: code, Message: msg, Data: ecode.DataFromError(err)})
}

// Fail 传输层自身失败(bind、限频、未登录等):保留原生 HTTP 状态,{ "error": msg }。
// 不进信封——前端按 HTTP status 分级(429 限频 / 401 未登录 / 400 请求错)。
func Fail(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": msg})
}
