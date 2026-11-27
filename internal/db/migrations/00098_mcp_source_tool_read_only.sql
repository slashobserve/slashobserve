-- +goose Up
ALTER TABLE mcp_source_tools ADD COLUMN read_only BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE mcp_source_tools ADD COLUMN seen_read_only BOOLEAN;
UPDATE mcp_source_tools SET read_only = true
WHERE state <> 'refused' OR refusal IN ('The description is longer than 4000 characters.', 'The input schema is larger than 32 KiB.');
UPDATE mcp_source_tools SET seen_read_only = true WHERE seen_hash IS NOT NULL;

-- +goose Down
UPDATE mcp_source_tools SET state = 'refused', refusal = 'The server does not mark this tool as read-only.',
  seen_description = NULL, seen_input_schema = NULL, seen_hash = NULL, approved_by = NULL, approved_at = NULL
WHERE state <> 'refused' AND (NOT read_only OR seen_read_only IS FALSE);
ALTER TABLE mcp_source_tools DROP COLUMN seen_read_only;
ALTER TABLE mcp_source_tools DROP COLUMN read_only;
