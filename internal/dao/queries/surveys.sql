-- name: CreateSurvey :exec
-- answer_access / display_mode 由 service 显式传入(不依赖列 DEFAULT):默认值是业务规则,归代码所有,
-- 避免「改了 001 DEFAULT 但已建库未 ALTER」导致新建落旧默认的漂移。
INSERT INTO surveys (survey_id, owner_id, type, title, status, draft_schema, answer_access, display_mode)
VALUES ($1, $2, $3, $4, 'draft', $5, $6, $7);

-- name: GetSurvey :one
SELECT survey_id, owner_id, type, title, status, draft_schema, published_version, answer_access, display_mode, created_at, updated_at
FROM surveys WHERE survey_id = $1;

-- name: ListSurveysByOwner :many
-- creator 本人列表。offset 分页:ORDER BY created_at DESC, id DESC(id=代理键,单调,兜底稳定序)。
-- 投影业务键 survey_id(对外身份);过滤参数为空(nil)时短路不生效。
-- 总行数由 CountSurveysByOwner 单独取(不用 COUNT(*) OVER():越界页返回空集会丢计数报 0)。
-- keyword 走 ILIKE + ESCAPE '\':调用方须先转义 % _ \(否则用户输入的通配符会改变匹配语义)。
SELECT survey_id, title, type, status, updated_at
FROM surveys
WHERE owner_id = $1
  AND (sqlc.narg('keyword')::text IS NULL OR title ILIKE '%' || sqlc.narg('keyword') || '%' ESCAPE '\')
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('type')::text IS NULL OR type = sqlc.narg('type'))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountSurveysByOwner :one
-- ListSurveysByOwner 的筛选后总行数(WHERE 必须与 List 逐字一致,否则计数与页内不匹配)。
SELECT COUNT(*) FROM surveys
WHERE owner_id = $1
  AND (sqlc.narg('keyword')::text IS NULL OR title ILIKE '%' || sqlc.narg('keyword') || '%' ESCAPE '\')
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('type')::text IS NULL OR type = sqlc.narg('type'));

-- name: ListAllSurveys :many
-- admin 全站视角:列出所有问卷(不限 owner)。creator/respondent 不走此查询。
-- 过滤/分页语义同 ListSurveysByOwner;总行数由 CountAllSurveys 单独取。
SELECT survey_id, title, type, status, updated_at
FROM surveys
WHERE (sqlc.narg('keyword')::text IS NULL OR title ILIKE '%' || sqlc.narg('keyword') || '%' ESCAPE '\')
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('type')::text IS NULL OR type = sqlc.narg('type'))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountAllSurveys :one
-- ListAllSurveys 的筛选后总行数(WHERE 必须与 List 逐字一致)。
SELECT COUNT(*) FROM surveys
WHERE (sqlc.narg('keyword')::text IS NULL OR title ILIKE '%' || sqlc.narg('keyword') || '%' ESCAPE '\')
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('type')::text IS NULL OR type = sqlc.narg('type'));

-- name: UpdateDraft :exec
UPDATE surveys
SET draft_schema = $2, title = $3, type = $4, updated_at = now()
WHERE survey_id = $1;

-- name: SetPublished :exec
-- 只冻结版本 + 转 live;不碰 answer_access —— 作答模式由 draft 阶段经 SetAnswerAccess 设定(单一真相源)。
UPDATE surveys
SET published_version = $2, status = 'live', updated_at = now()
WHERE survey_id = $1;

-- name: SetAnswerAccess :exec
-- 设作答访问模式(anonymous|login_required)。仅 draft 可改(状态守卫在 service 层),此处只写列。
UPDATE surveys
SET answer_access = $2, updated_at = now()
WHERE survey_id = $1;

-- name: SetDisplayMode :exec
-- 设作答页展示模式(paged|single)。仅 draft 可改(状态守卫在 service 层),此处只写列。
UPDATE surveys
SET display_mode = $2, updated_at = now()
WHERE survey_id = $1;

-- name: SetStatus :exec
UPDATE surveys
SET status = $2, updated_at = now()
WHERE survey_id = $1;

-- name: InsertVersion :exec
INSERT INTO survey_versions (survey_id, version, schema)
VALUES ($1, $2, $3);

-- name: GetPublishedSchema :one
SELECT sv.schema
FROM surveys s
JOIN survey_versions sv ON sv.survey_id = s.survey_id AND sv.version = s.published_version
WHERE s.survey_id = $1 AND s.status = 'live';

-- name: GetVersionSchema :one
-- 按显式版本号取历史发布快照(版本锚定提交:作答者交哪版就按哪版校验)。
-- 不含 status 过滤——status 由 handler 单独判定(live 才收);此处只负责按版取快照。
SELECT schema
FROM survey_versions
WHERE survey_id = $1 AND version = $2;

-- name: MaxVersion :one
SELECT COALESCE(MAX(version), 0)::int AS max_version
FROM survey_versions WHERE survey_id = $1;
