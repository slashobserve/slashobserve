-- +goose Up
-- +goose StatementBegin
SET LOCAL check_function_bodies = false;

CREATE TABLE access_requests (
    id uuid NOT NULL,
    dashboard_id uuid,
    observation_id uuid,
    requester_kind text NOT NULL,
    requester_id uuid NOT NULL,
    requester_name text NOT NULL,
    note text DEFAULT ''::text NOT NULL,
    granters uuid[] NOT NULL,
    activity_seq bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    granted_at timestamp with time zone,
    granted_by_kind text,
    granted_by_id uuid,
    CONSTRAINT access_requests_check CHECK ((num_nonnulls(dashboard_id, observation_id) = 1)),
    CONSTRAINT access_requests_note_check CHECK (((char_length(note) <= 200) AND (POSITION(('
'::text) IN (note)) = 0) AND (POSITION((''::text) IN (note)) = 0))),
    CONSTRAINT access_requests_requester_kind_check CHECK ((requester_kind = ANY (ARRAY['user'::text, 'service_account'::text])))
);

CREATE FUNCTION access_request_open(r access_requests) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
  SELECT r.granted_at IS NULL AND r.created_at >= now() - interval '30 days' AND NOT EXISTS (
    SELECT 1 FROM inbox_item_states s
    WHERE s.seq = r.activity_seq AND s.done AND s.user_id = ANY(r.granters)
  )
$$;

CREATE FUNCTION annotation_search_vector(body text, tags text[]) RETURNS tsvector
    LANGUAGE sql IMMUTABLE COST 1000 PARALLEL SAFE
    SET search_path TO '$user', 'public'
    AS $$
SELECT search_vector(split_part(body, E'\n', 1), search_words(tags), body, '')
$$;

CREATE FUNCTION audit_events_add_partition(day date) RETURNS boolean
    LANGUAGE sql
    AS $$
  SELECT audit_events_attach('audit_events_' || to_char(day, 'YYYYMMDD'), day, day + 1);
$$;

CREATE FUNCTION audit_events_attach(name text, starts date, ends date) RETURNS boolean
    LANGUAGE plpgsql
    AS $$
BEGIN
  PERFORM pg_advisory_xact_lock(hashtextextended('audit_events_partitions', 0));
  IF EXISTS (SELECT 1 FROM pg_inherits WHERE inhparent = 'audit_events'::regclass AND inhrelid = to_regclass(name)) THEN
    RETURN false;
  END IF;
  IF to_regclass(name) IS NOT NULL THEN
    RAISE EXCEPTION 'audit partition % already exists outside the audit trail and is refused', name USING ERRCODE = 'duplicate_table';
  END IF;
  EXECUTE format('CREATE TABLE %I (LIKE audit_events INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING GENERATED)', name);
  EXECUTE format('CREATE TRIGGER audit_events_no_truncate BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION audit_events_refuse_change()', name);
  EXECUTE format('ALTER TABLE audit_events ATTACH PARTITION %I FOR VALUES FROM (%L) TO (%L)', name,
    starts::timestamp AT TIME ZONE 'UTC', ends::timestamp AT TIME ZONE 'UTC');
  RETURN true;
END
$$;

CREATE FUNCTION audit_events_refuse_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  RAISE EXCEPTION 'audit_events is append-only: % is refused', TG_OP USING ERRCODE = 'insufficient_privilege';
END
$$;

CREATE FUNCTION create_personal_folder() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  INSERT INTO folders (title, personal, owner_id, created_by_kind, created_by_id, created_by_name)
  VALUES ('My Dashboards', true, NEW.id, 'user', NEW.id, coalesce(nullif(NEW.name, ''), NEW.email))
  ON CONFLICT (owner_id) WHERE owner_id IS NOT NULL DO NOTHING;
  RETURN NULL;
END
$$;

CREATE FUNCTION dashboard_has_run_in_flight(dashboard uuid) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
  SELECT EXISTS (SELECT 1 FROM agent_requests r WHERE r.dashboard_id = dashboard AND r.state IN ('queued', 'running', 'canceling'))
$$;

CREATE FUNCTION dashboard_holds_restricted_answer(dashboard uuid) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
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

CREATE FUNCTION dashboard_holds_restricted_run(dashboard uuid) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
  SELECT EXISTS (SELECT 1 FROM agent_requests r WHERE r.dashboard_id = dashboard AND run_answer_kept_with_admins(r.id, ARRAY[dashboard]))
$$;

CREATE FUNCTION dashboard_search_vector(spec jsonb) RETURNS tsvector
    LANGUAGE sql IMMUTABLE COST 1000 PARALLEL SAFE
    SET search_path TO '$user', 'public'
    AS $$
SELECT search_vector(spec ->> 'title', observespec_search_tags(spec), observespec_search_text(spec), observespec_search_code(spec))
$$;

CREATE FUNCTION delete_dashboard_findings() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  DELETE FROM findings WHERE dashboard_id = OLD.id AND observation_id IS NULL;
  DELETE FROM findings f USING agent_requests r
  WHERE r.dashboard_id = OLD.id AND f.agent_request_id = r.id AND f.observation_id IS NULL;
  RETURN OLD;
END
$$;

CREATE FUNCTION delete_personal_folder_contents() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  DELETE FROM saved_searches s USING folders f WHERE s.folder_id = f.id AND f.owner_id = OLD.id;
  DELETE FROM findings x USING folders f WHERE x.folder_id = f.id AND f.owner_id = OLD.id;
  DELETE FROM observations o USING folders f WHERE o.folder_id = f.id AND f.owner_id = OLD.id;
  DELETE FROM dashboards d USING folders f WHERE d.folder_id = f.id AND f.owner_id = OLD.id;
  RETURN OLD;
END
$$;

CREATE FUNCTION delete_unsaved_findings() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  DELETE FROM findings WHERE agent_request_id = OLD.id AND observation_id IS NULL;
  RETURN OLD;
END
$$;

CREATE FUNCTION general_folder_id() RETURNS uuid
    LANGUAGE sql STABLE
    AS $$ SELECT id FROM folders WHERE general $$;

CREATE FUNCTION health_problem_matches(kind text, query_failed boolean, no_data boolean, variable_failed boolean, duplicate_title boolean, last_viewed_at timestamp with time zone, created_at timestamp with time zone, unopened_before timestamp with time zone) RETURNS boolean
    LANGUAGE sql IMMUTABLE
    AS $$
  SELECT CASE kind
    WHEN 'any' THEN query_failed OR no_data OR variable_failed OR duplicate_title
      OR coalesce(last_viewed_at, created_at) < unopened_before
    WHEN 'query_failed' THEN query_failed
    WHEN 'no_data' THEN no_data
    WHEN 'variable_failed' THEN variable_failed
    WHEN 'duplicate_title' THEN duplicate_title
    WHEN 'unopened' THEN coalesce(last_viewed_at, created_at) < unopened_before
    ELSE false END
$$;

CREATE TABLE activity (
    seq bigint NOT NULL,
    xid xid8 DEFAULT pg_current_xact_id() NOT NULL,
    type text NOT NULL,
    target_kind text NOT NULL,
    target_id uuid NOT NULL,
    dashboard_id uuid,
    folder_id uuid,
    actor_kind text NOT NULL,
    actor_id uuid NOT NULL,
    actor_name text NOT NULL,
    origin text,
    origin_client text,
    run_id uuid,
    data jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    finding_id uuid,
    saved_search_id uuid,
    observation_id uuid,
    CONSTRAINT activity_actor_kind_check CHECK ((actor_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT activity_data_check CHECK ((jsonb_typeof(data) = 'object'::text)),
    CONSTRAINT activity_origin_check CHECK ((origin = ANY (ARRAY['web'::text, 'cli'::text, 'mcp'::text, 'api'::text]))),
    CONSTRAINT activity_origin_client_check CHECK (((origin_client IS NULL) OR ((origin = 'mcp'::text) AND ((length(origin_client) >= 1) AND (length(origin_client) <= 64))))),
    CONSTRAINT activity_target_kind_check CHECK ((target_kind = ANY (ARRAY['dashboard'::text, 'annotation'::text, 'finding'::text, 'run'::text, 'thread'::text, 'saved_search'::text, 'observation'::text, 'proposal'::text, 'user'::text, 'service_account'::text, 'webhook'::text, 'mcp_client'::text, 'outbound_domain'::text, 'grafana_connection'::text, 'public_link'::text, 'datasource'::text, 'access_request'::text]))),
    CONSTRAINT activity_type_check CHECK ((type ~ '^[a-z_]+\.[a-z_]+$'::text)),
    CONSTRAINT activity_type_target_check CHECK ((split_part(type, '.'::text, 1) = target_kind))
);

CREATE FUNCTION inbox_reason(a activity, reader uuid, editable uuid[], repair_prefix text) RETURNS text
    LANGUAGE sql STABLE
    AS $$
  SELECT CASE
    WHEN a.type = 'proposal.opened' AND a.folder_id = ANY(editable) AND EXISTS (
      SELECT 1 FROM proposals p
      WHERE p.id = a.target_id AND p.state = 'open' AND NOT (p.author_kind = 'user' AND p.author_id = reader)
    ) THEN 'proposal'
    WHEN a.type = 'observation.block_added' AND a.data->>'state' = 'requested'
      AND NOT (a.actor_kind = 'user' AND a.actor_id = reader) AND EXISTS (
      SELECT 1 FROM observations o
      JOIN observation_blocks b ON b.observation_id = o.id
      WHERE o.id = a.target_id AND b.id = (a.data->>'block_id')::uuid AND b.state = 'requested'
        AND o.created_by_kind = 'user' AND o.created_by_id = reader AND o.folder_id = ANY(editable)
    ) THEN 'requested_block'
    WHEN a.type = 'thread.mentioned' AND NOT (a.actor_kind = 'user' AND a.actor_id = reader)
      AND a.data->'mentioned'->>'kind' = 'user' AND a.data->'mentioned'->>'id' = reader::text
    THEN 'mention'
    WHEN a.type = 'thread.replied' AND (
      (NOT (a.actor_kind = 'user' AND a.actor_id = reader) AND EXISTS (
        SELECT 1 FROM thread_messages reply
        JOIN thread_messages earlier ON earlier.thread_id = reply.thread_id AND earlier.created_at < reply.created_at
        WHERE reply.id = (a.data->>'message_id')::uuid AND reply.thread_id = a.target_id
          AND earlier.author_kind = 'user' AND earlier.author_id = reader
      ))
      OR (a.run_id IS NOT NULL AND EXISTS (
        SELECT 1 FROM agent_requests r
        WHERE r.id = a.run_id AND r.requested_by_kind = 'user' AND r.requested_by_id = reader
      ))
    ) THEN 'reply'
    WHEN a.type = 'run.failed' AND EXISTS (
      SELECT 1 FROM agent_requests r
      WHERE r.id = a.target_id AND r.requested_by_kind = 'user' AND r.requested_by_id = reader AND r.state = 'failed'
        AND NOT EXISTS (SELECT 1 FROM agent_requests c WHERE c.continues_id = r.id)
    ) THEN 'run_failed'
    WHEN a.type = 'run.succeeded' AND EXISTS (
      SELECT 1 FROM agent_requests r
      WHERE r.id = a.target_id AND r.requested_by_kind = 'user' AND r.requested_by_id = reader
        AND r.intent = 'apply' AND r.base_version IS NOT NULL AND starts_with(r.prompt, repair_prefix) AND r.result_version IS NOT NULL
    ) THEN 'repaired'
    WHEN a.type IN ('proposal.merged', 'proposal.discarded')
      AND NOT (a.actor_kind = 'user' AND a.actor_id = reader) AND EXISTS (
      SELECT 1 FROM proposals p
      WHERE p.id = a.target_id AND p.author_kind = 'user' AND p.author_id = reader
    ) THEN 'proposal_closed'
    WHEN a.type = 'access_request.opened' AND EXISTS (
      SELECT 1 FROM access_requests r
      WHERE r.id = a.target_id AND reader = ANY(r.granters) AND (access_request_open(r) OR (r.granted_at IS NULL AND EXISTS (
        SELECT 1 FROM inbox_item_states s WHERE s.seq = a.seq AND s.user_id = reader AND s.done
      )))
    ) THEN 'access_request'
  END
$$;

CREATE FUNCTION inbox_superseded(a activity, reader uuid) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
  SELECT a.type = 'thread.replied' AND (
    EXISTS (
      SELECT 1 FROM activity m
      WHERE m.xid = a.xid AND m.type = 'thread.mentioned' AND m.target_id = a.target_id
        AND m.data->>'message_id' = a.data->>'message_id'
        AND m.data->'mentioned'->>'kind' = 'user' AND m.data->'mentioned'->>'id' = reader::text
    )
    OR EXISTS (
      SELECT 1 FROM thread_messages m
      JOIN agent_requests r ON r.id = m.run_id
      WHERE m.id = (a.data->>'message_id')::uuid AND m.run_failed
        AND r.state = 'failed' AND r.requested_by_kind = 'user' AND r.requested_by_id = reader
        AND NOT EXISTS (SELECT 1 FROM agent_requests c WHERE c.continues_id = r.id)
    )
  )
$$;

CREATE FUNCTION keep_copies_of_deleted_run() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  PERFORM keep_copies_of_run(OLD.id);
  RETURN OLD;
END
$$;

CREATE FUNCTION keep_copies_of_run(run uuid) RETURNS void
    LANGUAGE sql
    AS $$
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

CREATE FUNCTION mark_runs_kept_by_deleted_subject() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  UPDATE agent_requests r SET read_restricted = true
  WHERE OLD.id IN (r.dashboard_id, r.observation_id, r.subject_observation_id) AND NOT r.read_restricted
    AND run_answer_kept_with_admins(r.id, array_remove(ARRAY[r.dashboard_id, r.observation_id, r.subject_observation_id], OLD.id));
  RETURN OLD;
END
$$;

CREATE FUNCTION message_search_text(body text) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$ SELECT regexp_replace(body, '@\[([^\]\n]*)\]\(user:[0-9a-fA-F-]{36}\)', '@\1', 'g') $$;

CREATE FUNCTION message_search_vector(body text) RETURNS tsvector
    LANGUAGE sql IMMUTABLE COST 1000 PARALLEL SAFE
    SET search_path TO '$user', 'public'
    AS $$
SELECT search_vector('', '', message_search_text(body), '')
$$;

CREATE FUNCTION observation_block_search_vector(body jsonb) RETURNS tsvector
    LANGUAGE sql IMMUTABLE COST 1000 PARALLEL SAFE
    SET search_path TO '$user', 'public'
    AS $$
SELECT search_vector('', '', coalesce(body ->> 'markdown', ''), '')
$$;

CREATE FUNCTION observation_has_run_in_flight(observation uuid) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
  SELECT EXISTS (SELECT 1 FROM agent_requests r WHERE (r.observation_id = observation OR r.subject_observation_id = observation) AND r.state IN ('queued', 'running', 'canceling'))
$$;

CREATE FUNCTION observation_holds_restricted_answer(observation uuid) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
  SELECT EXISTS (SELECT 1 FROM observations o WHERE o.id = observation AND o.copied_restricted_answer)
    OR EXISTS (SELECT 1 FROM observation_blocks b JOIN agent_requests r ON r.id = b.run_id WHERE b.observation_id = observation AND run_answer_kept_with_admins(r.id, ARRAY[observation]))
    OR EXISTS (SELECT 1 FROM findings f JOIN agent_requests r ON r.id = f.agent_request_id WHERE f.observation_id = observation AND run_answer_kept_with_admins(r.id, ARRAY[observation]))
    OR EXISTS (SELECT 1 FROM threads t JOIN thread_messages m ON m.thread_id = t.id JOIN agent_requests r ON r.id = m.run_id WHERE t.observation_id = observation AND run_answer_kept_with_admins(r.id, ARRAY[observation]))
$$;

CREATE FUNCTION observation_holds_restricted_run(observation uuid) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
  SELECT EXISTS (SELECT 1 FROM agent_requests r WHERE (r.observation_id = observation OR r.subject_observation_id = observation) AND run_answer_kept_with_admins(r.id, ARRAY[observation]))
$$;

CREATE FUNCTION observation_rcas_immutable() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF NEW.run_id IS NOT NULL OR (to_jsonb(NEW) - 'run_id') <> (to_jsonb(OLD) - 'run_id') THEN
    RAISE EXCEPTION 'an RCA revision cannot be changed' USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END
$$;

CREATE FUNCTION observation_search_vector(title text, summary text) RETURNS tsvector
    LANGUAGE sql IMMUTABLE COST 1000 PARALLEL SAFE
    SET search_path TO '$user', 'public'
    AS $$
SELECT search_vector(title, '', coalesce(summary, ''), '')
$$;

CREATE FUNCTION observespec_datasource_uids(spec jsonb) RETURNS text[]
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    AS $_$
WITH refs AS (
  SELECT jsonb_path_query(spec, 'lax $.panels[*].datasource ? (@.type() == "string")') #>> '{}' AS ref
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].queries[*].datasource ? (@.type() == "string")') #>> '{}'
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].panels[*].datasource ? (@.type() == "string")') #>> '{}'
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].panels[*].queries[*].datasource ? (@.type() == "string")') #>> '{}'
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.variables[*].datasource ? (@.type() == "string")') #>> '{}'
), defaults AS (
  SELECT v ->> 'name' AS name, v ->> 'default' AS uid
  FROM jsonb_path_query(spec, 'lax $.variables[*] ? (@."type" == "datasource" && @."default".type() == "string")') AS v
)
SELECT coalesce(array_agg(DISTINCT uid COLLATE "C" ORDER BY uid COLLATE "C"), '{}'::text[]) FROM (
  SELECT ref AS uid FROM refs WHERE left(ref, 2) <> '${'
  UNION ALL SELECT d.uid FROM defaults d WHERE '${' || d.name || '}' IN (SELECT ref FROM refs)
) found
WHERE uid <> ''
$_$;

CREATE FUNCTION observespec_search_code(spec jsonb) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $_$
SELECT coalesce(string_agg(part, E'\n'), '') FROM (
  SELECT jsonb_path_query(spec, 'lax $.panels[*].queries[*].query ? (@.type() == "string")') #>> '{}' AS part
  UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].panels[*].queries[*].query ? (@.type() == "string")') #>> '{}'
) found
$_$;

CREATE FUNCTION observespec_search_tags(spec jsonb) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $_$
SELECT coalesce(string_agg(tag, ' '), '') FROM (
  SELECT jsonb_path_query(spec, 'lax $.tags[*] ? (@.type() == "string")') #>> '{}' AS tag
) found
$_$;

CREATE FUNCTION observespec_search_text(spec jsonb) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $_$
SELECT coalesce(spec ->> 'description', '') || ' ' || coalesce((
  SELECT string_agg(part, ' ') FROM (
    SELECT jsonb_path_query(spec, 'lax $.panels[*].title ? (@.type() == "string")') #>> '{}' AS part
    UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].description ? (@.type() == "string")') #>> '{}'
    UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].panels[*].title ? (@.type() == "string")') #>> '{}'
    UNION ALL SELECT jsonb_path_query(spec, 'lax $.panels[*].panels[*].description ? (@.type() == "string")') #>> '{}'
  ) found
), '')
$_$;

CREATE FUNCTION run_answer_kept_with_admins(run uuid, holders uuid[]) RETURNS boolean
    LANGUAGE sql STABLE
    AS $$
  SELECT r.read_restricted
    OR EXISTS (SELECT 1 FROM dashboards d JOIN folders f ON f.id = d.folder_id WHERE d.id = r.dashboard_id AND NOT d.id = ANY(array_remove(holders, NULL)) AND f.admins_only)
    OR EXISTS (SELECT 1 FROM observations o JOIN folders f ON f.id = o.folder_id WHERE o.id IN (r.observation_id, r.subject_observation_id) AND (o.run_id IS DISTINCT FROM r.id OR r.investigation) AND NOT o.id = ANY(array_remove(holders, NULL)) AND f.admins_only)
  FROM agent_requests r WHERE r.id = run
$$;

CREATE FUNCTION saved_search_search_vector(title text, query text, filters jsonb) RETURNS tsvector
    LANGUAGE sql IMMUTABLE COST 1000 PARALLEL SAFE
    SET search_path TO '$user', 'public'
    AS $$
SELECT search_vector(title, '', coalesce(filters ->> 'service', '') || ' ' || coalesce(filters ->> 'span', ''), query)
$$;

CREATE FUNCTION search_query(words text, prefix text) RETURNS tsquery
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$
SELECT CASE
  WHEN prefix IS NULL THEN exact.q
  ELSE exact.q || (websearch_to_tsquery('simple', left(words, length(words) - length(prefix))) && to_tsquery('simple', prefix || ':*'))
END
FROM (SELECT websearch_to_tsquery('simple', words) || websearch_to_tsquery('english', words) AS q) exact
$$;

CREATE FUNCTION search_recency(changed_at timestamp with time zone, as_of timestamp with time zone) RETURNS double precision
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$ SELECT 1 + 0.5 * power(0.5, greatest(0, extract(epoch FROM as_of - changed_at)) / 2592000.0) $$;

CREATE FUNCTION search_vector(title text, tags text, body text, code text) RETURNS tsvector
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$
SELECT setweight(to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(tags, '')), 'A') || ''' '':1'::tsvector
    || setweight(to_tsvector('english', coalesce(title, '') || ' ' || coalesce(tags, '')), 'A') || ''' '':1'::tsvector
    || setweight(to_tsvector('simple', coalesce(body, '')), 'B') || ''' '':1'::tsvector
    || setweight(to_tsvector('english', coalesce(body, '')), 'B') || ''' '':1'::tsvector
    || setweight(to_tsvector('simple', coalesce(code, '')), 'C') || ''' '':1'::tsvector
    || setweight(to_tsvector('english', coalesce(code, '')), 'C')
$$;

CREATE FUNCTION search_words(words text[]) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$ SELECT array_to_string(words, ' ') $$;

CREATE FUNCTION snapshots_immutable() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF NEW.run_id IS NOT NULL OR (to_jsonb(NEW) - 'run_id') <> (to_jsonb(OLD) - 'run_id') THEN
    RAISE EXCEPTION 'a snapshot cannot be changed' USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END
$$;

ALTER TABLE activity ALTER COLUMN seq ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME activity_seq_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

CREATE TABLE agent_request_builds (
    agent_request_id uuid NOT NULL,
    dashboard_id uuid,
    base_version integer,
    spec jsonb NOT NULL,
    unverified jsonb,
    summary text NOT NULL,
    saves integer DEFAULT 1 NOT NULL,
    panels integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    planned_panels integer DEFAULT 0 NOT NULL,
    folder_id uuid,
    CONSTRAINT agent_request_builds_folder_check CHECK (((folder_id IS NULL) OR (dashboard_id IS NULL))),
    CONSTRAINT agent_request_builds_planned_panels_check CHECK (((planned_panels >= 0) AND (planned_panels <= 200))),
    CONSTRAINT agent_request_builds_target_check CHECK (((dashboard_id IS NULL) = (base_version IS NULL))),
    CONSTRAINT agent_request_builds_unverified_check CHECK (((unverified IS NULL) OR (jsonb_typeof(unverified) = 'object'::text)))
);

CREATE TABLE agent_requests (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    dashboard_id uuid,
    comment_id uuid,
    prompt text NOT NULL,
    panel_id text,
    base_version integer,
    status text DEFAULT 'queued'::text NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    check_failures integer DEFAULT 0 NOT NULL,
    lease_until timestamp with time zone,
    result_version integer,
    error text,
    requested_by_kind text NOT NULL,
    requested_by_id uuid NOT NULL,
    requested_by_name text NOT NULL,
    requested_by_email text,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    steps jsonb DEFAULT '[]'::jsonb NOT NULL,
    run_usage jsonb,
    origin text,
    origin_client text,
    draft jsonb,
    suggestion jsonb,
    failure_code text,
    build_saved_at timestamp with time zone,
    cancel_requested_at timestamp with time zone,
    canceled_by_kind text,
    canceled_by_id uuid,
    canceled_by_name text,
    continues_id uuid,
    continue_note text,
    context_from timestamp with time zone,
    context_to timestamp with time zone,
    title text,
    pinned_at timestamp with time zone,
    archived_at timestamp with time zone,
    trigger text DEFAULT 'person'::text NOT NULL,
    state text DEFAULT 'queued'::text NOT NULL,
    result_kind text DEFAULT 'none'::text NOT NULL,
    assigned_kind text DEFAULT 'builtin'::text NOT NULL,
    assigned_id uuid,
    claimed_by text,
    observation_id uuid,
    thread_message_id uuid,
    thread_context text,
    chat_id uuid,
    retries_id uuid,
    time_zone text,
    thread_replied_at timestamp with time zone,
    anchor jsonb,
    block_id uuid,
    client_id uuid,
    intent text NOT NULL,
    kind text DEFAULT 'build'::text NOT NULL,
    applies_finding_id uuid,
    subject_observation_id uuid,
    investigation boolean DEFAULT false NOT NULL,
    read_restricted boolean DEFAULT false NOT NULL,
    as_of timestamp with time zone,
    claim_epoch integer DEFAULT 0 NOT NULL,
    thread_reply_abandoned_at timestamp with time zone,
    CONSTRAINT agent_requests_anchor_check CHECK (((anchor IS NULL) OR ((jsonb_typeof(anchor) = 'object'::text) AND (pg_column_size(anchor) <= 2048)))),
    CONSTRAINT agent_requests_applies_finding_check CHECK (((applies_finding_id IS NULL) OR (intent = 'apply'::text))),
    CONSTRAINT agent_requests_ask_shape_check CHECK (((intent <> 'ask'::text) OR ((base_version IS NULL) AND (comment_id IS NULL) AND ((panel_id IS NULL) OR (dashboard_id IS NOT NULL))))),
    CONSTRAINT agent_requests_ask_target_check CHECK (((intent = 'ask'::text) OR (dashboard_id IS NOT NULL))),
    CONSTRAINT agent_requests_assigned_check CHECK (((assigned_kind = 'builtin'::text) = (assigned_id IS NULL))),
    CONSTRAINT agent_requests_assigned_intent_check CHECK (((assigned_kind = 'builtin'::text) OR (intent <> 'suggest'::text))),
    CONSTRAINT agent_requests_assigned_kind_check CHECK ((assigned_kind = ANY (ARRAY['builtin'::text, 'grant'::text, 'service_account'::text]))),
    CONSTRAINT agent_requests_base_version_check CHECK ((base_version >= 1)),
    CONSTRAINT agent_requests_build_ask_check CHECK (((kind <> 'build'::text) OR (intent <> 'ask'::text) OR ((dashboard_id IS NULL) AND (subject_observation_id IS NULL)))),
    CONSTRAINT agent_requests_build_saved_check CHECK (((build_saved_at IS NULL) OR ((status = 'failed'::text) AND (dashboard_id IS NOT NULL) AND (result_version IS NOT NULL)))),
    CONSTRAINT agent_requests_canceled_by_check CHECK ((((cancel_requested_at IS NULL) = (canceled_by_kind IS NULL)) AND ((canceled_by_kind IS NULL) = (canceled_by_id IS NULL)) AND ((canceled_by_id IS NULL) = (canceled_by_name IS NULL)))),
    CONSTRAINT agent_requests_canceled_by_kind_check CHECK ((canceled_by_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT agent_requests_canceling_check CHECK (((state <> 'canceling'::text) OR (cancel_requested_at IS NOT NULL))),
    CONSTRAINT agent_requests_claimed_by_check CHECK (((length(claimed_by) >= 1) AND (length(claimed_by) <= 300))),
    CONSTRAINT agent_requests_error_check CHECK (((error IS NULL) OR (length(error) <= 500))),
    CONSTRAINT agent_requests_intent_check CHECK (((intent IS NOT NULL) AND (intent = ANY (ARRAY['ask'::text, 'apply'::text, 'suggest'::text])))),
    CONSTRAINT agent_requests_intent_context_check CHECK ((((context_from IS NULL) AND (context_to IS NULL)) OR ((intent = 'ask'::text) AND (context_from IS NOT NULL) AND (context_to IS NOT NULL) AND (context_from < context_to)))),
    CONSTRAINT agent_requests_intent_draft_check CHECK (((draft IS NULL) OR ((intent = 'suggest'::text) AND (jsonb_typeof(draft) = 'object'::text)))),
    CONSTRAINT agent_requests_intent_succeeded_check CHECK (((status <> 'succeeded'::text) OR (((intent <> 'suggest'::text) OR (suggestion IS NOT NULL)) AND ((result_version IS NULL) OR (dashboard_id IS NOT NULL))))),
    CONSTRAINT agent_requests_intent_suggestion_check CHECK (((suggestion IS NULL) OR ((intent = 'suggest'::text) AND (jsonb_typeof(suggestion) = 'object'::text)))),
    CONSTRAINT agent_requests_kind_check CHECK (((kind IS NOT NULL) AND ((kind = 'build'::text) OR ((kind = 'investigate'::text) AND (intent = 'ask'::text))))),
    CONSTRAINT agent_requests_origin_check CHECK ((origin = ANY (ARRAY['web'::text, 'cli'::text, 'mcp'::text, 'api'::text]))),
    CONSTRAINT agent_requests_origin_client_check CHECK (((origin_client IS NULL) OR ((origin IS NOT NULL) AND (origin = 'mcp'::text) AND ((length(origin_client) >= 1) AND (length(origin_client) <= 64))))),
    CONSTRAINT agent_requests_panel_id_check CHECK (((panel_id IS NULL) OR ((length(panel_id) >= 1) AND (length(panel_id) <= 64)))),
    CONSTRAINT agent_requests_prompt_check CHECK (((length(prompt) >= 1) AND (length(prompt) <= 4000))),
    CONSTRAINT agent_requests_requested_by_kind_check CHECK ((requested_by_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT agent_requests_result_kind_check CHECK ((result_kind = ANY (ARRAY['none'::text, 'version'::text, 'partial_version'::text, 'proposal'::text, 'suggestion'::text, 'observation'::text]))),
    CONSTRAINT agent_requests_run_usage_check CHECK (((run_usage IS NULL) OR (jsonb_typeof(run_usage) = 'object'::text))),
    CONSTRAINT agent_requests_state_check CHECK ((state = ANY (ARRAY['queued'::text, 'running'::text, 'canceling'::text, 'succeeded'::text, 'failed'::text, 'canceled'::text]))),
    CONSTRAINT agent_requests_state_status_check CHECK ((((state = 'queued'::text) AND (status = 'queued'::text)) OR ((state = ANY (ARRAY['running'::text, 'canceling'::text])) AND (status = 'running'::text)) OR ((state = 'succeeded'::text) AND (status = 'succeeded'::text)) OR ((state = 'failed'::text) AND (status = 'failed'::text) AND (failure_code IS DISTINCT FROM 'canceled'::text)) OR ((state = 'canceled'::text) AND (status = 'failed'::text) AND (NOT (failure_code IS DISTINCT FROM 'canceled'::text))))),
    CONSTRAINT agent_requests_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'succeeded'::text, 'failed'::text]))),
    CONSTRAINT agent_requests_steps_check CHECK ((jsonb_typeof(steps) = 'array'::text)),
    CONSTRAINT agent_requests_subject_observation_check CHECK (((subject_observation_id IS NULL) OR ((intent = 'ask'::text) AND (dashboard_id IS NULL)))),
    CONSTRAINT agent_requests_suggest_intent_shape_check CHECK (((intent <> 'suggest'::text) OR ((dashboard_id IS NOT NULL) AND (base_version IS NOT NULL) AND (panel_id IS NOT NULL) AND (comment_id IS NULL) AND (result_version IS NULL)))),
    CONSTRAINT agent_requests_thread_context_check CHECK (((length(thread_context) >= 1) AND (length(thread_context) <= 8000))),
    CONSTRAINT agent_requests_time_zone_check CHECK (((time_zone IS NULL) OR ((char_length(time_zone) >= 1) AND (char_length(time_zone) <= 64)))),
    CONSTRAINT agent_requests_title_check CHECK (((title IS NULL) OR ((char_length(title) >= 1) AND (char_length(title) <= 120)))),
    CONSTRAINT agent_requests_trigger_check CHECK ((trigger = ANY (ARRAY['person'::text, 'mention'::text, 'schedule'::text, 'webhook'::text, 'slack'::text])))
);

CREATE TABLE agent_workers (
    id text NOT NULL,
    principal_kind text NOT NULL,
    principal_id uuid,
    client text DEFAULT ''::text NOT NULL,
    seen_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT agent_workers_client_check CHECK ((length(client) <= 200)),
    CONSTRAINT agent_workers_id_check CHECK (((length(id) >= 1) AND (length(id) <= 300))),
    CONSTRAINT agent_workers_principal_check CHECK (((principal_kind = 'builtin'::text) = (principal_id IS NULL))),
    CONSTRAINT agent_workers_principal_kind_check CHECK ((principal_kind = ANY (ARRAY['builtin'::text, 'grant'::text, 'service_account'::text])))
);

CREATE TABLE annotations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    dashboard_id uuid,
    panel_id text DEFAULT ''::text NOT NULL,
    time_from timestamp with time zone NOT NULL,
    time_to timestamp with time zone,
    text text NOT NULL,
    tags text[] DEFAULT '{}'::text[] NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    created_by_kind text,
    origin text,
    origin_client text,
    copied_restricted_answer boolean DEFAULT false NOT NULL,
    CONSTRAINT annotations_created_by_kind_check CHECK ((created_by_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT annotations_created_by_kind_id_check CHECK (((created_by_kind IS NULL) OR (created_by IS NOT NULL))),
    CONSTRAINT annotations_origin_check CHECK ((origin = ANY (ARRAY['web'::text, 'cli'::text, 'mcp'::text, 'api'::text]))),
    CONSTRAINT annotations_origin_client_check CHECK (((origin_client IS NULL) OR ((origin IS NOT NULL) AND (origin = 'mcp'::text) AND ((length(origin_client) >= 1) AND (length(origin_client) <= 64))))),
    CONSTRAINT annotations_text_check CHECK ((length(text) <= 2000))
);

CREATE TABLE applied_dashboards (
    path text NOT NULL,
    dashboard_id uuid NOT NULL,
    content_sha256 text NOT NULL,
    applied_by_id uuid NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE audit_events (
    id uuid NOT NULL,
    seq bigint NOT NULL,
    xid xid8 DEFAULT pg_current_xact_id() NOT NULL,
    occurred_at timestamp with time zone NOT NULL,
    category text NOT NULL,
    action text NOT NULL,
    outcome text NOT NULL,
    reason text,
    status integer,
    duration_ms integer,
    actor_kind text NOT NULL,
    actor_id uuid,
    actor_name text,
    actor_role text,
    initiator text NOT NULL,
    agent_id uuid,
    agent_name text,
    run_id uuid,
    auth_method text,
    credential_id text,
    origin text NOT NULL,
    origin_client text,
    tool text,
    client_ip inet,
    user_agent text,
    request_id uuid,
    trace_id text,
    http_method text,
    route text,
    target_kind text,
    target_id text,
    target_name text,
    folder_id uuid,
    dashboard_id uuid,
    observation_id uuid,
    thread_id uuid,
    chat_id uuid,
    datasource_uid text,
    data jsonb DEFAULT '{}'::jsonb NOT NULL,
    activity_seq bigint,
    search tsvector GENERATED ALWAYS AS (to_tsvector('simple'::regconfig, ((((((((((((action || ' '::text) || COALESCE(actor_name, ''::text)) || ' '::text) || COALESCE(target_name, ''::text)) || ' '::text) || COALESCE(reason, ''::text)) || ' '::text) || COALESCE(tool, ''::text)) || ' '::text) || COALESCE(route, ''::text)) || ' '::text) || COALESCE((data ->> 'query'::text), ''::text)))) STORED NOT NULL,
    server_id text NOT NULL,
    server_version text NOT NULL,
    CONSTRAINT audit_events_action_check CHECK ((action ~ '^[a-z_]+\.[a-z_]+$'::text)),
    CONSTRAINT audit_events_actor_kind_check CHECK ((actor_kind = ANY (ARRAY['user'::text, 'service_account'::text, 'anonymous'::text, 'system'::text]))),
    CONSTRAINT audit_events_category_check CHECK ((category = ANY (ARRAY['auth'::text, 'access'::text, 'content'::text, 'conversation'::text, 'agent'::text, 'query'::text, 'admin'::text, 'system'::text]))),
    CONSTRAINT audit_events_data_check CHECK ((jsonb_typeof(data) = 'object'::text)),
    CONSTRAINT audit_events_initiator_check CHECK ((initiator = ANY (ARRAY['person'::text, 'agent'::text, 'system'::text]))),
    CONSTRAINT audit_events_outcome_check CHECK ((outcome = ANY (ARRAY['success'::text, 'denied'::text, 'failed'::text, 'unknown'::text])))
)
PARTITION BY RANGE (occurred_at);

CREATE SEQUENCE audit_events_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE audit_events_seq OWNED BY audit_events.seq;

CREATE TABLE audit_export_cursor (
    only_row boolean DEFAULT true NOT NULL,
    xid xid8 NOT NULL,
    seq bigint NOT NULL,
    occurred_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT audit_export_cursor_only_row_check CHECK (only_row)
);

CREATE TABLE chats (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_kind text NOT NULL,
    owner_id uuid NOT NULL,
    title text,
    dashboard_id uuid,
    observation_id uuid,
    pinned_at timestamp with time zone,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_turn_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chats_owner_kind_check CHECK ((owner_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT chats_subject_check CHECK (((dashboard_id IS NULL) OR (observation_id IS NULL))),
    CONSTRAINT chats_title_check CHECK (((title IS NULL) OR ((length(title) >= 1) AND (length(title) <= 120))))
);

CREATE TABLE dashboard_health (
    dashboard_id uuid NOT NULL,
    version integer NOT NULL,
    checked_at timestamp with time zone NOT NULL,
    skipped text DEFAULT ''::text NOT NULL,
    issues jsonb DEFAULT '[]'::jsonb NOT NULL
);

CREATE TABLE dashboard_views (
    user_id uuid NOT NULL,
    dashboard_id uuid NOT NULL,
    viewed_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE dashboards (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    title text NOT NULL,
    tags text[] DEFAULT '{}'::text[] NOT NULL,
    latest_version integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    created_by_kind text NOT NULL,
    created_by_id uuid NOT NULL,
    created_by_name text NOT NULL,
    created_by_email text,
    created_origin text,
    created_origin_client text,
    created_via text NOT NULL,
    folder_id uuid DEFAULT general_folder_id() NOT NULL,
    created_run_id uuid,
    copied_restricted_answer boolean DEFAULT false NOT NULL,
    CONSTRAINT dashboards_created_by_kind_check CHECK ((created_by_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT dashboards_created_origin_check CHECK ((created_origin = ANY (ARRAY['web'::text, 'cli'::text, 'mcp'::text, 'api'::text]))),
    CONSTRAINT dashboards_created_via_check CHECK ((created_via = ANY (ARRAY['direct'::text, 'agent'::text]))),
    CONSTRAINT dashboards_latest_version_check CHECK ((latest_version >= 1)),
    CONSTRAINT dashboards_title_check CHECK (((length(title) >= 1) AND (length(title) <= 200)))
);

CREATE TABLE folders (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    title text NOT NULL,
    general boolean DEFAULT false NOT NULL,
    created_by_kind text,
    created_by_id uuid,
    created_by_name text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    personal boolean DEFAULT false NOT NULL,
    owner_id uuid,
    admins_only boolean DEFAULT false NOT NULL,
    CONSTRAINT folders_admins_only_check CHECK ((NOT (admins_only AND (general OR personal)))),
    CONSTRAINT folders_created_by_kind_check CHECK ((created_by_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT folders_kind_check CHECK ((NOT (personal AND general))),
    CONSTRAINT folders_owner_check CHECK (((owner_id IS NOT NULL) = personal)),
    CONSTRAINT folders_title_check CHECK ((((char_length(title) >= 1) AND (char_length(title) <= 100)) AND (title = btrim(title))))
);

CREATE VIEW dashboard_health_status AS
SELECT d.id, d.title, d.created_at, d.folder_id,
  f.title AS folder_title, f.general AS folder_general, f.personal AS folder_personal,
  h.version AS checked_version, h.checked_at, coalesce(h.skipped, '')::text AS skipped,
  coalesce(h.issues, '[]'::jsonb)::jsonb AS issues,
  (h.version >= d.latest_version AND coalesce(h.issues, '[]'::jsonb) @> '[{"kind":"query_failed"}]'::jsonb)::boolean AS query_failed,
  (h.version >= d.latest_version AND coalesce(h.issues, '[]'::jsonb) @> '[{"kind":"no_data"}]'::jsonb)::boolean AS no_data,
  (h.version >= d.latest_version AND coalesce(h.issues, '[]'::jsonb) @> '[{"kind":"variable_failed"}]'::jsonb)::boolean AS variable_failed,
  (SELECT max(dv.viewed_at) FROM dashboard_views dv WHERE dv.dashboard_id = d.id)::timestamptz AS last_viewed_at,
  EXISTS (
    SELECT 1 FROM dashboards o
    WHERE o.folder_id = d.folder_id AND o.id <> d.id AND lower(o.title) = lower(d.title)
  ) AS duplicate_title,
  coalesce(h.version < d.latest_version, false) AS outdated,
  (coalesce(h.issues, '[]'::jsonb) @> '[{"kind":"query_failed"}]'::jsonb
    OR coalesce(h.issues, '[]'::jsonb) @> '[{"kind":"no_data"}]'::jsonb
    OR coalesce(h.issues, '[]'::jsonb) @> '[{"kind":"variable_failed"}]'::jsonb)::boolean AS old_check_problems
FROM dashboards d
JOIN folders f ON f.id = d.folder_id
LEFT JOIN dashboard_health h ON h.dashboard_id = d.id;

CREATE TABLE dashboard_stars (
    user_id uuid NOT NULL,
    dashboard_id uuid NOT NULL,
    starred_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE dashboard_versions (
    dashboard_id uuid NOT NULL,
    version integer NOT NULL,
    parent_version integer,
    restored_from integer,
    spec jsonb NOT NULL,
    summary text NOT NULL,
    author_kind text NOT NULL,
    author_id uuid NOT NULL,
    author_name text NOT NULL,
    author_email text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    agent_request_id uuid,
    origin text,
    origin_client text,
    datasource_uids text[] GENERATED ALWAYS AS (observespec_datasource_uids(spec)) STORED NOT NULL,
    unverified jsonb,
    merged_proposal_id uuid,
    CONSTRAINT dashboard_versions_author_kind_check CHECK ((author_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT dashboard_versions_check CHECK ((((version = 1) AND (parent_version IS NULL)) OR (parent_version = (version - 1)))),
    CONSTRAINT dashboard_versions_check1 CHECK (((restored_from IS NULL) OR (restored_from < version))),
    CONSTRAINT dashboard_versions_origin_check CHECK ((origin = ANY (ARRAY['web'::text, 'cli'::text, 'mcp'::text, 'api'::text]))),
    CONSTRAINT dashboard_versions_origin_client_check CHECK (((origin_client IS NULL) OR ((origin IS NOT NULL) AND (origin = 'mcp'::text) AND ((length(origin_client) >= 1) AND (length(origin_client) <= 64))))),
    CONSTRAINT dashboard_versions_spec_check CHECK ((jsonb_typeof(spec) = 'object'::text)),
    CONSTRAINT dashboard_versions_summary_check CHECK (((length(summary) >= 1) AND (length(summary) <= 500))),
    CONSTRAINT dashboard_versions_unverified_check CHECK (((unverified IS NULL) OR (jsonb_typeof(unverified) = 'object'::text))),
    CONSTRAINT dashboard_versions_version_check CHECK ((version >= 1))
);

CREATE TABLE dashboard_view_days (
    dashboard_id uuid NOT NULL,
    user_id uuid NOT NULL,
    day date NOT NULL
);

CREATE TABLE dashboard_viewers (
    dashboard_id uuid NOT NULL,
    user_id uuid NOT NULL,
    seen_at timestamp with time zone NOT NULL
);

CREATE TABLE datasources (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    uid text NOT NULL,
    type text NOT NULL,
    name text NOT NULL,
    config jsonb NOT NULL,
    secrets bytea,
    secret_names text[] DEFAULT '{}'::text[] NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    audience text DEFAULT 'everyone'::text NOT NULL,
    CONSTRAINT datasources_audience_check CHECK ((audience = ANY (ARRAY['everyone'::text, 'admins'::text]))),
    CONSTRAINT datasources_config_check CHECK ((jsonb_typeof(config) = 'object'::text)),
    CONSTRAINT datasources_name_check CHECK (((length(name) >= 1) AND (length(name) <= 100))),
    CONSTRAINT datasources_secret_names_check CHECK (((secrets IS NULL) = (cardinality(secret_names) = 0))),
    CONSTRAINT datasources_secrets_check CHECK (((secrets IS NULL) OR ((octet_length(secrets) >= 1) AND (octet_length(secrets) <= 65536)))),
    CONSTRAINT datasources_type_check CHECK ((type = ANY (ARRAY['prometheus'::text, 'clickhouse'::text, 'loki'::text, 'tempo'::text, 'elasticsearch'::text, 'opensearch'::text]))),
    CONSTRAINT datasources_uid_check CHECK ((uid ~ '^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$'::text))
);

CREATE TABLE draft_query_spend (
    principal_key text NOT NULL,
    window_start timestamp with time zone NOT NULL,
    calls integer NOT NULL,
    CONSTRAINT draft_query_spend_calls_check CHECK ((calls > 0))
);

CREATE TABLE findings (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    agent_request_id uuid,
    dashboard_id uuid,
    title text NOT NULL,
    markdown text NOT NULL,
    evidence jsonb DEFAULT '[]'::jsonb NOT NULL,
    time_from timestamp with time zone NOT NULL,
    time_to timestamp with time zone NOT NULL,
    created_by_kind text NOT NULL,
    created_by_id uuid NOT NULL,
    created_by_name text NOT NULL,
    annotation_id uuid,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    origin text,
    origin_client text,
    folder_id uuid,
    observation_id uuid,
    ask_dashboard_id uuid,
    suggested_change text,
    observe_worthy boolean DEFAULT false NOT NULL,
    as_of timestamp with time zone,
    root_cause_component text,
    root_cause_kind text,
    root_cause_summary text,
    onset timestamp with time zone,
    impact text,
    confidence text,
    ruled_out text[],
    next_steps text[],
    no_incident boolean,
    unverified jsonb,
    causes jsonb,
    CONSTRAINT findings_ask_dashboard_check CHECK (((ask_dashboard_id IS NULL) OR (dashboard_id IS NULL))),
    CONSTRAINT findings_created_by_kind_check CHECK ((created_by_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT findings_evidence_check CHECK (
CASE
    WHEN (jsonb_typeof(evidence) = 'array'::text) THEN (jsonb_array_length(evidence) <= 4)
    ELSE false
END),
    CONSTRAINT findings_markdown_check CHECK (((char_length(markdown) >= 1) AND (char_length(markdown) <= 4000))),
    CONSTRAINT findings_origin_check CHECK ((origin = ANY (ARRAY['web'::text, 'cli'::text, 'mcp'::text, 'api'::text]))),
    CONSTRAINT findings_origin_client_check CHECK (((origin_client IS NULL) OR ((origin IS NOT NULL) AND (origin = 'mcp'::text) AND ((length(origin_client) >= 1) AND (length(origin_client) <= 64))))),
    CONSTRAINT findings_placement_check CHECK (((dashboard_id IS NULL) OR (folder_id IS NULL))),
    CONSTRAINT findings_suggested_change_check CHECK (((suggested_change IS NULL) OR ((char_length(suggested_change) >= 1) AND (char_length(suggested_change) <= 500)))),
    CONSTRAINT findings_title_check CHECK (((char_length(title) >= 1) AND (char_length(title) <= 120))),
    CONSTRAINT findings_unsaved_run_check CHECK (((observation_id IS NOT NULL) OR (agent_request_id IS NOT NULL))),
    CONSTRAINT findings_window_check CHECK ((time_from < time_to))
);

CREATE TABLE folder_permissions (
    folder_id uuid NOT NULL,
    user_id uuid,
    group_name text,
    level text NOT NULL,
    created_by_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT folder_permissions_check CHECK (((user_id IS NULL) <> (group_name IS NULL))),
    CONSTRAINT folder_permissions_group_name_check CHECK (((octet_length(group_name) >= 1) AND (octet_length(group_name) <= 256))),
    CONSTRAINT folder_permissions_level_check CHECK ((level = ANY (ARRAY['view'::text, 'comment'::text, 'edit'::text, 'admin'::text])))
);

CREATE TABLE grafana_connections (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    url text NOT NULL,
    token_ciphertext bytea NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    last_test_at timestamp with time zone,
    last_test_ok boolean DEFAULT false NOT NULL,
    last_test_message text DEFAULT ''::text NOT NULL,
    last_test_failure_kind text,
    CONSTRAINT grafana_connections_failure_kind_check CHECK (((last_test_failure_kind IS NULL) OR ((NOT last_test_ok) AND (last_test_failure_kind = ANY (ARRAY['transient'::text, 'credentials'::text, 'configuration'::text, 'unsupported'::text, 'unknown'::text]))))),
    CONSTRAINT grafana_connections_name_check CHECK (((length(name) >= 1) AND (length(name) <= 100))),
    CONSTRAINT grafana_connections_token_ciphertext_check CHECK (((octet_length(token_ciphertext) >= 1) AND (octet_length(token_ciphertext) <= 8192))),
    CONSTRAINT grafana_connections_untested_check CHECK (((last_test_at IS NOT NULL) OR ((NOT last_test_ok) AND (last_test_message = ''::text) AND (last_test_failure_kind IS NULL)))),
    CONSTRAINT grafana_connections_url_check CHECK (((length(url) >= 1) AND (length(url) <= 2048)))
);

CREATE TABLE grafana_sync_items (
    id bigint NOT NULL,
    sync_id uuid NOT NULL,
    uid text NOT NULL,
    title text NOT NULL,
    outcome text NOT NULL,
    dashboard_id uuid,
    unverified_panels integer DEFAULT 0 NOT NULL,
    placeholder_panels integer DEFAULT 0 NOT NULL,
    error text,
    CONSTRAINT grafana_sync_items_error_check CHECK (((outcome = 'failed'::text) = (error IS NOT NULL))),
    CONSTRAINT grafana_sync_items_outcome_check CHECK ((outcome = ANY (ARRAY['imported'::text, 'updated'::text, 'skipped'::text, 'failed'::text])))
);

ALTER TABLE grafana_sync_items ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME grafana_sync_items_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

CREATE TABLE grafana_syncs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    connection_id uuid NOT NULL,
    state text NOT NULL,
    request jsonb,
    started_by_kind text,
    started_by_id uuid,
    total integer DEFAULT 0 NOT NULL,
    done integer DEFAULT 0 NOT NULL,
    failed integer DEFAULT 0 NOT NULL,
    skipped integer DEFAULT 0 NOT NULL,
    error text,
    started_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    finished_at timestamp with time zone,
    lease_id uuid,
    leased_by text,
    lease_until timestamp with time zone,
    lease_losses integer DEFAULT 0 NOT NULL,
    CONSTRAINT grafana_syncs_counts_check CHECK (((failed + skipped) <= done)),
    CONSTRAINT grafana_syncs_done_check CHECK ((done >= 0)),
    CONSTRAINT grafana_syncs_error_check CHECK (((error IS NULL) OR (state = 'failed'::text))),
    CONSTRAINT grafana_syncs_failed_check CHECK ((failed >= 0)),
    CONSTRAINT grafana_syncs_finished_check CHECK (((state = 'running'::text) = (finished_at IS NULL))),
    CONSTRAINT grafana_syncs_lease_check CHECK ((((lease_id IS NULL) = (lease_until IS NULL)) AND ((lease_id IS NULL) = (leased_by IS NULL)))),
    CONSTRAINT grafana_syncs_lease_losses_check CHECK (((lease_losses >= 0) AND (lease_losses <= 2))),
    CONSTRAINT grafana_syncs_lease_state_check CHECK (((state = 'running'::text) OR (lease_id IS NULL))),
    CONSTRAINT grafana_syncs_request_check CHECK (((request IS NULL) OR (jsonb_typeof(request) = 'object'::text))),
    CONSTRAINT grafana_syncs_resumable_check CHECK (((state <> 'running'::text) OR ((request IS NOT NULL) AND (started_by_kind IS NOT NULL) AND (started_by_id IS NOT NULL)))),
    CONSTRAINT grafana_syncs_skipped_check CHECK ((skipped >= 0)),
    CONSTRAINT grafana_syncs_started_by_kind_check CHECK ((started_by_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT grafana_syncs_state_check CHECK ((state = ANY (ARRAY['running'::text, 'finished'::text, 'failed'::text, 'canceled'::text]))),
    CONSTRAINT grafana_syncs_total_check CHECK ((total >= 0))
);

CREATE TABLE home_dashboards (
    user_id uuid NOT NULL,
    dashboard_id uuid NOT NULL,
    set_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE imports (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    dashboard_id uuid NOT NULL,
    version integer NOT NULL,
    source text NOT NULL,
    importer_version integer NOT NULL,
    original bytea NOT NULL,
    mapping jsonb NOT NULL,
    summary jsonb NOT NULL,
    datasources jsonb NOT NULL,
    report jsonb NOT NULL,
    title text NOT NULL,
    created_by_kind text NOT NULL,
    created_by_id uuid NOT NULL,
    created_by_name text NOT NULL,
    created_by_email text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    grafana_uid text DEFAULT ''::text NOT NULL,
    CONSTRAINT imports_created_by_kind_check CHECK ((created_by_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT imports_datasources_check CHECK ((jsonb_typeof(datasources) = 'array'::text)),
    CONSTRAINT imports_importer_version_check CHECK ((importer_version >= 1)),
    CONSTRAINT imports_mapping_check CHECK ((jsonb_typeof(mapping) = 'object'::text)),
    CONSTRAINT imports_original_check CHECK (((octet_length(original) >= 1) AND (octet_length(original) <= 8388608))),
    CONSTRAINT imports_report_check CHECK ((jsonb_typeof(report) = 'object'::text)),
    CONSTRAINT imports_source_check CHECK ((source = 'grafana'::text)),
    CONSTRAINT imports_summary_check CHECK ((jsonb_typeof(summary) = 'object'::text)),
    CONSTRAINT imports_title_check CHECK (((length(title) >= 1) AND (length(title) <= 200)))
);

CREATE TABLE inbox_cursors (
    user_id uuid NOT NULL,
    read_xid xid8 NOT NULL,
    read_seq bigint NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE inbox_item_states (
    user_id uuid NOT NULL,
    seq bigint NOT NULL,
    unread boolean NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    done boolean DEFAULT false NOT NULL
);

CREATE TABLE instance (
    singleton boolean DEFAULT true NOT NULL,
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT instance_singleton_check CHECK (singleton)
);

CREATE TABLE jobs (
    name text NOT NULL,
    every_seconds integer NOT NULL,
    next_run_at timestamp with time zone NOT NULL,
    leased_by text DEFAULT ''::text NOT NULL,
    lease_until timestamp with time zone,
    last_started_at timestamp with time zone,
    last_finished_at timestamp with time zone,
    last_outcome text DEFAULT ''::text NOT NULL,
    last_error text DEFAULT ''::text NOT NULL
);

CREATE TABLE mcp_source_tools (
    source_id uuid NOT NULL,
    name text NOT NULL,
    state text NOT NULL,
    refusal text,
    description text NOT NULL,
    input_schema jsonb NOT NULL,
    hash text NOT NULL,
    seen_description text,
    seen_input_schema jsonb,
    seen_hash text,
    approved_by uuid,
    approved_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    read_only boolean DEFAULT false NOT NULL,
    seen_read_only boolean,
    CONSTRAINT mcp_source_tools_approved_check CHECK (((state = ANY (ARRAY['approved'::text, 'changed'::text, 'gone'::text])) = (approved_at IS NOT NULL))),
    CONSTRAINT mcp_source_tools_hash_check CHECK ((hash ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT mcp_source_tools_name_check CHECK (((length(name) >= 1) AND (length(name) <= 128))),
    CONSTRAINT mcp_source_tools_refusal_check CHECK (((state = 'refused'::text) = (refusal IS NOT NULL))),
    CONSTRAINT mcp_source_tools_schema_check CHECK ((jsonb_typeof(input_schema) = 'object'::text)),
    CONSTRAINT mcp_source_tools_seen_check CHECK (((state = 'changed'::text) = ((seen_hash IS NOT NULL) AND (seen_description IS NOT NULL) AND (seen_input_schema IS NOT NULL)))),
    CONSTRAINT mcp_source_tools_state_check CHECK ((state = ANY (ARRAY['offered'::text, 'approved'::text, 'changed'::text, 'gone'::text, 'refused'::text])))
);

CREATE TABLE mcp_sources (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    uid text NOT NULL,
    name text NOT NULL,
    description text,
    url text NOT NULL,
    audience text DEFAULT 'everyone'::text NOT NULL,
    secret_header text,
    secret bytea,
    provisioned boolean DEFAULT false NOT NULL,
    checked_at timestamp with time zone,
    failure text,
    created_by uuid,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    allow_private boolean DEFAULT false NOT NULL,
    CONSTRAINT mcp_sources_audience_check CHECK ((audience = ANY (ARRAY['everyone'::text, 'admins'::text]))),
    CONSTRAINT mcp_sources_description_check CHECK (((description IS NULL) OR ((length(description) >= 1) AND (length(description) <= 300)))),
    CONSTRAINT mcp_sources_failure_check CHECK (((failure IS NULL) OR (checked_at IS NOT NULL))),
    CONSTRAINT mcp_sources_name_check CHECK (((length(name) >= 1) AND (length(name) <= 100))),
    CONSTRAINT mcp_sources_secret_check CHECK (((secret IS NULL) OR ((octet_length(secret) >= 1) AND (octet_length(secret) <= 8192)))),
    CONSTRAINT mcp_sources_secret_header_check CHECK (((secret_header IS NULL) OR ((length(secret_header) >= 1) AND (length(secret_header) <= 100)))),
    CONSTRAINT mcp_sources_secret_header_pair_check CHECK (((secret IS NULL) OR (secret_header IS NOT NULL))),
    CONSTRAINT mcp_sources_uid_check CHECK ((uid ~ '^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$'::text)),
    CONSTRAINT mcp_sources_url_check CHECK (((length(url) >= 1) AND (length(url) <= 2048)))
);

CREATE TABLE oauth_clients (
    id text DEFAULT (gen_random_uuid())::text NOT NULL,
    name text,
    redirect_uris text[] NOT NULL,
    registered_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT oauth_clients_id_check CHECK (((length(id) >= 1) AND (length(id) <= 255))),
    CONSTRAINT oauth_clients_name_check CHECK (((name IS NULL) OR ((char_length(name) >= 1) AND (char_length(name) <= 64)))),
    CONSTRAINT oauth_clients_redirect_uris_check CHECK (((cardinality(redirect_uris) >= 1) AND (cardinality(redirect_uris) <= 10)))
);

CREATE TABLE oauth_codes (
    code_hash bytea NOT NULL,
    client_id text NOT NULL,
    user_id uuid NOT NULL,
    redirect_uri text NOT NULL,
    code_challenge text NOT NULL,
    resource text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    grant_id uuid,
    CONSTRAINT oauth_codes_code_hash_check CHECK ((length(code_hash) = 32)),
    CONSTRAINT oauth_codes_used_check CHECK (((used_at IS NULL) = (grant_id IS NULL)))
);

CREATE TABLE oauth_grants (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    client_id text NOT NULL,
    user_id uuid NOT NULL,
    resource text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    last_used_at timestamp with time zone,
    revoked_at timestamp with time zone,
    revoked_reason text,
    revoked_by uuid,
    mode text DEFAULT 'direct'::text NOT NULL,
    CONSTRAINT oauth_grants_mode_check CHECK ((mode = ANY (ARRAY['direct'::text, 'propose'::text]))),
    CONSTRAINT oauth_grants_revoked_check CHECK (((revoked_at IS NULL) = (revoked_reason IS NULL))),
    CONSTRAINT oauth_grants_revoked_reason_check CHECK ((revoked_reason = ANY (ARRAY['person'::text, 'admin'::text, 'client'::text, 'replaced'::text, 'refresh_reuse'::text, 'code_reuse'::text])))
);

CREATE TABLE oauth_requests (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    client_id text NOT NULL,
    redirect_uri text NOT NULL,
    state text,
    code_challenge text NOT NULL,
    resource text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT oauth_requests_code_challenge_check CHECK ((code_challenge ~ '^[A-Za-z0-9_-]{43}$'::text)),
    CONSTRAINT oauth_requests_redirect_uri_check CHECK (((length(redirect_uri) >= 1) AND (length(redirect_uri) <= 2048))),
    CONSTRAINT oauth_requests_state_check CHECK (((state IS NULL) OR ((length(state) >= 1) AND (length(state) <= 1024))))
);

CREATE TABLE oauth_tokens (
    token_hash bytea NOT NULL,
    grant_id uuid NOT NULL,
    kind text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    rotated_at timestamp with time zone,
    CONSTRAINT oauth_tokens_kind_check CHECK ((kind = ANY (ARRAY['access'::text, 'refresh'::text]))),
    CONSTRAINT oauth_tokens_rotated_check CHECK (((kind = 'refresh'::text) OR (rotated_at IS NULL))),
    CONSTRAINT oauth_tokens_token_hash_check CHECK ((length(token_hash) = 32))
);

CREATE TABLE observation_blocks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    observation_id uuid NOT NULL,
    kind text NOT NULL,
    state text DEFAULT 'live'::text NOT NULL,
    "position" integer NOT NULL,
    width text DEFAULT 'full'::text NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    body jsonb NOT NULL,
    snapshot_id uuid,
    run_id uuid,
    created_by_kind text NOT NULL,
    created_by_id uuid NOT NULL,
    created_by_name text NOT NULL,
    origin text,
    origin_client text,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    height integer,
    CONSTRAINT observation_blocks_body_check CHECK ((jsonb_typeof(body) = 'object'::text)),
    CONSTRAINT observation_blocks_created_by_kind_check CHECK ((created_by_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT observation_blocks_height_check CHECK (((height IS NULL) OR ((height >= 120) AND (height <= 960)))),
    CONSTRAINT observation_blocks_kind_check CHECK ((kind = ANY (ARRAY['text'::text, 'panel'::text, 'snapshot'::text, 'run'::text]))),
    CONSTRAINT observation_blocks_origin_check CHECK ((origin = ANY (ARRAY['web'::text, 'cli'::text, 'mcp'::text, 'api'::text]))),
    CONSTRAINT observation_blocks_origin_client_check CHECK (((origin_client IS NULL) OR ((origin IS NOT NULL) AND (origin = 'mcp'::text) AND ((length(origin_client) >= 1) AND (length(origin_client) <= 64))))),
    CONSTRAINT observation_blocks_position_check CHECK (("position" >= 0)),
    CONSTRAINT observation_blocks_snapshot_check CHECK (((kind = 'snapshot'::text) = (snapshot_id IS NOT NULL))),
    CONSTRAINT observation_blocks_state_check CHECK ((state = ANY (ARRAY['live'::text, 'requested'::text]))),
    CONSTRAINT observation_blocks_version_check CHECK ((version >= 1)),
    CONSTRAINT observation_blocks_width_check CHECK ((width = ANY (ARRAY['third'::text, 'half'::text, 'two_thirds'::text, 'full'::text])))
);

CREATE TABLE observation_dashboards (
    observation_id uuid NOT NULL,
    block_id uuid,
    dashboard_id uuid NOT NULL,
    created_by_kind text NOT NULL,
    created_by_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT observation_dashboards_created_by_kind_check CHECK ((created_by_kind = ANY (ARRAY['user'::text, 'service_account'::text])))
);

CREATE TABLE observation_pins (
    observation_id uuid NOT NULL,
    dashboard_id uuid NOT NULL,
    annotation_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

CREATE TABLE observation_rcas (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    observation_id uuid NOT NULL,
    revision integer NOT NULL,
    title text NOT NULL,
    summary text,
    observation_state text NOT NULL,
    time_from timestamp with time zone,
    time_to timestamp with time zone,
    content jsonb NOT NULL,
    created_by_kind text NOT NULL,
    created_by_id uuid NOT NULL,
    created_by_name text NOT NULL,
    origin text,
    origin_client text,
    run_id uuid,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT observation_rcas_content_check CHECK ((jsonb_typeof(content) = 'object'::text)),
    CONSTRAINT observation_rcas_created_by_kind_check CHECK ((created_by_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT observation_rcas_observation_state_check CHECK ((observation_state = ANY (ARRAY['open'::text, 'resolved'::text]))),
    CONSTRAINT observation_rcas_origin_check CHECK ((origin = ANY (ARRAY['web'::text, 'cli'::text, 'mcp'::text, 'api'::text]))),
    CONSTRAINT observation_rcas_origin_client_check CHECK (((origin_client IS NULL) OR ((origin IS NOT NULL) AND (origin = 'mcp'::text) AND ((length(origin_client) >= 1) AND (length(origin_client) <= 64))))),
    CONSTRAINT observation_rcas_revision_check CHECK ((revision >= 1)),
    CONSTRAINT observation_rcas_summary_check CHECK (((summary IS NULL) OR ((char_length(summary) >= 1) AND (char_length(summary) <= 500)))),
    CONSTRAINT observation_rcas_title_check CHECK (((char_length(title) >= 1) AND (char_length(title) <= 200)))
);

CREATE TABLE observations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    folder_id uuid NOT NULL,
    title text NOT NULL,
    state text DEFAULT 'open'::text NOT NULL,
    summary text,
    time_from timestamp with time zone,
    time_to timestamp with time zone,
    created_by_kind text NOT NULL,
    created_by_id uuid NOT NULL,
    created_by_name text NOT NULL,
    origin text,
    origin_client text,
    run_id uuid,
    resolved_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    copied_restricted_answer boolean DEFAULT false NOT NULL,
    CONSTRAINT observations_created_by_kind_check CHECK ((created_by_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT observations_origin_check CHECK ((origin = ANY (ARRAY['web'::text, 'cli'::text, 'mcp'::text, 'api'::text]))),
    CONSTRAINT observations_origin_client_check CHECK (((origin_client IS NULL) OR ((origin IS NOT NULL) AND (origin = 'mcp'::text) AND ((length(origin_client) >= 1) AND (length(origin_client) <= 64))))),
    CONSTRAINT observations_resolved_at_check CHECK (((state = 'resolved'::text) = (resolved_at IS NOT NULL))),
    CONSTRAINT observations_resolved_check CHECK (((state = 'open'::text) OR (summary IS NOT NULL))),
    CONSTRAINT observations_state_check CHECK ((state = ANY (ARRAY['open'::text, 'resolved'::text]))),
    CONSTRAINT observations_summary_check CHECK (((summary IS NULL) OR ((char_length(summary) >= 1) AND (char_length(summary) <= 500)))),
    CONSTRAINT observations_time_check CHECK ((((time_from IS NULL) = (time_to IS NULL)) AND ((time_from IS NULL) OR (time_from < time_to)))),
    CONSTRAINT observations_title_check CHECK (((char_length(title) >= 1) AND (char_length(title) <= 200)))
);

CREATE TABLE oidc_login_states (
    state_hash bytea NOT NULL,
    nonce text NOT NULL,
    code_verifier text NOT NULL,
    return_to text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT oidc_login_states_state_hash_check CHECK ((length(state_hash) = 32))
);

CREATE TABLE outbound_domains (
    domain text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    created_by_id uuid,
    created_by_name text NOT NULL,
    for_connection boolean DEFAULT false NOT NULL,
    CONSTRAINT outbound_domains_domain_check CHECK (((domain = lower(domain)) AND (domain <> ''::text)))
);

CREATE TABLE proposal_revisions (
    proposal_id uuid NOT NULL,
    revision integer NOT NULL,
    base_version integer NOT NULL,
    spec jsonb NOT NULL,
    summary text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT proposal_revisions_base_version_check CHECK ((base_version >= 1)),
    CONSTRAINT proposal_revisions_revision_check CHECK ((revision >= 1)),
    CONSTRAINT proposal_revisions_spec_check CHECK ((jsonb_typeof(spec) = 'object'::text)),
    CONSTRAINT proposal_revisions_summary_check CHECK (((char_length(summary) >= 1) AND (char_length(summary) <= 500)))
);

CREATE TABLE proposals (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    dashboard_id uuid NOT NULL,
    state text DEFAULT 'open'::text NOT NULL,
    title text NOT NULL,
    revision integer DEFAULT 1 NOT NULL,
    author_kind text NOT NULL,
    author_id uuid NOT NULL,
    author_name text NOT NULL,
    origin text,
    origin_client text,
    run_id uuid,
    closed_at timestamp with time zone,
    closed_by_kind text,
    closed_by_id uuid,
    closed_by_name text,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    close_reason text,
    CONSTRAINT proposals_author_kind_check CHECK ((author_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT proposals_close_reason_check CHECK (((close_reason IS NULL) OR ((state = 'discarded'::text) AND ((char_length(close_reason) >= 1) AND (char_length(close_reason) <= 500))))),
    CONSTRAINT proposals_closed_by_kind_check CHECK ((closed_by_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT proposals_closed_check CHECK ((((state = 'open'::text) = (closed_at IS NULL)) AND ((state = 'open'::text) = (closed_by_kind IS NULL)) AND ((closed_by_kind IS NULL) = (closed_by_id IS NULL)) AND ((closed_by_kind IS NULL) = (closed_by_name IS NULL)))),
    CONSTRAINT proposals_origin_check CHECK ((origin = ANY (ARRAY['web'::text, 'cli'::text, 'mcp'::text, 'api'::text]))),
    CONSTRAINT proposals_origin_client_check CHECK (((origin_client IS NULL) OR ((origin = 'mcp'::text) AND ((length(origin_client) >= 1) AND (length(origin_client) <= 64))))),
    CONSTRAINT proposals_revision_check CHECK ((revision >= 1)),
    CONSTRAINT proposals_state_check CHECK ((state = ANY (ARRAY['open'::text, 'merged'::text, 'discarded'::text]))),
    CONSTRAINT proposals_title_check CHECK (((char_length(title) >= 1) AND (char_length(title) <= 200)))
);

CREATE TABLE provision_errors (
    path text NOT NULL,
    message text NOT NULL,
    seen_at timestamp with time zone NOT NULL
);

CREATE TABLE provisioned_dashboards (
    path text NOT NULL,
    dashboard_id uuid NOT NULL,
    content_sha256 text NOT NULL,
    folder_title text NOT NULL,
    last_error text DEFAULT ''::text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public_links (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    dashboard_id uuid,
    token_hash bytea NOT NULL,
    display_prefix text NOT NULL,
    variables text NOT NULL,
    selections jsonb DEFAULT '{}'::jsonb NOT NULL,
    expires_at timestamp with time zone,
    created_by_kind text NOT NULL,
    created_by_id uuid NOT NULL,
    created_by_name text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_viewed_at timestamp with time zone,
    rca_id uuid,
    label text DEFAULT ''::text NOT NULL,
    CONSTRAINT public_links_label_check CHECK ((char_length(label) <= 80)),
    CONSTRAINT public_links_subject_check CHECK ((num_nonnulls(dashboard_id, rca_id) = 1)),
    CONSTRAINT public_links_token_hash_check CHECK ((length(token_hash) = 32)),
    CONSTRAINT public_links_variables_check CHECK ((variables = ANY (ARRAY['frozen'::text, 'free'::text])))
);

CREATE TABLE run_sessions (
    agent_request_id uuid NOT NULL,
    attempt integer NOT NULL,
    state jsonb NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT run_sessions_attempt_check CHECK ((attempt >= 1))
);

CREATE TABLE run_tokens (
    token_hash bytea NOT NULL,
    agent_request_id uuid NOT NULL,
    attempt integer NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    claim_epoch integer NOT NULL,
    CONSTRAINT run_tokens_attempt_check CHECK ((attempt >= 1)),
    CONSTRAINT run_tokens_token_hash_check CHECK ((length(token_hash) = 32))
);

CREATE TABLE saved_prompts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    name text NOT NULL,
    body text NOT NULL,
    shared boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT saved_prompts_body_check CHECK (((char_length(body) >= 1) AND (char_length(body) <= 4000))),
    CONSTRAINT saved_prompts_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 64)))
);

CREATE TABLE saved_searches (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    kind text NOT NULL,
    title text NOT NULL,
    datasource_uid text NOT NULL,
    query text,
    filters jsonb DEFAULT '{}'::jsonb NOT NULL,
    time_from text NOT NULL,
    time_to text NOT NULL,
    folder_id uuid NOT NULL,
    created_by_kind text NOT NULL,
    created_by_id uuid NOT NULL,
    created_by_name text NOT NULL,
    origin text,
    origin_client text,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT saved_searches_created_by_kind_check CHECK ((created_by_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT saved_searches_datasource_uid_check CHECK ((datasource_uid ~ '^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$'::text)),
    CONSTRAINT saved_searches_filters_check CHECK ((jsonb_typeof(filters) = 'object'::text)),
    CONSTRAINT saved_searches_kind_check CHECK ((kind = ANY (ARRAY['logs'::text, 'traces'::text, 'metrics'::text]))),
    CONSTRAINT saved_searches_logs_query_check CHECK (((kind <> 'logs'::text) OR (query IS NOT NULL))),
    CONSTRAINT saved_searches_metrics_query_check CHECK (((kind <> 'metrics'::text) OR (query IS NOT NULL))),
    CONSTRAINT saved_searches_origin_check CHECK ((origin = ANY (ARRAY['web'::text, 'cli'::text, 'mcp'::text, 'api'::text]))),
    CONSTRAINT saved_searches_origin_client_check CHECK (((origin_client IS NULL) OR ((origin IS NOT NULL) AND (origin = 'mcp'::text) AND ((length(origin_client) >= 1) AND (length(origin_client) <= 64))))),
    CONSTRAINT saved_searches_query_check CHECK (((char_length(query) >= 1) AND (char_length(query) <= 8000))),
    CONSTRAINT saved_searches_time_from_check CHECK (((char_length(time_from) >= 1) AND (char_length(time_from) <= 64))),
    CONSTRAINT saved_searches_time_to_check CHECK (((char_length(time_to) >= 1) AND (char_length(time_to) <= 64))),
    CONSTRAINT saved_searches_title_check CHECK (((char_length(title) >= 1) AND (char_length(title) <= 120)))
);

CREATE TABLE server_instances (
    id text NOT NULL,
    version text NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    seen_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT server_instances_id_check CHECK (((length(id) >= 1) AND (length(id) <= 200)))
);

CREATE TABLE service_account_keys (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    service_account_id uuid NOT NULL,
    name text NOT NULL,
    display_prefix text NOT NULL,
    token_hash bytea NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone,
    last_used_at timestamp with time zone,
    revoked_at timestamp with time zone,
    CONSTRAINT service_account_keys_name_check CHECK (((length(name) >= 1) AND (length(name) <= 64))),
    CONSTRAINT service_account_keys_token_hash_check CHECK ((length(token_hash) = 32))
);

CREATE TABLE service_accounts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    role text DEFAULT 'editor'::text NOT NULL,
    provisioned boolean DEFAULT false NOT NULL,
    takes_runs boolean DEFAULT false NOT NULL,
    CONSTRAINT service_accounts_description_check CHECK ((length(description) <= 256)),
    CONSTRAINT service_accounts_name_check CHECK ((name ~ '^[a-z0-9][a-z0-9-]{0,62}$'::text)),
    CONSTRAINT service_accounts_role_check CHECK ((role = ANY (ARRAY['admin'::text, 'editor'::text, 'commenter'::text, 'viewer'::text])))
);

CREATE TABLE sessions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    token_hash bytea NOT NULL,
    user_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT sessions_token_hash_check CHECK ((length(token_hash) = 32))
);

CREATE TABLE snapshots (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    observation_id uuid NOT NULL,
    panel jsonb NOT NULL,
    query jsonb NOT NULL,
    frames jsonb NOT NULL,
    user_scoped boolean DEFAULT false NOT NULL,
    time_from timestamp with time zone NOT NULL,
    time_to timestamp with time zone NOT NULL,
    created_by_kind text NOT NULL,
    created_by_id uuid NOT NULL,
    created_by_name text NOT NULL,
    origin text,
    origin_client text,
    run_id uuid,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    datasource_uids text[] DEFAULT '{}'::text[] NOT NULL,
    CONSTRAINT snapshots_created_by_kind_check CHECK ((created_by_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT snapshots_frames_check CHECK ((jsonb_typeof(frames) = 'object'::text)),
    CONSTRAINT snapshots_origin_check CHECK ((origin = ANY (ARRAY['web'::text, 'cli'::text, 'mcp'::text, 'api'::text]))),
    CONSTRAINT snapshots_origin_client_check CHECK (((origin_client IS NULL) OR ((origin IS NOT NULL) AND (origin = 'mcp'::text) AND ((length(origin_client) >= 1) AND (length(origin_client) <= 64))))),
    CONSTRAINT snapshots_panel_check CHECK ((jsonb_typeof(panel) = 'object'::text)),
    CONSTRAINT snapshots_query_check CHECK ((jsonb_typeof(query) = 'array'::text)),
    CONSTRAINT snapshots_size_check CHECK ((pg_column_size(frames) <= 2097152)),
    CONSTRAINT snapshots_time_check CHECK ((time_from < time_to))
);

CREATE TABLE starter_content (
    key text NOT NULL,
    ref uuid NOT NULL,
    seeded_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE team_members (
    team_id uuid NOT NULL,
    user_id uuid NOT NULL,
    added_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE teams (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    created_by_name text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT teams_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 100)))
);

CREATE TABLE thread_mentions (
    message_id uuid NOT NULL,
    user_id uuid,
    agent text,
    name text NOT NULL,
    agent_grant_id uuid,
    agent_service_account_id uuid,
    CONSTRAINT thread_mentions_agent_check CHECK ((agent = 'observe'::text)),
    CONSTRAINT thread_mentions_name_check CHECK ((length(name) <= 256)),
    CONSTRAINT thread_mentions_one_check CHECK ((num_nonnulls(user_id, agent, agent_grant_id, agent_service_account_id) = 1))
);

CREATE TABLE thread_messages (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    thread_id uuid NOT NULL,
    body text NOT NULL,
    author_kind text NOT NULL,
    author_id uuid NOT NULL,
    author_name text NOT NULL,
    author_email text,
    origin text,
    origin_client text,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    edited_at timestamp with time zone,
    run_id uuid,
    run_assigned_kind text,
    reply_to_id uuid,
    observation_block_id uuid,
    client_id uuid,
    run_failed boolean DEFAULT false NOT NULL,
    run_refusal_code text,
    run_refusal_message text,
    CONSTRAINT thread_messages_author_kind_check CHECK ((author_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT thread_messages_body_check CHECK (((length(body) >= 1) AND (length(body) <= 4000))),
    CONSTRAINT thread_messages_origin_check CHECK ((origin = ANY (ARRAY['web'::text, 'cli'::text, 'mcp'::text, 'api'::text]))),
    CONSTRAINT thread_messages_origin_client_check CHECK (((origin_client IS NULL) OR ((origin IS NOT NULL) AND (origin = 'mcp'::text) AND ((length(origin_client) >= 1) AND (length(origin_client) <= 64))))),
    CONSTRAINT thread_messages_run_assigned_kind_check CHECK ((run_assigned_kind = ANY (ARRAY['builtin'::text, 'grant'::text, 'service_account'::text])))
);

CREATE TABLE thread_reads (
    user_id uuid NOT NULL,
    thread_id uuid NOT NULL,
    read_at timestamp with time zone NOT NULL
);

CREATE TABLE threads (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    dashboard_id uuid,
    dashboard_version integer,
    panel_id text,
    finding_id uuid,
    annotation_id uuid,
    status text DEFAULT 'open'::text NOT NULL,
    resolved_at timestamp with time zone,
    resolved_by_kind text,
    resolved_by_id uuid,
    resolved_by_name text,
    resolved_origin text,
    resolved_origin_client text,
    resolved_run_id uuid,
    resolved_by_version integer,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    observation_id uuid,
    proposal_id uuid,
    agent_kind text,
    agent_id uuid,
    agent_off boolean DEFAULT false NOT NULL,
    anchor jsonb,
    block_id uuid,
    CONSTRAINT threads_agent_check CHECK ((((agent_kind IS NULL) AND (agent_id IS NULL)) OR ((agent_kind = 'builtin'::text) AND (agent_id IS NULL)) OR ((agent_kind = ANY (ARRAY['grant'::text, 'service_account'::text])) AND (agent_id IS NOT NULL)))),
    CONSTRAINT threads_agent_kind_check CHECK ((agent_kind = ANY (ARRAY['builtin'::text, 'grant'::text, 'service_account'::text]))),
    CONSTRAINT threads_agent_off_check CHECK (((NOT agent_off) OR (agent_kind IS NOT NULL))),
    CONSTRAINT threads_anchor_check CHECK ((((dashboard_id IS NULL) = (dashboard_version IS NULL)) AND ((panel_id IS NULL) OR (dashboard_id IS NOT NULL)))),
    CONSTRAINT threads_block_point_check CHECK (((block_id IS NULL) OR (anchor IS NOT NULL))),
    CONSTRAINT threads_panel_id_check CHECK (((length(panel_id) >= 1) AND (length(panel_id) <= 64))),
    CONSTRAINT threads_point_check CHECK (((anchor IS NULL) OR ((jsonb_typeof(anchor) = 'object'::text) AND (pg_column_size(anchor) <= 2048)))),
    CONSTRAINT threads_resolved_by_kind_check CHECK ((resolved_by_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT threads_resolved_by_version_check CHECK (((resolved_by_version IS NULL) OR ((status = 'resolved'::text) AND (dashboard_id IS NOT NULL) AND (resolved_by_version >= dashboard_version)))),
    CONSTRAINT threads_resolved_check CHECK ((((status = 'resolved'::text) = (resolved_at IS NOT NULL)) AND ((status = 'resolved'::text) OR ((resolved_by_kind IS NULL) AND (resolved_by_id IS NULL) AND (resolved_by_name IS NULL) AND (resolved_origin IS NULL) AND (resolved_run_id IS NULL))))),
    CONSTRAINT threads_resolved_origin_check CHECK ((resolved_origin = ANY (ARRAY['web'::text, 'cli'::text, 'mcp'::text, 'api'::text]))),
    CONSTRAINT threads_resolved_origin_client_check CHECK (((resolved_origin_client IS NULL) OR ((resolved_origin IS NOT NULL) AND (resolved_origin = 'mcp'::text) AND ((length(resolved_origin_client) >= 1) AND (length(resolved_origin_client) <= 64))))),
    CONSTRAINT threads_status_check CHECK ((status = ANY (ARRAY['open'::text, 'resolved'::text]))),
    CONSTRAINT threads_target_check CHECK ((num_nonnulls(dashboard_id, finding_id, annotation_id, observation_id, proposal_id) = 1))
);

CREATE TABLE user_invites (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    email text NOT NULL,
    role text NOT NULL,
    created_by_kind text NOT NULL,
    created_by_id uuid NOT NULL,
    created_by_name text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_invites_created_by_kind_check CHECK ((created_by_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT user_invites_email_check CHECK (((octet_length(email) >= 3) AND (octet_length(email) <= 254))),
    CONSTRAINT user_invites_role_check CHECK ((role = ANY (ARRAY['admin'::text, 'editor'::text, 'commenter'::text, 'viewer'::text])))
);

CREATE TABLE users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    oidc_issuer text NOT NULL,
    oidc_subject text NOT NULL,
    email text NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_login_at timestamp with time zone DEFAULT now() NOT NULL,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    role text DEFAULT 'editor'::text NOT NULL,
    role_source text DEFAULT 'default'::text NOT NULL,
    role_group text,
    manual_role text,
    manual_role_by_id uuid,
    manual_role_by_name text,
    groups text[] DEFAULT '{}'::text[] NOT NULL,
    threads_seen_since timestamp with time zone DEFAULT now() NOT NULL,
    deactivated_at timestamp with time zone,
    deactivated_by_id uuid,
    deactivated_by_name text,
    first_user boolean DEFAULT false NOT NULL,
    CONSTRAINT users_manual_role_check CHECK ((manual_role = ANY (ARRAY['admin'::text, 'editor'::text, 'commenter'::text, 'viewer'::text]))),
    CONSTRAINT users_role_check CHECK ((role = ANY (ARRAY['admin'::text, 'editor'::text, 'commenter'::text, 'viewer'::text]))),
    CONSTRAINT users_role_source_check CHECK ((role_source = ANY (ARRAY['admin_emails'::text, 'first_user'::text, 'group'::text, 'manual'::text, 'default'::text])))
);

CREATE TABLE webhook_deliveries (
    id bigint NOT NULL,
    webhook_id uuid NOT NULL,
    delivery_id text NOT NULL,
    event_type text NOT NULL,
    activity_seq bigint,
    attempted_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    duration_ms integer NOT NULL,
    status_code integer,
    outcome text NOT NULL,
    error_code text,
    CONSTRAINT webhook_deliveries_duration_check CHECK ((duration_ms >= 0)),
    CONSTRAINT webhook_deliveries_error_check CHECK ((error_code = ANY (ARRAY['status'::text, 'unreachable'::text, 'blocked_address'::text, 'domain_not_allowed'::text, 'too_large'::text, 'skipped_by_admin'::text, 'expired'::text]))),
    CONSTRAINT webhook_deliveries_error_presence_check CHECK (((outcome = 'delivered'::text) = (error_code IS NULL))),
    CONSTRAINT webhook_deliveries_outcome_check CHECK ((outcome = ANY (ARRAY['delivered'::text, 'failed'::text, 'skipped'::text]))),
    CONSTRAINT webhook_deliveries_status_check CHECK (((status_code >= 100) AND (status_code <= 599))),
    CONSTRAINT webhook_deliveries_test_check CHECK (((event_type = 'webhook.test'::text) = (activity_seq IS NULL)))
);

ALTER TABLE webhook_deliveries ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME webhook_deliveries_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

CREATE TABLE webhooks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    url text NOT NULL,
    secret_ciphertext bytea NOT NULL,
    event_types text[] NOT NULL,
    owner_kind text NOT NULL,
    owner_id uuid NOT NULL,
    cursor_xid xid8 NOT NULL,
    cursor_seq bigint NOT NULL,
    consecutive_failures integer DEFAULT 0 NOT NULL,
    next_attempt_at timestamp with time zone DEFAULT now() NOT NULL,
    lease_id uuid,
    leased_by text,
    lease_until timestamp with time zone,
    disabled_at timestamp with time zone,
    disabled_reason text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    format text DEFAULT 'generic'::text NOT NULL,
    CONSTRAINT webhooks_disabled_check CHECK (((disabled_at IS NULL) = (disabled_reason IS NULL))),
    CONSTRAINT webhooks_disabled_reason_check CHECK ((disabled_reason = ANY (ARRAY['manual'::text, 'gone'::text, 'failing'::text, 'owner'::text, 'secret_unreadable'::text]))),
    CONSTRAINT webhooks_event_types_check CHECK ((cardinality(event_types) >= 1)),
    CONSTRAINT webhooks_failures_check CHECK ((consecutive_failures >= 0)),
    CONSTRAINT webhooks_format_check CHECK ((format = ANY (ARRAY['generic'::text, 'slack'::text, 'teams'::text]))),
    CONSTRAINT webhooks_lease_check CHECK ((((lease_id IS NULL) = (lease_until IS NULL)) AND ((lease_id IS NULL) = (leased_by IS NULL)))),
    CONSTRAINT webhooks_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 100))),
    CONSTRAINT webhooks_owner_kind_check CHECK ((owner_kind = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT webhooks_url_check CHECK (((octet_length(url) <= 2048) AND (url ~~ 'https://%'::text)))
);

ALTER TABLE ONLY audit_events ALTER COLUMN seq SET DEFAULT nextval('audit_events_seq'::regclass);

ALTER TABLE ONLY access_requests
    ADD CONSTRAINT access_requests_pkey PRIMARY KEY (id);

ALTER TABLE ONLY activity
    ADD CONSTRAINT activity_pkey PRIMARY KEY (seq);

ALTER TABLE ONLY agent_request_builds
    ADD CONSTRAINT agent_request_builds_pkey PRIMARY KEY (agent_request_id);

ALTER TABLE ONLY agent_requests
    ADD CONSTRAINT agent_requests_pkey PRIMARY KEY (id);

ALTER TABLE ONLY agent_workers
    ADD CONSTRAINT agent_workers_pkey PRIMARY KEY (id);

ALTER TABLE ONLY annotations
    ADD CONSTRAINT annotations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY applied_dashboards
    ADD CONSTRAINT applied_dashboards_dashboard_id_key UNIQUE (dashboard_id);

ALTER TABLE ONLY applied_dashboards
    ADD CONSTRAINT applied_dashboards_pkey PRIMARY KEY (path);

ALTER TABLE ONLY audit_events
    ADD CONSTRAINT audit_events_pkey PRIMARY KEY (occurred_at, id);

ALTER TABLE ONLY audit_export_cursor
    ADD CONSTRAINT audit_export_cursor_pkey PRIMARY KEY (only_row);

ALTER TABLE ONLY chats
    ADD CONSTRAINT chats_pkey PRIMARY KEY (id);

ALTER TABLE ONLY dashboard_health
    ADD CONSTRAINT dashboard_health_pkey PRIMARY KEY (dashboard_id);

ALTER TABLE ONLY dashboard_stars
    ADD CONSTRAINT dashboard_stars_pkey PRIMARY KEY (user_id, dashboard_id);

ALTER TABLE ONLY dashboard_versions
    ADD CONSTRAINT dashboard_versions_agent_request_id_key UNIQUE (agent_request_id);

ALTER TABLE ONLY dashboard_versions
    ADD CONSTRAINT dashboard_versions_pkey PRIMARY KEY (dashboard_id, version);

ALTER TABLE ONLY dashboard_view_days
    ADD CONSTRAINT dashboard_view_days_pkey PRIMARY KEY (dashboard_id, user_id, day);

ALTER TABLE ONLY dashboard_viewers
    ADD CONSTRAINT dashboard_viewers_pkey PRIMARY KEY (dashboard_id, user_id);

ALTER TABLE ONLY dashboard_views
    ADD CONSTRAINT dashboard_views_pkey PRIMARY KEY (user_id, dashboard_id);

ALTER TABLE ONLY dashboards
    ADD CONSTRAINT dashboards_pkey PRIMARY KEY (id);

ALTER TABLE ONLY datasources
    ADD CONSTRAINT datasources_pkey PRIMARY KEY (id);

ALTER TABLE ONLY datasources
    ADD CONSTRAINT datasources_uid_key UNIQUE (uid);

ALTER TABLE ONLY draft_query_spend
    ADD CONSTRAINT draft_query_spend_pkey PRIMARY KEY (principal_key, window_start);

ALTER TABLE ONLY findings
    ADD CONSTRAINT findings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY folders
    ADD CONSTRAINT folders_pkey PRIMARY KEY (id);

ALTER TABLE ONLY grafana_connections
    ADD CONSTRAINT grafana_connections_pkey PRIMARY KEY (id);

ALTER TABLE ONLY grafana_sync_items
    ADD CONSTRAINT grafana_sync_items_pkey PRIMARY KEY (id);

ALTER TABLE ONLY grafana_sync_items
    ADD CONSTRAINT grafana_sync_items_uid_key UNIQUE (sync_id, uid);

ALTER TABLE ONLY grafana_syncs
    ADD CONSTRAINT grafana_syncs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY home_dashboards
    ADD CONSTRAINT home_dashboards_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY imports
    ADD CONSTRAINT imports_dashboard_id_key UNIQUE (dashboard_id);

ALTER TABLE ONLY imports
    ADD CONSTRAINT imports_pkey PRIMARY KEY (id);

ALTER TABLE ONLY inbox_cursors
    ADD CONSTRAINT inbox_cursors_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY inbox_item_states
    ADD CONSTRAINT inbox_item_states_pkey PRIMARY KEY (user_id, seq);

ALTER TABLE ONLY instance
    ADD CONSTRAINT instance_pkey PRIMARY KEY (singleton);

ALTER TABLE ONLY jobs
    ADD CONSTRAINT jobs_pkey PRIMARY KEY (name);

ALTER TABLE ONLY mcp_source_tools
    ADD CONSTRAINT mcp_source_tools_pkey PRIMARY KEY (source_id, name);

ALTER TABLE ONLY mcp_sources
    ADD CONSTRAINT mcp_sources_pkey PRIMARY KEY (id);

ALTER TABLE ONLY mcp_sources
    ADD CONSTRAINT mcp_sources_uid_key UNIQUE (uid);

ALTER TABLE ONLY oauth_clients
    ADD CONSTRAINT oauth_clients_pkey PRIMARY KEY (id);

ALTER TABLE ONLY oauth_codes
    ADD CONSTRAINT oauth_codes_pkey PRIMARY KEY (code_hash);

ALTER TABLE ONLY oauth_grants
    ADD CONSTRAINT oauth_grants_pkey PRIMARY KEY (id);

ALTER TABLE ONLY oauth_requests
    ADD CONSTRAINT oauth_requests_pkey PRIMARY KEY (id);

ALTER TABLE ONLY oauth_tokens
    ADD CONSTRAINT oauth_tokens_pkey PRIMARY KEY (token_hash);

ALTER TABLE ONLY observation_blocks
    ADD CONSTRAINT observation_blocks_pkey PRIMARY KEY (id);

ALTER TABLE ONLY observation_blocks
    ADD CONSTRAINT observation_blocks_position_key UNIQUE (observation_id, "position") DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ONLY observation_pins
    ADD CONSTRAINT observation_pins_pkey PRIMARY KEY (observation_id, dashboard_id);

ALTER TABLE ONLY observation_rcas
    ADD CONSTRAINT observation_rcas_pkey PRIMARY KEY (id);

ALTER TABLE ONLY observation_rcas
    ADD CONSTRAINT observation_rcas_revision_key UNIQUE (observation_id, revision);

ALTER TABLE ONLY observations
    ADD CONSTRAINT observations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY oidc_login_states
    ADD CONSTRAINT oidc_login_states_pkey PRIMARY KEY (state_hash);

ALTER TABLE ONLY outbound_domains
    ADD CONSTRAINT outbound_domains_pkey PRIMARY KEY (domain);

ALTER TABLE ONLY proposal_revisions
    ADD CONSTRAINT proposal_revisions_pkey PRIMARY KEY (proposal_id, revision);

ALTER TABLE ONLY proposals
    ADD CONSTRAINT proposals_pkey PRIMARY KEY (id);

ALTER TABLE ONLY provision_errors
    ADD CONSTRAINT provision_errors_pkey PRIMARY KEY (path);

ALTER TABLE ONLY provisioned_dashboards
    ADD CONSTRAINT provisioned_dashboards_dashboard_id_key UNIQUE (dashboard_id);

ALTER TABLE ONLY provisioned_dashboards
    ADD CONSTRAINT provisioned_dashboards_pkey PRIMARY KEY (path);

ALTER TABLE ONLY public_links
    ADD CONSTRAINT public_links_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public_links
    ADD CONSTRAINT public_links_token_hash_key UNIQUE (token_hash);

ALTER TABLE ONLY run_sessions
    ADD CONSTRAINT run_sessions_pkey PRIMARY KEY (agent_request_id, attempt);

ALTER TABLE ONLY run_tokens
    ADD CONSTRAINT run_tokens_pkey PRIMARY KEY (token_hash);

ALTER TABLE ONLY saved_prompts
    ADD CONSTRAINT saved_prompts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY saved_searches
    ADD CONSTRAINT saved_searches_pkey PRIMARY KEY (id);

ALTER TABLE ONLY server_instances
    ADD CONSTRAINT server_instances_pkey PRIMARY KEY (id);

ALTER TABLE ONLY service_account_keys
    ADD CONSTRAINT service_account_keys_pkey PRIMARY KEY (id);

ALTER TABLE ONLY service_account_keys
    ADD CONSTRAINT service_account_keys_token_hash_key UNIQUE (token_hash);

ALTER TABLE ONLY service_accounts
    ADD CONSTRAINT service_accounts_name_key UNIQUE (name);

ALTER TABLE ONLY service_accounts
    ADD CONSTRAINT service_accounts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY sessions
    ADD CONSTRAINT sessions_token_hash_key UNIQUE (token_hash);

ALTER TABLE ONLY snapshots
    ADD CONSTRAINT snapshots_pkey PRIMARY KEY (id);

ALTER TABLE ONLY starter_content
    ADD CONSTRAINT starter_content_pkey PRIMARY KEY (key);

ALTER TABLE ONLY team_members
    ADD CONSTRAINT team_members_pkey PRIMARY KEY (team_id, user_id);

ALTER TABLE ONLY teams
    ADD CONSTRAINT teams_pkey PRIMARY KEY (id);

ALTER TABLE ONLY thread_messages
    ADD CONSTRAINT thread_messages_pkey PRIMARY KEY (id);

ALTER TABLE ONLY thread_reads
    ADD CONSTRAINT thread_reads_pkey PRIMARY KEY (user_id, thread_id);

ALTER TABLE ONLY threads
    ADD CONSTRAINT threads_pkey PRIMARY KEY (id);

ALTER TABLE ONLY user_invites
    ADD CONSTRAINT user_invites_pkey PRIMARY KEY (id);

ALTER TABLE ONLY users
    ADD CONSTRAINT users_oidc_issuer_oidc_subject_key UNIQUE (oidc_issuer, oidc_subject);

ALTER TABLE ONLY users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

ALTER TABLE ONLY webhook_deliveries
    ADD CONSTRAINT webhook_deliveries_pkey PRIMARY KEY (id);

ALTER TABLE ONLY webhooks
    ADD CONSTRAINT webhooks_pkey PRIMARY KEY (id);

CREATE INDEX access_requests_activity_idx ON access_requests USING btree (activity_seq);

CREATE INDEX access_requests_object_idx ON access_requests USING btree (COALESCE(dashboard_id, observation_id), requester_kind, requester_id);

CREATE INDEX access_requests_requester_idx ON access_requests USING btree (requester_kind, requester_id) WHERE (granted_at IS NULL);

CREATE INDEX activity_created_idx ON activity USING btree (created_at);

CREATE INDEX activity_dashboard_idx ON activity USING btree (dashboard_id, xid, seq) WHERE (dashboard_id IS NOT NULL);

CREATE INDEX activity_finding_idx ON activity USING btree (finding_id) WHERE (finding_id IS NOT NULL);

CREATE INDEX activity_folder_idx ON activity USING btree (folder_id, xid, seq);

CREATE INDEX activity_inbox_idx ON activity USING btree (xid, seq) WHERE (type = ANY (ARRAY['proposal.opened'::text, 'observation.block_added'::text, 'thread.mentioned'::text, 'thread.replied'::text, 'run.failed'::text, 'run.succeeded'::text, 'proposal.merged'::text, 'proposal.discarded'::text, 'access_request.opened'::text]));

CREATE INDEX activity_observation_idx ON activity USING btree (observation_id) WHERE (observation_id IS NOT NULL);

CREATE INDEX activity_position_idx ON activity USING btree (xid, seq);

CREATE INDEX activity_saved_search_idx ON activity USING btree (saved_search_id) WHERE (saved_search_id IS NOT NULL);

CREATE UNIQUE INDEX agent_requests_applies_finding_idx ON agent_requests USING btree (applies_finding_id) WHERE ((applies_finding_id IS NOT NULL) AND (status = ANY (ARRAY['queued'::text, 'running'::text, 'succeeded'::text])));

CREATE INDEX agent_requests_assigned_idx ON agent_requests USING btree (assigned_kind, assigned_id, created_at) WHERE ((status = ANY (ARRAY['queued'::text, 'running'::text])) AND (assigned_kind <> 'builtin'::text));

CREATE INDEX agent_requests_assignee_recent_idx ON agent_requests USING btree (assigned_kind, assigned_id, created_at DESC, id DESC);

CREATE INDEX agent_requests_awaiting_reply_idx ON agent_requests USING btree (finished_at DESC, id) WHERE ((thread_message_id IS NOT NULL) AND (thread_replied_at IS NULL));

CREATE INDEX agent_requests_chat_idx ON agent_requests USING btree (chat_id) WHERE (chat_id IS NOT NULL);

CREATE INDEX agent_requests_chat_order_idx ON agent_requests USING btree (chat_id, created_at, id) WHERE ((chat_id IS NOT NULL) AND (status = ANY (ARRAY['queued'::text, 'running'::text])));

CREATE INDEX agent_requests_claim_idx ON agent_requests USING btree (created_at) WHERE (status = ANY (ARRAY['queued'::text, 'running'::text]));

CREATE INDEX agent_requests_claimed_by_idx ON agent_requests USING btree (claimed_by) WHERE (status = 'running'::text);

CREATE UNIQUE INDEX agent_requests_client_idx ON agent_requests USING btree (requested_by_kind, requested_by_id, client_id) WHERE (client_id IS NOT NULL);

CREATE INDEX agent_requests_comment_idx ON agent_requests USING btree (comment_id, created_at DESC) WHERE (comment_id IS NOT NULL);

CREATE UNIQUE INDEX agent_requests_continues_key ON agent_requests USING btree (continues_id) WHERE (continues_id IS NOT NULL);

CREATE INDEX agent_requests_create_active_idx ON agent_requests USING btree (requested_by_id) WHERE ((kind = 'build'::text) AND (intent = 'ask'::text) AND (status = ANY (ARRAY['queued'::text, 'running'::text])));

CREATE INDEX agent_requests_dashboard_idx ON agent_requests USING btree (dashboard_id, created_at DESC, id DESC);

CREATE INDEX agent_requests_investigation_active_idx ON agent_requests USING btree (requested_by_id) WHERE (investigation AND (status = ANY (ARRAY['queued'::text, 'running'::text])));

CREATE INDEX agent_requests_listed_intent_idx ON agent_requests USING btree (created_at DESC, id DESC) WHERE ((intent <> 'suggest'::text) AND (archived_at IS NULL));

CREATE INDEX agent_requests_observation_idx ON agent_requests USING btree (observation_id) WHERE (observation_id IS NOT NULL);

CREATE INDEX agent_requests_requester_idx ON agent_requests USING btree (requested_by_id, created_at DESC, id DESC);

CREATE INDEX agent_requests_retries_key ON agent_requests USING btree (retries_id) WHERE (retries_id IS NOT NULL);

CREATE UNIQUE INDEX agent_requests_running_dashboard_idx ON agent_requests USING btree (dashboard_id) WHERE ((status = 'running'::text) AND (intent = 'apply'::text));

CREATE INDEX agent_requests_subject_observation_idx ON agent_requests USING btree (subject_observation_id) WHERE (subject_observation_id IS NOT NULL);

CREATE UNIQUE INDEX agent_requests_successor_key ON agent_requests USING btree (COALESCE(retries_id, continues_id)) WHERE ((retries_id IS NOT NULL) OR (continues_id IS NOT NULL));

CREATE INDEX agent_requests_suggest_intent_active_idx ON agent_requests USING btree (requested_by_id) WHERE ((intent = 'suggest'::text) AND (status = ANY (ARRAY['queued'::text, 'running'::text])));

CREATE INDEX agent_requests_thread_message_idx ON agent_requests USING btree (thread_message_id) WHERE (thread_message_id IS NOT NULL);

CREATE INDEX annotations_dashboard_time_idx ON annotations USING btree (dashboard_id, time_from);

CREATE INDEX annotations_search_idx ON annotations USING gin (annotation_search_vector(text, tags));

CREATE INDEX annotations_tags_idx ON annotations USING gin (tags);

CREATE INDEX audit_events_action_idx ON ONLY audit_events USING btree (action, occurred_at DESC);

CREATE UNIQUE INDEX audit_events_activity_key ON ONLY audit_events USING btree (activity_seq, occurred_at);

CREATE INDEX audit_events_actor_idx ON ONLY audit_events USING btree (actor_id, occurred_at DESC);

CREATE INDEX audit_events_position_idx ON ONLY audit_events USING btree (xid, seq);

CREATE INDEX audit_events_request_idx ON ONLY audit_events USING btree (request_id);

CREATE INDEX audit_events_run_idx ON ONLY audit_events USING btree (run_id) WHERE (run_id IS NOT NULL);

CREATE INDEX audit_events_search_idx ON ONLY audit_events USING gin (search);

CREATE INDEX audit_events_target_idx ON ONLY audit_events USING btree (target_kind, target_id, occurred_at DESC);

CREATE INDEX chats_owner_dashboard_idx ON chats USING btree (owner_id, dashboard_id) WHERE (dashboard_id IS NOT NULL);

CREATE INDEX chats_owner_observation_idx ON chats USING btree (owner_id, observation_id) WHERE (observation_id IS NOT NULL);

CREATE INDEX chats_owner_recent_idx ON chats USING btree (owner_id, last_turn_at DESC, id DESC) WHERE (archived_at IS NULL);

CREATE INDEX dashboard_stars_dashboard_idx ON dashboard_stars USING btree (dashboard_id);

CREATE UNIQUE INDEX dashboard_versions_merged_proposal_key ON dashboard_versions USING btree (merged_proposal_id) WHERE (merged_proposal_id IS NOT NULL);

CREATE INDEX dashboard_versions_search_idx ON dashboard_versions USING gin (dashboard_search_vector(spec));

CREATE INDEX dashboard_view_days_day_idx ON dashboard_view_days USING btree (day);

CREATE INDEX dashboard_viewers_seen_idx ON dashboard_viewers USING btree (seen_at);

CREATE INDEX dashboard_views_dashboard_idx ON dashboard_views USING btree (dashboard_id, viewed_at DESC);

CREATE INDEX dashboards_created_by_idx ON dashboards USING btree (created_by_id);

CREATE INDEX dashboards_folder_idx ON dashboards USING btree (folder_id);

CREATE INDEX dashboards_tags_idx ON dashboards USING gin (tags);

CREATE INDEX dashboards_updated_at_id_idx ON dashboards USING btree (updated_at DESC, id DESC);

CREATE UNIQUE INDEX findings_agent_request_key ON findings USING btree (agent_request_id) WHERE (agent_request_id IS NOT NULL);

CREATE INDEX findings_creator_idx ON findings USING btree (created_by_id, created_at DESC, id DESC);

CREATE INDEX findings_dashboard_idx ON findings USING btree (dashboard_id, created_at DESC, id DESC) WHERE (dashboard_id IS NOT NULL);

CREATE INDEX findings_folder_idx ON findings USING btree (folder_id, created_at DESC, id DESC) WHERE (folder_id IS NOT NULL);

CREATE INDEX findings_observation_idx ON findings USING btree (observation_id);

CREATE INDEX findings_root_cause_component_search_idx ON findings USING gin (search_vector(root_cause_component, ''::text, ''::text, ''::text)) WHERE (root_cause_component IS NOT NULL);

CREATE UNIQUE INDEX folder_permissions_group_idx ON folder_permissions USING btree (folder_id, group_name) WHERE (group_name IS NOT NULL);

CREATE UNIQUE INDEX folder_permissions_user_idx ON folder_permissions USING btree (folder_id, user_id) WHERE (user_id IS NOT NULL);

CREATE UNIQUE INDEX folders_general_idx ON folders USING btree (general) WHERE general;

CREATE UNIQUE INDEX folders_owner_idx ON folders USING btree (owner_id) WHERE (owner_id IS NOT NULL);

CREATE UNIQUE INDEX folders_title_idx ON folders USING btree (lower(title)) WHERE (NOT personal);

CREATE INDEX grafana_connections_created_at_id_idx ON grafana_connections USING btree (created_at, id);

CREATE INDEX grafana_sync_items_sync_idx ON grafana_sync_items USING btree (sync_id, id);

CREATE INDEX grafana_syncs_connection_idx ON grafana_syncs USING btree (connection_id, started_at DESC, id DESC);

CREATE UNIQUE INDEX grafana_syncs_running_idx ON grafana_syncs USING btree (connection_id) WHERE (state = 'running'::text);

CREATE INDEX imports_created_at_id_idx ON imports USING btree (created_at DESC, id DESC);

CREATE UNIQUE INDEX imports_grafana_uid_key ON imports USING btree (grafana_uid) WHERE (grafana_uid <> ''::text);

CREATE INDEX oauth_clients_registered_idx ON oauth_clients USING btree (registered_at);

CREATE INDEX oauth_codes_expires_idx ON oauth_codes USING btree (expires_at);

CREATE INDEX oauth_grants_created_idx ON oauth_grants USING btree (created_at DESC, id DESC);

CREATE UNIQUE INDEX oauth_grants_live_idx ON oauth_grants USING btree (client_id, user_id) WHERE (revoked_at IS NULL);

CREATE INDEX oauth_grants_user_idx ON oauth_grants USING btree (user_id, created_at DESC, id DESC);

CREATE INDEX oauth_requests_expires_idx ON oauth_requests USING btree (expires_at);

CREATE INDEX oauth_tokens_expires_idx ON oauth_tokens USING btree (expires_at);

CREATE INDEX oauth_tokens_grant_idx ON oauth_tokens USING btree (grant_id);

CREATE INDEX observation_blocks_run_idx ON observation_blocks USING btree (run_id) WHERE (run_id IS NOT NULL);

CREATE INDEX observation_blocks_search_idx ON observation_blocks USING gin (observation_block_search_vector(body)) WHERE ((kind = 'text'::text) AND (state = 'live'::text));

CREATE INDEX observation_blocks_snapshot_idx ON observation_blocks USING btree (snapshot_id) WHERE (snapshot_id IS NOT NULL);

CREATE UNIQUE INDEX observation_dashboards_block_key ON observation_dashboards USING btree (block_id, created_by_kind, created_by_id) WHERE (block_id IS NOT NULL);

CREATE INDEX observation_dashboards_dashboard_idx ON observation_dashboards USING btree (dashboard_id);

CREATE UNIQUE INDEX observation_dashboards_observation_key ON observation_dashboards USING btree (observation_id, created_by_kind, created_by_id) WHERE (block_id IS NULL);

CREATE INDEX observation_pins_annotation_idx ON observation_pins USING btree (annotation_id);

CREATE INDEX observations_folder_idx ON observations USING btree (folder_id, updated_at DESC, id DESC);

CREATE INDEX observations_run_idx ON observations USING btree (run_id) WHERE (run_id IS NOT NULL);

CREATE INDEX observations_search_idx ON observations USING gin (observation_search_vector(title, summary));

CREATE INDEX observations_updated_idx ON observations USING btree (updated_at DESC, id DESC);

CREATE INDEX oidc_login_states_expires_at_idx ON oidc_login_states USING btree (expires_at);

CREATE INDEX proposals_dashboard_idx ON proposals USING btree (dashboard_id, created_at);

CREATE UNIQUE INDEX proposals_open_author_key ON proposals USING btree (dashboard_id, author_kind, author_id) WHERE ((state = 'open'::text) AND (run_id IS NULL));

CREATE UNIQUE INDEX proposals_open_run_key ON proposals USING btree (dashboard_id, run_id) WHERE ((state = 'open'::text) AND (run_id IS NOT NULL));

CREATE INDEX proposals_run_idx ON proposals USING btree (run_id) WHERE (run_id IS NOT NULL);

CREATE INDEX public_links_dashboard_id_idx ON public_links USING btree (dashboard_id);

CREATE INDEX public_links_rca_idx ON public_links USING btree (rca_id) WHERE (rca_id IS NOT NULL);

CREATE INDEX run_tokens_expires_idx ON run_tokens USING btree (expires_at);

CREATE INDEX run_tokens_request_idx ON run_tokens USING btree (agent_request_id);

CREATE UNIQUE INDEX saved_prompts_owner_name_key ON saved_prompts USING btree (owner_id, lower(name));

CREATE INDEX saved_prompts_shared_idx ON saved_prompts USING btree (shared) WHERE shared;

CREATE UNIQUE INDEX saved_prompts_shared_name_key ON saved_prompts USING btree (lower(name)) WHERE shared;

CREATE INDEX saved_searches_folder_idx ON saved_searches USING btree (folder_id, updated_at DESC, id DESC);

CREATE INDEX saved_searches_search_idx ON saved_searches USING gin (saved_search_search_vector(title, query, filters));

CREATE INDEX saved_searches_updated_idx ON saved_searches USING btree (updated_at DESC, id DESC);

CREATE INDEX service_account_keys_service_account_id_idx ON service_account_keys USING btree (service_account_id);

CREATE INDEX sessions_expires_at_idx ON sessions USING btree (expires_at);

CREATE INDEX sessions_user_id_idx ON sessions USING btree (user_id);

CREATE INDEX snapshots_observation_idx ON snapshots USING btree (observation_id);

CREATE INDEX team_members_user_idx ON team_members USING btree (user_id);

CREATE UNIQUE INDEX teams_name_idx ON teams USING btree (lower(name));

CREATE UNIQUE INDEX thread_mentions_agent_key ON thread_mentions USING btree (message_id, agent) WHERE (agent IS NOT NULL);

CREATE UNIQUE INDEX thread_mentions_grant_key ON thread_mentions USING btree (message_id, agent_grant_id) WHERE (agent_grant_id IS NOT NULL);

CREATE UNIQUE INDEX thread_mentions_service_account_key ON thread_mentions USING btree (message_id, agent_service_account_id) WHERE (agent_service_account_id IS NOT NULL);

CREATE INDEX thread_mentions_user_idx ON thread_mentions USING btree (user_id) WHERE (user_id IS NOT NULL);

CREATE UNIQUE INDEX thread_mentions_user_key ON thread_mentions USING btree (message_id, user_id) WHERE (user_id IS NOT NULL);

CREATE UNIQUE INDEX thread_messages_client_idx ON thread_messages USING btree (author_kind, author_id, client_id) WHERE (client_id IS NOT NULL);

CREATE INDEX thread_messages_observation_block_idx ON thread_messages USING btree (observation_block_id) WHERE (observation_block_id IS NOT NULL);

CREATE INDEX thread_messages_reply_to_idx ON thread_messages USING btree (reply_to_id) WHERE (reply_to_id IS NOT NULL);

CREATE INDEX thread_messages_run_idx ON thread_messages USING btree (run_id) WHERE (run_id IS NOT NULL);

CREATE INDEX thread_messages_search_idx ON thread_messages USING gin (message_search_vector(body));

CREATE INDEX thread_messages_thread_created_idx ON thread_messages USING btree (thread_id, created_at, id);

CREATE INDEX thread_reads_thread_idx ON thread_reads USING btree (thread_id);

CREATE INDEX threads_annotation_idx ON threads USING btree (annotation_id) WHERE (annotation_id IS NOT NULL);

CREATE INDEX threads_block_idx ON threads USING btree (block_id) WHERE (block_id IS NOT NULL);

CREATE INDEX threads_dashboard_idx ON threads USING btree (dashboard_id) WHERE (dashboard_id IS NOT NULL);

CREATE INDEX threads_dashboard_open_idx ON threads USING btree (dashboard_id) WHERE ((dashboard_id IS NOT NULL) AND (status = 'open'::text));

CREATE INDEX threads_finding_idx ON threads USING btree (finding_id) WHERE (finding_id IS NOT NULL);

CREATE INDEX threads_observation_idx ON threads USING btree (observation_id) WHERE (observation_id IS NOT NULL);

CREATE UNIQUE INDEX threads_observation_page_idx ON threads USING btree (observation_id) WHERE ((observation_id IS NOT NULL) AND (anchor IS NULL) AND (block_id IS NULL));

CREATE INDEX threads_proposal_idx ON threads USING btree (proposal_id) WHERE (proposal_id IS NOT NULL);

CREATE INDEX user_invites_created_idx ON user_invites USING btree (created_at DESC, id DESC);

CREATE UNIQUE INDEX user_invites_email_idx ON user_invites USING btree (lower(email));

CREATE INDEX users_created_idx ON users USING btree (created_at DESC, id DESC);

CREATE UNIQUE INDEX users_first_user_idx ON users USING btree (first_user) WHERE first_user;

CREATE INDEX users_groups_idx ON users USING gin (groups);

CREATE INDEX webhook_deliveries_attempted_idx ON webhook_deliveries USING btree (attempted_at);

CREATE INDEX webhook_deliveries_webhook_idx ON webhook_deliveries USING btree (webhook_id, id DESC);

CREATE INDEX webhooks_due_idx ON webhooks USING btree (next_attempt_at) WHERE (disabled_at IS NULL);

CREATE TRIGGER agent_requests_keep_copies BEFORE DELETE ON agent_requests FOR EACH ROW EXECUTE FUNCTION keep_copies_of_deleted_run();

CREATE TRIGGER agent_requests_unsaved_findings BEFORE DELETE ON agent_requests FOR EACH ROW EXECUTE FUNCTION delete_unsaved_findings();

CREATE TRIGGER audit_events_append_only BEFORE DELETE OR UPDATE ON audit_events FOR EACH ROW EXECUTE FUNCTION audit_events_refuse_change();

CREATE TRIGGER audit_events_no_truncate BEFORE TRUNCATE ON audit_events FOR EACH STATEMENT EXECUTE FUNCTION audit_events_refuse_change();

CREATE TRIGGER dashboards_delete_findings BEFORE DELETE ON dashboards FOR EACH ROW EXECUTE FUNCTION delete_dashboard_findings();

CREATE TRIGGER dashboards_mark_runs_kept_by_it BEFORE DELETE ON dashboards FOR EACH ROW EXECUTE FUNCTION mark_runs_kept_by_deleted_subject();

CREATE TRIGGER observation_rcas_immutable BEFORE UPDATE ON observation_rcas FOR EACH ROW EXECUTE FUNCTION observation_rcas_immutable();

CREATE TRIGGER observations_mark_runs_kept_by_it BEFORE DELETE ON observations FOR EACH ROW EXECUTE FUNCTION mark_runs_kept_by_deleted_subject();

CREATE TRIGGER snapshots_immutable BEFORE UPDATE ON snapshots FOR EACH ROW EXECUTE FUNCTION snapshots_immutable();

CREATE TRIGGER users_personal_folder AFTER INSERT ON users FOR EACH ROW EXECUTE FUNCTION create_personal_folder();

CREATE TRIGGER users_personal_folder_contents BEFORE DELETE ON users FOR EACH ROW EXECUTE FUNCTION delete_personal_folder_contents();

ALTER TABLE ONLY access_requests
    ADD CONSTRAINT access_requests_activity_seq_fkey FOREIGN KEY (activity_seq) REFERENCES activity(seq) ON DELETE SET NULL;

ALTER TABLE ONLY access_requests
    ADD CONSTRAINT access_requests_dashboard_id_fkey FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE;

ALTER TABLE ONLY access_requests
    ADD CONSTRAINT access_requests_observation_id_fkey FOREIGN KEY (observation_id) REFERENCES observations(id) ON DELETE CASCADE;

ALTER TABLE ONLY activity
    ADD CONSTRAINT activity_folder_id_fkey FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_request_builds
    ADD CONSTRAINT agent_request_builds_agent_request_id_fkey FOREIGN KEY (agent_request_id) REFERENCES agent_requests(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_request_builds
    ADD CONSTRAINT agent_request_builds_dashboard_id_fkey FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_request_builds
    ADD CONSTRAINT agent_request_builds_folder_id_fkey FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE SET NULL;

ALTER TABLE ONLY agent_requests
    ADD CONSTRAINT agent_requests_applies_finding_id_fkey FOREIGN KEY (applies_finding_id) REFERENCES findings(id) ON DELETE SET NULL;

ALTER TABLE ONLY agent_requests
    ADD CONSTRAINT agent_requests_block_id_fkey FOREIGN KEY (block_id) REFERENCES observation_blocks(id) ON DELETE SET NULL;

ALTER TABLE ONLY agent_requests
    ADD CONSTRAINT agent_requests_chat_id_fkey FOREIGN KEY (chat_id) REFERENCES chats(id) ON DELETE SET NULL;

ALTER TABLE ONLY agent_requests
    ADD CONSTRAINT agent_requests_comment_id_fkey FOREIGN KEY (comment_id) REFERENCES threads(id) ON DELETE SET NULL;

ALTER TABLE ONLY agent_requests
    ADD CONSTRAINT agent_requests_continues_id_fkey FOREIGN KEY (continues_id) REFERENCES agent_requests(id) ON DELETE SET NULL;

ALTER TABLE ONLY agent_requests
    ADD CONSTRAINT agent_requests_dashboard_id_fkey FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_requests
    ADD CONSTRAINT agent_requests_observation_id_fkey FOREIGN KEY (observation_id) REFERENCES observations(id) ON DELETE SET NULL;

ALTER TABLE ONLY agent_requests
    ADD CONSTRAINT agent_requests_retries_id_fkey FOREIGN KEY (retries_id) REFERENCES agent_requests(id) ON DELETE SET NULL;

ALTER TABLE ONLY agent_requests
    ADD CONSTRAINT agent_requests_subject_observation_id_fkey FOREIGN KEY (subject_observation_id) REFERENCES observations(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_requests
    ADD CONSTRAINT agent_requests_thread_message_id_fkey FOREIGN KEY (thread_message_id) REFERENCES thread_messages(id) ON DELETE SET NULL;

ALTER TABLE ONLY annotations
    ADD CONSTRAINT annotations_dashboard_id_fkey FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE;

ALTER TABLE ONLY applied_dashboards
    ADD CONSTRAINT applied_dashboards_dashboard_id_fkey FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE;

ALTER TABLE ONLY chats
    ADD CONSTRAINT chats_dashboard_id_fkey FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE SET NULL;

ALTER TABLE ONLY chats
    ADD CONSTRAINT chats_observation_id_fkey FOREIGN KEY (observation_id) REFERENCES observations(id) ON DELETE SET NULL;

ALTER TABLE ONLY dashboard_health
    ADD CONSTRAINT dashboard_health_dashboard_id_fkey FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE;

ALTER TABLE ONLY dashboard_stars
    ADD CONSTRAINT dashboard_stars_dashboard_id_fkey FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE;

ALTER TABLE ONLY dashboard_stars
    ADD CONSTRAINT dashboard_stars_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY dashboard_versions
    ADD CONSTRAINT dashboard_versions_agent_request_id_fkey FOREIGN KEY (agent_request_id) REFERENCES agent_requests(id) ON DELETE SET NULL;

ALTER TABLE ONLY dashboard_versions
    ADD CONSTRAINT dashboard_versions_dashboard_id_fkey FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE;

ALTER TABLE ONLY dashboard_versions
    ADD CONSTRAINT dashboard_versions_merged_proposal_id_fkey FOREIGN KEY (merged_proposal_id) REFERENCES proposals(id) ON DELETE SET NULL;

ALTER TABLE ONLY dashboard_view_days
    ADD CONSTRAINT dashboard_view_days_dashboard_id_fkey FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE;

ALTER TABLE ONLY dashboard_view_days
    ADD CONSTRAINT dashboard_view_days_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY dashboard_viewers
    ADD CONSTRAINT dashboard_viewers_dashboard_id_fkey FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE;

ALTER TABLE ONLY dashboard_viewers
    ADD CONSTRAINT dashboard_viewers_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY dashboard_views
    ADD CONSTRAINT dashboard_views_dashboard_id_fkey FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE;

ALTER TABLE ONLY dashboard_views
    ADD CONSTRAINT dashboard_views_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY dashboards
    ADD CONSTRAINT dashboards_created_run_id_fkey FOREIGN KEY (created_run_id) REFERENCES agent_requests(id) ON DELETE SET NULL;

ALTER TABLE ONLY dashboards
    ADD CONSTRAINT dashboards_folder_id_fkey FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE RESTRICT;

ALTER TABLE ONLY findings
    ADD CONSTRAINT findings_agent_request_id_fkey FOREIGN KEY (agent_request_id) REFERENCES agent_requests(id) ON DELETE SET NULL;

ALTER TABLE ONLY findings
    ADD CONSTRAINT findings_annotation_id_fkey FOREIGN KEY (annotation_id) REFERENCES annotations(id) ON DELETE SET NULL;

ALTER TABLE ONLY findings
    ADD CONSTRAINT findings_ask_dashboard_id_fkey FOREIGN KEY (ask_dashboard_id) REFERENCES dashboards(id) ON DELETE SET NULL;

ALTER TABLE ONLY findings
    ADD CONSTRAINT findings_dashboard_id_fkey FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE;

ALTER TABLE ONLY findings
    ADD CONSTRAINT findings_folder_id_fkey FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE RESTRICT;

ALTER TABLE ONLY findings
    ADD CONSTRAINT findings_observation_id_fkey FOREIGN KEY (observation_id) REFERENCES observations(id) ON DELETE CASCADE;

ALTER TABLE ONLY folder_permissions
    ADD CONSTRAINT folder_permissions_created_by_id_fkey FOREIGN KEY (created_by_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE ONLY folder_permissions
    ADD CONSTRAINT folder_permissions_folder_id_fkey FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE CASCADE;

ALTER TABLE ONLY folder_permissions
    ADD CONSTRAINT folder_permissions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY folders
    ADD CONSTRAINT folders_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY grafana_sync_items
    ADD CONSTRAINT grafana_sync_items_sync_id_fkey FOREIGN KEY (sync_id) REFERENCES grafana_syncs(id) ON DELETE CASCADE;

ALTER TABLE ONLY grafana_syncs
    ADD CONSTRAINT grafana_syncs_connection_id_fkey FOREIGN KEY (connection_id) REFERENCES grafana_connections(id) ON DELETE CASCADE;

ALTER TABLE ONLY home_dashboards
    ADD CONSTRAINT home_dashboards_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY imports
    ADD CONSTRAINT imports_version_fkey FOREIGN KEY (dashboard_id, version) REFERENCES dashboard_versions(dashboard_id, version) ON DELETE CASCADE;

ALTER TABLE ONLY inbox_cursors
    ADD CONSTRAINT inbox_cursors_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY inbox_item_states
    ADD CONSTRAINT inbox_item_states_seq_fkey FOREIGN KEY (seq) REFERENCES activity(seq) ON DELETE CASCADE;

ALTER TABLE ONLY inbox_item_states
    ADD CONSTRAINT inbox_item_states_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY mcp_source_tools
    ADD CONSTRAINT mcp_source_tools_source_id_fkey FOREIGN KEY (source_id) REFERENCES mcp_sources(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_codes
    ADD CONSTRAINT oauth_codes_client_id_fkey FOREIGN KEY (client_id) REFERENCES oauth_clients(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_codes
    ADD CONSTRAINT oauth_codes_grant_id_fkey FOREIGN KEY (grant_id) REFERENCES oauth_grants(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_codes
    ADD CONSTRAINT oauth_codes_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_grants
    ADD CONSTRAINT oauth_grants_client_id_fkey FOREIGN KEY (client_id) REFERENCES oauth_clients(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_grants
    ADD CONSTRAINT oauth_grants_revoked_by_fkey FOREIGN KEY (revoked_by) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE ONLY oauth_grants
    ADD CONSTRAINT oauth_grants_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_requests
    ADD CONSTRAINT oauth_requests_client_id_fkey FOREIGN KEY (client_id) REFERENCES oauth_clients(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_tokens
    ADD CONSTRAINT oauth_tokens_grant_id_fkey FOREIGN KEY (grant_id) REFERENCES oauth_grants(id) ON DELETE CASCADE;

ALTER TABLE ONLY observation_blocks
    ADD CONSTRAINT observation_blocks_observation_id_fkey FOREIGN KEY (observation_id) REFERENCES observations(id) ON DELETE CASCADE;

ALTER TABLE ONLY observation_blocks
    ADD CONSTRAINT observation_blocks_run_id_fkey FOREIGN KEY (run_id) REFERENCES agent_requests(id) ON DELETE SET NULL;

ALTER TABLE ONLY observation_blocks
    ADD CONSTRAINT observation_blocks_snapshot_id_fkey FOREIGN KEY (snapshot_id) REFERENCES snapshots(id);

ALTER TABLE ONLY observation_dashboards
    ADD CONSTRAINT observation_dashboards_block_id_fkey FOREIGN KEY (block_id) REFERENCES observation_blocks(id) ON DELETE CASCADE;

ALTER TABLE ONLY observation_dashboards
    ADD CONSTRAINT observation_dashboards_dashboard_id_fkey FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE;

ALTER TABLE ONLY observation_dashboards
    ADD CONSTRAINT observation_dashboards_observation_id_fkey FOREIGN KEY (observation_id) REFERENCES observations(id) ON DELETE CASCADE;

ALTER TABLE ONLY observation_pins
    ADD CONSTRAINT observation_pins_annotation_id_fkey FOREIGN KEY (annotation_id) REFERENCES annotations(id) ON DELETE CASCADE;

ALTER TABLE ONLY observation_pins
    ADD CONSTRAINT observation_pins_dashboard_id_fkey FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE;

ALTER TABLE ONLY observation_pins
    ADD CONSTRAINT observation_pins_observation_id_fkey FOREIGN KEY (observation_id) REFERENCES observations(id) ON DELETE CASCADE;

ALTER TABLE ONLY observation_rcas
    ADD CONSTRAINT observation_rcas_observation_id_fkey FOREIGN KEY (observation_id) REFERENCES observations(id) ON DELETE CASCADE;

ALTER TABLE ONLY observation_rcas
    ADD CONSTRAINT observation_rcas_run_id_fkey FOREIGN KEY (run_id) REFERENCES agent_requests(id) ON DELETE SET NULL;

ALTER TABLE ONLY observations
    ADD CONSTRAINT observations_folder_id_fkey FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE RESTRICT;

ALTER TABLE ONLY observations
    ADD CONSTRAINT observations_run_id_fkey FOREIGN KEY (run_id) REFERENCES agent_requests(id) ON DELETE SET NULL;

ALTER TABLE ONLY outbound_domains
    ADD CONSTRAINT outbound_domains_created_by_id_fkey FOREIGN KEY (created_by_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE ONLY proposal_revisions
    ADD CONSTRAINT proposal_revisions_proposal_id_fkey FOREIGN KEY (proposal_id) REFERENCES proposals(id) ON DELETE CASCADE;

ALTER TABLE ONLY proposals
    ADD CONSTRAINT proposals_dashboard_id_fkey FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE;

ALTER TABLE ONLY proposals
    ADD CONSTRAINT proposals_run_id_fkey FOREIGN KEY (run_id) REFERENCES agent_requests(id) ON DELETE SET NULL;

ALTER TABLE ONLY provisioned_dashboards
    ADD CONSTRAINT provisioned_dashboards_dashboard_id_fkey FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE;

ALTER TABLE ONLY public_links
    ADD CONSTRAINT public_links_dashboard_id_fkey FOREIGN KEY (dashboard_id) REFERENCES dashboards(id) ON DELETE CASCADE;

ALTER TABLE ONLY public_links
    ADD CONSTRAINT public_links_rca_id_fkey FOREIGN KEY (rca_id) REFERENCES observation_rcas(id) ON DELETE CASCADE;

ALTER TABLE ONLY run_sessions
    ADD CONSTRAINT run_sessions_agent_request_id_fkey FOREIGN KEY (agent_request_id) REFERENCES agent_requests(id) ON DELETE CASCADE;

ALTER TABLE ONLY run_tokens
    ADD CONSTRAINT run_tokens_agent_request_id_fkey FOREIGN KEY (agent_request_id) REFERENCES agent_requests(id) ON DELETE CASCADE;

ALTER TABLE ONLY saved_prompts
    ADD CONSTRAINT saved_prompts_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY saved_searches
    ADD CONSTRAINT saved_searches_folder_id_fkey FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE RESTRICT;

ALTER TABLE ONLY service_account_keys
    ADD CONSTRAINT service_account_keys_created_by_fkey FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE ONLY service_account_keys
    ADD CONSTRAINT service_account_keys_service_account_id_fkey FOREIGN KEY (service_account_id) REFERENCES service_accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY service_accounts
    ADD CONSTRAINT service_accounts_created_by_fkey FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE ONLY sessions
    ADD CONSTRAINT sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY snapshots
    ADD CONSTRAINT snapshots_observation_id_fkey FOREIGN KEY (observation_id) REFERENCES observations(id) ON DELETE CASCADE;

ALTER TABLE ONLY snapshots
    ADD CONSTRAINT snapshots_run_id_fkey FOREIGN KEY (run_id) REFERENCES agent_requests(id) ON DELETE SET NULL;

ALTER TABLE ONLY team_members
    ADD CONSTRAINT team_members_team_id_fkey FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;

ALTER TABLE ONLY team_members
    ADD CONSTRAINT team_members_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY thread_mentions
    ADD CONSTRAINT thread_mentions_agent_grant_id_fkey FOREIGN KEY (agent_grant_id) REFERENCES oauth_grants(id) ON DELETE CASCADE;

ALTER TABLE ONLY thread_mentions
    ADD CONSTRAINT thread_mentions_agent_service_account_id_fkey FOREIGN KEY (agent_service_account_id) REFERENCES service_accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY thread_mentions
    ADD CONSTRAINT thread_mentions_message_id_fkey FOREIGN KEY (message_id) REFERENCES thread_messages(id) ON DELETE CASCADE;

ALTER TABLE ONLY thread_mentions
    ADD CONSTRAINT thread_mentions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY thread_messages
    ADD CONSTRAINT thread_messages_observation_block_id_fkey FOREIGN KEY (observation_block_id) REFERENCES observation_blocks(id) ON DELETE SET NULL;

ALTER TABLE ONLY thread_messages
    ADD CONSTRAINT thread_messages_reply_to_id_fkey FOREIGN KEY (reply_to_id) REFERENCES thread_messages(id) ON DELETE SET NULL;

ALTER TABLE ONLY thread_messages
    ADD CONSTRAINT thread_messages_run_id_fkey FOREIGN KEY (run_id) REFERENCES agent_requests(id) ON DELETE SET NULL;

ALTER TABLE ONLY thread_messages
    ADD CONSTRAINT thread_messages_thread_id_fkey FOREIGN KEY (thread_id) REFERENCES threads(id) ON DELETE CASCADE;

ALTER TABLE ONLY thread_reads
    ADD CONSTRAINT thread_reads_thread_id_fkey FOREIGN KEY (thread_id) REFERENCES threads(id) ON DELETE CASCADE;

ALTER TABLE ONLY thread_reads
    ADD CONSTRAINT thread_reads_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY threads
    ADD CONSTRAINT threads_annotation_id_fkey FOREIGN KEY (annotation_id) REFERENCES annotations(id) ON DELETE CASCADE;

ALTER TABLE ONLY threads
    ADD CONSTRAINT threads_block_id_fkey FOREIGN KEY (block_id) REFERENCES observation_blocks(id) ON DELETE SET NULL;

ALTER TABLE ONLY threads
    ADD CONSTRAINT threads_dashboard_version_fkey FOREIGN KEY (dashboard_id, dashboard_version) REFERENCES dashboard_versions(dashboard_id, version) ON DELETE CASCADE;

ALTER TABLE ONLY threads
    ADD CONSTRAINT threads_finding_id_fkey FOREIGN KEY (finding_id) REFERENCES findings(id) ON DELETE CASCADE;

ALTER TABLE ONLY threads
    ADD CONSTRAINT threads_observation_id_fkey FOREIGN KEY (observation_id) REFERENCES observations(id) ON DELETE CASCADE;

ALTER TABLE ONLY threads
    ADD CONSTRAINT threads_proposal_id_fkey FOREIGN KEY (proposal_id) REFERENCES proposals(id) ON DELETE CASCADE;

ALTER TABLE ONLY threads
    ADD CONSTRAINT threads_resolved_by_version_fkey FOREIGN KEY (dashboard_id, resolved_by_version) REFERENCES dashboard_versions(dashboard_id, version) ON DELETE SET NULL (resolved_by_version);

ALTER TABLE ONLY users
    ADD CONSTRAINT users_deactivated_by_id_fkey FOREIGN KEY (deactivated_by_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE ONLY users
    ADD CONSTRAINT users_manual_role_by_id_fkey FOREIGN KEY (manual_role_by_id) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE ONLY webhook_deliveries
    ADD CONSTRAINT webhook_deliveries_webhook_id_fkey FOREIGN KEY (webhook_id) REFERENCES webhooks(id) ON DELETE CASCADE;

INSERT INTO instance DEFAULT VALUES;

INSERT INTO folders (title, general) VALUES ('General', true);

DO $$
DECLARE
  day DATE;
BEGIN
  FOR day IN SELECT generate_series(((now() - interval '1 hour') AT TIME ZONE 'UTC')::date, (now() AT TIME ZONE 'UTC')::date + 14, interval '1 day')::date LOOP
    PERFORM audit_events_add_partition(day);
  END LOOP;
END
$$;
-- +goose StatementEnd
