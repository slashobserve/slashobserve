package revisions_test

import (
	"errors"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestAnInvestigationAsOfAPastTimeStoresItAndResolvesItsWindowThere(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	admin := user
	admin.Role = auth.RoleAdmin
	at := time.Date(2026, 11, 20, 10, 30, 0, 0, time.UTC)
	req := revisions.StartRequest{Prompt: text("why did checkout errors rise"), Kind: text(revisions.KindInvestigate), From: text("now-1h"), To: text("now"), AsOf: text("2026-11-20T10:30:00Z")}
	started, err := svc.Start(reading(t.Context()), admin, req)
	if err != nil {
		t.Fatal(err)
	}
	row := started
	if !row.AsOf.Valid || !row.AsOf.Time.Equal(at) {
		t.Fatalf("as_of = %+v, want %s", row.AsOf, at)
	}
	if !row.ContextFrom.Time.Equal(at.Add(-time.Hour)) || !row.ContextTo.Time.Equal(at) {
		t.Fatalf("window = %s to %s, want the hour before %s", row.ContextFrom.Time, row.ContextTo.Time, at)
	}
	if _, err := svc.Cancel(reading(t.Context()), row.ID, admin); err != nil {
		t.Fatal(err)
	}
	retried, err := svc.Retry(reading(t.Context()), row.ID, admin)
	if err != nil {
		t.Fatal(err)
	}
	if !retried.AsOf.Valid || !retried.AsOf.Time.Equal(at) {
		t.Fatalf("the retry's as_of = %+v, want %s", retried.AsOf, at)
	}
}

func TestOnlyAnAdminSetsAsOf(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	req := revisions.StartRequest{Prompt: text("why did checkout errors rise"), Kind: text(revisions.KindInvestigate), AsOf: text("2026-11-20T10:30:00Z")}
	if _, err := svc.Start(reading(t.Context()), user, req); !errors.Is(err, revisions.ErrForbidden) {
		t.Fatalf("an editor's as_of = %v, want %v", err, revisions.ErrForbidden)
	}
	live, err := svc.Start(reading(t.Context()), user, revisions.StartRequest{Prompt: text("why did checkout errors rise"), Kind: text(revisions.KindInvestigate)})
	if err != nil {
		t.Fatal(err)
	}
	if live.AsOf.Valid {
		t.Fatalf("a live run stored as_of %+v", live.AsOf)
	}
}

func TestOnlyAnAdminRetriesAnAsOfRun(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	admin := user
	admin.Role = auth.RoleAdmin
	req := revisions.StartRequest{Prompt: text("why did checkout errors rise"), Kind: text(revisions.KindInvestigate), AsOf: text("2026-11-20T10:30:00Z")}
	started, err := svc.Start(reading(t.Context()), admin, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Cancel(reading(t.Context()), started.ID, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Retry(reading(t.Context()), started.ID, user); !errors.Is(err, revisions.ErrForbidden) {
		t.Fatalf("an editor's retry of an as_of run = %v, want %v", err, revisions.ErrForbidden)
	}
}

func TestAsOfNeedsAnInvestigationNotABuild(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	admin := user
	admin.Role = auth.RoleAdmin
	asOf := text("2026-11-20T10:30:00Z")
	for name, req := range map[string]revisions.StartRequest{
		"a new dashboard": {Prompt: text("a checkout dashboard"), AsOf: asOf},
		"a revision":      {Prompt: text("add a panel"), Kind: text(revisions.KindRevise), DashboardID: text("11111111-1111-4111-8111-111111111111"), AsOf: asOf},
	} {
		_, err := svc.Start(reading(t.Context()), admin, req)
		var invalid *revisions.StartError
		if !errors.As(err, &invalid) {
			t.Errorf("%s: err = %v, want a refusal", name, err)
		}
	}
}

func TestAnAsOfInvestigationWindowEndsAtOrBeforeIt(t *testing.T) {
	svc, _ := newService(t, succeedWith(2))
	admin := user
	admin.Role = auth.RoleAdmin
	req := revisions.StartRequest{Prompt: text("why did checkout errors rise"), Kind: text(revisions.KindInvestigate), From: text("1795170600000"), To: text("1795174200000"), AsOf: text("2026-11-20T10:30:00Z")}
	_, err := svc.Start(reading(t.Context()), admin, req)
	var invalid *revisions.StartError
	if !errors.As(err, &invalid) {
		t.Fatalf("a window ending after as_of: err = %v, want a refusal", err)
	}
}
