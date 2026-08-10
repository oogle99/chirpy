-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email)
VALUES (
    id = gen_random_uuid(),
    created_at = NOW(),
    updated_at = NOW(),
    email = $1
)
RETURNING *;