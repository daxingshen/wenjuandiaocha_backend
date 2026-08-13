// seed:建一个初始账号。账号参数用命令行 flag 指定(见 -h);DATABASE_URL 仍走 .env/config。
// 幂等:账号已存在则跳过。gate① 决策 D1:MVP 用 seed 固定账号跑通登录,不做注册端点。
//
// 用法:go run ./cmd/seed -account admin -password admin123 -name 管理员 -role admin
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"wenjuandiaocha_backend/internal/auth"
	"wenjuandiaocha_backend/internal/config"
	"wenjuandiaocha_backend/internal/dao"
)

func main() {
	account := flag.String("account", "admin", "登录账号")
	password := flag.String("password", "admin123", "登录密码")
	name := flag.String("name", "管理员", "显示名")
	role := flag.String("role", "admin", "RBAC 角色:admin|creator|respondent(预置账号默认平台管理员)")
	flag.Parse()

	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("配置加载失败", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("连接 Postgres 失败", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	st := dao.New(pool)

	// 幂等:已存在则跳过。
	if _, err := st.GetUserByAccount(ctx, *account); err == nil {
		slog.Info("seed 账号已存在,跳过", "account", *account)
		return
	}

	hash, err := auth.HashPassword(*password)
	if err != nil {
		slog.Error("密码哈希失败", "err", err)
		os.Exit(1)
	}
	u := dao.User{ID: "u_" + randSuffix(), Account: *account, PasswordHash: hash, Name: *name, Role: *role}
	if err := st.CreateUser(ctx, u); err != nil {
		slog.Error("建账号失败", "err", err)
		os.Exit(1)
	}
	slog.Info("seed 账号已建", "account", *account, "id", u.ID, "role", *role)
}
