-- +goose Up
ALTER TABLE findings
  ADD COLUMN root_cause_component TEXT,
  ADD COLUMN root_cause_kind TEXT,
  ADD COLUMN root_cause_summary TEXT,
  ADD COLUMN onset TIMESTAMPTZ,
  ADD COLUMN impact TEXT,
  ADD COLUMN confidence TEXT,
  ADD COLUMN ruled_out TEXT[],
  ADD COLUMN next_steps TEXT[],
  ADD COLUMN no_incident BOOLEAN;

CREATE INDEX findings_root_cause_component_search_idx ON findings
  USING GIN (search_vector(root_cause_component, '', '', ''))
  WHERE root_cause_component IS NOT NULL;

-- +goose Down
DROP INDEX findings_root_cause_component_search_idx;
ALTER TABLE findings
  DROP COLUMN no_incident,
  DROP COLUMN next_steps,
  DROP COLUMN ruled_out,
  DROP COLUMN confidence,
  DROP COLUMN impact,
  DROP COLUMN onset,
  DROP COLUMN root_cause_summary,
  DROP COLUMN root_cause_kind,
  DROP COLUMN root_cause_component;
