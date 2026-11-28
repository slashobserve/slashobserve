-- +goose Up
ALTER TABLE agent_requests
    ADD COLUMN committed_kind text,
    ADD COLUMN committed_version integer,
    ADD COLUMN committed_dashboard_id uuid,
    ADD CONSTRAINT agent_requests_committed_kind_check CHECK ((committed_kind IS NULL) OR (committed_kind = ANY (ARRAY['version'::text, 'proposal'::text, 'finding'::text]))),
    ADD CONSTRAINT agent_requests_committed_version_check CHECK ((committed_version IS NULL) = (committed_dashboard_id IS NULL)),
    ADD CONSTRAINT agent_requests_committed_version_fkey FOREIGN KEY (committed_dashboard_id, committed_version) REFERENCES dashboard_versions(dashboard_id, version) ON DELETE SET NULL;

CREATE INDEX agent_requests_committed_version_idx ON agent_requests (committed_dashboard_id, committed_version) WHERE committed_dashboard_id IS NOT NULL;

-- +goose Down
DROP INDEX agent_requests_committed_version_idx;
ALTER TABLE agent_requests
    DROP CONSTRAINT agent_requests_committed_version_fkey,
    DROP CONSTRAINT agent_requests_committed_version_check,
    DROP CONSTRAINT agent_requests_committed_kind_check,
    DROP COLUMN committed_dashboard_id,
    DROP COLUMN committed_version,
    DROP COLUMN committed_kind;
