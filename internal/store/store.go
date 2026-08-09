// store 是数据访问门面:持有 pgxpool,把 sqlc 生成的原子查询(gen 包)组合成业务方法。
// 事务(答卷双写、发布快照)在此手写;单条 SQL 交 sqlc。http 层只认识本包方法,不碰 gen/pgx。
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"wenjuandiaocha_backend/internal/domain"
	"wenjuandiaocha_backend/internal/store/gen"
)

// ErrNotFound 统一的「查无」错误,http 层据此回 404。
var ErrNotFound = errors.New("not found")

// Store 数据访问门面。
type Store struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

// New 用连接池建 Store。
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, q: gen.New(pool)}
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
		ID: respID, SurveyID: surveyID, SurveyVersion: int32(version), Raw: raw, Meta: meta,
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

// Publish 冻结当前草稿为新版本快照,并把 surveys 指向它、status=live。返回新版本号。
func (s *Store) Publish(ctx context.Context, surveyID string, draftSchema []byte) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := s.q.WithTx(tx)
	maxV, err := q.MaxVersion(ctx, surveyID)
	if err != nil {
		return 0, fmt.Errorf("max version: %w", err)
	}
	next := int(maxV) + 1
	// 把快照内 schema.version 盖成分配到的版本号:草稿 JSON 里的 version 恒为 1(前端不递增),
	// 若直接存快照,第二次发布时行版本=2 而 JSON 里=1,提交时 responses.survey_version 取 JSON 值会串版。
	// 在此对齐,保证「行版本 == 快照 JSON version == 落库 survey_version」(版本隔离,约束 5)。
	stamped, err := stampVersion(draftSchema, next)
	if err != nil {
		return 0, fmt.Errorf("stamp version: %w", err)
	}
	if err := q.InsertVersion(ctx, gen.InsertVersionParams{
		SurveyID: surveyID, Version: int32(next), Schema: stamped,
	}); err != nil {
		return 0, fmt.Errorf("insert version: %w", err)
	}
	if err := q.SetPublished(ctx, gen.SetPublishedParams{
		ID: surveyID, PublishedVersion: ptrInt32(int32(next)),
	}); err != nil {
		return 0, fmt.Errorf("set published: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return next, nil
}

func ptrInt32(v int32) *int32 { return &v }

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
