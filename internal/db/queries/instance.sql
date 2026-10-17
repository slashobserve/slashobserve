-- name: EnsureInstance :one
INSERT INTO instance (singleton) VALUES (TRUE)
ON CONFLICT (singleton) DO UPDATE SET singleton = EXCLUDED.singleton
RETURNING id, created_at;
