package revisions

import (
	"bytes"
	"context"
	"encoding/json"
	"time"
)

const (
	finishTimeout = 15 * time.Second

	maxCheckFailures   = 3
	checkFailedMessage = "could not check the request's saved version"

	requesterGoneMessage = "the person or key that asked for this run no longer exists"

	noSuggestionMessage = "the agent did not propose a panel"
	noFindingMessage    = "the agent finished without saving a finding"
)

func canceledOutcome(cause error, usage *Usage) outcome {
	return outcome{message: CanceledMessage, code: "agent_run_canceled", failure: FailureCanceled, cause: cause, usage: usage}
}

func suggestionOutcome(suggestion json.RawMessage, usage *Usage) outcome {
	proposed := bytes.TrimSpace(suggestion)
	if len(proposed) == 0 || proposed[0] != '{' || !json.Valid(proposed) {
		return outcome{message: noSuggestionMessage, code: "agent_no_suggestion", failure: FailureNoSuggestion, usage: usage}
	}
	return outcome{suggestion: proposed, usage: usage}
}

func finishContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
}
