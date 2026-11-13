package revisions

import "testing"

func TestCanceledOutcomeIsAFailureWithTheCanceledCode(t *testing.T) {
	o := canceledOutcome(nil, nil)
	if o.message != CanceledMessage || o.failure != FailureCanceled || o.code != "agent_run_canceled" || o.version != nil {
		t.Fatalf("canceledOutcome = %+v", o)
	}
}
