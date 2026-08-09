-- name: CreateUser :exec
INSERT INTO users (id, account, password_hash, name, level)
VALUES ($1, $2, $3, $4, $5);

-- name: GetUserByAccount :one
SELECT id, account, password_hash, name, level, created_at
FROM users WHERE account = $1;

-- name: GetUserByID :one
SELECT id, account, password_hash, name, level, created_at
FROM users WHERE id = $1;
