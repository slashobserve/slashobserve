-- +goose Up
ALTER TABLE agent_requests ADD COLUMN read_restricted BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE dashboards ADD COLUMN copied_restricted_answer BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE annotations ADD COLUMN copied_restricted_answer BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE observations ADD COLUMN copied_restricted_answer BOOLEAN NOT NULL DEFAULT false;

UPDATE agent_requests r SET read_restricted = true
WHERE EXISTS (SELECT 1 FROM audit_events e
  WHERE e.run_id = r.id AND e.action = 'query.run' AND e.outcome <> 'denied'
    AND (e.datasource_uid = 'slashobserve-audit' OR e.datasource_uid IN (SELECT uid FROM datasources WHERE audience = 'admins')));

-- +goose StatementBegin
CREATE FUNCTION run_answer_kept_with_admins(run UUID, holders UUID[]) RETURNS BOOLEAN
LANGUAGE sql STABLE AS $$
  SELECT r.read_restricted
    OR EXISTS (SELECT 1 FROM dashboards d JOIN folders f ON f.id = d.folder_id WHERE d.id = r.dashboard_id AND NOT d.id = ANY(array_remove(holders, NULL)) AND f.admins_only)
    OR EXISTS (SELECT 1 FROM observations o JOIN folders f ON f.id = o.folder_id WHERE o.id IN (r.observation_id, r.subject_observation_id) AND (o.run_id IS DISTINCT FROM r.id OR r.investigation) AND NOT o.id = ANY(array_remove(holders, NULL)) AND f.admins_only)
  FROM agent_requests r WHERE r.id = run
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION dashboard_holds_restricted_answer(dashboard UUID) RETURNS BOOLEAN
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (SELECT 1 FROM dashboard_versions v JOIN agent_requests r ON r.id = v.agent_request_id WHERE v.dashboard_id = dashboard AND run_answer_kept_with_admins(r.id, ARRAY[dashboard]))
    OR EXISTS (SELECT 1 FROM dashboards d WHERE d.id = dashboard AND d.copied_restricted_answer)
    OR EXISTS (SELECT 1 FROM annotations n WHERE n.dashboard_id = dashboard AND n.copied_restricted_answer)
    OR EXISTS (SELECT 1 FROM dashboards d JOIN agent_requests r ON r.id = d.created_run_id WHERE d.id = dashboard AND run_answer_kept_with_admins(r.id, ARRAY[dashboard]))
    OR EXISTS (SELECT 1 FROM findings f JOIN agent_requests r ON r.id = f.agent_request_id WHERE f.dashboard_id = dashboard AND f.annotation_id IS NOT NULL AND run_answer_kept_with_admins(r.id, ARRAY[dashboard]))
    OR EXISTS (SELECT 1 FROM proposals p JOIN agent_requests r ON r.id = p.run_id WHERE p.dashboard_id = dashboard AND run_answer_kept_with_admins(r.id, ARRAY[dashboard]))
    OR EXISTS (SELECT 1 FROM threads t JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE t.dashboard_id = dashboard AND run_answer_kept_with_admins(r.id, ARRAY[dashboard]))
    OR EXISTS (SELECT 1 FROM annotations n JOIN threads t ON t.annotation_id = n.id JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE n.dashboard_id = dashboard AND run_answer_kept_with_admins(r.id, ARRAY[dashboard]))
    OR EXISTS (SELECT 1 FROM proposals p JOIN threads t ON t.proposal_id = p.id JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE p.dashboard_id = dashboard AND run_answer_kept_with_admins(r.id, ARRAY[dashboard]))
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION observation_holds_restricted_answer(observation UUID) RETURNS BOOLEAN
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (SELECT 1 FROM observations o WHERE o.id = observation AND o.copied_restricted_answer)
    OR EXISTS (SELECT 1 FROM observation_blocks b JOIN agent_requests r ON r.id = b.run_id WHERE b.observation_id = observation AND run_answer_kept_with_admins(r.id, ARRAY[observation]))
    OR EXISTS (SELECT 1 FROM findings f JOIN agent_requests r ON r.id = f.agent_request_id WHERE f.observation_id = observation AND run_answer_kept_with_admins(r.id, ARRAY[observation]))
    OR EXISTS (SELECT 1 FROM threads t JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE t.observation_id = observation AND run_answer_kept_with_admins(r.id, ARRAY[observation]))
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION dashboard_holds_restricted_run(dashboard UUID) RETURNS BOOLEAN
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (SELECT 1 FROM agent_requests r WHERE r.dashboard_id = dashboard AND run_answer_kept_with_admins(r.id, ARRAY[dashboard]))
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION observation_holds_restricted_run(observation UUID) RETURNS BOOLEAN
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (SELECT 1 FROM agent_requests r WHERE (r.observation_id = observation OR r.subject_observation_id = observation) AND run_answer_kept_with_admins(r.id, ARRAY[observation]))
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

