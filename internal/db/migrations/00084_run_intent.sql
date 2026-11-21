-- +goose NO TRANSACTION
-- +goose Up
ALTER TABLE agent_requests
  ADD COLUMN IF NOT EXISTS intent TEXT,
  ADD COLUMN IF NOT EXISTS kind TEXT,
  ADD COLUMN IF NOT EXISTS applies_finding_id UUID,
  ADD COLUMN IF NOT EXISTS subject_observation_id UUID;

ALTER TABLE findings
  ADD COLUMN IF NOT EXISTS suggested_change TEXT,
  ADD COLUMN IF NOT EXISTS observe_worthy BOOLEAN NOT NULL DEFAULT false;

-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'agent_requests' AND column_name = 'source') THEN
    EXECUTE $f$
      CREATE OR REPLACE FUNCTION agent_requests_intent_of_source() RETURNS trigger LANGUAGE plpgsql AS $b$
      BEGIN
        IF NEW.intent IS NULL OR (NEW.source = 'observe' AND NEW.result_version IS NOT NULL AND NEW.intent = 'ask') THEN
          NEW.intent := CASE
            WHEN NEW.source = 'suggest' THEN 'suggest'
            WHEN NEW.source IN ('prompt', 'comment') THEN 'apply'
            WHEN NEW.source = 'observe' AND NEW.result_version IS NOT NULL THEN 'apply'
            ELSE 'ask' END;
        END IF;
        IF NEW.kind IS NULL THEN
          NEW.kind := CASE
            WHEN NEW.source = 'investigate' THEN 'investigate'
            WHEN NEW.intent = 'ask' AND (NEW.dashboard_id IS NOT NULL OR NEW.observation_id IS NOT NULL OR NEW.subject_observation_id IS NOT NULL) THEN 'investigate'
            ELSE 'build' END;
        END IF;
        RETURN NEW;
      END
      $b$
    $f$;
    EXECUTE 'CREATE OR REPLACE TRIGGER agent_requests_intent_of_source BEFORE INSERT OR UPDATE ON agent_requests FOR EACH ROW EXECUTE FUNCTION agent_requests_intent_of_source()';
  END IF;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
DECLARE
  batch uuid[];
  after_id uuid := '00000000-0000-0000-0000-000000000000';
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'agent_requests' AND column_name = 'source') THEN
    RETURN;
  END IF;
  LOOP
    SELECT array_agg(id ORDER BY id) INTO batch FROM (
      SELECT id FROM agent_requests WHERE id > after_id AND (intent IS NULL OR kind IS NULL) ORDER BY id LIMIT 1000) b;
    EXIT WHEN batch IS NULL;
    UPDATE agent_requests SET intent = NULL WHERE id = ANY (batch) AND intent IS NULL;
    UPDATE agent_requests SET kind = NULL WHERE id = ANY (batch) AND kind IS NULL;
    after_id := batch[array_upper(batch, 1)];
    COMMIT;
  END LOOP;
END
$$;
-- +goose StatementEnd

