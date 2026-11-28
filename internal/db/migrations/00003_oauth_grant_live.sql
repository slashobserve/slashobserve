-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION oauth_grant_live(g oauth_grants) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$ SELECT g.revoked_at IS NULL AND g.expires_at > now() $$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION oauth_grant_live(oauth_grants);
