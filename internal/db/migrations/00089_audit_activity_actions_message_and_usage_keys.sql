-- +goose Up
UPDATE audit_activity_actions SET keys = keys || '{anchor_kind,body_length}' WHERE type IN ('thread.opened', 'thread.replied');
UPDATE audit_activity_actions SET keys = keys || '{model,input_tokens,output_tokens}' WHERE type IN ('run.succeeded', 'run.failed', 'run.canceled');

-- +goose Down
UPDATE audit_activity_actions SET keys = array_remove(array_remove(keys, 'anchor_kind'), 'body_length') WHERE type IN ('thread.opened', 'thread.replied');
UPDATE audit_activity_actions SET keys = array_remove(array_remove(array_remove(keys, 'model'), 'input_tokens'), 'output_tokens') WHERE type IN ('run.succeeded', 'run.failed', 'run.canceled');
