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
		{FailureNoFixture, SourceObserve, "model call failed: no replay fixture"},
		{FailureInternal, SourceObserve, "the agent run failed"},
		{FailureConflict, SourceObserve, "the dashboard changed twice while the agent was revising it"},
		{FailureNotFound, SourceObserve, "the dashboard no longer exists"},
		{FailureNoProgress, SourceObserve, "the agent stopped making progress"},
		{FailureBudgetExhausted, SourceObserve, "the run reached the server's cost limit"},
		{FailureRestrictedAnswer, SourceObserve, "the run read data only Admins can read, so what it saved cannot go where people who are not Admins read"},
		{FailureNoToolCall, SourceObserve, "the model finished without saving a dashboard"},
		{FailureNoToolCall, SourceSuggest, "the model finished without proposing a panel"},
		{FailureNoToolCall, SourceInvestigate, "the model finished without saving a finding"},
		{FailureNoFinding, SourceInvestigate, "the model finished without saving a finding"},
		{FailureOutputLimit, SourceObserve, "the model finished without saving a dashboard after reaching max tokens"},
		{FailureOutputLimit, SourceSuggest, "the model finished without proposing a panel after reaching max tokens"},
		{FailureValidationExhausted, SourceObserve, "the dashboard still failed validation after 3 rejected saves in a row"},
		{FailureValidationExhausted, SourceSuggest, "the proposed panel still failed validation after 3 rejected saves in a row"},
		{FailureValidationExhausted, SourceInvestigate, "the finding was rejected in 3 saves in a row"},
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

func TestFailingPanelsSuffix(t *testing.T) {
	cases := []struct {
		name  string
		ids   []string
		other int
		want  string
	}{
		{"none", nil, 0, ""},
		{"only ids that are no panel ids", nil, 2, "; still failing: 2 more; ask the agent to fix them one panel at a time"},
		{"one id and one that is no panel id", []string{"p1"}, 1, "; still failing: p1 and 1 more; ask the agent to fix them one panel at a time"},
		{"one", []string{"p1"}, 0, "; still failing: p1"},
		{"two", []string{"p1", "p2"}, 0, "; still failing: p1, p2; ask the agent to fix them one panel at a time"},
		{"seven", []string{"a", "b", "c", "d", "e", "f", "g"}, 0, "; still failing: a, b, c, d, e and 2 more; ask the agent to fix them one panel at a time"},
	}
	for _, tc := range cases {
		if got := FailingPanelsSuffix(tc.ids, tc.other); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestValidationExhaustedMessageFitsWhatWasRejected(t *testing.T) {
	cases := []struct {
		name   string
		source Source
		r      Rejection
		want   string
	}{
		{"dashboard", SourceObserve, Rejection{Panels: []string{"p1"}, Codes: []string{"query_empty"}}, "the dashboard still failed validation after 3 rejected saves in a row; still failing: p1 (last issues: query_empty)"},
		{"investigation with timed out evidence", SourceInvestigate, Rejection{Finding: true, Transient: true, Panels: []string{"evidence-1", "evidence-3"}, Codes: []string{"query_timeout"}}, "the finding was rejected in 3 saves in a row; its evidence that failed: evidence-1, evidence-3 (last issues: query_timeout); a datasource did not answer in time, so ask again when it is healthy"},
		{"question run with a finding", SourceObserve, Rejection{Finding: true, Codes: []string{"finding_too_long"}}, "the finding was rejected in 3 saves in a row (last issues: finding_too_long)"},
		{"investigation with unfixable evidence", SourceInvestigate, Rejection{Panels: []string{"a", "b", "c", "d", "e"}, Other: 2, Codes: []string{"query_invalid"}}, "the finding was rejected in 3 saves in a row; its evidence that failed: a, b, c, d, e and 2 more (last issues: query_invalid)"},
	}
	for _, tc := range cases {
		if got := ValidationExhaustedMessage(tc.source, tc.r); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestARunThatWroteIsNeverReportedAsEndingWithoutAResult(t *testing.T) {
	cases := []struct {
		name   string
		ending Ending
		want   string
	}{
		{"no call, wrote", Ending{Failure: FailureNoToolCall, Message: messageNoDashboard, Wrote: true}, messageWroteUnanswered},
		{"no finding, wrote", Ending{Failure: FailureNoFinding, Message: messageNoFindingRun, Wrote: true}, messageWroteUnanswered},
		{"no call, nothing written", Ending{Failure: FailureNoToolCall, Message: messageNoDashboard}, messageNoDashboard},
		{"another failure, wrote", Ending{Failure: FailureRateLimited, Message: "the model provider is rate limiting requests", Wrote: true}, "the model provider is rate limiting requests"},
	}
	for _, c := range cases {
		if got := c.ending.failureMessage(); got != c.want {
			t.Errorf("%s: message = %q, want %q", c.name, got, c.want)
		}
	}
	if got := (Ending{Wrote: true}).unansweredMessage(noSaveMessage); got != messageWroteUnanswered {
		t.Errorf("a committed run that wrote and saved nothing reads %q", got)
	}
	if got := (Ending{}).unansweredMessage(noSaveMessage); got != noSaveMessage {
		t.Errorf("a run that wrote nothing reads %q", got)
	}
}
