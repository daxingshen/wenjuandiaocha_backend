-- name: CreateSurvey :exec
-- answer_access 由 service 显式传入(不依赖列 DEFAULT):默认值是业务规则,归代码所有,
-- 避免「改了 001 DEFAULT 但已建库未 ALTER」导致新建落旧默认的漂移。
INSERT INTO surveys (id, owner_id, type, title, status, draft_schema, answer_access)
VALUES ($1, $2, $3, $4, 'draft', $5, $6);

-- name: GetSurvey :one
SELECT id, owner_id, type, title, status, draft_schema, published_version, answer_access, created_at, updated_at
FROM surveys WHERE id = $1;

-- name: ListSurveysByOwner :many
SELECT id, title, type, status, updated_at
FROM surveys WHERE owner_id = $1
ORDER BY created_at DESC;

-- name: ListAllSurveys :many
-- admin 全站视角:列出所有问卷(不限 owner)。creator/respondent 不走此查询。
SELECT id, title, type, status, updated_at
FROM surveys
ORDER BY created_at DESC;

-- name: UpdateDraft :exec
UPDATE surveys
SET draft_schema = $2, title = $3, type = $4, updated_at = now()
WHERE id = $1;

-- name: SetPublished :exec
-- 只冻结版本 + 转 live;不碰 answer_access —— 作答模式由 draft 阶段经 SetAnswerAccess 设定(单一真相源)。
UPDATE surveys
SET published_version = $2, status = 'live', updated_at = now()
WHERE id = $1;

-- name: SetAnswerAccess :exec
-- 设作答访问模式(anonymous|login_required)。仅 draft 可改(状态守卫在 service 层),此处只写列。
UPDATE surveys
SET answer_access = $2, updated_at = now()
WHERE id = $1;

-- name: SetStatus :exec
UPDATE surveys
SET status = $2, updated_at = now()
WHERE id = $1;

-- name: InsertVersion :exec
INSERT INTO survey_versions (survey_id, version, schema)
VALUES ($1, $2, $3);

-- name: GetPublishedSchema :one
SELECT sv.schema
FROM surveys s
JOIN survey_versions sv ON sv.survey_id = s.id AND sv.version = s.published_version
WHERE s.id = $1 AND s.status = 'live';

-- name: GetVersionSchema :one
-- 按显式版本号取历史发布快照(版本锚定提交:作答者交哪版就按哪版校验)。
-- 不含 status 过滤——status 由 handler 单独判定(live 才收);此处只负责按版取快照。
SELECT schema
FROM survey_versions
WHERE survey_id = $1 AND version = $2;

-- name: MaxVersion :one
SELECT COALESCE(MAX(version), 0)::int AS max_version
FROM survey_versions WHERE survey_id = $1;
