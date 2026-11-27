package revisions

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func TestCanceledOutcomeIsAFailureWithTheCanceledCode(t *testing.T) {
	o := canceledOutcome(nil, nil)
	if o.message != CanceledMessage || o.failure != FailureCanceled || o.code != "agent_run_canceled" || o.version != nil {
		t.Fatalf("canceledOutcome = %+v", o)
	}
}

func TestTheThirdFailedCheckIsTheOneThatFailsTheRun(t *testing.T) {
	for failures := int32(0); failures < maxCheckFailures; failures++ {
		got := checksExhausted(dbsqlc.AgentRequest{CheckFailures: failures})
		if want := failures == maxCheckFailures-1; got != want {
			t.Errorf("checksExhausted with %d earlier failures = %v, want %v", failures, got, want)
		}
	}
}
