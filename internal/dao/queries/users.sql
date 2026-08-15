-- name: CreateUser :exec
INSERT INTO users (user_id, account, password_hash, name, role)
VALUES ($1, $2, $3, $4, $5);

-- name: GetUserByAccount :one
SELECT user_id, account, password_hash, name, role, created_at
FROM users WHERE account = $1;

-- name: GetUserByID :one
SELECT user_id, account, password_hash, name, role, created_at
FROM users WHERE user_id = $1;
