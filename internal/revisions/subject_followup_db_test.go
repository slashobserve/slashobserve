package revisions_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/internal/placement"
)

func TestAFollowUpCarryingAnAnswerAboutAnotherAdminsOnlySubjectIsMarked(t *testing.T) {
	cases := map[string]struct {
		about    string
		movedOut bool
		marked   bool
	}{
		"a follow-up about nothing":                 {"", false, true},
		"a follow-up on a dashboard in General":     {"other", false, true},
		"a follow-up on the same dashboard":         {"subject", false, false},
		"a follow-up after the subject moved out":   {"", true, false},
		"a follow-up on General after it moved out": {"other", true, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			svc, d := newService(t, succeedWith(2))
			ctx := reading(context.Background())
			subject, other := seedDashboard(t, d, 1), seedDashboard(t, d, 1)
			d.Exec(t, `UPDATE dashboards SET folder_id = $2 WHERE id = $1`, subject, foldertest.AdminsOnly(t, d, "Leadership"))
			first, err := svc.SubmitObserve(ctx, user, "what does the leadership dashboard say")
			if err != nil {
				t.Fatal(err)
			}
			answerWith(t, d, first.ID, "pending layoffs: payments team")
			second, err := followUp(ctx, svc, user, "summarise it", first.ID)
			if err != nil {
				t.Fatal(err)
			}
			d.Exec(t, `UPDATE agent_requests SET kind = 'investigate', dashboard_id = $2 WHERE id = $1`, first.ID, subject)
			switch c.about {
			case "subject":
				d.Exec(t, `UPDATE agent_requests SET kind = 'investigate', dashboard_id = $2 WHERE id = $1`, second.ID, subject)
			case "other":
				d.Exec(t, `UPDATE agent_requests SET kind = 'investigate', dashboard_id = $2 WHERE id = $1`, second.ID, other)
			}
			if c.movedOut {
				d.Exec(t, `UPDATE dashboards SET folder_id = general_folder_id() WHERE id = $1`, subject)
			}

			if got := claimedContext(t, svc, second.ID); !strings.Contains(got, "pending layoffs") {
				t.Fatalf("thread context = %q, want the first answer", got)
			}
			stored, err := svc.GetByID(ctx, second.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.ReadRestricted != c.marked {
				t.Fatalf("the follow-up is marked %v, want %v", stored.ReadRestricted, c.marked)
			}
		})
	}
}

func TestARunThatStartsFromAnAnswerAboutAnAdminsOnlySubjectIsMarkedUntilTheSubjectMovesOut(t *testing.T) {
	cases := map[string]func(answeredRun, *testing.T, *dbtest.Database, pgtype.UUID){
		"a thread ask in a thread that holds the answer": answeredRun.threadAsk,
		"an apply of the answer":                         answeredRun.apply,
		"a continuation of the run":                      answeredRun.continuation,
		"an ask about the observation that holds it":     answeredRun.askAbout,
	}
	for name, startFrom := range cases {
		for _, movedOut := range []bool{false, true} {
			t.Run(name, func(t *testing.T) {
				svc, d := newService(t, succeedWith(2))
				ctx := reading(context.Background())
				subject := seedDashboard(t, d, 1)
				d.Exec(t, `UPDATE dashboards SET folder_id = $2 WHERE id = $1`, subject, foldertest.AdminsOnly(t, d, "Leadership"))
				first, err := svc.SubmitObserve(ctx, user, "what does the leadership dashboard say")
				if err != nil {
					t.Fatal(err)
				}
				answerWith(t, d, first.ID, "pending layoffs: payments team")
				d.Exec(t, `UPDATE agent_requests SET kind = 'investigate', dashboard_id = $2 WHERE id = $1`, first.ID, subject)
				if movedOut {
					d.Exec(t, `UPDATE dashboards SET folder_id = general_folder_id() WHERE id = $1`, subject)
				}
				next, err := svc.SubmitObserve(ctx, user, "what else is pending")
				if err != nil {
					t.Fatal(err)
				}
				startFrom(answeredObservation(t, d, first.ID), t, d, next.ID)

				queued, err := d.Queries().GetAgentRequestByID(ctx, next.ID)
				if err != nil {
					t.Fatal(err)
				}
				marked, err := placement.InheritRestricted(ctx, d.Queries(), queued)
				if err != nil || marked.ReadRestricted == movedOut {
					t.Fatalf("subject moved out %v: the run is marked %v, %v", movedOut, marked.ReadRestricted, err)
				}
			})
		}
	}
}
