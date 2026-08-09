-- name: CreateSurvey :exec
INSERT INTO surveys (id, owner_id, type, title, status, draft_schema)
VALUES ($1, $2, $3, $4, 'draft', $5);

-- name: GetSurvey :one
SELECT id, owner_id, type, title, status, draft_schema, published_version, created_at, updated_at
FROM surveys WHERE id = $1;

-- name: ListSurveysByOwner :many
SELECT id, title, type, status, updated_at
FROM surveys WHERE owner_id = $1
ORDER BY updated_at DESC;

-- name: UpdateDraft :exec
UPDATE surveys
SET draft_schema = $2, title = $3, type = $4, updated_at = now()
WHERE id = $1;

-- name: SetPublished :exec
UPDATE surveys
SET published_version = $2, status = 'live', updated_at = now()
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

-- name: MaxVersion :one
SELECT COALESCE(MAX(version), 0)::int AS max_version
FROM survey_versions WHERE survey_id = $1;