ALTER TABLE agent_requests
  DROP CONSTRAINT IF EXISTS agent_requests_intent_check,
  DROP CONSTRAINT IF EXISTS agent_requests_kind_check,
  DROP CONSTRAINT IF EXISTS agent_requests_build_ask_check,
  DROP CONSTRAINT IF EXISTS agent_requests_ask_target_check,
  DROP CONSTRAINT IF EXISTS agent_requests_ask_shape_check,
  DROP CONSTRAINT IF EXISTS agent_requests_suggest_intent_shape_check,
  DROP CONSTRAINT IF EXISTS agent_requests_intent_context_check,
  DROP CONSTRAINT IF EXISTS agent_requests_intent_draft_check,
  DROP CONSTRAINT IF EXISTS agent_requests_intent_suggestion_check,
  DROP CONSTRAINT IF EXISTS agent_requests_intent_succeeded_check,
  DROP CONSTRAINT IF EXISTS agent_requests_assigned_intent_check,
  DROP CONSTRAINT IF EXISTS agent_requests_applies_finding_check,
  DROP CONSTRAINT IF EXISTS agent_requests_applies_finding_id_fkey,
  DROP CONSTRAINT IF EXISTS agent_requests_subject_observation_check,
  DROP CONSTRAINT IF EXISTS agent_requests_subject_observation_id_fkey,
  ADD CONSTRAINT agent_requests_intent_check CHECK (intent IS NOT NULL AND intent IN ('ask', 'apply', 'suggest')) NOT VALID,
  ADD CONSTRAINT agent_requests_kind_check CHECK (kind IS NOT NULL AND (kind = 'build' OR (kind = 'investigate' AND intent = 'ask'))) NOT VALID,
  ADD CONSTRAINT agent_requests_build_ask_check
    CHECK (kind <> 'build' OR intent <> 'ask' OR (dashboard_id IS NULL AND observation_id IS NULL AND subject_observation_id IS NULL)) NOT VALID,
  ADD CONSTRAINT agent_requests_ask_target_check CHECK (intent = 'ask' OR dashboard_id IS NOT NULL) NOT VALID,
  ADD CONSTRAINT agent_requests_ask_shape_check
    CHECK (intent <> 'ask' OR (base_version IS NULL AND comment_id IS NULL AND (panel_id IS NULL OR dashboard_id IS NOT NULL))) NOT VALID,
  ADD CONSTRAINT agent_requests_suggest_intent_shape_check
    CHECK (intent <> 'suggest' OR (dashboard_id IS NOT NULL AND base_version IS NOT NULL AND panel_id IS NOT NULL AND comment_id IS NULL AND result_version IS NULL)) NOT VALID,
  ADD CONSTRAINT agent_requests_intent_context_check
    CHECK ((context_from IS NULL AND context_to IS NULL)
      OR (intent = 'ask' AND context_from IS NOT NULL AND context_to IS NOT NULL AND context_from < context_to)) NOT VALID,
  ADD CONSTRAINT agent_requests_intent_draft_check CHECK (draft IS NULL OR (intent = 'suggest' AND jsonb_typeof(draft) = 'object')) NOT VALID,
  ADD CONSTRAINT agent_requests_intent_suggestion_check CHECK (suggestion IS NULL OR (intent = 'suggest' AND jsonb_typeof(suggestion) = 'object')) NOT VALID,
  ADD CONSTRAINT agent_requests_intent_succeeded_check
    CHECK (status <> 'succeeded' OR ((intent <> 'suggest' OR suggestion IS NOT NULL) AND (result_version IS NULL OR dashboard_id IS NOT NULL))) NOT VALID,
  ADD CONSTRAINT agent_requests_assigned_intent_check CHECK (assigned_kind = 'builtin' OR intent <> 'suggest') NOT VALID,
  ADD CONSTRAINT agent_requests_applies_finding_check CHECK (applies_finding_id IS NULL OR intent = 'apply') NOT VALID,
  ADD CONSTRAINT agent_requests_applies_finding_id_fkey FOREIGN KEY (applies_finding_id) REFERENCES findings (id) ON DELETE SET NULL NOT VALID,
  ADD CONSTRAINT agent_requests_subject_observation_check
    CHECK (subject_observation_id IS NULL OR (intent = 'ask' AND observation_id IS NULL AND dashboard_id IS NULL)) NOT VALID,
  ADD CONSTRAINT agent_requests_subject_observation_id_fkey FOREIGN KEY (subject_observation_id) REFERENCES observations (id) ON DELETE CASCADE NOT VALID;

ALTER TABLE agent_requests VALIDATE CONSTRAINT agent_requests_intent_check;
ALTER TABLE agent_requests VALIDATE CONSTRAINT agent_requests_kind_check;
ALTER TABLE agent_requests VALIDATE CONSTRAINT agent_requests_build_ask_check;
ALTER TABLE agent_requests VALIDATE CONSTRAINT agent_requests_ask_target_check;
ALTER TABLE agent_requests VALIDATE CONSTRAINT agent_requests_ask_shape_check;
ALTER TABLE agent_requests VALIDATE CONSTRAINT agent_requests_suggest_intent_shape_check;
ALTER TABLE agent_requests VALIDATE CONSTRAINT agent_requests_intent_context_check;
ALTER TABLE agent_requests VALIDATE CONSTRAINT agent_requests_intent_draft_check;
ALTER TABLE agent_requests VALIDATE CONSTRAINT agent_requests_intent_suggestion_check;
ALTER TABLE agent_requests VALIDATE CONSTRAINT agent_requests_intent_succeeded_check;
ALTER TABLE agent_requests VALIDATE CONSTRAINT agent_requests_assigned_intent_check;
ALTER TABLE agent_requests VALIDATE CONSTRAINT agent_requests_applies_finding_check;
ALTER TABLE agent_requests VALIDATE CONSTRAINT agent_requests_applies_finding_id_fkey;
ALTER TABLE agent_requests VALIDATE CONSTRAINT agent_requests_subject_observation_check;
ALTER TABLE agent_requests VALIDATE CONSTRAINT agent_requests_subject_observation_id_fkey;
ALTER TABLE agent_requests ALTER COLUMN intent SET NOT NULL, ALTER COLUMN kind SET NOT NULL;

