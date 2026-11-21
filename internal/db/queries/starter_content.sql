-- name: StarterContent :one
SELECT ref FROM starter_content WHERE key = @key;

-- name: RecordStarterContent :exec
INSERT INTO starter_content (key, ref) VALUES (@key, @ref) ON CONFLICT (key) DO NOTHING;
