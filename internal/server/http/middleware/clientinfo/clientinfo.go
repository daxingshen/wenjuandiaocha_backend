// Package clientinfo 提供客户端信息中间件:每个请求都把 ip/ua 注入 ctx 的 api.Metadata。
// 不限鉴权端点;后续中间件/handler 在此基础上「补充」身份等字段(enrich 不覆盖)。
package clientinfo

import (
	"github.com/gin-gonic/gin"

	"wenjuandiaocha_backend/api"
)

// New 返回客户端信息中间件。
func New() gin.HandlerFunc {
	return func(c *gin.Context) {
		md := api.MetadataFrom(c.Request.Context())
		md.ClientIP = c.ClientIP()
		md.UserAgent = c.Request.UserAgent()
		c.Request = c.Request.WithContext(api.WithMetadata(c.Request.Context(), md))
		c.Next()
	}
}