ALTER TABLE findings
  DROP CONSTRAINT IF EXISTS findings_suggested_change_check,
  ADD CONSTRAINT findings_suggested_change_check
    CHECK (suggested_change IS NULL OR char_length(suggested_change) BETWEEN 1 AND 500) NOT VALID;
ALTER TABLE findings VALIDATE CONSTRAINT findings_suggested_change_check;

DROP INDEX CONCURRENTLY IF EXISTS agent_requests_running_dashboard_idx;
CREATE UNIQUE INDEX CONCURRENTLY agent_requests_running_dashboard_idx ON agent_requests (dashboard_id)
  WHERE status = 'running' AND intent = 'apply';
DROP INDEX CONCURRENTLY IF EXISTS agent_requests_create_active_idx;
CREATE INDEX CONCURRENTLY agent_requests_create_active_idx ON agent_requests (requested_by_id)
  WHERE kind = 'build' AND intent = 'ask' AND status IN ('queued', 'running');
DROP INDEX CONCURRENTLY IF EXISTS agent_requests_investigation_active_idx;
CREATE INDEX CONCURRENTLY agent_requests_investigation_active_idx ON agent_requests (requested_by_id)
  WHERE kind = 'investigate' AND observation_id IS NOT NULL AND thread_message_id IS NULL AND status IN ('queued', 'running');
DROP INDEX CONCURRENTLY IF EXISTS agent_requests_suggest_intent_active_idx;
CREATE INDEX CONCURRENTLY agent_requests_suggest_intent_active_idx ON agent_requests (requested_by_id)
  WHERE intent = 'suggest' AND status IN ('queued', 'running');
DROP INDEX CONCURRENTLY IF EXISTS agent_requests_listed_intent_idx;
CREATE INDEX CONCURRENTLY agent_requests_listed_intent_idx ON agent_requests (created_at DESC, id DESC)
  WHERE intent <> 'suggest' AND archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS agent_requests_applies_finding_idx;
CREATE UNIQUE INDEX CONCURRENTLY agent_requests_applies_finding_idx ON agent_requests (applies_finding_id)
  WHERE applies_finding_id IS NOT NULL AND status IN ('queued', 'running', 'succeeded');
DROP INDEX CONCURRENTLY IF EXISTS agent_requests_subject_observation_idx;
CREATE INDEX CONCURRENTLY agent_requests_subject_observation_idx ON agent_requests (subject_observation_id)
  WHERE subject_observation_id IS NOT NULL;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION inbox_reason(a activity, reader uuid, editable uuid[], repair_prefix text) RETURNS text
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
-- +goose StatementEnd

DROP TRIGGER IF EXISTS agent_requests_old_server_chat ON agent_requests;
DROP FUNCTION IF EXISTS agent_requests_old_server_chat();
DROP TRIGGER IF EXISTS agent_requests_intent_of_source ON agent_requests;
DROP FUNCTION IF EXISTS agent_requests_intent_of_source();

DROP INDEX CONCURRENTLY IF EXISTS agent_requests_observe_active_idx;
DROP INDEX CONCURRENTLY IF EXISTS agent_requests_investigate_active_idx;
DROP INDEX CONCURRENTLY IF EXISTS agent_requests_suggest_active_idx;
DROP INDEX CONCURRENTLY IF EXISTS agent_requests_listed_recent_idx;
ALTER TABLE agent_requests DROP COLUMN IF EXISTS source;

UPDATE audit_activity_actions SET keys = array_replace(keys, 'source', 'intent') WHERE type LIKE 'run.%';

-- +goose StatementBegin
DO $$
DECLARE
  after_seq bigint := 0;
  last_seq bigint;
BEGIN
  LOOP
    SELECT max(seq) INTO last_seq FROM (
      SELECT seq FROM activity WHERE seq > after_seq AND type LIKE 'run.%' AND data ? 'source' ORDER BY seq LIMIT 1000) b;
    EXIT WHEN last_seq IS NULL;
    UPDATE activity a SET data = (a.data - 'source') || jsonb_build_object('intent', coalesce(
      (SELECT r.intent FROM agent_requests r WHERE r.id = a.target_id),
      CASE
        WHEN a.data->>'source' = 'suggest' THEN 'suggest'
        WHEN a.data->>'source' IN ('prompt', 'comment') THEN 'apply'
        WHEN a.data->>'source' = 'observe' AND a.data->>'result_version' IS NOT NULL THEN 'apply'
        ELSE 'ask' END))
    WHERE a.seq > after_seq AND a.seq <= last_seq AND a.type LIKE 'run.%' AND a.data ? 'source';
    after_seq := last_seq;
    COMMIT;
  END LOOP;
