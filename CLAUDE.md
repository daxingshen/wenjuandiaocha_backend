# CLAUDE.md — 星卷问卷系统 后端(wenjuandiaocha_backend)

给 AI 助手的工作指南。本仓是星卷问卷系统的 Go 后端服务。

## 项目一句话

单体 Go 服务:**框架无关的 `internal/domain` 纯函数核心**(问卷类型 + 逻辑求值 + 校验 + 规范化 + 6 题型 handler),外面分层包着:`server/http`(gin 传输)→ `service`(业务)→ `dao`(sqlc/pgx),`di` 用 google/wire 组装,`api` 存 proto 信封契约。domain 是前端 `wenjuandiaocha_ui/packages/engine` 的后端孪生。

设计的**结论与理由**在 wiki:`../wenjuandiaocha_wiki/PRD/backend-integration/`(01 需求 / 02 事实 / 03 方案含 DDL / 04 任务)。动手前先读 03-design.md。

## 仓库关系

- `../wenjuandiaocha_ui` — 前端(engine 是共享契约的权威侧)
- `../wenjuandiaocha_wiki` — 知识库(PRD / 契约 / 本轮 PRD 流水线产物)
- 本仓 = 后端

## 技术栈(gate① 确认)

Go 1.25 · gin · sqlc + pgx/v5 · goose(迁移)· PostgreSQL 17 · bcrypt + DB-backed session cookie。不用公司内部库。`GOPROXY=goproxy.bilibili.co`(仅代理公共模块)。

## 核心不变量(改代码时必须守住)

1. **domain 纯函数、零框架依赖**:`internal/domain` 不 import gin/pgx。逻辑求值/校验/规范化是纯函数,传输与 DB 是外壳。依赖只从外向内(server/http → service → dao → domain,domain 谁都不认识)。
2. **黄金向量即跨语言契约**:`internal/domain` 的求值器必须与前端 `packages/engine/src/logic.ts` 行为逐位一致。`golden_test.go` 读**前端那份** `../wenjuandiaocha_ui/packages/engine/src/__tests__/golden-vectors.json`(单一真相源,不复制进本仓)。改逻辑语义先改那份 JSON,再改两端实现。
3. **永不信任客户端**:public 提交端点必须用后端载入的**发布版 schema** 完整重跑 Evaluate→Validate→Normalize;客户端传的隐藏题答案由后端求值剔除,落库规范化行以后端为准(frontend.md 决策 6 安全底线)。
4. **必答判断双层**:multi-choice / matrix-single 的 required 判断在各自 handler 内(空数组/空对象在通用层算「已答」),不在通用 ValidateSurvey。照抄前端 handler 结构,别只在通用层判 required。
5. **jsonb 整存 schema**:问卷整份 SurveySchema 存 `surveys.draft_schema` / `survey_versions.schema`(jsonb)。dao 层当 []byte 转发,不解析;只有 domain 在求值时解析。加题型零 DDL。
6. **service I/O 一处定义、传输无关**:业务输入输出类型只在各 service 包定义(`XReq`/`XResp`),`Service` 接口是契约;传输层(现 http、未来 gRPC)只做「本传输格式 ↔ service 类型」翻译,**不得另立业务 I/O**。调用方身份(ownerID)由传输层取得后填入 Req,不进 domain/service 的隐式 ctx。HTTP 响应 struct 的 json tag(逐字节对齐前端)属传输层关切,留 server/http。schema 主体仍以 `[]byte` 穿过 service,不拆字段。

## 仓库结构

分层对齐 prompt_hub(见 wiki `PRD/backend-dir-restructure/`):依赖只从外向内 `server/http → service → dao → gen`,`service/dao → domain`,`di` 组装全部,`lib` 被各层用不反向依赖。

```
api/              proto 信封契约:api.proto + api.pb.go(buf generate,勿手改)
                  只建模信封(登录/发布/统计/列表项);SurveySchema 主体仍 []byte 透传
cmd/server/       main:config→pgxpool→di.InitServer(wire)→起服务
cmd/seed/         seed 账号(读 .env)
internal/
  domain/         ★纯函数:schema.go / logic.go / validate.go / normalize.go / qtype/
  server/http/    gin handler(bind→调 service→render)+ 中间件 + render
  service/        业务层:survey/ submission/ auth/。每包暴露 Service 接口 + 统一 Req/Resp
                  (Method(ctx,XReq)(XResp,err)),I/O 类型一处定义;传输层持接口、只做格式翻译
  dao/            queries/*.sql(手写)+ gen/(sqlc 生成,勿手改)+ dao.go(门面+事务)
  di/             google/wire 组装:wire.go(wireinject)+ wire_gen.go(生成,勿手改)
  ecode/          业务错误码(带 HTTP 状态);service 返回,server/http 用 FromError 映射
  lib/            通用原语:id/(短 id) ratelimit/(令牌桶)
  auth/           bcrypt + session token(纯密码学原语)
  config/         env 读取
db/migrations/    goose 建表 SQL(001_init.sql)
buf.yaml buf.gen.yaml sqlc.yaml docker-compose.yml Makefile .env.example
```

生成物三处,改源后重跑:sqlc(`make sqlc`,改 queries/迁移)、proto(`make proto`,改 api.proto)、wire(`make wire`,改 provider set)。

## 本地开发

```bash
cp .env.example .env       # 按需改
make db-up                 # 起 postgres(docker,本机已有 pg17 镜像)
make migrate               # 建表
make sqlc                  # 从 SQL 生成 Go(改了 queries/迁移后跑)
make seed                  # 建 seed 账号
make run                   # 起服务 :8080
make test                  # go test ./...(含黄金向量)
make vet
```

改动后至少跑 `make vet && make test`;动了 domain 求值/校验/规范化,黄金向量 + 对拍测试必须绿。

## API(对齐前端已写死形状,见 wiki 03-design §4)

- public(匿名):`GET /api/public/surveys/:id`、`POST /api/public/surveys/:id/answers`。
- studio(session cookie):`/api/auth/{login,logout,me}`、`/api/surveys`(GET/POST)、`/api/surveys/:id`(GET/PUT)、`/api/surveys/:id/publish`。

wire 字段名严格对齐前端 `packages/engine/src/schema.ts`,前端无适配层。

## 工程约定

- 中文写作(注释/文档),与前端仓一致。
- sqlc 生成物在 `internal/dao/gen/`,**不手改**;改查询改 `queries/*.sql` 后 `make sqlc`。
- 事务(双写、发布快照)在 `dao.go` 手写,单条 SQL 交 sqlc。
- 无鉴权的 public 端点在代码/README 显式标注公开 + 防滥用现状。
