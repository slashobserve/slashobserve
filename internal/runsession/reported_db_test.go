package runsession

import (
	"context"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestFinishTakesTheFailuresOfAnAgentsOwnGuardsWithItsMessage(t *testing.T) {
	cases := []struct {
		code    revisions.FailureCode
		message string
	}{
		{revisions.FailureNoProgress, "Stopped: 2 turns in a row had every call refused"},
		{revisions.FailureRestrictedAnswer, ""},
		{revisions.FailureValidationExhausted, "  the finding was still over the word limit after 5 saves\x00"},
	}
	for _, tc := range cases {
		r := newRig(t)
		req := r.revise()
		if _, err := r.svc.Finish(context.Background(), req, tools.RunFinish{Outcome: outcomeFailed, FailureCode: string(tc.code), Message: tc.message}); err != nil {
			t.Fatalf("%s: %v", tc.code, err)
		}
		row := r.stored(req)
		want := map[revisions.FailureCode]string{
			revisions.FailureNoProgress:          tc.message,
			revisions.FailureRestrictedAnswer:    revisions.FailureMessage(revisions.FailureRestrictedAnswer, req.Source),
			revisions.FailureValidationExhausted: "the finding was still over the word limit after 5 saves",
		}[tc.code]
		if row.State != string(revisions.StateFailed) || row.FailureCode.String != string(tc.code) || row.Error.String != want {
			t.Errorf("%s: run %s %q, message %q, want %q", tc.code, row.State, row.FailureCode.String, row.Error.String, want)
		}
	}
}