END
$$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE agent_requests ADD COLUMN IF NOT EXISTS source TEXT;
UPDATE agent_requests SET source = CASE
  WHEN intent = 'suggest' THEN 'suggest'
  WHEN intent = 'apply' AND base_version IS NULL THEN 'observe'
  WHEN intent = 'apply' AND (comment_id IS NOT NULL OR thread_message_id IS NOT NULL) THEN 'comment'
  WHEN intent = 'apply' THEN 'prompt'
  WHEN kind = 'investigate' THEN 'investigate'
  ELSE 'observe' END
WHERE source IS NULL;
DELETE FROM agent_requests WHERE source = 'investigate' AND continues_id IS NOT NULL;
UPDATE agent_requests SET context_from = NULL, context_to = NULL WHERE source <> 'investigate' AND context_from IS NOT NULL;
ALTER TABLE agent_requests ALTER COLUMN source SET NOT NULL;

UPDATE audit_activity_actions SET keys = array_replace(keys, 'intent', 'source') WHERE type LIKE 'run.%';
UPDATE activity a SET data = (a.data - 'intent') || jsonb_build_object('source', coalesce(
  (SELECT r.source FROM agent_requests r WHERE r.id = a.target_id),
  CASE a.data->>'intent' WHEN 'suggest' THEN 'suggest' WHEN 'apply' THEN 'prompt' ELSE 'observe' END))
WHERE a.type LIKE 'run.%' AND a.data ? 'intent';

ALTER TABLE agent_requests
  DROP CONSTRAINT IF EXISTS agent_requests_subject_observation_id_fkey,
  DROP CONSTRAINT IF EXISTS agent_requests_subject_observation_check,
  DROP CONSTRAINT IF EXISTS agent_requests_applies_finding_id_fkey,
  DROP CONSTRAINT IF EXISTS agent_requests_applies_finding_check,
  DROP CONSTRAINT IF EXISTS agent_requests_assigned_intent_check,
  DROP CONSTRAINT IF EXISTS agent_requests_intent_succeeded_check,
  DROP CONSTRAINT IF EXISTS agent_requests_intent_suggestion_check,
  DROP CONSTRAINT IF EXISTS agent_requests_intent_draft_check,
  DROP CONSTRAINT IF EXISTS agent_requests_intent_context_check,
  DROP CONSTRAINT IF EXISTS agent_requests_suggest_intent_shape_check,
  DROP CONSTRAINT IF EXISTS agent_requests_ask_shape_check,
  DROP CONSTRAINT IF EXISTS agent_requests_ask_target_check,
  DROP CONSTRAINT IF EXISTS agent_requests_build_ask_check,
  DROP CONSTRAINT IF EXISTS agent_requests_kind_check,
  DROP CONSTRAINT IF EXISTS agent_requests_intent_check;

ALTER TABLE agent_requests
  ADD CONSTRAINT agent_requests_source_check CHECK (source IN ('comment', 'prompt', 'observe', 'suggest', 'investigate')),
  ADD CONSTRAINT agent_requests_check CHECK (source = 'comment' OR comment_id IS NULL),
  ADD CONSTRAINT agent_requests_target_check CHECK (source IN ('observe', 'investigate') OR (dashboard_id IS NOT NULL AND base_version IS NOT NULL)),
  ADD CONSTRAINT agent_requests_observe_shape_check CHECK (source <> 'observe' OR (base_version IS NULL AND panel_id IS NULL AND comment_id IS NULL)),
  ADD CONSTRAINT agent_requests_draft_check CHECK (draft IS NULL OR (source = 'suggest' AND jsonb_typeof(draft) = 'object')),
  ADD CONSTRAINT agent_requests_suggestion_check CHECK (suggestion IS NULL OR (source = 'suggest' AND jsonb_typeof(suggestion) = 'object')),
  ADD CONSTRAINT agent_requests_suggest_shape_check
    CHECK (source <> 'suggest' OR (dashboard_id IS NOT NULL AND base_version IS NOT NULL AND panel_id IS NOT NULL AND comment_id IS NULL AND result_version IS NULL)),
  ADD CONSTRAINT agent_requests_context_check
    CHECK ((context_from IS NULL AND context_to IS NULL)
      OR (source = 'investigate' AND context_from IS NOT NULL AND context_to IS NOT NULL AND context_from < context_to)),
  ADD CONSTRAINT agent_requests_investigate_shape_check
    CHECK (source <> 'investigate' OR (base_version IS NULL AND comment_id IS NULL AND result_version IS NULL
      AND draft IS NULL AND suggestion IS NULL AND continues_id IS NULL AND (panel_id IS NULL OR dashboard_id IS NOT NULL))),
  ADD CONSTRAINT agent_requests_succeeded_check
    CHECK (status <> 'succeeded' OR source = 'investigate'
      OR (source = 'suggest' AND suggestion IS NOT NULL)
      OR (source = 'observe' AND dashboard_id IS NULL AND result_version IS NULL)
      OR (source IN ('prompt', 'comment') AND dashboard_id IS NOT NULL AND result_version IS NULL)
      OR (source NOT IN ('suggest', 'investigate') AND dashboard_id IS NOT NULL AND result_version IS NOT NULL)
      OR (source NOT IN ('suggest', 'investigate') AND dashboard_id IS NOT NULL AND result_version IS NULL AND result_kind = 'proposal')),
  ADD CONSTRAINT agent_requests_assigned_source_check CHECK (assigned_kind = 'builtin' OR source <> 'suggest');

