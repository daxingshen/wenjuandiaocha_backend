//go:build wireinject
// +build wireinject

package di

import (
	"context"

	"github.com/google/wire"

	"wenjuandiaocha_backend/internal/config"
	"wenjuandiaocha_backend/internal/dao"
	xhttp "wenjuandiaocha_backend/internal/server/http"
	svcauth "wenjuandiaocha_backend/internal/service/auth"
	"wenjuandiaocha_backend/internal/service/submission"
	"wenjuandiaocha_backend/internal/service/survey"
)

// InitServer 组装整个依赖图:config(读环境)→ 连接池 → dao → 3 managers → *http.Server。
// 返回 cleanup(关连接池)与 error(config 缺项 / 连库失败),由 main 负责调用/退出。
func InitServer(ctx context.Context) (*xhttp.Server, func(), error) {
	wire.Build(
		config.ProviderSet,
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
	return nil, nil, nil
}
