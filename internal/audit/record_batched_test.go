package audit

import (
	"testing"
	"time"
)

func TestADeniedChangeIsBatchedLikeTheMiddlewareBatchesIt(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	denied := Event{Action: "oauth.token", Outcome: OutcomeDenied, Reason: "invalid_grant"}
	if err := s.recorder.RecordNow(t.Context(), denied); err != nil {
		t.Fatal(err)
	}
	if n := s.count(t, "action = 'oauth.token'"); n != 0 {
		t.Fatalf("a denied change was written on the request path: %d rows before the writer ran", n)
	}
	s.run(t)()
	if n := s.count(t, "action = 'oauth.token' AND outcome = 'denied'"); n != 1 {
		t.Fatalf("denied change rows after the writer ran = %d, want 1", n)
	}
}

func TestASuccessfulChangeIsStillWrittenOnTheRequestPath(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	if err := s.recorder.RecordNow(t.Context(), Event{Action: "oauth.token"}); err != nil {
		t.Fatal(err)
	}
	if n := s.count(t, "action = 'oauth.token' AND outcome = 'success'"); n != 1 {
		t.Fatalf("a successful change was not written at once: %d rows", n)
	}
}
