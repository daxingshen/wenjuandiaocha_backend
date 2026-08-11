-- name: InsertResponse :exec
INSERT INTO responses (id, survey_id, survey_version, raw, meta)
VALUES ($1, $2, $3, $4, $5);

-- name: InsertAnswerRow :exec
INSERT INTO answer_rows (response_id, survey_id, survey_version, qid, sub_id, value_text, value_num)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: CountResponses :one
SELECT COUNT(*)::int AS count FROM responses WHERE survey_id = $1;
