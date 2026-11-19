-- +goose Up
ALTER TABLE findings ALTER COLUMN observation_id DROP NOT NULL;
ALTER TABLE findings ADD CONSTRAINT findings_unsaved_run_check CHECK (observation_id IS NOT NULL OR agent_request_id IS NOT NULL);
-- +goose StatementBegin
CREATE FUNCTION delete_unsaved_findings() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  DELETE FROM findings WHERE agent_request_id = OLD.id AND observation_id IS NULL;
  RETURN OLD;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER agent_requests_unsaved_findings BEFORE DELETE ON agent_requests
FOR EACH ROW EXECUTE FUNCTION delete_unsaved_findings();

-- +goose Down
DROP TRIGGER agent_requests_unsaved_findings ON agent_requests;
DROP FUNCTION delete_unsaved_findings();
ALTER TABLE findings DROP CONSTRAINT findings_unsaved_run_check;
DELETE FROM findings WHERE observation_id IS NULL;
ALTER TABLE findings ALTER COLUMN observation_id SET NOT NULL;
