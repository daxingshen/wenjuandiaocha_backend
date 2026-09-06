// Package config 从环境变量读取服务配置(12-factor)。dev 下 .env 由 main 用 godotenv 载入。
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/google/wire"
)

// ProviderSet 供 wire 组装:直接把 Load 作为 config.Config 的 provider(缺关键项时注入报错)。
var ProviderSet = wire.NewSet(Load)

// Config 服务配置。
type Config struct {
	DatabaseURL  string
	HTTPAddr     string
	SessionTTL   time.Duration
	CookieSecure bool // 生产置 true(仅 HTTPS 下发 cookie)
}

// Load 读环境变量,缺关键项报错。
func Load() (Config, error) {
	c := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		// 监听端口用纯数字 HTTP_PORT(默认 8089),内部拼成 gin 要的 ":port" 地址。
		// 用纯数字而非带冒号的地址:同一个值能在 docker-compose 里直接当端口映射用,不会踩 "8089::8089"。
		HTTPAddr: ":" + getenv("HTTP_PORT", "8089"),
	}
	if c.DatabaseURL == "" {
		return Config{}, fmt.Errorf("缺 DATABASE_URL")
	}
	ttlHours := 168
	if v := os.Getenv("SESSION_TTL_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			ttlHours = n
		}
	}
	c.SessionTTL = time.Duration(ttlHours) * time.Hour
	c.CookieSecure = os.Getenv("COOKIE_SECURE") == "true"
	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
