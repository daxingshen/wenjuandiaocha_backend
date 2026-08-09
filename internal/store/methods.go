// store 的非事务业务方法:用户 / 会话 / 问卷 CRUD / 发布快照读取。
// 直接委托 sqlc 生成的查询,归一「查无」错误。
package store

import (
	"context"
	"time"

	"wenjuandiaocha_backend/internal/store/gen"
)

// ---------- 用户 ----------

type User struct {
	ID           string
	Account      string
	PasswordHash string
	Name         string
	Level        string
}

func (s *Store) CreateUser(ctx context.Context, u User) error {
	return s.q.CreateUser(ctx, gen.CreateUserParams{
		ID: u.ID, Account: u.Account, PasswordHash: u.PasswordHash, Name: u.Name, Level: u.Level,
	})
}

func (s *Store) GetUserByAccount(ctx context.Context, account string) (User, error) {
	r, err := s.q.GetUserByAccount(ctx, account)
	if err != nil {
		return User{}, notFound(err)
	}
	return User{ID: r.ID, Account: r.Account, PasswordHash: r.PasswordHash, Name: r.Name, Level: r.Level}, nil
}

func (s *Store) GetUserByID(ctx context.Context, id string) (User, error) {
	r, err := s.q.GetUserByID(ctx, id)
	if err != nil {
		return User{}, notFound(err)
	}
	return User{ID: r.ID, Account: r.Account, PasswordHash: r.PasswordHash, Name: r.Name, Level: r.Level}, nil
}

// ---------- 会话 ----------

func (s *Store) CreateSession(ctx context.Context, token, userID string, expires time.Time) error {
	return s.q.CreateSession(ctx, gen.CreateSessionParams{
		Token: token, UserID: userID, ExpiresAt: tstamp(expires),
	})
}

// GetSession 返回 (userID, expiresAt);查无返回 ErrNotFound。
func (s *Store) GetSession(ctx context.Context, token string) (userID string, expires time.Time, err error) {
	r, err := s.q.GetSession(ctx, token)
	if err != nil {
		return "", time.Time{}, notFound(err)
	}
	return r.UserID, r.ExpiresAt.Time, nil
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
		DraftSchema: r.DraftSchema, PublishedVersion: r.PublishedVersion,
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
