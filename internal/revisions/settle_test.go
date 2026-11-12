package revisions

import "testing"

func TestSettlementForChoosesTheEndingByPrecedence(t *testing.T) {
	cases := []struct {
		name         string
		run          runEnd
		shuttingDown bool
		crashed      bool
		want         settlement
	}{
		{"plain failure", runEnd{}, false, false, settleFailed},
		{"crash", runEnd{}, false, true, settleFailed},
		{"timeout", runEnd{timedOut: true}, false, false, settleTimedOut},
		{"timeout and crash", runEnd{timedOut: true}, false, true, settleTimedOut},
		{"shutdown", runEnd{}, true, false, settleRelease},
		{"shutdown and crash", runEnd{}, true, true, settleRelease},
		{"shutdown and timeout", runEnd{timedOut: true}, true, false, settleRelease},
		{"shutdown, timeout and crash", runEnd{timedOut: true}, true, true, settleRelease},
		{"cancel", runEnd{canceled: true}, false, false, settleCanceled},
		{"cancel and crash", runEnd{canceled: true}, false, true, settleFailed},
		{"cancel and timeout", runEnd{canceled: true, timedOut: true}, false, false, settleCanceled},
		{"cancel, timeout and crash", runEnd{canceled: true, timedOut: true}, false, true, settleTimedOut},
		{"cancel and shutdown", runEnd{canceled: true}, true, false, settleCanceled},
		{"cancel, shutdown and crash", runEnd{canceled: true}, true, true, settleRelease},
		{"cancel, shutdown and timeout", runEnd{canceled: true, timedOut: true}, true, false, settleCanceled},
		{"cancel, shutdown, timeout and crash", runEnd{canceled: true, timedOut: true}, true, true, settleRelease},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := settlementFor(tc.run, tc.shuttingDown, tc.crashed); got != tc.want {
				t.Fatalf("settlementFor(%+v, shuttingDown=%t, crashed=%t) = %d, want %d", tc.run, tc.shuttingDown, tc.crashed, got, tc.want)
			}
		})
	}
}

func TestCanceledOutcomeIsAFailureWithTheCanceledCode(t *testing.T) {
	o := canceledOutcome(nil, nil)
	if o.message != CanceledMessage || o.failure != FailureCanceled || o.code != "agent_run_canceled" || o.version != nil {
		t.Fatalf("canceledOutcome = %+v", o)
	}
}

func TestAnInvestigationSucceedsOnlyWithAFinding(t *testing.T) {
	if o := successOutcome(SourceInvestigate, Result{}); o.finding || o.failure != FailureNoFinding || o.message != noFindingMessage {
		t.Errorf("without a finding = %+v", o)
	}
	if o := successOutcome(SourceInvestigate, Result{FindingID: "f1"}); !o.finding || o.version != nil || o.failure != "" {
		t.Errorf("with a finding = %+v", o)
	}
	if o := successOutcome(SourcePrompt, Result{Version: 3}); o.finding || o.version == nil || *o.version != 3 {
		t.Errorf("a build = %+v", o)
	}
	if o := successOutcome(SourceObserve, Result{FindingID: "f1"}); !o.finding || o.version != nil || o.failure != "" {
		t.Errorf("an answered new-dashboard request = %+v", o)
	}
	if o := successOutcome(SourcePrompt, Result{FindingID: "f1"}); !o.finding || o.version != nil || o.failure != "" {
		t.Errorf("an answered dashboard prompt = %+v", o)
	}
	if o := successOutcome(SourceObserve, Result{Version: 1}); o.finding || o.version == nil || *o.version != 1 {
		t.Errorf("a new dashboard = %+v", o)
	}
}

func TestAResultWithAProposalSucceedsAsAProposalAndNeverAsAVersion(t *testing.T) {
	if o := successOutcome(SourcePrompt, Result{ProposalID: "p1"}); !o.proposal || o.version != nil || o.finding || o.failure != "" {
		t.Errorf("a proposed dashboard prompt = %+v", o)
	}
	if o := successOutcome(SourceComment, Result{ProposalID: "p1", Version: 0}); !o.proposal || o.version != nil {
		t.Errorf("a proposed comment = %+v", o)
	}
}
