-- +goose Up
ALTER TABLE agent_requests ADD COLUMN read_restricted BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE dashboards ADD COLUMN copied_restricted_answer BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE annotations ADD COLUMN copied_restricted_answer BOOLEAN NOT NULL DEFAULT false;

UPDATE agent_requests r SET read_restricted = true
WHERE EXISTS (SELECT 1 FROM audit_events e
  WHERE e.run_id = r.id AND e.action = 'query.run' AND e.outcome <> 'denied'
    AND (e.datasource_uid = 'slashobserve-audit' OR e.datasource_uid IN (SELECT uid FROM datasources WHERE audience = 'admins')));

-- +goose StatementBegin
CREATE FUNCTION run_answer_kept_with_admins(run UUID, holder UUID) RETURNS BOOLEAN
LANGUAGE sql STABLE AS $$
  SELECT r.read_restricted
    OR EXISTS (SELECT 1 FROM dashboards d JOIN folders f ON f.id = d.folder_id WHERE d.id = r.dashboard_id AND d.id IS DISTINCT FROM holder AND f.admins_only)
    OR EXISTS (SELECT 1 FROM observations o JOIN folders f ON f.id = o.folder_id WHERE o.id IN (r.observation_id, r.subject_observation_id) AND o.id IS DISTINCT FROM holder AND f.admins_only)
  FROM agent_requests r WHERE r.id = run
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION dashboard_holds_restricted_answer(dashboard UUID) RETURNS BOOLEAN
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (SELECT 1 FROM dashboard_versions v JOIN agent_requests r ON r.id = v.agent_request_id WHERE v.dashboard_id = dashboard AND run_answer_kept_with_admins(r.id, dashboard))
    OR EXISTS (SELECT 1 FROM dashboards d WHERE d.id = dashboard AND d.copied_restricted_answer)
    OR EXISTS (SELECT 1 FROM annotations n WHERE n.dashboard_id = dashboard AND n.copied_restricted_answer)
    OR EXISTS (SELECT 1 FROM dashboards d JOIN agent_requests r ON r.id = d.created_run_id WHERE d.id = dashboard AND run_answer_kept_with_admins(r.id, dashboard))
    OR EXISTS (SELECT 1 FROM findings f JOIN agent_requests r ON r.id = f.agent_request_id WHERE f.dashboard_id = dashboard AND f.annotation_id IS NOT NULL AND run_answer_kept_with_admins(r.id, dashboard))
    OR EXISTS (SELECT 1 FROM proposals p JOIN agent_requests r ON r.id = p.run_id WHERE p.dashboard_id = dashboard AND run_answer_kept_with_admins(r.id, dashboard))
    OR EXISTS (SELECT 1 FROM threads t JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE t.dashboard_id = dashboard AND run_answer_kept_with_admins(r.id, dashboard))
    OR EXISTS (SELECT 1 FROM annotations n JOIN threads t ON t.annotation_id = n.id JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE n.dashboard_id = dashboard AND run_answer_kept_with_admins(r.id, dashboard))
    OR EXISTS (SELECT 1 FROM proposals p JOIN threads t ON t.proposal_id = p.id JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE p.dashboard_id = dashboard AND run_answer_kept_with_admins(r.id, dashboard))
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION observation_holds_restricted_answer(observation UUID) RETURNS BOOLEAN
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (SELECT 1 FROM observation_blocks b JOIN agent_requests r ON r.id = b.run_id WHERE b.observation_id = observation AND run_answer_kept_with_admins(r.id, observation))
    OR EXISTS (SELECT 1 FROM findings f JOIN agent_requests r ON r.id = f.agent_request_id WHERE f.observation_id = observation AND run_answer_kept_with_admins(r.id, observation))
    OR EXISTS (SELECT 1 FROM threads t JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE t.observation_id = observation AND run_answer_kept_with_admins(r.id, observation))
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION dashboard_holds_restricted_run(dashboard UUID) RETURNS BOOLEAN
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (SELECT 1 FROM agent_requests r WHERE r.dashboard_id = dashboard AND run_answer_kept_with_admins(r.id, dashboard))
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION observation_holds_restricted_run(observation UUID) RETURNS BOOLEAN
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (SELECT 1 FROM agent_requests r WHERE (r.observation_id = observation OR r.subject_observation_id = observation) AND run_answer_kept_with_admins(r.id, observation))
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
DROP FUNCTION run_answer_kept_with_admins(UUID, UUID);
ALTER TABLE annotations DROP COLUMN copied_restricted_answer;
ALTER TABLE dashboards DROP COLUMN copied_restricted_answer;
ALTER TABLE agent_requests DROP COLUMN read_restricted;
