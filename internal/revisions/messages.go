package revisions

import (
	"fmt"
	"strings"
)

const (
	maxRejectionsInRow = 3

	messageNoDashboard  = "the model finished without saving a dashboard"
	messageNoProposal   = "the model finished without proposing a panel"
	messageNoFindingRun = "the model finished without saving a finding"

	OutputLimitSuffix = " after reaching max tokens"
)

var failureMessages = map[FailureCode]string{
	FailureCallTimeout:     "the model did not answer in time",
	FailureRateLimited:     "the model provider is rate limiting requests",
	FailureProviderError:   "the model provider returned an error",
	FailureRefused:         "the model declined the request",
	FailureNoFixture:       "model call failed: no replay fixture",
	FailureInternal:        "the agent run failed",
	FailureConflict:        "the dashboard changed twice while the agent was revising it",
	FailureNotFound:        "the dashboard no longer exists",
	FailureNoProgress:      "the agent stopped making progress",
	FailureBudgetExhausted: "the run reached the server's cost limit",
}

func noResultMessage(source Source) string {
	switch source {
	case SourceSuggest:
		return messageNoProposal
	case SourceInvestigate:
		return messageNoFindingRun
	}
	return messageNoDashboard
}

func FailureMessage(code FailureCode, source Source) string {
	switch code {
	case FailureNoToolCall:
		return noResultMessage(source)
	case FailureNoFinding:
		return messageNoFindingRun
	case FailureOutputLimit:
		return noResultMessage(source) + OutputLimitSuffix
	case FailureValidationExhausted:
		if source == SourceSuggest {
			return fmt.Sprintf("the proposed panel still failed validation after %d rejected saves in a row", maxRejectionsInRow)
		}
		return fmt.Sprintf("the dashboard still failed validation after %d rejected saves in a row", maxRejectionsInRow)
	}
	return failureMessages[code]
}

func IssuesSuffix(codes []string) string {
	if len(codes) == 0 {
		return ""
	}
	return " (last issues: " + strings.Join(codes, ", ") + ")"
}
