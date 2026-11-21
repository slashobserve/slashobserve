package findings_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type chatAttach struct {
	run, dashboard pgtype.UUID
	inTx           bool
}

func TestMakingADashboardAttachesTheRequestersRunChatInTheSameTransaction(t *testing.T) {
	r := newRig(t)
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger)
	if err != nil {
		t.Fatal(err)
	}
	access := folders.NewAccess(r.db.DB)
	var calls []chatAttach
	var refuse error
	attach := func(ctx context.Context, q *dbsqlc.Queries, run, dashboard pgtype.UUID) error {
		_, err := q.GetDashboard(ctx, dashboard)
		inTx := err == nil
		calls = append(calls, chatAttach{run: run, dashboard: dashboard, inTx: inTx})
		return refuse
	}
	dash := dashboards.New(r.db.DB, queryproxy.NewRunner(registry, logger), access, logger)
	r.service, err = findings.New(r.db.DB, access, annotations.New(r.db.DB, access), dash, r.observations, logger, findings.AttachChatsWith(attach))
	if err != nil {
		t.Fatal(err)
	}
	notes := []spec.Panel{panelOf(t, `{"id":"evidence-1","type":"text","title":"Notes","gridPos":{"x":0,"y":0,"w":12,"h":8},"content":"checkout-0"}`)}
	run := r.request(t, editor, pgtype.UUID{})
	answer := r.save(t, editor, findings.SaveInput{AgentRequestID: run, Evidence: notes})

	made, _, err := r.service.SaveAsDashboard(reading(t.Context()), editor, mustUUID(t, answer.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].run != run || calls[0].dashboard.String() != made.ID || !calls[0].inTx {
		t.Fatalf("attach calls = %+v, want one for run %s and dashboard %s inside the create", calls, run.String(), made.ID)
	}

	refuse = errors.New("the chat is gone")
	before := r.count(t, `SELECT count(*) FROM dashboards`)
	if _, _, err := r.service.SaveAsDashboard(reading(t.Context()), editor, mustUUID(t, answer.ID)); !errors.Is(err, refuse) {
		t.Fatalf("a refused attach: %v", err)
	}
	if after := r.count(t, `SELECT count(*) FROM dashboards`); after != before {
		t.Fatalf("dashboards = %d after a refused attach, want %d", after, before)
	}
	refuse = nil

	calls = nil
	saved := r.save(t, editor, findings.SaveInput{AgentRequestID: r.investigation(t, editor, r.open), DashboardID: r.open, Evidence: notes})
	if _, _, err := r.service.SaveAsDashboard(reading(t.Context()), admin, mustUUID(t, saved.ID)); err != nil {
		t.Fatal(err)
	}
	unrun := r.save(t, editor, findings.SaveInput{Evidence: notes})
	if _, _, err := r.service.SaveAsDashboard(reading(t.Context()), editor, mustUUID(t, unrun.ID)); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 0 {
		t.Fatalf("attach calls = %+v, want none for another person's run or a finding without one", calls)
	}
}
