-- name: SpendDraftQuery :one
WITH stale AS (
  DELETE FROM draft_query_spend WHERE principal_key = @principal_key AND window_start < @window_start
)
INSERT INTO draft_query_spend (principal_key, window_start, calls)
VALUES (@principal_key, @window_start, 1)
ON CONFLICT (principal_key, window_start) DO UPDATE SET calls = draft_query_spend.calls + 1
WHERE draft_query_spend.calls < @max_calls::int
RETURNING calls;
