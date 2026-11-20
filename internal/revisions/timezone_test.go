package revisions_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestCheckTimeZone(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"", "", true},
		{"UTC", "", true},
		{" Asia/Kolkata ", "Asia/Kolkata", true},
		{"America/New_York", "America/New_York", true},
		{"Local", "", false},
		{"Mars/Olympus", "", false},
		{"../etc/passwd", "", false},
	}
	for _, tt := range tests {
		got, err := revisions.CheckTimeZone(tt.in)
		if got != tt.want || (err == nil) != tt.ok {
			t.Errorf("CheckTimeZone(%q) = %q, %v", tt.in, got, err)
		}
	}
}

func TestEveryRunKeepsThePersonsTimeZone(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	dash := seedDashboard(t, d, 1)
	ctx := revisions.WithTimeZone(reading(context.Background()), "Asia/Kolkata")
	observe, err := svc.SubmitObserve(ctx, user, "which dashboards show checkout")
	if err != nil {
		t.Fatal(err)
	}
	followUp, err := svc.SubmitFollowUp(ctx, dash, nil, "", "rename it", user)
	if err != nil {
		t.Fatal(err)
	}
	investigation, err := svc.SubmitInvestigate(ctx, revisions.InvestigateParams{Prompt: "why is checkout slow", By: user})
	if err != nil {
		t.Fatal(err)
	}
	for name, row := range map[string]string{"observe": observe.TimeZone.String, "follow-up": followUp.TimeZone.String, "investigate": investigation.TimeZone.String} {
		if row != "Asia/Kolkata" {
			t.Errorf("%s run stored zone %q", name, row)
		}
	}
	if got := revisions.RequestOf(observe).TimeZone; got != "Asia/Kolkata" {
		t.Fatalf("request zone = %q", got)
	}
	if _, err := svc.Cancel(reading(context.Background()), observe.ID, user); err != nil {
		t.Fatal(err)
	}
	retried, err := svc.Retry(reading(context.Background()), observe.ID, user)
	if err != nil || retried.TimeZone.String != "Asia/Kolkata" {
		t.Fatalf("retry = %+v, %v; want the zone kept", retried, err)
	}
	plain, err := svc.SubmitObserve(reading(context.Background()), user, "list dashboards")
	if err != nil || plain.TimeZone.Valid {
		t.Fatalf("a run without a zone = %+v, %v", plain, err)
	}
}

func TestObserveTakesTheTimeZoneThePersonReads(t *testing.T) {
	svc, d := newService(t, succeedWith(2))
	r := newObserveRouter(svc)
	if rec := call(r, http.MethodPost, "/api/v1/observe", `{"prompt":"go","time_zone":"Mars/Olympus"}`); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_request" {
		t.Fatalf("unknown zone: %d %s", rec.Code, rec.Body)
	}
	rec := call(r, http.MethodPost, "/api/v1/observe", `{"prompt":"why is checkout slow","kind":"investigate","time_zone":"Asia/Kolkata"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("investigate: %d %s", rec.Code, rec.Body)
	}
	var zone string
	if err := d.SQLDB(t).QueryRow(`SELECT time_zone FROM agent_requests WHERE prompt = 'why is checkout slow'`).Scan(&zone); err != nil || zone != "Asia/Kolkata" {
		t.Fatalf("stored zone %q, %v", zone, err)
	}
}
