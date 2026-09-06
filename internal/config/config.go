// Package config 从环境变量读取服务配置(12-factor)。dev 下 .env 由 main 用 godotenv 载入。
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

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
		HTTPAddr:    getenv("HTTP_ADDR", ":18080"),
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
