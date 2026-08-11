// 星卷后端服务入口。装配层:读 config → 连 pg → 建 store → 挂 router → 起服务。
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"wenjuandiaocha_backend/internal/config"
	"wenjuandiaocha_backend/internal/dao"
	"wenjuandiaocha_backend/internal/domain/qtype"
	xhttp "wenjuandiaocha_backend/internal/server/http"
	svcauth "wenjuandiaocha_backend/internal/service/auth"
	"wenjuandiaocha_backend/internal/service/submission"
	"wenjuandiaocha_backend/internal/service/survey"
)

func main() {
	// dev:载入 .env(存在则),生产靠真实环境变量。
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("配置加载失败", "err", err)
		os.Exit(1)
	}

	// 注册题型 handler(domain 注册表);求值/校验/规范化依赖它。
	qtype.RegisterAll()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("连接 Postgres 失败", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		slog.Error("Postgres ping 失败", "err", err)
		os.Exit(1)
	}

	st := dao.New(pool)
	surveys := survey.New(st)
	submissions := submission.New(st)
	auth := svcauth.New(st, cfg.SessionTTL)
	srv := xhttp.NewServer(surveys, submissions, auth, cfg)

	slog.Info("星卷后端启动", "addr", cfg.HTTPAddr)
	if err := srv.Router().Run(cfg.HTTPAddr); err != nil {
		slog.Error("HTTP 服务退出", "err", err)
		os.Exit(1)
	}
}
