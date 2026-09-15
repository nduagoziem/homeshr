-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (user_id, token, created_at, expires_at, revoked) 
VALUES ($1, $2, $3, $4, $5)
RETURNING id, user_id, token, created_at, expires_at, revoked;

-- name: GetRefreshToken :one
SELECT * FROM refresh_tokens WHERE token = $1;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens SET revoked = true WHERE token = $1;

-- name: RevokeRefreshTokenByID :exec
UPDATE refresh_tokens SET revoked = true WHERE id = $1;
