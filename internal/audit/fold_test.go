package audit

import (
	"testing"
	"time"
)

func credentialRejected(reason, prefix string) Event {
	return Event{Action: "auth.credential_rejected", Outcome: OutcomeDenied, Reason: reason, Target: Target{Kind: "api_key", Name: prefix}}
}

func TestRefusalsOfOneKindFoldIntoOneEventWithACount(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: time.Hour}, 0)
	for range 3 {
		if err := s.recorder.RecordNow(t.Context(), credentialRejected("unknown_credential", "sok_abcdefgh")); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.recorder.RecordNow(t.Context(), credentialRejected("unknown_credential", "sok_zzzzzzzz")); err != nil {
		t.Fatal(err)
	}
	if err := s.recorder.RecordNow(t.Context(), credentialRejected("malformed_credential", "")); err != nil {
		t.Fatal(err)
	}
	s.run(t)()
	if n := s.count(t, "action = 'auth.credential_rejected'"); n != 3 {
		t.Fatalf("events = %d, want one per reason and key prefix", n)
	}
	if n := s.count(t, "action = 'auth.credential_rejected' AND target_name = 'sok_abcdefgh' AND data = '{\"count\": 3}'"); n != 1 {
		t.Fatalf("events for the repeated key with a count of 3 = %d, want 1", n)
	}
}

func TestAFoldedEventIsWrittenOnceItsWindowCloses(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: 20 * time.Millisecond, fold: 50 * time.Millisecond}, 0)
	stop := s.run(t)
	defer stop()
	if err := s.recorder.RecordNow(t.Context(), credentialRejected("unknown_credential", "sok_abcdefgh")); err != nil {
		t.Fatal(err)
	}
	s.waitForBatch(t)
	if n := s.count(t, "action = 'auth.credential_rejected' AND data = '{\"count\": 1}'"); n != 1 {
		t.Fatalf("events after the window = %d, want 1", n)
	}
}

func TestFoldingKeepsPersonsApart(t *testing.T) {
	f := newFolds(time.Minute, 10)
	s := newStoreRig(t, writerLimits{}, 0)
	first, err := s.recorder.row(t.Context(), Event{Action: "auth.csrf_rejected", Outcome: OutcomeDenied, Reason: "csrf_rejected", By: &By{Actor: ada}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.recorder.row(t.Context(), Event{Action: "auth.csrf_rejected", Outcome: OutcomeDenied, Reason: "csrf_rejected", By: &By{Actor: someone}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, row := range []struct{ absorbed bool }{{f.absorb(first, now)}, {f.absorb(first, now)}, {f.absorb(second, now)}} {
		if !row.absorbed {
			t.Fatal("a refusal was not folded")
		}
	}
	if got := f.due(now); len(got) != 0 {
		t.Fatalf("rows due before the window closes = %d, want 0", len(got))
	}
	if got := f.due(now.Add(time.Minute)); len(got) != 2 {
		t.Fatalf("rows due after the window = %d, want one per person", len(got))
	}
}

func TestFoldingIsBoundedByAnOverflowBucket(t *testing.T) {
	f := newFolds(time.Minute, 2)
	s := newStoreRig(t, writerLimits{}, 0)
	now := time.Now()
	for _, prefix := range []string{"sok_aaaaaaaa", "sok_bbbbbbbb", "sok_cccccccc", "sok_dddddddd"} {
		row, err := s.recorder.row(t.Context(), credentialRejected("unknown_credential", prefix))
		if err != nil {
			t.Fatal(err)
		}
		f.absorb(row, now)
	}
	if got := f.due(now.Add(time.Minute)); len(got) != 3 {
		t.Fatalf("folded events = %d, want 2 keyed and 1 overflow", len(got))
	}
}

func TestAnActionThatIsNotAFailureIsNotFolded(t *testing.T) {
	f := newFolds(time.Minute, 10)
	s := newStoreRig(t, writerLimits{}, 0)
	row, err := s.recorder.row(t.Context(), Event{Action: "dashboard.view"})
	if err != nil {
		t.Fatal(err)
	}
	if f.absorb(row, time.Now()) {
		t.Fatal("a dashboard view was folded")
	}
}
