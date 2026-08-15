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
	UserID       string // 业务键
	Account      string
	PasswordHash string
	Name         string
	Role         string // 账号角色 admin|creator|respondent(RBAC 角色轴)
}

func (s *Store) CreateUser(ctx context.Context, u User) error {
	return s.q.CreateUser(ctx, gen.CreateUserParams{
		UserID: u.UserID, Account: u.Account, PasswordHash: u.PasswordHash, Name: u.Name, Role: u.Role,
	})
}

func (s *Store) GetUserByAccount(ctx context.Context, account string) (User, error) {
	r, err := s.q.GetUserByAccount(ctx, account)
	if err != nil {
		return User{}, notFound(err)
	}
	return User{UserID: r.UserID, Account: r.Account, PasswordHash: r.PasswordHash, Name: r.Name, Role: r.Role}, nil
}

func (s *Store) GetUserByID(ctx context.Context, userID string) (User, error) {
	r, err := s.q.GetUserByID(ctx, userID)
	if err != nil {
		return User{}, notFound(err)
	}
	return User{UserID: r.UserID, Account: r.Account, PasswordHash: r.PasswordHash, Name: r.Name, Role: r.Role}, nil
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
	SurveyID         string // 业务键
	OwnerID          string
	Type             string
	Title            string
	Status           string
	DraftSchema      []byte
	PublishedVersion *int32
	AnswerAccess     string // anonymous|login_required:谁能作答(发布时设定,D6)
}

func (s *Store) CreateSurvey(ctx context.Context, surveyID, ownerID, typ, title string, draftSchema []byte, answerAccess string) error {
	return s.q.CreateSurvey(ctx, gen.CreateSurveyParams{
		SurveyID: surveyID, OwnerID: ownerID, Type: typ, Title: title, DraftSchema: draftSchema, AnswerAccess: answerAccess,
	})
}

func (s *Store) GetSurvey(ctx context.Context, surveyID string) (SurveyMeta, error) {
	r, err := s.q.GetSurvey(ctx, surveyID)
	if err != nil {
		return SurveyMeta{}, notFound(err)
	}
	return SurveyMeta{
		SurveyID: r.SurveyID, OwnerID: r.OwnerID, Type: r.Type, Title: r.Title, Status: r.Status,
		DraftSchema: r.DraftSchema, PublishedVersion: r.PublishedVersion, AnswerAccess: r.AnswerAccess,
	}, nil
}

// SurveyListItem 列表项(轻量,对齐前端 SurveyListItem)。
type SurveyListItem struct {
	SurveyID  string // 业务键
	Title     string
	Type      string
	Status    string
	UpdatedAt time.Time
}

// SurveyListParams 列表查询参数(过滤 + offset 分页)。
// 指针字段为 nil 即该条件不生效(SQL 侧 narg 短路)。Limit/Offset 由 service 计算并 clamp。
type SurveyListParams struct {
	Keyword *string
	Status  *string
	Type    *string
	Limit   int32
	Offset  int32
}

// 两个 List 方法返回 (页内项, 筛选后总行数, error)。total 由独立 COUNT 查询取——
// 不用 COUNT(*) OVER():窗口计数搭在返回行上,越界页(OFFSET 越过全部行)返回空集会丢计数报 0。
func (s *Store) ListSurveysByOwner(ctx context.Context, ownerID string, p SurveyListParams) ([]SurveyListItem, int64, error) {
	rows, err := s.q.ListSurveysByOwner(ctx, gen.ListSurveysByOwnerParams{
		OwnerID: ownerID,
		Keyword: p.Keyword,
		Status:  p.Status,
		Type:    p.Type,
		Lim:     p.Limit,
		Off:     p.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountSurveysByOwner(ctx, gen.CountSurveysByOwnerParams{
		OwnerID: ownerID,
		Keyword: p.Keyword,
		Status:  p.Status,
		Type:    p.Type,
	})
	if err != nil {
		return nil, 0, err
	}
	return mapSurveyRows(rows, func(r gen.ListSurveysByOwnerRow) SurveyListItem {
		return SurveyListItem{SurveyID: r.SurveyID, Title: r.Title, Type: r.Type, Status: r.Status, UpdatedAt: r.UpdatedAt.Time}
	}), total, nil
}

// ListAllSurveys 列出全站问卷(admin 全站视角,不限 owner)。过滤/分页语义同 ByOwner。
func (s *Store) ListAllSurveys(ctx context.Context, p SurveyListParams) ([]SurveyListItem, int64, error) {
	rows, err := s.q.ListAllSurveys(ctx, gen.ListAllSurveysParams{
		Keyword: p.Keyword,
		Status:  p.Status,
		Type:    p.Type,
		Lim:     p.Limit,
		Off:     p.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountAllSurveys(ctx, gen.CountAllSurveysParams{
		Keyword: p.Keyword,
		Status:  p.Status,
		Type:    p.Type,
	})
	if err != nil {
		return nil, 0, err
	}
	return mapSurveyRows(rows, func(r gen.ListAllSurveysRow) SurveyListItem {
		return SurveyListItem{SurveyID: r.SurveyID, Title: r.Title, Type: r.Type, Status: r.Status, UpdatedAt: r.UpdatedAt.Time}
	}), total, nil
}

// mapSurveyRows 把生成行按 conv 投影成对外 SurveyListItem,消除两个 List 方法里重复的 make+循环样板。
// 两个 gen 行类型(ByOwner/All)字段同构但类型不同,故投影表达式由各调用方给出。
func mapSurveyRows[T any](rows []T, conv func(T) SurveyListItem) []SurveyListItem {
	out := make([]SurveyListItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, conv(r))
	}
	return out
}

func (s *Store) UpdateDraft(ctx context.Context, surveyID, title, typ string, draftSchema []byte) error {
	return s.q.UpdateDraft(ctx, gen.UpdateDraftParams{
		SurveyID: surveyID, DraftSchema: draftSchema, Title: title, Type: typ,
	})
}

// SetStatus 只改 status 单列(生命周期状态机:close/reopen 用)。
// 不校验合法性——状态机守卫在 http 层(据 SurveyMeta.Status/PublishedVersion 判断)。
func (s *Store) SetStatus(ctx context.Context, surveyID, status string) error {
	return s.q.SetStatus(ctx, gen.SetStatusParams{SurveyID: surveyID, Status: status})
}

// SetAnswerAccess 只改 answer_access 单列(作答访问模式)。
// 不校验状态/归属——「仅 draft + owner」守卫在 service 层(复用 owned() + 状态判断)。
func (s *Store) SetAnswerAccess(ctx context.Context, surveyID, access string) error {
	return s.q.SetAnswerAccess(ctx, gen.SetAnswerAccessParams{SurveyID: surveyID, AnswerAccess: access})
}

// GetPublishedSchema 取已发布快照的 schema jsonb;未发布/非 live 返回 ErrNotFound。
func (s *Store) GetPublishedSchema(ctx context.Context, surveyID string) ([]byte, error) {
	b, err := s.q.GetPublishedSchema(ctx, surveyID)
	if err != nil {
		return nil, notFound(err)
	}
	return b, nil
}

// GetVersionSchema 按显式版本号取历史发布快照(版本锚定提交)。该版不存在返回 ErrNotFound。
// 不做 status 过滤——status(live 才收)由 http 层单独判定。
func (s *Store) GetVersionSchema(ctx context.Context, surveyID string, version int32) ([]byte, error) {
	b, err := s.q.GetVersionSchema(ctx, gen.GetVersionSchemaParams{SurveyID: surveyID, Version: version})
	if err != nil {
		return nil, notFound(err)
	}
	return b, nil
}

// CountResponses 统计某问卷的答卷数(COUNT :one 恒返回一行,无 no-rows,直接透传 err)。
func (s *Store) CountResponses(ctx context.Context, surveyID string) (int32, error) {
	return s.q.CountResponses(ctx, surveyID)
}
