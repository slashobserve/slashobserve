-- +goose Up
ALTER TABLE agent_requests ADD COLUMN read_restricted BOOLEAN NOT NULL DEFAULT false;

-- +goose StatementBegin
CREATE FUNCTION dashboard_holds_restricted_answer(dashboard UUID) RETURNS BOOLEAN
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (SELECT 1 FROM dashboard_versions v JOIN agent_requests r ON r.id = v.agent_request_id WHERE v.dashboard_id = dashboard AND r.read_restricted)
    OR EXISTS (SELECT 1 FROM findings f JOIN agent_requests r ON r.id = f.agent_request_id WHERE f.dashboard_id = dashboard AND f.annotation_id IS NOT NULL AND r.read_restricted)
    OR EXISTS (SELECT 1 FROM threads t JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE t.dashboard_id = dashboard AND r.read_restricted)
    OR EXISTS (SELECT 1 FROM annotations n JOIN threads t ON t.annotation_id = n.id JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE n.dashboard_id = dashboard AND r.read_restricted)
    OR EXISTS (SELECT 1 FROM proposals p JOIN threads t ON t.proposal_id = p.id JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE p.dashboard_id = dashboard AND r.read_restricted)
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION observation_holds_restricted_answer(observation UUID) RETURNS BOOLEAN
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (SELECT 1 FROM observation_blocks b JOIN agent_requests r ON r.id = b.run_id WHERE b.observation_id = observation AND r.read_restricted)
    OR EXISTS (SELECT 1 FROM findings f JOIN agent_requests r ON r.id = f.agent_request_id WHERE f.observation_id = observation AND r.read_restricted)
    OR EXISTS (SELECT 1 FROM threads t JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE t.observation_id = observation AND r.read_restricted)
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION dashboard_holds_restricted_run(dashboard UUID) RETURNS BOOLEAN
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (SELECT 1 FROM agent_requests r WHERE r.dashboard_id = dashboard AND r.read_restricted)
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION observation_holds_restricted_run(observation UUID) RETURNS BOOLEAN
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (SELECT 1 FROM agent_requests r WHERE (r.observation_id = observation OR r.subject_observation_id = observation) AND r.read_restricted)
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION dashboard_has_run_in_flight(dashboard UUID) RETURNS BOOLEAN
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (SELECT 1 FROM agent_requests r WHERE r.dashboard_id = dashboard AND r.state IN ('queued', 'running', 'canceling'))
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION observation_has_run_in_flight(observation UUID) RETURNS BOOLEAN
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (SELECT 1 FROM agent_requests r WHERE (r.observation_id = observation OR r.subject_observation_id = observation) AND r.state IN ('queued', 'running', 'canceling'))
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION observation_has_run_in_flight(UUID);
DROP FUNCTION dashboard_has_run_in_flight(UUID);
DROP FUNCTION observation_holds_restricted_run(UUID);
DROP FUNCTION dashboard_holds_restricted_run(UUID);
DROP FUNCTION observation_holds_restricted_answer(UUID);
DROP FUNCTION dashboard_holds_restricted_answer(UUID);
ALTER TABLE agent_requests DROP COLUMN read_restricted;
