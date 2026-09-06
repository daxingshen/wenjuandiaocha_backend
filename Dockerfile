# 星卷后端多阶段镜像。产出 server + seed 应用二进制,并装一个锁定版 goose 二进制
# (替代 Makefile 里的 `go run goose@latest`:消除运行时拉网 + 版本不可复现)。
# 同一镜像既跑迁移/seed(init service),又跑服务(backend service)——见 docker-compose.yml。

# ---- builder ----
# 用 go1.25(对齐 go.mod go 1.25.x);goose v3.27.3 的 go.mod 要 go1.25.7,同线兼容不触发工具链下载。
FROM golang:1.25 AS builder
WORKDIR /src

# 模块代理换国内源(阿里云),避免直连 proxy.golang.org 超时;
# 作用于下方 go mod download 与 go install goose 两处拉网。
ENV GOPROXY=https://goproxy.cn,direct

# 先拷 go.mod/go.sum 拉依赖,利用层缓存(源码变动不必重拉)。
COPY go.mod go.sum ./
RUN go mod download

# 纯 Go、无 CGO(pgx/v5 纯 Go);静态二进制便于精简 runtime 镜像。
ENV CGO_ENABLED=0 GOOS=linux
COPY . .
RUN go build -o /out/server ./cmd/server \
 && go build -o /out/seed   ./cmd/seed

# 锁定版 goose CLI(GOTOOLCHAIN=local 避免它按自身 go.mod 拉 go1.26;go1.25 builder 本地即可编)。
# -tags no_*:goose 默认把八种数据库驱动全编进去,只留 postgres,关掉其余七个。
# 否则会拖入 ClickHouse 依赖树(ch-go 等),代理偶发返回损坏 zip 致 `zip: not a valid zip file`;
# 且这些驱动本就用不到,排除后编译更快、镜像更小。
RUN GOTOOLCHAIN=local go install \
      -tags='no_clickhouse no_mssql no_mysql no_sqlite3 no_libsql no_vertica no_ydb' \
      github.com/pressly/goose/v3/cmd/goose@v3.27.3

# ---- runtime ----
# 静态二进制 + alpine:体积小、带 sh(init service 要串 `goose up && seed`)。
FROM alpine:3.20
WORKDIR /app
# 迁移 SQL 需随镜像分发(goose up 读 db/migrations/)。
COPY --from=builder /out/server /out/seed /app/
COPY --from=builder /go/bin/goose /usr/local/bin/goose
COPY db/migrations /app/db/migrations

EXPOSE 8089
# 默认起服务;init service 在 compose 里覆写 command 跑迁移+seed。
CMD ["/app/server"]
