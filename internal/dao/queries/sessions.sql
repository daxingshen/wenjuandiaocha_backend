-- name: CreateSession :exec
INSERT INTO sessions (token, user_id, expires_at)
VALUES ($1, $2, $3);

-- name: GetSession :one
-- 带出会话用户的 role,供 RequireAuth 一并注入 ctx metadata(判定层零额外查库)。
SELECT s.token, s.user_id, s.expires_at, s.created_at, u.role
FROM sessions s
JOIN users u ON u.user_id = s.user_id
WHERE s.token = $1;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at < now();
