package revisions

import (
	"fmt"
	"strings"
)

const (
	maxRejectionsInRow    = 3
	maxFailingPanelsShown = 5

	messageNoDashboard  = "the model finished without saving a dashboard"
	messageNoProposal   = "the model finished without proposing a panel"
	messageNoFindingRun = "the model finished without saving a finding"

	OutputLimitSuffix = " after reaching max tokens"
)

var failureMessages = map[FailureCode]string{
	FailureCallTimeout:      "the model did not answer in time",
	FailureRateLimited:      "the model provider is rate limiting requests",
	FailureProviderError:    "the model provider returned an error",
	FailureRefused:          "the model declined the request",
	FailureNoFixture:        "model call failed: no replay fixture",
	FailureInternal:         "the agent run failed",
	FailureConflict:         "the dashboard changed twice while the agent was revising it",
	FailureNotFound:         "the dashboard no longer exists",
	FailureNoProgress:       "the agent stopped making progress",
	FailureBudgetExhausted:  "the run reached the server's cost limit",
	FailureRestrictedAnswer: "the run read data only Admins can read, so what it saved cannot go where people who are not Admins read",
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
		if source == SourceInvestigate {
			return fmt.Sprintf("the finding was rejected in %d saves in a row", maxRejectionsInRow)
		}
		if source == SourceSuggest {
			return fmt.Sprintf("the proposed panel still failed validation after %d rejected saves in a row", maxRejectionsInRow)
		}
		return fmt.Sprintf("the dashboard still failed validation after %d rejected saves in a row", maxRejectionsInRow)
	}
	return failureMessages[code]
}

type Rejection struct {
	Finding   bool
	Transient bool
	Panels    []string
	Other     int
	Codes     []string
}

func ValidationExhaustedMessage(source Source, r Rejection) string {
	if r.Finding || source == SourceInvestigate {
		return findingRejectedMessage(r)
	}
	return FailureMessage(FailureValidationExhausted, source) + FailingPanelsSuffix(r.Panels, r.Other) + IssuesSuffix(r.Codes)
}

func findingRejectedMessage(r Rejection) string {
	message := fmt.Sprintf("the finding was rejected in %d saves in a row", maxRejectionsInRow)
	if total := len(r.Panels) + r.Other; total > 0 {
		shown := r.Panels[:min(len(r.Panels), maxFailingPanelsShown)]
		message += "; its evidence that failed: " + strings.Join(shown, ", ")
		if more := total - len(shown); more > 0 {
			message += fmt.Sprintf(" and %d more", more)
		}
	}
	message += IssuesSuffix(r.Codes)
	if r.Transient {
		message += "; a datasource did not answer in time, so ask again when it is healthy"
	}
	return message
}

func FailingPanelsSuffix(ids []string, other int) string {
	total := len(ids) + other
	if total == 0 {
		return ""
	}
	shown := ids[:min(len(ids), maxFailingPanelsShown)]
	suffix := "; still failing: " + strings.Join(shown, ", ")
	if more := total - len(shown); more > 0 && len(shown) > 0 {
		suffix += fmt.Sprintf(" and %d more", more)
	} else if more > 0 {
		suffix += fmt.Sprintf("%d more", more)
	}
	if total > 1 {
		suffix += "; ask the agent to fix them one panel at a time"
	}
	return suffix
}

func IssuesSuffix(codes []string) string {
	if len(codes) == 0 {
		return ""
	}
	return " (last issues: " + strings.Join(codes, ", ") + ")"
}
