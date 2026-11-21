package audit

import (
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func credentialRejected(reason, prefix string) Event {
	return Event{Action: "auth.credential_rejected", Outcome: OutcomeDenied, Reason: reason, Target: Target{Kind: "api_key", Name: prefix}}
}

func TestTheFirstRefusalOfAKeyIsWrittenAtOnceAndTheRestFoldIntoOneLaterEvent(t *testing.T) {
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
	if n := s.count(t, "action = 'auth.credential_rejected'"); n != 4 {
		t.Fatalf("events = %d, want the 3 first refusals and one later row for the repeated key", n)
	}
	if n := s.count(t, "action = 'auth.credential_rejected' AND target_name = 'sok_abcdefgh' AND data = '{\"count\": 1}'"); n != 1 {
		t.Fatalf("first events of the repeated key with a count of 1 = %d, want 1", n)
	}
	if n := s.count(t, "action = 'auth.credential_rejected' AND target_name = 'sok_abcdefgh' AND data = '{\"count\": 2}'"); n != 1 {
		t.Fatalf("later events of the repeated key with the remaining count of 2 = %d, want 1", n)
	}
}

func TestARefusalThatIsAloneInItsWindowIsWrittenOnceAndNeverAgain(t *testing.T) {
	s := newStoreRig(t, writerLimits{every: 20 * time.Millisecond, fold: 50 * time.Millisecond}, 0)
	stop := s.run(t)
	if err := s.recorder.RecordNow(t.Context(), credentialRejected("unknown_credential", "sok_abcdefgh")); err != nil {
		t.Fatal(err)
	}
	s.waitForBatch(t)
	stop()
	if n := s.count(t, "action = 'auth.credential_rejected' AND data = '{\"count\": 1}'"); n != 1 {
		t.Fatalf("events = %d, want 1 and no later row", n)
	}
	if n := s.count(t, "action = 'auth.credential_rejected'"); n != 1 {
		t.Fatalf("events = %d, want 1 and no later row", n)
	}
}

func TestEveryRefusalIsCountedOnceWhateverTheirOrderAcrossGoroutines(t *testing.T) {
	const goroutines, each = 8, 250
	f := newFolds(time.Minute, 10)
	s := newStoreRig(t, writerLimits{}, 0)
	row, err := s.recorder.row(t.Context(), credentialRejected("unknown_credential", "sok_abcdefgh"))
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var written []dbsqlc.InsertAuditEventParams
	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			for range each {
				if first, enqueue := f.absorb(row, time.Now()); enqueue {
					mu.Lock()
					written = append(written, first)
					mu.Unlock()
				}
			}
		})
	}
	wg.Wait()
	written = append(written, f.drain()...)
	if len(written) != 2 {
		t.Fatalf("rows = %d, want the first refusal and one later row", len(written))
	}
	if got := weigh(written); got != goroutines*each {
		t.Fatalf("refusals counted across the rows = %d, want %d", got, goroutines*each)
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
	var written int
	for _, row := range []dbsqlc.InsertAuditEventParams{first, first, second} {
		if _, enqueue := f.absorb(row, now); enqueue {
			written++
		}
	}
	if written != 2 {
		t.Fatalf("rows written at once = %d, want the first refusal of each person", written)
	}
	if got := f.due(now); len(got) != 0 {
		t.Fatalf("rows due before the window closes = %d, want 0", len(got))
	}
	got := f.due(now.Add(time.Minute))
	if len(got) != 1 || weightOf(got[0]) != 1 {
		t.Fatalf("rows due after the window = %d, want one later row for the person who was refused twice", len(got))
	}
}

func TestFoldingIsBoundedByAnOverflowBucket(t *testing.T) {
	f := newFolds(time.Minute, 2)
	s := newStoreRig(t, writerLimits{}, 0)
	now := time.Now()
	var written []dbsqlc.InsertAuditEventParams
	for _, prefix := range []string{"sok_aaaaaaaa", "sok_bbbbbbbb", "sok_cccccccc", "sok_dddddddd"} {
		row, err := s.recorder.row(t.Context(), credentialRejected("unknown_credential", prefix))
		if err != nil {
			t.Fatal(err)
		}
		if first, enqueue := f.absorb(row, now); enqueue {
			written = append(written, first)
		}
	}
	written = append(written, f.due(now.Add(time.Minute))...)
	if len(written) != 4 || weigh(written) != 4 {
		t.Fatalf("rows = %d standing for %d refusals, want 4 for 4: 2 keyed, the first overflow refusal and a later overflow row", len(written), weigh(written))
	}
}

func TestAnActionThatIsNotAFailureIsNotFolded(t *testing.T) {
	f := newFolds(time.Minute, 10)
	s := newStoreRig(t, writerLimits{}, 0)
	row, err := s.recorder.row(t.Context(), Event{Action: "dashboard.view"})
	if err != nil {
		t.Fatal(err)
	}
	if _, enqueue := f.absorb(row, time.Now()); !enqueue || len(f.open) != 0 {
		t.Fatal("a dashboard view was folded")
	}
}
