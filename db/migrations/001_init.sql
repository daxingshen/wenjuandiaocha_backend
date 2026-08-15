-- +goose Up
-- +goose StatementBegin

-- 数据建模约定(全表)见 wiki/backend.md 数据模型小节:
--  · 代理主键 id BIGINT GENERATED ALWAYS AS IDENTITY(无业务含义,顺序插入不裂页,作 ORDER BY 兜底键)
--    + 业务键 xxx_id TEXT(应用生成的随机短 id,对外不可枚举)加 UNIQUE;外键指业务键(路 B)。
--  · 外键一律不带 ON DELETE CASCADE:append-only 语义,业务实体不物理删除(用 status 表达生命周期);
--    sessions 例外(瞬态表,按 expires_at 清理)。
--  · 时间列:每表 created_at(TIMESTAMPTZ NOT NULL DEFAULT now())。updated_at 只 surveys 有——
--    PG 无 MySQL 式列级 ON UPDATE,updated_at 需显式 SET now() 维护;仅 surveys 有会更新它的写操作
--    (草稿/发布/状态/作答模式),前端列表「更新于」用它。其余表 append-only 或无更新语义,不设 updated_at。
--  · 每表 created_at 建索引 idx_<表>_created_at(统一按创建时间可翻页/审计/导出);updated_at 不建索引。
-- 豁免代理键:sessions(token 自然密钥主键)、answer_rows(BIGSERIAL、无对外业务键)。

-- 用户(studio 登录 + 作答账号)
-- user_id:业务键(如 "u_xxxx")。role:RBAC 角色轴(身份能对谁的资源做什么)。
CREATE TABLE users (
  id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,  -- 代理键
  user_id       TEXT NOT NULL,                        -- 业务键
  account       TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  name          TEXT NOT NULL,
  role          TEXT NOT NULL DEFAULT 'creator',   -- admin|creator|respondent(RBAC)
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT users_user_id_key UNIQUE (user_id),
  CONSTRAINT users_account_key UNIQUE (account),
  CONSTRAINT users_role_chk CHECK (role IN ('admin', 'creator', 'respondent'))
);
CREATE INDEX idx_users_created_at ON users(created_at);

-- 会话(DB-backed session)—— 瞬态表:token 自然主键,无代理键;按 expires_at 清理(非 append-only)。
CREATE TABLE sessions (
  token      TEXT PRIMARY KEY,                  -- crypto/rand base64url
  user_id    TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(user_id)
);
CREATE INDEX idx_sessions_user ON sessions(user_id);
CREATE INDEX idx_sessions_created_at ON sessions(created_at);

-- 问卷(元信息 + 当前草稿)
-- survey_id:业务键。status:生命周期状态机 draft→live→closed→live。answer_access:作答访问模式(与 status 正交)。
CREATE TABLE surveys (
  id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,  -- 代理键
  survey_id         TEXT NOT NULL,                        -- 业务键
  owner_id          TEXT NOT NULL,                        -- → users.user_id
  type              TEXT NOT NULL DEFAULT 'survey',       -- SurveyType
  title             TEXT NOT NULL,
  status            TEXT NOT NULL DEFAULT 'draft',        -- draft|live|closed
  draft_schema      JSONB NOT NULL,                       -- 整份 SurveySchema(编辑中)
  published_version INT,                                  -- → survey_versions.version;未发布 NULL
  answer_access     TEXT NOT NULL DEFAULT 'login_required', -- anonymous|login_required(新建默认需登录;发布前可在发布页改)
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT surveys_survey_id_key UNIQUE (survey_id),
  CONSTRAINT surveys_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES users(user_id),
  CONSTRAINT surveys_status_chk CHECK (status IN ('draft', 'live', 'closed')),
  CONSTRAINT surveys_answer_access_chk CHECK (answer_access IN ('anonymous', 'login_required'))
);
CREATE INDEX idx_surveys_owner ON surveys(owner_id);
-- 问卷列表 offset 分页排序键 created_at DESC(id 代理键兜底),加索引支撑范围扫描。
CREATE INDEX idx_surveys_created_at ON surveys(created_at);

-- 已发布版本快照(版本隔离,PRD §9 约束 5)—— created_at 即发布时间(原 published_at,统一命名)。
CREATE TABLE survey_versions (
  id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,  -- 代理键
  survey_id    TEXT NOT NULL,                         -- → surveys.survey_id
  version      INT NOT NULL,
  schema       JSONB NOT NULL,                       -- 发布冻结快照
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),   -- 发布时间(原 published_at)
  CONSTRAINT survey_versions_survey_id_version_key UNIQUE (survey_id, version),
  CONSTRAINT survey_versions_survey_id_fkey FOREIGN KEY (survey_id) REFERENCES surveys(survey_id)
);
CREATE INDEX idx_survey_versions_created_at ON survey_versions(created_at);

-- 答卷(双写之一:完整 JSON,原样回放/导出)
CREATE TABLE responses (
  id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,  -- 代理键
  response_id    TEXT NOT NULL,                        -- 业务键
  survey_id      TEXT NOT NULL,                        -- → surveys.survey_id
  survey_version INT NOT NULL,                        -- 按哪版 schema 解释
  raw            JSONB NOT NULL,                       -- 完整 answers(后端权威版)
  meta           JSONB,                                -- ip/ua/耗时,防刷预留(PRD §4.3)
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT responses_response_id_key UNIQUE (response_id),
  CONSTRAINT responses_survey_id_fkey FOREIGN KEY (survey_id) REFERENCES surveys(survey_id)
);
CREATE INDEX idx_responses_survey ON responses(survey_id, survey_version);
CREATE INDEX idx_responses_created_at ON responses(created_at);

-- 答卷(双写之二:规范化行,喂统计/交叉分析,PRD §9 约束 4)—— BIGSERIAL 代理键(海量 append,无对外业务键,豁免)。
CREATE TABLE answer_rows (
  id             BIGSERIAL PRIMARY KEY,
  response_id    TEXT NOT NULL,                        -- → responses.response_id
  survey_id      TEXT NOT NULL,                        -- 反规范化冗余列(直连,不设 FK)
  survey_version INT NOT NULL,
  qid            TEXT NOT NULL,
  sub_id         TEXT,                                 -- 矩阵子行;标量题 NULL
  value_text     TEXT,                                 -- choice/text/textarea/matrix
  value_num      DOUBLE PRECISION,                     -- scale(供均值);二者其一非空
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT answer_rows_response_id_fkey FOREIGN KEY (response_id) REFERENCES responses(response_id)
);
CREATE INDEX idx_answer_rows_agg ON answer_rows(survey_id, survey_version, qid, sub_id);
CREATE INDEX idx_answer_rows_created_at ON answer_rows(created_at);

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
