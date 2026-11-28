package tools

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/proposals"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/threads"
	"github.com/slashobserve/slashobserve/internal/viewers"
)

type answerRig struct {
	db       *dbtest.Database
	registry *Registry
	threads  *threads.Service
	asks     *threads.Asks
	queue    *revisions.Service
	findings *findings.Service
	desk     *revisions.RunDesk
	target   threads.Target
}

type findingAnswers struct{ findings *findings.Service }

func (a findingAnswers) RunAnswer(ctx context.Context, by auth.Principal, run pgtype.UUID) (threads.Answer, bool, error) {
	f, found, err := a.findings.ForRequest(ctx, by, run)
	if err != nil || !found {
		return threads.Answer{}, false, err
	}
	raw, err := json.Marshal(findings.JSONOf(f, ""))
	if err != nil {
		return threads.Answer{}, false, err
	}
	return threads.Answer{RunID: run.String(), FindingID: f.ID, Title: f.Title, Markdown: f.Markdown, ObservationID: f.ObservationID, CanSave: f.SavableBy(by), Finding: raw}, true, nil
}

func newAnswerRig(t *testing.T) *answerRig {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	ds := &fakeDS{
		rangeFn: func(context.Context, datasource.Query, datasource.TimeRange, time.Duration) ([]datasource.Frame, error) {
			return []datasource.Frame{timeNumberFrame(nil, 1, 2)}, nil
		},
		instantFn: func(context.Context, datasource.Query, datasource.TimeRange) ([]datasource.Frame, error) {
			return []datasource.Frame{timeNumberFrame(nil, 1, 2)}, nil
		},
	}
	datasources, err := datasource.NewRegistry(logger, datasource.Instance{UID: fakeUID, Name: "Fake", Datasource: ds})
	if err != nil {
		t.Fatal(err)
	}
	runner := queryproxy.NewRunner(datasources, logger)
	access := folders.NewAccess(database.DB)
	service := dashboards.New(database.DB, runner, access, logger)
	notes := annotations.New(database.DB, access)
	observationService := observations.New(database.DB, access, nil, nil, logger)
	found, err := findings.New(database.DB, access, service, observationService, logger)
	if err != nil {
		t.Fatal(err)
	}
	commentThreads := threads.NewService(database.DB, access, service, notes, observationService, proposals.New(database.DB, access, service, logger))
	queue := revisions.NewService(database.DB, logger, revisions.WithBuiltInAgent(), revisions.WithAccess(access))
	readMessage := func(ctx context.Context, by auth.Principal, messageID pgtype.UUID) error {
		_, err := commentThreads.ReadMessage(ctx, by, messageID)
		return err
	}
	found.ReadThreadsWith(readMessage)
	queue.ReadThreadsWith(readMessage)
	commentThreads.ReadAnswersWith(queue, findingAnswers{found})
	asks := threads.NewAsks(commentThreads, queue, true)
	registry := New(Deps{Datasources: datasources, Runner: runner, Dashboards: service, Threads: commentThreads, Asks: asks, Folders: access, Annotations: notes, Observations: observationService, Viewers: viewers.New(database.DB), Logger: logger})

	ctx := auth.WithPrincipal(reading(t.Context()), placeholderUser)
	dash, _, err := service.Create(ctx, placeholderUser, pgtype.UUID{}, []byte(specOf(statPanel("p1", 0))), "seed")
	if err != nil {
		t.Fatal(err)
	}
	dashboardID, err := db.ParseUUID(dash.ID)
	if err != nil {
		t.Fatal(err)
	}
	desk := revisions.NewRunDesk(queue, revisions.ObserveOptions{Finding: func(ctx context.Context, by auth.Principal, run pgtype.UUID) (json.RawMessage, bool, error) {
		a, ok, err := findingAnswers{found}.RunAnswer(ctx, by, run)
		return a.Finding, ok, err
	}})
	return &answerRig{db: database, registry: registry, threads: commentThreads, asks: asks, queue: queue, findings: found, desk: desk, target: threads.Target{Kind: threads.KindDashboard, ID: dashboardID}}
}

