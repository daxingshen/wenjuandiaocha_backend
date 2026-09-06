// 星卷后端服务入口。装配层:读 config → 连 pg → di.InitServer(wire 组装) → 起服务。
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/joho/godotenv"

	"wenjuandiaocha_backend/internal/di"
	"wenjuandiaocha_backend/internal/domain/qtype"
)

func main() {
	// dev:载入 .env(存在则),生产靠真实环境变量。
	_ = godotenv.Load()

	// 注册题型 handler(domain 注册表);求值/校验/规范化依赖它。
	qtype.RegisterAll()

	// wire 组装整个依赖图(config → 连接池 → dao → 3 managers → http.Server)。
	// cleanup 关连接池;config 缺项 / 连库失败在此冒泡。
	ctx := context.Background()
	srv, cleanup, err := di.InitServer(ctx)
	if err != nil {
		slog.Error("装配失败", "err", err)
		os.Exit(1)
	}
	defer cleanup()

	slog.Info("星卷后端启动", "addr", srv.Addr())
	if err := srv.Router().Run(srv.Addr()); err != nil {
		slog.Error("HTTP 服务退出", "err", err)
		os.Exit(1)
	}
}
