# 星卷后端 · 常用命令。变量从 .env 读(存在则 include)。
ifneq (,$(wildcard .env))
include .env
export
endif

GOOSE_DRIVER ?= postgres
GOOSE_DBSTRING ?= $(DATABASE_URL)
MIGRATIONS := db/migrations

.PHONY: help db-up db-down migrate migrate-down sqlc run seed test vet tidy

help:
	@echo "db-up        起 postgres(docker compose)"
	@echo "db-down      停 postgres"
	@echo "migrate      应用迁移(建表)"
	@echo "migrate-down 回滚最近一个迁移"
	@echo "sqlc         从 SQL 生成 Go(internal/store/gen)"
	@echo "run          起 HTTP 服务"
	@echo "seed         建 seed 账号(读 .env)"
	@echo "test         go test ./..."
	@echo "vet          go vet ./..."

db-up:
	docker compose up -d db

db-down:
	docker compose down

migrate:
	go run github.com/pressly/goose/v3/cmd/goose@latest -dir $(MIGRATIONS) $(GOOSE_DRIVER) "$(GOOSE_DBSTRING)" up

migrate-down:
	go run github.com/pressly/goose/v3/cmd/goose@latest -dir $(MIGRATIONS) $(GOOSE_DRIVER) "$(GOOSE_DBSTRING)" down

# sqlc v1.29 是最后一批支持 go1.25 工具链的版本;GOTOOLCHAIN=local 避免它拉 go1.26
sqlc:
	GOTOOLCHAIN=local go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate

run:
	go run ./cmd/server

seed:
	go run ./cmd/seed

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy
