-- +goose Up
-- +goose StatementBegin

-- 用户(studio 登录 + 作答账号)
-- role:RBAC 角色轴(身份能对谁的资源做什么)。
CREATE TABLE users (
  id            TEXT PRIMARY KEY,
  account       TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  name          TEXT NOT NULL,
  role          TEXT NOT NULL DEFAULT 'creator',   -- admin|creator|respondent(RBAC)
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT users_role_chk CHECK (role IN ('admin', 'creator', 'respondent'))
);

-- 会话(DB-backed session)
CREATE TABLE sessions (
  token      TEXT PRIMARY KEY,                  -- crypto/rand base64url
  user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sessions_user ON sessions(user_id);

-- 问卷(元信息 + 当前草稿)
-- status:生命周期状态机 draft→live→closed→live。answer_access:作答访问模式(发布时设定,与 status 正交)。
CREATE TABLE surveys (
  id                TEXT PRIMARY KEY,
  owner_id          TEXT NOT NULL REFERENCES users(id),
  type              TEXT NOT NULL DEFAULT 'survey',       -- SurveyType
  title             TEXT NOT NULL,
  status            TEXT NOT NULL DEFAULT 'draft',        -- draft|live|closed
  draft_schema      JSONB NOT NULL,                       -- 整份 SurveySchema(编辑中)
  published_version INT,                                  -- → survey_versions.version;未发布 NULL
  answer_access     TEXT NOT NULL DEFAULT 'anonymous',    -- anonymous|login_required
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT surveys_status_chk CHECK (status IN ('draft', 'live', 'closed')),
  CONSTRAINT surveys_answer_access_chk CHECK (answer_access IN ('anonymous', 'login_required'))
);
CREATE INDEX idx_surveys_owner ON surveys(owner_id);

-- 已发布版本快照(版本隔离,PRD §9 约束 5)
CREATE TABLE survey_versions (
  survey_id    TEXT NOT NULL REFERENCES surveys(id) ON DELETE CASCADE,
  version      INT NOT NULL,
  schema       JSONB NOT NULL,                       -- 发布冻结快照
  published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (survey_id, version)
);

-- 答卷(双写之一:完整 JSON,原样回放/导出)
CREATE TABLE responses (
  id             TEXT PRIMARY KEY,
  survey_id      TEXT NOT NULL REFERENCES surveys(id) ON DELETE CASCADE,
  survey_version INT NOT NULL,                        -- 按哪版 schema 解释
  raw            JSONB NOT NULL,                       -- 完整 answers(后端权威版)
  meta           JSONB,                                -- ip/ua/耗时,防刷预留(PRD §4.3)
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_responses_survey ON responses(survey_id, survey_version);

-- 答卷(双写之二:规范化行,喂统计/交叉分析,PRD §9 约束 4)
CREATE TABLE answer_rows (
  id             BIGSERIAL PRIMARY KEY,
  response_id    TEXT NOT NULL REFERENCES responses(id) ON DELETE CASCADE,
  survey_id      TEXT NOT NULL,
  survey_version INT NOT NULL,
  qid            TEXT NOT NULL,
  sub_id         TEXT,                                 -- 矩阵子行;标量题 NULL
  value_text     TEXT,                                 -- choice/text/textarea/matrix
  value_num      DOUBLE PRECISION                      -- scale(供均值);二者其一非空
);
CREATE INDEX idx_answer_rows_agg ON answer_rows(survey_id, survey_version, qid, sub_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS answer_rows;
DROP TABLE IF EXISTS responses;
DROP TABLE IF EXISTS survey_versions;
DROP TABLE IF EXISTS surveys;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
