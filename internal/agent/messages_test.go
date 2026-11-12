package agent

import (
	"errors"
	"fmt"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/guard"
	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestStoredMessagesAreTheSharedFailureMessages(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code revisions.FailureCode
		src  revisions.Source
	}{
		{"no save", ErrNoSave, revisions.FailureNoToolCall, revisions.SourceObserve},
		{"no suggestion", ErrNoSuggestion, revisions.FailureNoToolCall, revisions.SourceSuggest},
		{"no finding", ErrNoFinding, revisions.FailureNoFinding, revisions.SourceInvestigate},
		{"refused", ErrRefused, revisions.FailureRefused, revisions.SourceObserve},
		{"conflict", ErrConflict, revisions.FailureConflict, revisions.SourceObserve},
		{"no progress", ErrNoProgress, revisions.FailureNoProgress, revisions.SourceObserve},
		{"budget", ErrBudgetExhausted, revisions.FailureBudgetExhausted, revisions.SourceObserve},
		{"repair exhausted", ErrRepairExhausted, revisions.FailureValidationExhausted, revisions.SourceObserve},
		{"suggest repair exhausted", ErrSuggestRepairExhausted, revisions.FailureValidationExhausted, revisions.SourceSuggest},
		{"no fixture", llm.ErrNoFixture, revisions.FailureNoFixture, revisions.SourceObserve},
		{"no fixture on a model call", fmt.Errorf("%w: %w", errModelCall, llm.ErrNoFixture), revisions.FailureNoFixture, revisions.SourceObserve},
		{"internal", errors.New("boom"), revisions.FailureInternal, revisions.SourceObserve},
		{"timeout", fmt.Errorf("%w: %w", errModelCall, llm.NewCallError("p", llm.CallTimeout, 0)), revisions.FailureCallTimeout, revisions.SourceObserve},
		{"rate limited", fmt.Errorf("%w: %w", errModelCall, llm.NewCallError("p", llm.RateLimited, 429)), revisions.FailureRateLimited, revisions.SourceObserve},
		{"provider", fmt.Errorf("%w: %w", errModelCall, llm.NewCallError("p", "other", 500)), revisions.FailureProviderError, revisions.SourceObserve},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, want := storedMessage(tc.err), revisions.FailureMessage(tc.code, tc.src); got != want {
				t.Fatalf("stored message = %q, want %q", got, want)
			}
		})
	}
}

func TestOutputLimitErrorKeepsTheSharedMessage(t *testing.T) {
	err := &outputLimitError{base: ErrNoSave}
	if got, want := err.Error(), revisions.FailureMessage(revisions.FailureOutputLimit, revisions.SourceObserve); got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
	suggest := &outputLimitError{base: ErrNoSuggestion}
	if got, want := suggest.Error(), revisions.FailureMessage(revisions.FailureOutputLimit, revisions.SourceSuggest); got != want {
		t.Fatalf("suggest message = %q, want %q", got, want)
	}
}

func TestRepairExhaustedMessageEndsWithTheSharedIssuesSuffix(t *testing.T) {
	codes := []string{"a", "b"}
	got := repairExhausted(tools.ModeCreate, codes).Error()
	want := revisions.FailureMessage(revisions.FailureValidationExhausted, revisions.SourceObserve) + revisions.IssuesSuffix(codes)
	if got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestTheSharedRejectionLimitMatchesTheGuards(t *testing.T) {
	want := fmt.Sprintf("the dashboard still failed validation after %d rejected saves in a row", guard.MaxRejectionsInRow)
	if got := revisions.FailureMessage(revisions.FailureValidationExhausted, revisions.SourceObserve); got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}