DROP INDEX IF EXISTS agent_requests_applies_finding_idx;
DROP INDEX IF EXISTS agent_requests_listed_intent_idx;
DROP INDEX IF EXISTS agent_requests_suggest_intent_active_idx;
DROP INDEX IF EXISTS agent_requests_investigation_active_idx;
DROP INDEX IF EXISTS agent_requests_create_active_idx;
DROP INDEX IF EXISTS agent_requests_running_dashboard_idx;
CREATE UNIQUE INDEX agent_requests_running_dashboard_idx ON agent_requests (dashboard_id)
  WHERE status = 'running' AND source NOT IN ('suggest', 'investigate');
CREATE INDEX IF NOT EXISTS agent_requests_observe_active_idx ON agent_requests (requested_by_id)
  WHERE source = 'observe' AND status IN ('queued', 'running');
CREATE INDEX IF NOT EXISTS agent_requests_investigate_active_idx ON agent_requests (requested_by_id)
  WHERE source = 'investigate' AND status IN ('queued', 'running');
CREATE INDEX IF NOT EXISTS agent_requests_suggest_active_idx ON agent_requests (requested_by_id)
  WHERE source = 'suggest' AND status IN ('queued', 'running');
CREATE INDEX IF NOT EXISTS agent_requests_listed_recent_idx ON agent_requests (created_at DESC, id DESC)
  WHERE source <> 'suggest' AND archived_at IS NULL;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION inbox_reason(a activity, reader uuid, editable uuid[], repair_prefix text) RETURNS text
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
        AND r.source = 'prompt' AND starts_with(r.prompt, repair_prefix) AND r.result_version IS NOT NULL
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
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION agent_requests_old_server_chat() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.chat_id IS NULL THEN
    IF NEW.source = 'observe' AND NEW.thread_message_id IS NULL THEN
      INSERT INTO chats (id, owner_kind, owner_id, created_at, last_turn_at)
      VALUES (NEW.id, NEW.requested_by_kind, NEW.requested_by_id, NEW.created_at, NEW.created_at)
      ON CONFLICT (id) DO NOTHING;
      NEW.chat_id := NEW.id;
    END IF;
    RETURN NEW;
  END IF;
  IF NEW.chat_id <> NEW.id THEN
    INSERT INTO chats (id, owner_kind, owner_id, title, pinned_at, archived_at, created_at, last_turn_at)
    SELECT p.id, p.requested_by_kind, p.requested_by_id, p.title, p.pinned_at, NULL, p.created_at, NEW.created_at
    FROM agent_requests p WHERE p.id = NEW.chat_id
    ON CONFLICT (id) DO UPDATE SET last_turn_at = greatest(chats.last_turn_at, EXCLUDED.last_turn_at);
    UPDATE agent_requests SET chat_id = id WHERE id = NEW.chat_id AND chat_id IS NULL;
  END IF;
  RETURN NEW;
END
$$;
-- +goose StatementEnd

CREATE OR REPLACE TRIGGER agent_requests_old_server_chat BEFORE INSERT ON agent_requests
FOR EACH ROW EXECUTE FUNCTION agent_requests_old_server_chat();

ALTER TABLE findings DROP CONSTRAINT IF EXISTS findings_suggested_change_check, DROP COLUMN IF EXISTS observe_worthy, DROP COLUMN IF EXISTS suggested_change;
DROP INDEX IF EXISTS agent_requests_subject_observation_idx;
ALTER TABLE agent_requests DROP COLUMN IF EXISTS subject_observation_id, DROP COLUMN IF EXISTS applies_finding_id, DROP COLUMN IF EXISTS kind, DROP COLUMN IF EXISTS intent;
