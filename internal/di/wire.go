//go:build wireinject
// +build wireinject

package di

import (
	"github.com/google/wire"
	"github.com/jackc/pgx/v5/pgxpool"

	"wenjuandiaocha_backend/internal/config"
	"wenjuandiaocha_backend/internal/dao"
	xhttp "wenjuandiaocha_backend/internal/server/http"
	svcauth "wenjuandiaocha_backend/internal/service/auth"
	"wenjuandiaocha_backend/internal/service/submission"
	"wenjuandiaocha_backend/internal/service/survey"
)

// InitServer 从连接池 + config 组装出 *http.Server。
// pool 的生命周期(创建/ping/close)由调用方(main)负责,不进 wire。
func InitServer(pool *pgxpool.Pool, cfg config.Config) *xhttp.Server {
	wire.Build(
		provideSessionTTL,
		dao.ProviderSet,
		survey.ProviderSet,
		submission.ProviderSet,
		svcauth.ProviderSet,
		xhttp.ProviderSet,
		// *dao.Store 绑定到各 service 的 Store 接口(与 dao.ProviderSet 同作用域)。
		wire.Bind(new(survey.Store), new(*dao.Store)),
		wire.Bind(new(submission.Store), new(*dao.Store)),
		wire.Bind(new(svcauth.Store), new(*dao.Store)),
	)
	return nil
}
