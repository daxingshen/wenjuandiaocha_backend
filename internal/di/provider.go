// Package di 用 google/wire 编译期装配依赖图。
// pool 的创建/ping/close 是生命周期关切,留在 cmd/server/main;wire 组装 pool 下游。
package di

import (
	"time"

	"wenjuandiaocha_backend/internal/config"
)

// provideSessionTTL 从 config 抽出 auth manager 需要的会话 TTL。
// (wire 按类型注入,config.Config 无法直接满足 time.Duration 参数,故显式提供。)
func provideSessionTTL(cfg config.Config) time.Duration {
	return cfg.SessionTTL
}
