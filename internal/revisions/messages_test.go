package revisions

import "testing"

func TestFailureMessageReadsPerCodeAndSource(t *testing.T) {
	cases := []struct {
		code   FailureCode
		source Source
		want   string
	}{
		{FailureCallTimeout, SourceObserve, "the model did not answer in time"},
		{FailureRateLimited, SourceObserve, "the model provider is rate limiting requests"},
		{FailureProviderError, SourceObserve, "the model provider returned an error"},
		{FailureRefused, SourceObserve, "the model declined the request"},
		{FailureNoFixture, SourceObserve, "no replay fixture"},
		{FailureInternal, SourceObserve, "the agent run failed"},
		{FailureConflict, SourceObserve, "the dashboard changed twice while the agent was revising it"},
		{FailureNotFound, SourceObserve, "the dashboard no longer exists"},
		{FailureNoProgress, SourceObserve, "the agent stopped making progress"},
		{FailureBudgetExhausted, SourceObserve, "the run reached the server's cost limit"},
		{FailureNoToolCall, SourceObserve, "the model finished without saving a dashboard"},
		{FailureNoToolCall, SourceSuggest, "the model finished without proposing a panel"},
		{FailureNoToolCall, SourceInvestigate, "the model finished without saving a finding"},
		{FailureNoFinding, SourceInvestigate, "the model finished without saving a finding"},
		{FailureOutputLimit, SourceObserve, "the model finished without saving a dashboard after reaching max tokens"},
		{FailureOutputLimit, SourceSuggest, "the model finished without proposing a panel after reaching max tokens"},
		{FailureValidationExhausted, SourceObserve, "the dashboard still failed validation after 3 rejected saves in a row"},
		{FailureValidationExhausted, SourceSuggest, "the proposed panel still failed validation after 3 rejected saves in a row"},
	}
	for _, tc := range cases {
		if got := FailureMessage(tc.code, tc.source); got != tc.want {
			t.Errorf("FailureMessage(%s, %s) = %q, want %q", tc.code, tc.source, got, tc.want)
		}
	}
}

func TestFailureMessageIsEmptyForACodeWithoutOne(t *testing.T) {
	if got := FailureMessage(FailureCanceled, SourceObserve); got != "" {
		t.Fatalf("message = %q", got)
	}
}

func TestIssuesSuffix(t *testing.T) {
	if got := IssuesSuffix(nil); got != "" {
		t.Errorf("empty = %q", got)
	}
	if got := IssuesSuffix([]string{"a", "b"}); got != " (last issues: a, b)" {
		t.Errorf("suffix = %q", got)
	}
}