func (r *answerRig) answeredAsk(t *testing.T, restricted bool) pgtype.UUID {
	t.Helper()
	ctx := auth.WithPrincipal(reading(t.Context()), placeholderUser)
	sent, err := r.asks.Open(ctx, placeholderUser, threads.OpenInput{Target: r.target, Version: new(1), PanelID: new("p1"), Body: "@observe why did it rise"}, threads.ToAuto)
	if err != nil || sent.Run == nil {
		t.Fatalf("ask = %+v, %v", sent, err)
	}
	run, err := db.ParseUUID(sent.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `UPDATE agent_requests SET state = 'succeeded', status = 'succeeded', result_kind = 'observation', finished_at = now() WHERE id = $1`, run)
	if _, _, err := r.findings.Save(ctx, placeholderUser, findings.SaveInput{AgentRequestID: run, Title: "Checkout is slow", Markdown: "the 5xx rate rose", From: time.Now().Add(-time.Hour), To: time.Now()}); err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, `UPDATE agent_requests SET read_restricted = $2 WHERE id = $1`, run, restricted)
	err = r.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		row, err := q.GetAgentRequestByID(ctx, run)
		if err != nil {
			return err
		}
		return threads.NewRuns().RunEnded(ctx, q, row)
	})
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func (r *answerRig) listedAnswers(t *testing.T, by auth.Principal) map[string]*answerOut {
	t.Helper()
	res, err := r.registry.Call(auth.WithPrincipal(reading(t.Context()), by), "list_comments", []byte(`{"target":"`+r.target.String()+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	out, ok := res.Content.(commentListOut)
	if !ok {
		t.Fatalf("list_comments answered %#v", res.Content)
	}
	answers := map[string]*answerOut{}
	for _, c := range out.Comments {
		if c.Answer != nil {
			answers[c.Answer.RunID] = c.Answer
		}
	}
	return answers
}

func (r *answerRig) singleReadFindingID(t *testing.T, by auth.Principal, run pgtype.UUID) string {
	t.Helper()
	raw, err := r.desk.Run(reading(t.Context()), run, by, 0)
	if errors.Is(err, revisions.ErrRequestNotFound) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	var shown struct {
		Finding *findings.FindingJSON `json:"finding"`
	}
	if err := json.Unmarshal(raw, &shown); err != nil {
		t.Fatal(err)
	}
	if shown.Finding == nil {
		return ""
	}
	return shown.Finding.ID
}

func TestListCommentsGivesAnAgentKeyTheAnswersItsRunReadGivesIt(t *testing.T) {
	r := newAnswerRig(t)
	open := r.answeredAsk(t, false)
	kept := r.answeredAsk(t, true)
	key := auth.Principal{Kind: auth.KindServiceAccount, ID: "88888888-8888-4888-8888-888888888888", Name: "ci", Role: auth.RoleViewer}

	for _, tt := range []struct {
		reader     auth.Principal
		open, kept bool
	}{{key, true, false}, {placeholderUser, true, true}, {adminCaller, true, true}, {editorCaller, true, false}, {viewerCaller, true, false}} {
		listed := r.listedAnswers(t, tt.reader)
		for _, run := range []struct {
			id   pgtype.UUID
			want bool
		}{{open, tt.open}, {kept, tt.kept}} {
			a := listed[run.id.String()]
			single := r.singleReadFindingID(t, tt.reader, run.id)
			if (a != nil) != run.want || (single != "") != run.want {
				t.Errorf("%s reads run %s: list_comments %v, the run read %q, want %v", tt.reader.Name, run.id, a != nil, single, run.want)
				continue
			}
			if a != nil && (a.FindingID != single || a.Title != "Checkout is slow" || a.Markdown != "the 5xx rate rose") {
				t.Errorf("%s reads run %s as %+v, the run read gives finding %s", tt.reader.Name, run.id, a, single)
			}
		}
	}
}
