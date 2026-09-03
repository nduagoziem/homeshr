-- name: CreateUser :one
INSERT INTO users (id, email, password_hash, full_name)
VALUES ($1, $2, $3, $4)
RETURNING id, full_name, email, is_host, last_login, created_at, updated_at;

-- name: GetAllUsers :many
SELECT * FROM users ORDER BY id;

-- name: FindUserByEmail :one
SELECT id, email, full_name, password_hash FROM users WHERE email = $1;

-- name: FindUserByID :one
SELECT id, email, full_name FROM users WHERE id = $1;

-- name: UpdateLastLoginStatus :exec
UPDATE users SET last_login = $1 WHERE id = $2;