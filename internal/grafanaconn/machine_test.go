package grafanaconn

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func TestSyncMachineIsConsistent(t *testing.T) {
	if err := SyncMachine.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestSyncMachineMoves(t *testing.T) {
	if got := SyncMachine.Offered(); !slices.Equal(got, []SyncAction{ActionCancel}) {
		t.Fatalf("offered %v, want [cancel]", got)
	}
	if got := SyncMachine.From(ActionClaim); !slices.Equal(got, []string{"running"}) {
		t.Fatalf("claim leaves %v, want [running]", got)
	}
	events := []struct {
		action SyncAction
		to     SyncState
		want   string
	}{
		{ActionStart, SyncRunning, "grafana sync started"},
		{ActionClaim, SyncRunning, "grafana sync resumed"},
		{ActionRelease, SyncRunning, "grafana sync released"},
		{ActionFinish, SyncFinished, "grafana sync finished"},
		{ActionFail, SyncFailed, "grafana sync failed"},
		{ActionExhaust, SyncFailed, "grafana sync lease expired twice"},
		{ActionCancel, SyncCanceled, "grafana sync canceled"},
	}
	for _, e := range events {
		if got := SyncMachine.EventFor(e.action, e.to); got != e.want {
			t.Errorf("EventFor(%s, %s) = %q, want %q", e.action, e.to, got, e.want)
		}
	}
}

func TestSyncActions(t *testing.T) {
	admin := auth.Principal{Role: auth.RoleAdmin}
	editor := auth.Principal{Role: auth.RoleEditor}
	cases := []struct {
		name   string
		state  SyncState
		caller auth.Principal
		want   []string
	}{
		{"an admin may cancel a running import", SyncRunning, admin, []string{"cancel"}},
		{"an editor may do nothing to a running import", SyncRunning, editor, []string{}},
		{"a finished import offers nothing", SyncFinished, admin, []string{}},
		{"a failed import offers nothing", SyncFailed, admin, []string{}},
		{"a canceled import offers nothing", SyncCanceled, admin, []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := syncActions(c.state, c.caller)
			if !slices.Equal(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if len(c.want) == 0 && string(raw) != "[]" {
				t.Fatalf("marshals to %s, want []", raw)
			}
		})
	}
}

func TestUniqueHitsKeepsTheFirstOfEachUID(t *testing.T) {
	got := uniqueHits([]apiHit{{UID: "a", Title: "first"}, {UID: "b"}, {UID: "a", Title: "second"}, {UID: "c"}})
	want := []apiHit{{UID: "a", Title: "first"}, {UID: "b"}, {UID: "c"}}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].UID != want[i].UID || got[i].Title != want[i].Title {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	}
	if got := uniqueHits(nil); len(got) != 0 {
		t.Fatalf("nil gave %+v", got)
	}
}

func TestRemainingHitsDropsTheRecordedOnes(t *testing.T) {
	hits := []apiHit{{UID: "a"}, {UID: "b"}, {UID: "c"}}
	got := remainingHits(hits, map[string]bool{"b": true})
	if len(got) != 2 || got[0].UID != "a" || got[1].UID != "c" {
		t.Fatalf("got %+v, want a and c", got)
	}
	if got := remainingHits(nil, nil); len(got) != 0 {
		t.Fatalf("nil gave %+v", got)
	}
	if got := remainingHits(hits, nil); len(got) != 3 {
		t.Fatalf("no recorded set gave %+v", got)
	}
}
