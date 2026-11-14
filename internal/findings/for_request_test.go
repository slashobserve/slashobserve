package findings_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/findings"
)

func TestTheFindingOfARequestReachesOnlyWhoCanSeeItsObservation(t *testing.T) {
	r := newRig(t)
	private := r.request(t, editor, pgtype.UUID{})
	r.save(t, editor, findings.SaveInput{AgentRequestID: private})
	shared := r.request(t, editor, r.open)
	r.save(t, editor, findings.SaveInput{AgentRequestID: shared, DashboardID: r.open})

	cases := []struct {
		name    string
		request pgtype.UUID
		who     auth.Principal
		want    bool
	}{
		{"the requester, in My Dashboards", private, editor, true},
		{"an Admin, in someone's My Dashboards", private, admin, false},
		{"another Editor, in someone's My Dashboards", private, otherEditor, false},
		{"a Viewer, in someone's My Dashboards", private, viewer, false},
		{"an Admin, on a dashboard in General", shared, admin, true},
		{"a Viewer, on a dashboard in General", shared, viewer, true},
	}
	for _, c := range cases {
		f, found, err := r.service.ForRequest(t.Context(), c.who, c.request)
		if err != nil || found != c.want {
			t.Errorf("%s: found %v, err %v, want found %v", c.name, found, err, c.want)
		}
		if !found && f.ID != "" {
			t.Errorf("%s: a hidden finding came back: %+v", c.name, f)
		}
	}
}

func TestARequestWithoutAFindingHasNoneForAnyone(t *testing.T) {
	r := newRig(t)
	request := r.request(t, editor, r.open)
	if _, found, err := r.service.ForRequest(t.Context(), admin, request); err != nil || found {
		t.Fatalf("found %v, err %v", found, err)
	}
}
