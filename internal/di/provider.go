// Package di 用 google/wire 编译期装配依赖图(config → 连接池 → dao → service → http)。
// 连接池的创建/ping 收拢在 dao.New,cleanup 经 InitServer 冒泡给 main 关闭。
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
