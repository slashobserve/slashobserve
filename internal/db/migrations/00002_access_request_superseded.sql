-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION access_request_open(r access_requests) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
  SELECT r.granted_at IS NULL AND r.created_at >= now() - interval '30 days' AND NOT EXISTS (
    SELECT 1 FROM inbox_item_states s
    WHERE s.seq = r.activity_seq AND s.done AND s.user_id = ANY(r.granters)
  ) AND NOT EXISTS (
    SELECT 1 FROM access_requests n
    WHERE coalesce(n.dashboard_id, n.observation_id) = coalesce(r.dashboard_id, r.observation_id)
      AND n.requester_kind = r.requester_kind AND n.requester_id = r.requester_id
      AND (n.created_at, n.id) > (r.created_at, r.id)
  )
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION access_request_open(r access_requests) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
  SELECT r.granted_at IS NULL AND r.created_at >= now() - interval '30 days' AND NOT EXISTS (
    SELECT 1 FROM inbox_item_states s
    WHERE s.seq = r.activity_seq AND s.done AND s.user_id = ANY(r.granters)
  )
$$;
-- +goose StatementEnd
