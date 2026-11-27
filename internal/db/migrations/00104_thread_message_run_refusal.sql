-- +goose Up
ALTER TABLE thread_messages ADD COLUMN run_refusal_code text, ADD COLUMN run_refusal_message text;

-- +goose Down
ALTER TABLE thread_messages DROP COLUMN run_refusal_message, DROP COLUMN run_refusal_code;
