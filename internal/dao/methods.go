// store 的非事务业务方法:用户 / 会话 / 问卷 CRUD / 发布快照读取。
// 直接委托 sqlc 生成的查询,归一「查无」错误。
package dao

import (
	"context"
	"time"

	"wenjuandiaocha_backend/internal/dao/gen"
)

// ---------- 用户 ----------

type User struct {
	ID           string
	Account      string
	PasswordHash string
	Name         string
	Level        string
	Role         string // 账号角色 admin|creator|respondent(RBAC 角色轴,正交于 Level)
}

func (s *Store) CreateUser(ctx context.Context, u User) error {
	return s.q.CreateUser(ctx, gen.CreateUserParams{
		ID: u.ID, Account: u.Account, PasswordHash: u.PasswordHash, Name: u.Name, Level: u.Level, Role: u.Role,
	})
}

func (s *Store) GetUserByAccount(ctx context.Context, account string) (User, error) {
	r, err := s.q.GetUserByAccount(ctx, account)
	if err != nil {
		return User{}, notFound(err)
	}
	return User{ID: r.ID, Account: r.Account, PasswordHash: r.PasswordHash, Name: r.Name, Level: r.Level, Role: r.Role}, nil
}

func (s *Store) GetUserByID(ctx context.Context, id string) (User, error) {
	r, err := s.q.GetUserByID(ctx, id)
	if err != nil {
		return User{}, notFound(err)
	}
	return User{ID: r.ID, Account: r.Account, PasswordHash: r.PasswordHash, Name: r.Name, Level: r.Level, Role: r.Role}, nil
}

// ---------- 会话 ----------

func (s *Store) CreateSession(ctx context.Context, token, userID string, expires time.Time) error {
	return s.q.CreateSession(ctx, gen.CreateSessionParams{
		Token: token, UserID: userID, ExpiresAt: tstamp(expires),
	})
}

// GetSession 返回 (userID, role, expiresAt);查无返回 ErrNotFound。
// role 由 sessions→users join 带出,供 RequireAuth 一并注入 ctx metadata。
func (s *Store) GetSession(ctx context.Context, token string) (userID, role string, expires time.Time, err error) {
	r, err := s.q.GetSession(ctx, token)
	if err != nil {
		return "", "", time.Time{}, notFound(err)
	}
	return r.UserID, r.Role, r.ExpiresAt.Time, nil
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	return s.q.DeleteSession(ctx, token)
}

// ---------- 问卷 CRUD ----------

type SurveyMeta struct {
	ID               string
	OwnerID          string
	Type             string
	Title            string
	Status           string
	DraftSchema      []byte
	PublishedVersion *int32
	AnswerAccess     string // anonymous|login_required:谁能作答(发布时设定,D6)
}

func (s *Store) CreateSurvey(ctx context.Context, id, ownerID, typ, title string, draftSchema []byte) error {
	return s.q.CreateSurvey(ctx, gen.CreateSurveyParams{
		ID: id, OwnerID: ownerID, Type: typ, Title: title, DraftSchema: draftSchema,
	})
}

func (s *Store) GetSurvey(ctx context.Context, id string) (SurveyMeta, error) {
	r, err := s.q.GetSurvey(ctx, id)
	if err != nil {
		return SurveyMeta{}, notFound(err)
	}
	return SurveyMeta{
		ID: r.ID, OwnerID: r.OwnerID, Type: r.Type, Title: r.Title, Status: r.Status,
		DraftSchema: r.DraftSchema, PublishedVersion: r.PublishedVersion, AnswerAccess: r.AnswerAccess,
	}, nil
}

// SurveyListItem 列表项(轻量,对齐前端 SurveyListItem)。
type SurveyListItem struct {
	ID        string
	Title     string
	Type      string
	Status    string
	UpdatedAt time.Time
}

func (s *Store) ListSurveysByOwner(ctx context.Context, ownerID string) ([]SurveyListItem, error) {
	rows, err := s.q.ListSurveysByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	out := make([]SurveyListItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, SurveyListItem{
			ID: r.ID, Title: r.Title, Type: r.Type, Status: r.Status, UpdatedAt: r.UpdatedAt.Time,
		})
	}
	return out, nil
}

// ListAllSurveys 列出全站问卷(admin 全站视角,不限 owner)。
func (s *Store) ListAllSurveys(ctx context.Context) ([]SurveyListItem, error) {
	rows, err := s.q.ListAllSurveys(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SurveyListItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, SurveyListItem{
			ID: r.ID, Title: r.Title, Type: r.Type, Status: r.Status, UpdatedAt: r.UpdatedAt.Time,
		})
	}
	return out, nil
}

func (s *Store) UpdateDraft(ctx context.Context, id, title, typ string, draftSchema []byte) error {
	return s.q.UpdateDraft(ctx, gen.UpdateDraftParams{
		ID: id, DraftSchema: draftSchema, Title: title, Type: typ,
	})
}

// SetStatus 只改 status 单列(生命周期状态机:close/reopen 用)。
// 不校验合法性——状态机守卫在 http 层(据 SurveyMeta.Status/PublishedVersion 判断)。
func (s *Store) SetStatus(ctx context.Context, id, status string) error {
	return s.q.SetStatus(ctx, gen.SetStatusParams{ID: id, Status: status})
}

// GetPublishedSchema 取已发布快照的 schema jsonb;未发布/非 live 返回 ErrNotFound。
func (s *Store) GetPublishedSchema(ctx context.Context, id string) ([]byte, error) {
	b, err := s.q.GetPublishedSchema(ctx, id)
	if err != nil {
		return nil, notFound(err)
	}
	return b, nil
}

// GetVersionSchema 按显式版本号取历史发布快照(版本锚定提交)。该版不存在返回 ErrNotFound。
// 不做 status 过滤——status(live 才收)由 http 层单独判定。
func (s *Store) GetVersionSchema(ctx context.Context, id string, version int32) ([]byte, error) {
	b, err := s.q.GetVersionSchema(ctx, gen.GetVersionSchemaParams{SurveyID: id, Version: version})
	if err != nil {
		return nil, notFound(err)
	}
	return b, nil
}

// CountResponses 统计某问卷的答卷数(COUNT :one 恒返回一行,无 no-rows,直接透传 err)。
func (s *Store) CountResponses(ctx context.Context, id string) (int32, error) {
	return s.q.CountResponses(ctx, id)
}
