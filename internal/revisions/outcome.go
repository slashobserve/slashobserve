package revisions

import (
	"bytes"
	"context"
	"encoding/json"
	"time"
)

const (
	finishTimeout = 15 * time.Second

	maxCheckFailures = 3

	requesterGoneMessage = "the person or key that asked for this run no longer exists"

	noSuggestionMessage = "the agent did not propose a panel"
	noFindingMessage    = "the agent finished without saving a finding"
)

type admissionCheck string

const (
	checkSavedResult     admissionCheck = "could not check the request's saved version"
	checkRequester       admissionCheck = "could not load the person or key that asked for this run"
	checkRequesterAccess admissionCheck = "could not check that the requester may still run this request"
)

func checkFailedOutcome(check admissionCheck, cause error) outcome {
	return outcome{message: string(check), code: "agent_request_check_failed", failure: FailureCheckFailed, cause: cause}
}

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
