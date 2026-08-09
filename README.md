# 星卷问卷系统 · 后端(wenjuandiaocha_backend)

Go 单体服务:问卷 CRUD + 公开取 schema + 匿名作答提交(后端权威校验)+ 账密登录。前端 `../wenjuandiaocha_ui` 通过 HTTP 接入。

架构与决策见 wiki:`../wenjuandiaocha_wiki/PRD/backend-integration/`(03-design.md 含分层、DDL、API 契约)。工作约定见 `CLAUDE.md`。

## 技术栈

Go 1.25 · gin · sqlc + pgx/v5 · goose(迁移)· PostgreSQL 17 · bcrypt + DB-backed session cookie。

## 快速开始

```bash
cp .env.example .env          # 按需改;默认 pg 映射到本机 5433(避开 5432 已有实例)
make db-up                    # docker 起 postgres 17
make migrate                  # 建表(goose 跑 db/migrations/001_init.sql)
make seed                     # 建初始账号(默认 admin / admin123,读 .env SEED_*)
make run                      # 起服务 → :8080
```

前后端联调:后端 `make run`,前端 `pnpm dev:studio`(5173)/ `pnpm dev:runtime`(5174)。
两个 vite 已配 `/api` 代理到 `:8080`,同源带 cookie、免 CORS。studio 用 seed 账号登录。

## 开发命令

```bash
make sqlc        # 改了 queries/*.sql 或迁移后,重新生成 internal/store/gen/
make test        # go test ./...(含黄金向量 + 6 题型对拍)
make vet
make migrate-down # 回滚最近一个迁移
```

> sqlc 锁 v1.29.0 + `GOTOOLCHAIN=local`(v1.31 要 go1.26;本机 go1.25)。Makefile 已处理。

## API

base = `/api`。字段严格对齐前端 `packages/engine/src/schema.ts`(前端无适配层)。

### public(匿名公开,runtime 用)

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/public/surveys/:id` | 取已发布版本快照 `SurveySchema`;未发布/不存在 404 |
| POST | `/api/public/surveys/:id/answers` | body `{answers}`;后端权威校验+落库;校验失败 400 `{errors:[{qid,message}]}` |

> **无鉴权**(匿名作答,决策 7)。提交端点做 IP 限频(内存令牌桶,1/s 突发 10)防滥用。
> 「同 IP/微信限答次数、密码访问、时长下限」是 PRD §4.3 范围,本轮埋 `responses.meta` 字段未实现。

### auth / studio(需登录,session cookie)

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/auth/login` | `{account,password}` → `{id,name,level}` + Set-Cookie `sid` |
| POST | `/api/auth/logout` | 删 session + 清 cookie |
| GET | `/api/auth/me` | 当前用户;未登录 401 |
| GET | `/api/surveys` | 本人问卷列表 `[{id,title,type,status,updatedAt}]` |
| POST | `/api/surveys` | 建空草稿 → `{id}` |
| GET | `/api/surveys/:id` | 草稿 `SurveySchema`(供编辑);非本人 404 |
| PUT | `/api/surveys/:id` | 存草稿(整份 schema);非本人 404 |
| POST | `/api/surveys/:id/publish` | 冻结草稿为新版本快照 + status=live → `{version}` |

## 安全底线(决策 6)

提交答卷时,后端用**自己载入的发布版 schema** 完整重跑 `Evaluate → Validate → Normalize`,
**永不信任客户端**:客户端多传的隐藏题答案由后端求值剔除,落库规范化行以后端为准。
求值器与前端行为靠 `internal/domain/golden_test.go` 读**前端那份** `golden-vectors.json` 锁一致。

## 目录

```
cmd/server/  服务入口(装配)         cmd/seed/  建初始账号
internal/
  domain/    纯函数核心:schema/logic/validate/normalize + qtype/(6 题型)
  http/      gin handler + 中间件 + 限频 + id
  store/     queries/*.sql + gen/(sqlc)+ store.go(门面+事务)
  auth/      bcrypt + session token       config/  env 读取
db/migrations/  goose 建表 SQL
```
