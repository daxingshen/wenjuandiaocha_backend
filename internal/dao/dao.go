// dao 是数据访问门面:持有 pgxpool,把 sqlc 生成的原子查询(gen 包)组合成业务方法。
// 事务(答卷双写、发布快照)在此手写;单条 SQL 交 sqlc。上层只认识本包方法,不碰 gen/pgx。
package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/google/wire"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"wenjuandiaocha_backend/internal/config"
	"wenjuandiaocha_backend/internal/dao/gen"
	"wenjuandiaocha_backend/internal/domain"
)

// ErrNotFound 统一的「查无」错误,http 层据此回 404。
var ErrNotFound = errors.New("not found")

// ProviderSet 供 wire 组装:从 config 建连接池并组装 *Store(New)。
var ProviderSet = wire.NewSet(New)

// Store 数据访问门面。
type Store struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

// New 从 config 建连接池、ping 探活,再组装 *Store。连接池的创建/ping/close 收拢在本包:
// 上层只认 *Store,不碰 pgxpool。返回 cleanup(关池)供 wire 汇总;连接失败即报错冒泡给调用方。
func New(ctx context.Context, cfg config.Config) (*Store, func(), error) {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("连接 Postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("Postgres ping: %w", err)
	}
	return &Store{pool: pool, q: gen.New(pool)}, func() { pool.Close() }, nil
}

// Pool 暴露底层池(供健康检查/关闭)。
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// --------- 答卷提交:双写(responses + answer_rows),单事务 ---------

// SaveSubmission 在一个事务里写完整答卷 + N 条规范化行。要么全成,要么全滚。
// raw 是后端权威版完整答案 JSON;rows 是后端 Normalize 的结果。
func (s *Store) SaveSubmission(ctx context.Context, respID, surveyID string, version int, raw, meta []byte, rows []domain.NormalizedRow) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // commit 后 rollback 是 no-op

	q := s.q.WithTx(tx)
	if err := q.InsertResponse(ctx, gen.InsertResponseParams{
		ResponseID: respID, SurveyID: surveyID, SurveyVersion: int32(version), Raw: raw, Meta: meta,
	}); err != nil {
		return fmt.Errorf("insert response: %w", err)
	}
	for _, r := range rows {
		p := gen.InsertAnswerRowParams{
			ResponseID: respID, SurveyID: surveyID, SurveyVersion: int32(version), Qid: r.QID,
		}
		if r.SubID != "" {
			sub := r.SubID
			p.SubID = &sub
		}
		// value 分流:scale 是数字 → value_num;其余是字符串 → value_text。
		switch v := r.Value.(type) {
		case float64:
			n := v
			p.ValueNum = &n
		case string:
			t := v
			p.ValueText = &t
		}
		if err := q.InsertAnswerRow(ctx, p); err != nil {
			return fmt.Errorf("insert answer row: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// --------- 发布:快照 draft_schema → survey_versions + 更新指针,单事务 ---------

// Publish 冻结当前草稿为新版本快照,并把 surveys 指向它、status=live、写入作答访问模式。返回 (新版本号, 是否免发).
// 发布只冻结版本 + 转 live,不碰 answer_access —— 作答模式是 draft 阶段经 SetAnswerAccess 设定的独立列,
// 单一真相源,发布不动它(消除发布与作答模式的双写)。
// 免发(unchanged=true):待发布草稿与「当前对外版本」内容一致时,不造新版本、不动指针,
// 返回当前版本号——避免重新发布空转出无意义的版本膨胀。首发(无历史版)不判等,照常发。
func (s *Store) Publish(ctx context.Context, surveyID string, draftSchema []byte) (int, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := s.q.WithTx(tx)
	maxV, err := q.MaxVersion(ctx, surveyID)
	if err != nil {
		return 0, false, fmt.Errorf("max version: %w", err)
	}
	// 免发判等:publish 每次 version+1 且 published_version=新 max,close/reopen 不造版本,
	// 故 maxV 版快照 == 当前对外快照。与草稿语义判等(忽略 version 字段——发布必改写它,是设计性差异)。
	if maxV > 0 {
		curr, err := q.GetVersionSchema(ctx, gen.GetVersionSchemaParams{SurveyID: surveyID, Version: maxV})
		if err != nil {
			return 0, false, fmt.Errorf("get current version: %w", err)
		}
		same, err := schemaEqualIgnoringVersion(draftSchema, curr)
		if err != nil {
			return 0, false, fmt.Errorf("compare schema: %w", err)
		}
		if same {
			// 内容未变:不插新版、不动指针。返回当前版本号 + unchanged。
			return int(maxV), true, nil
		}
	}
	next := int(maxV) + 1
	// 把快照内 schema.version 盖成分配到的版本号:草稿 JSON 里的 version 恒为 1(前端不递增),
	// 若直接存快照,第二次发布时行版本=2 而 JSON 里=1,提交时 responses.survey_version 取 JSON 值会串版。
	// 在此对齐,保证「行版本 == 快照 JSON version == 落库 survey_version」(版本隔离,约束 5)。
	stamped, err := stampVersion(draftSchema, next)
	if err != nil {
		return 0, false, fmt.Errorf("stamp version: %w", err)
	}
	if err := q.InsertVersion(ctx, gen.InsertVersionParams{
		SurveyID: surveyID, Version: int32(next), Schema: stamped,
	}); err != nil {
		return 0, false, fmt.Errorf("insert version: %w", err)
	}
	if err := q.SetPublished(ctx, gen.SetPublishedParams{
		SurveyID: surveyID, PublishedVersion: ptrInt32(int32(next)),
	}); err != nil {
		return 0, false, fmt.Errorf("set published: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, err
	}
	return next, false, nil
}

func ptrInt32(v int32) *int32 { return &v }

// schemaEqualIgnoringVersion 语义判等两份 schema JSON,忽略 version 字段
// (发布时 stampVersion 必然改写 version,属设计性差异,不算内容变化)。
// 用 map 判等而非字节比较:草稿是前端 PUT 的原样 body,快照是发布时 re-marshal 的,字节序/空白可能不同。
func schemaEqualIgnoringVersion(a, b []byte) (bool, error) {
	var ma, mb map[string]any
	if err := json.Unmarshal(a, &ma); err != nil {
		return false, err
	}
	if err := json.Unmarshal(b, &mb); err != nil {
		return false, err
	}
	delete(ma, "version")
	delete(mb, "version")
	return reflect.DeepEqual(ma, mb), nil
}

// stampVersion 把 schema JSON 的 version 字段改写为 v,其余字段原样保留。
func stampVersion(schemaJSON []byte, v int) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(schemaJSON, &m); err != nil {
		return nil, err
	}
	m["version"] = v
	return json.Marshal(m)
}

// notFound 把 pgx.ErrNoRows 归一成 ErrNotFound。
func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
