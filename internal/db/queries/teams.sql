-- name: ListTeams :many
SELECT id, name, created_by_name, created_at FROM teams ORDER BY lower(name), id;

-- name: ListTeamMembers :many
SELECT tm.team_id, u.id, u.name, u.email
FROM team_members tm JOIN users u ON u.id = tm.user_id
WHERE tm.team_id = ANY(@team_ids::uuid[])
ORDER BY lower(coalesce(nullif(u.name, ''), u.email)), u.id;

-- name: GetTeam :one
SELECT id, name, created_by_name, created_at FROM teams WHERE id = @id;

-- name: CountTeams :one
SELECT count(*)::integer FROM teams;

-- name: CreateTeam :one
INSERT INTO teams (name, created_by_name) VALUES (@name, @created_by_name)
ON CONFLICT ((lower(name))) DO NOTHING
RETURNING id, name, created_by_name, created_at;

-- name: DeleteTeam :execrows
DELETE FROM teams WHERE id = @id;

-- name: AddTeamMember :execrows
INSERT INTO team_members (team_id, user_id) VALUES (@team_id, @user_id) ON CONFLICT DO NOTHING;

-- name: RemoveTeamMember :execrows
DELETE FROM team_members WHERE team_id = @team_id AND user_id = @user_id;

-- name: GetUserDisplayName :one
SELECT coalesce(nullif(name, ''), email)::text FROM users WHERE id = @id;