-- +goose StatementBegin
CREATE FUNCTION keep_copies_of_run(run UUID) RETURNS void
LANGUAGE sql VOLATILE AS $$
  UPDATE dashboards x SET copied_restricted_answer = true
  WHERE NOT x.copied_restricted_answer
    AND (x.created_run_id = run
      OR x.id IN (SELECT v.dashboard_id FROM dashboard_versions v WHERE v.agent_request_id = run)
      OR x.id IN (SELECT f.dashboard_id FROM findings f WHERE f.agent_request_id = run AND f.annotation_id IS NOT NULL)
      OR x.id IN (SELECT p.dashboard_id FROM proposals p WHERE p.run_id = run)
      OR x.id IN (SELECT t.dashboard_id FROM threads t JOIN thread_messages m ON m.thread_id = t.id WHERE m.run_id = run)
      OR x.id IN (SELECT n.dashboard_id FROM annotations n JOIN threads t ON t.annotation_id = n.id JOIN thread_messages m ON m.thread_id = t.id WHERE m.run_id = run)
      OR x.id IN (SELECT p.dashboard_id FROM proposals p JOIN threads t ON t.proposal_id = p.id JOIN thread_messages m ON m.thread_id = t.id WHERE m.run_id = run))
    AND run_answer_kept_with_admins(run, ARRAY[x.id]);
  UPDATE observations x SET copied_restricted_answer = true
  WHERE NOT x.copied_restricted_answer
    AND (x.id IN (SELECT b.observation_id FROM observation_blocks b WHERE b.run_id = run)
      OR x.id IN (SELECT f.observation_id FROM findings f WHERE f.agent_request_id = run)
      OR x.id IN (SELECT t.observation_id FROM threads t JOIN thread_messages m ON m.thread_id = t.id WHERE m.run_id = run))
    AND run_answer_kept_with_admins(run, ARRAY[x.id]);
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION keep_copies_of_deleted_run() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM keep_copies_of_run(OLD.id);
  RETURN OLD;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION mark_runs_kept_by_deleted_subject() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  UPDATE agent_requests r SET read_restricted = true
  WHERE OLD.id IN (r.dashboard_id, r.observation_id, r.subject_observation_id) AND NOT r.read_restricted
    AND run_answer_kept_with_admins(r.id, array_remove(ARRAY[r.dashboard_id, r.observation_id, r.subject_observation_id], OLD.id));
  RETURN OLD;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER agent_requests_keep_copies BEFORE DELETE ON agent_requests FOR EACH ROW EXECUTE FUNCTION keep_copies_of_deleted_run();
CREATE TRIGGER dashboards_mark_runs_kept_by_it BEFORE DELETE ON dashboards FOR EACH ROW EXECUTE FUNCTION mark_runs_kept_by_deleted_subject();
CREATE TRIGGER observations_mark_runs_kept_by_it BEFORE DELETE ON observations FOR EACH ROW EXECUTE FUNCTION mark_runs_kept_by_deleted_subject();

-- +goose Down
DROP TRIGGER observations_mark_runs_kept_by_it ON observations;
DROP TRIGGER dashboards_mark_runs_kept_by_it ON dashboards;
DROP TRIGGER agent_requests_keep_copies ON agent_requests;
DROP FUNCTION mark_runs_kept_by_deleted_subject();
DROP FUNCTION keep_copies_of_deleted_run();
DROP FUNCTION keep_copies_of_run(UUID);
DROP FUNCTION observation_has_run_in_flight(UUID);
DROP FUNCTION dashboard_has_run_in_flight(UUID);
DROP FUNCTION observation_holds_restricted_run(UUID);
DROP FUNCTION dashboard_holds_restricted_run(UUID);
DROP FUNCTION observation_holds_restricted_answer(UUID);
DROP FUNCTION dashboard_holds_restricted_answer(UUID);
DROP FUNCTION run_answer_kept_with_admins(UUID, UUID[]);
ALTER TABLE observations DROP COLUMN copied_restricted_answer;
ALTER TABLE annotations DROP COLUMN copied_restricted_answer;
ALTER TABLE dashboards DROP COLUMN copied_restricted_answer;
ALTER TABLE agent_requests DROP COLUMN read_restricted;
