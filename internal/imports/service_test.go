package imports_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/grafana"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var ada = auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Ada", Email: "ada@example.com"}

const brokenQuery = "broken_metric"

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type promStub struct{}

func (promStub) Type() string                      { return "prometheus" }
func (promStub) CheckHealth(context.Context) error { return nil }
func (promStub) Close() error                      { return nil }
func (promStub) Lint(context.Context, string) ([]datasource.Diagnostic, error) {
	return nil, nil
}

func (promStub) Discover(context.Context, datasource.DiscoverRequest) (*datasource.Catalog, error) {
	return &datasource.Catalog{}, nil
}

func (promStub) VariableValues(_ context.Context, q datasource.Query, _ datasource.TimeRange) ([]string, error) {
	if strings.Contains(q.Text, brokenQuery) {
		return nil, errors.New("upstream rejected the query")
	}
	return []string{"v"}, nil
}

func (promStub) QueryRange(_ context.Context, q datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
	return frames(q)
}

func (promStub) QueryInstant(_ context.Context, q datasource.Query, _ datasource.TimeRange) ([]datasource.Frame, error) {
	return frames(q)
}

func frames(q datasource.Query) ([]datasource.Frame, error) {
	if strings.Contains(q.Text, brokenQuery) {
		return nil, errors.New("upstream rejected the query")
	}
	return []datasource.Frame{{Fields: []datasource.Field{
		{Name: "time", Type: datasource.FieldTime, Values: []any{1.0}},
		{Name: "value", Type: datasource.FieldNumber, Values: []any{2.0}},
	}}}, nil
}

type nulErrorStub struct{ promStub }

func (nulErrorStub) VariableValues(_ context.Context, q datasource.Query, _ datasource.TimeRange) ([]string, error) {
	if strings.Contains(q.Text, brokenQuery) {
		return nil, fmt.Errorf("%w: bad label a\x00b", datasource.ErrInvalidQuery)
	}
	return []string{"v"}, nil
}

type nulPanelErrorStub struct{ promStub }

func (nulPanelErrorStub) QueryRange(_ context.Context, q datasource.Query, _ datasource.TimeRange, _ time.Duration) ([]datasource.Frame, error) {
	if strings.Contains(q.Text, brokenQuery) {
		return nil, fmt.Errorf("%w: bad label a\x00b", datasource.ErrInvalidQuery)
	}
	return frames(q)
}

func (s nulPanelErrorStub) QueryInstant(ctx context.Context, q datasource.Query, tr datasource.TimeRange) ([]datasource.Frame, error) {
	return s.QueryRange(ctx, q, tr, 0)
}

type blockingStub struct {
	promStub
	entered chan struct{}
	release chan struct{}
}

func (b blockingStub) VariableValues(ctx context.Context, q datasource.Query, tr datasource.TimeRange) ([]string, error) {
	b.entered <- struct{}{}
	select {
	case <-b.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return b.promStub.VariableValues(ctx, q, tr)
}

type noopReviser struct{}

func (noopReviser) Revise(context.Context, revisions.Request) (revisions.Result, error) {
	return revisions.Result{}, nil
}

type rig struct {
	db        *dbtest.Database
	dashboard *dashboards.Service
	imports   *imports.Service
	queue     *revisions.Service
}

func newRig(t *testing.T) *rig {
	t.Helper()
	return newRigWith(t, promStub{})
}

func newRigWith(t *testing.T, ds datasource.Datasource) *rig {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger,
		datasource.Instance{UID: "prom", Datasource: ds},
		datasource.Instance{UID: "prom2", Datasource: ds},
	)
	if err != nil {
		t.Fatal(err)
	}
	dash := dashboards.New(database.DB, queryproxy.NewRunner(registry, logger), logger)
	return &rig{
		db:        database,
		dashboard: dash,
		imports:   imports.New(database.DB, dash, registry, logger),
		queue:     revisions.NewService(database.DB, noopReviser{}, logger),
	}
}

func grafanaJSON(title, datasourceUID string, panels int) []byte {
	items := make([]string, panels)
	for i := range items {
		items[i] = fmt.Sprintf(`{"id":%d,"type":"timeseries","title":"Panel %d","gridPos":{"x":0,"y":%d,"w":12,"h":8},"datasource":{"type":"prometheus","uid":%q},"targets":[{"refId":"A","expr":"up"}]}`, i+1, i+1, i*8, datasourceUID)
	}
	return fmt.Appendf(nil, `{"title":%q,"uid":"abc","schemaVersion":39,"panels":[%s]}`, title, strings.Join(items, ","))
}

func grafanaWithVariables(title, datasourceUID string, queries ...string) []byte {
	items := make([]string, len(queries))
	for i, query := range queries {
		items[i] = fmt.Sprintf(`{"type":"query","name":"v%d","datasource":{"type":"prometheus","uid":%q},"query":{"query":%q},"refresh":1}`, i, datasourceUID, query)
	}
	panel := fmt.Sprintf(`{"id":1,"type":"timeseries","title":"One","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":{"type":"prometheus","uid":%q},"targets":[{"refId":"A","expr":"up"}]}`, datasourceUID)
	return fmt.Appendf(nil, `{"title":%q,"uid":"abc","schemaVersion":39,"templating":{"list":[%s]},"panels":[%s]}`, title, strings.Join(items, ","), panel)
}

func specWith(query string) []byte {
	return fmt.Appendf(nil, `{"specVersion":"0","title":"Hand made","time":{"from":"now-15m","to":"now"},"panels":[{"id":"p","type":"timeseries","title":"p","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":"prom","queries":[{"query":%q}]}]}`, query)
}

func uuid(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestImportStoresTheVerifiedReport(t *testing.T) {
	r := newRig(t)
	mapping := map[string]string{"prom": "prom2"}
	got, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: grafanaJSON("Shop", "prom", 2), Mapping: mapping})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version.Number != 1 || got.Replayed || got.Dashboard.Title != "Shop" || got.Dashboard.Creator.Via != "direct" {
		t.Fatalf("version %d, replayed %t, title %q, via %q", got.Version.Number, got.Replayed, got.Dashboard.Title, got.Dashboard.Creator.Via)
	}
	if got.Version.Summary != `Imported "Shop" from Grafana` {
		t.Errorf("summary = %q", got.Version.Summary)
	}
	if !got.Record.Report.Verified || got.Record.Report.Counts.Total != 2 {
		t.Fatalf("report verified %t, counts %+v", got.Record.Report.Verified, got.Record.Report.Counts)
	}

	stored, err := r.imports.ForDashboard(t.Context(), uuid(t, got.Dashboard.ID))
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != got.Record.ID || stored.Version != 1 || stored.ImporterVersion != grafana.ImporterVersion || stored.Title != "Shop" {
		t.Errorf("stored record: %+v", stored)
	}
	if !stored.Report.Verified || stored.Report.Counts != got.Record.Report.Counts || stored.CreatedBy.ID != ada.ID || stored.CreatedBy.Email != ada.Email {
		t.Errorf("stored report %+v, author %+v", stored.Report.Counts, stored.CreatedBy)
	}
	if stored.Mapping["prom"] != "prom2" || stored.Summary.Title != "Shop" || len(stored.Datasources) != 1 {
		t.Errorf("stored mapping %v, summary %+v, datasources %+v", stored.Mapping, stored.Summary, stored.Datasources)
	}
}

func TestImportWithoutAMappingStoresAnEmptyObject(t *testing.T) {
	r := newRig(t)
	got, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: grafanaJSON("Shop", "prom", 1)})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := r.imports.ForDashboard(t.Context(), uuid(t, got.Dashboard.ID))
	if err != nil || stored.Mapping == nil || len(stored.Mapping) != 0 {
		t.Fatalf("mapping %v, err %v", stored.Mapping, err)
	}
}

func TestCreateImportedKeepsFailingQueriesAsIssues(t *testing.T) {
	r := newRig(t)
	var seen []spec.Issue
	var seenVersion int
	hook := func(_ context.Context, _ *dbsqlc.Queries, _ dashboards.Dashboard, ver dashboards.Version, issues []spec.Issue) error {
		seen, seenVersion = issues, ver.Number
		return nil
	}
	dash, ver, issues, replayed, err := r.dashboard.CreateImported(t.Context(), ada, specWith(brokenQuery), "Imported", pgtype.UUID{}, hook)
	if err != nil || replayed || ver.Number != 1 {
		t.Fatalf("version %d, replayed %t, err %v", ver.Number, replayed, err)
	}
	if len(issues) == 0 || len(seen) != len(issues) || seenVersion != 1 || !strings.HasPrefix(issues[0].Path, "/panels/0") {
		t.Fatalf("issues %+v, hook saw %+v at version %d", issues, seen, seenVersion)
	}
	if latest, err := r.dashboard.LatestVersion(t.Context(), uuid(t, dash.ID)); err != nil || latest != 1 {
		t.Fatalf("latest = %d, err %v", latest, err)
	}

	_, _, err = r.dashboard.Create(t.Context(), ada, specWith(brokenQuery), "create")
	var invalid *dashboards.InvalidSpecError
	if !errors.As(err, &invalid) {
		t.Fatalf("Create with the same spec: err = %v, want InvalidSpecError", err)
	}
	_, again, _, err := r.dashboard.AddVersion(t.Context(), ada, uuid(t, dash.ID), 1, specWith(brokenQuery), "again")
	if err != nil || again.Number != 2 || len(again.Unverified.Panels) != 1 {
		t.Fatalf("AddVersion with the same spec: version %d, unverified %+v, err %v, want version 2 keeping the panel unverified", again.Number, again.Unverified, err)
	}
}

func TestCreateImportedWithoutAHookSavesAndReportsIssues(t *testing.T) {
	r := newRig(t)
	dash, ver, issues, replayed, err := r.dashboard.CreateImported(t.Context(), ada, specWith(brokenQuery), "Imported", pgtype.UUID{}, nil)
	if err != nil || replayed || ver.Number != 1 || dash.ID == "" || len(issues) == 0 {
		t.Fatalf("version %d, replayed %t, issues %d, err %v", ver.Number, replayed, len(issues), err)
	}
}

func TestCreateImportedStillRejectsAnInvalidSpec(t *testing.T) {
	r := newRig(t)
	hook := func(context.Context, *dbsqlc.Queries, dashboards.Dashboard, dashboards.Version, []spec.Issue) error {
		t.Error("the hook ran for an invalid spec")
		return nil
	}
	_, _, _, _, err := r.dashboard.CreateImported(t.Context(), ada, []byte(`{"specVersion":"0"}`), "", pgtype.UUID{}, hook)
	var invalid *dashboards.InvalidSpecError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidSpecError", err)
	}
}

func TestAFailingHookRollsTheSaveBack(t *testing.T) {
	r := newRig(t)
	boom := errors.New("hook failed")
	hook := func(context.Context, *dbsqlc.Queries, dashboards.Dashboard, dashboards.Version, []spec.Issue) error {
		return boom
	}
	if _, _, _, _, err := r.dashboard.CreateImported(t.Context(), ada, specWith("up"), "Imported", pgtype.UUID{}, hook); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the hook's error", err)
	}
	page, err := r.dashboard.List(t.Context(), dashboards.ListParams{Limit: 10})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("dashboards after a failed hook: %d, %v", len(page.Items), err)
	}
}

func TestPreviewWritesNothing(t *testing.T) {
	r := newRig(t)
	result, err := r.imports.Preview(t.Context(), ada, imports.Request{JSON: grafanaJSON("Shop", "prom", 2)})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Ready() || result.Report.Verified {
		t.Fatalf("ready %t, verified %t", result.Ready(), result.Report.Verified)
	}
	metas, next, err := r.imports.List(t.Context(), 0, "")
	if err != nil || len(metas) != 0 || next != "" {
		t.Fatalf("list after preview: %v, %q, %v", metas, next, err)
	}
	page, err := r.dashboard.List(t.Context(), dashboards.ListParams{Limit: 10})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("dashboards after preview: %d, %v", len(page.Items), err)
	}
}

func TestPreviewReturnsInputErrors(t *testing.T) {
	r := newRig(t)
	_, err := r.imports.Preview(t.Context(), ada, imports.Request{JSON: []byte(`not json`)})
	var input *grafana.InputError
	if !errors.As(err, &input) {
		t.Fatalf("err = %v, want InputError", err)
	}
}

func TestImportNeedsEveryDatasourceMatched(t *testing.T) {
	r := newRig(t)
	_, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: grafanaJSON("Shop", "elsewhere", 1)})
	var unresolved *imports.UnresolvedError
	if !errors.As(err, &unresolved) || len(unresolved.Datasources) != 1 || unresolved.Datasources[0].Match != nil {
		t.Fatalf("err = %v, want UnresolvedError with the unmatched datasource", err)
	}
	if metas, _, err := r.imports.List(t.Context(), 0, ""); err != nil || len(metas) != 0 {
		t.Fatalf("list: %v, %v", metas, err)
	}
	if _, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: grafanaJSON("Shop", "elsewhere", 1), Mapping: map[string]string{"elsewhere": "prom"}}); err != nil {
		t.Fatalf("with a mapping: %v", err)
	}
}

func TestForDashboardTellsMissingFromNotImported(t *testing.T) {
	r := newRig(t)
	plain, _, err := r.dashboard.Create(t.Context(), ada, specWith("up"), "create")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.imports.ForDashboard(t.Context(), uuid(t, plain.ID)); !errors.Is(err, imports.ErrNotFound) {
		t.Errorf("not imported: err = %v, want ErrNotFound", err)
	}
	missing := uuid(t, "99999999-9999-4999-8999-999999999999")
	if _, err := r.imports.ForDashboard(t.Context(), missing); !errors.Is(err, dashboards.ErrNotFound) {
		t.Errorf("missing dashboard: err = %v, want dashboards.ErrNotFound", err)
	}
}

func TestListPagesNewestFirst(t *testing.T) {
	r := newRig(t)
	for _, title := range []string{"One", "Two", "Three"} {
		if _, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: grafanaJSON(title, "prom", 1)}); err != nil {
			t.Fatal(err)
		}
	}
	first, next, err := r.imports.List(t.Context(), 2, "")
	if err != nil || len(first) != 2 || next == "" {
		t.Fatalf("first page: %d items, cursor %q, err %v", len(first), next, err)
	}
	if first[0].Title != "Three" || first[1].Title != "Two" {
		t.Errorf("first page titles: %q, %q", first[0].Title, first[1].Title)
	}
	rest, next, err := r.imports.List(t.Context(), 2, next)
	if err != nil || len(rest) != 1 || next != "" || rest[0].Title != "One" || rest[0].Counts.Total != 1 {
		t.Fatalf("second page: %+v, cursor %q, err %v", rest, next, err)
	}
	if _, _, err := r.imports.List(t.Context(), 2, "!"); !errors.Is(err, imports.ErrInvalidCursor) {
		t.Errorf("bad cursor: err = %v", err)
	}
}

func TestAnAgentRequestImportsOnce(t *testing.T) {
	r := newRig(t)
	row, err := r.queue.SubmitObserve(t.Context(), ada, "import it")
	if err != nil {
		t.Fatal(err)
	}
	req := imports.Request{JSON: grafanaJSON("Shop", "prom", 2), RequestID: row.ID}
	first, err := r.imports.Import(t.Context(), ada, req)
	if err != nil || first.Replayed || first.Dashboard.Creator.Via != "agent" {
		t.Fatalf("first: replayed %t, err %v", first.Replayed, err)
	}
	again, err := r.imports.Import(t.Context(), ada, req)
	if err != nil || !again.Replayed || again.Dashboard.ID != first.Dashboard.ID {
		t.Fatalf("repeat: replayed %t, err %v", again.Replayed, err)
	}
	if again.Record.ID != first.Record.ID || !again.Record.Report.Verified || again.Record.Report.Counts.Total != 2 {
		t.Errorf("replayed record: %+v", again.Record)
	}
	if metas, _, err := r.imports.List(t.Context(), 0, ""); err != nil || len(metas) != 1 {
		t.Fatalf("list: %d, %v", len(metas), err)
	}
}

func principal(n int) auth.Principal {
	return auth.Principal{Kind: auth.KindServiceAccount, ID: fmt.Sprintf("44444444-4444-4444-8444-%012d", n), Name: fmt.Sprintf("bot-%d", n)}
}

func TestImportSlotsRefuseAndFree(t *testing.T) {
	gate := blockingStub{entered: make(chan struct{}, 16), release: make(chan struct{})}
	r := newRigWith(t, gate)
	start := func(who auth.Principal) <-chan error {
		done := make(chan error, 1)
		go func() {
			_, err := r.imports.Import(t.Context(), who, imports.Request{JSON: grafanaWithVariables("Shop", "prom", "label_values(up"+who.Name+", pod)")})
			done <- err
		}()
		select {
		case <-gate.entered:
		case <-time.After(10 * time.Second):
			t.Fatalf("import for %s never reached the datasource", who.Name)
		}
		return done
	}
	refused := func(who auth.Principal) {
		t.Helper()
		_, err := r.imports.Import(t.Context(), who, imports.Request{JSON: grafanaJSON("Shop", "prom", 1)})
		if !errors.Is(err, imports.ErrBusy) {
			t.Fatalf("import for %s: err = %v, want ErrBusy", who.Name, err)
		}
	}

	var running []<-chan error
	running = append(running, start(principal(0)))
	refused(principal(0))
	for i := 1; i < 4; i++ {
		running = append(running, start(principal(i)))
	}
	refused(principal(4))
	refused(principal(0))

	close(gate.release)
	for i, done := range running {
		if err := <-done; err != nil {
			t.Errorf("import %d: %v", i, err)
		}
	}
	for _, who := range []auth.Principal{principal(0), principal(4)} {
		if _, err := r.imports.Import(t.Context(), who, imports.Request{JSON: grafanaWithVariables("Shop", "prom", "label_values(up"+who.Name+", pod)")}); err != nil {
			t.Errorf("import for %s after the slots freed: %v", who.Name, err)
		}
	}
}

func TestAnImportKeepsAFailingVariableQueryAsAnUnverifiedNote(t *testing.T) {
	r := newRig(t)
	body := grafanaWithVariables("Shop", "prom", "label_values(up, pod)", "label_values("+brokenQuery+", pod)")
	got, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: body})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version.Number != 1 || !got.Record.Report.Verified {
		t.Fatalf("version %d, verified %t", got.Version.Number, got.Record.Report.Verified)
	}
	unverifiedNotes := func(notes []grafana.Note) (subjects []string) {
		for _, n := range notes {
			if n.Status == "unverified" {
				subjects = append(subjects, n.Subject)
			}
		}
		return subjects
	}
	if subjects := unverifiedNotes(got.Record.Report.Notes); len(subjects) != 1 || subjects[0] != "variable v1" {
		t.Fatalf("unverified notes on the import: %v", subjects)
	}
	stored, err := r.imports.ForDashboard(t.Context(), uuid(t, got.Dashboard.ID))
	if err != nil || stored.Version != 1 || !stored.Report.Verified {
		t.Fatalf("stored version %d, verified %t, err %v", stored.Version, stored.Report.Verified, err)
	}
	if subjects := unverifiedNotes(stored.Report.Notes); len(subjects) != 1 || subjects[0] != "variable v1" {
		t.Fatalf("unverified notes in the stored report: %v", subjects)
	}
	metas, _, err := r.imports.List(t.Context(), 0, "")
	if err != nil || len(metas) != 1 || metas[0].Version != 1 || metas[0].Counts != stored.Report.Counts {
		t.Fatalf("list: %+v, %v", metas, err)
	}
}

func TestAnImportReplacesANULFromADatasourceErrorInTheReason(t *testing.T) {
	r := newRigWith(t, nulErrorStub{})
	body := grafanaWithVariables("Shop", "prom", "label_values("+brokenQuery+", pod)")
	got, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: body})
	if err != nil {
		t.Fatal(err)
	}
	want := "bad label a\uFFFDb"
	reasonOf := func(report grafana.Report) string {
		for _, n := range report.Notes {
			if n.Subject == "variable v0" {
				return n.Reason
			}
		}
		return ""
	}
	if reason := reasonOf(got.Record.Report); !strings.Contains(reason, want) || strings.ContainsRune(reason, 0) {
		t.Fatalf("reason on the import = %q, want it to hold %q", reason, want)
	}
	stored, err := r.imports.ForDashboard(t.Context(), uuid(t, got.Dashboard.ID))
	if err != nil {
		t.Fatal(err)
	}
	if reason := reasonOf(stored.Report); !strings.Contains(reason, want) {
		t.Fatalf("stored reason = %q, want it to hold %q", reason, want)
	}
}

func TestAnImportReplacesANULFromAPanelQueryErrorInTheReason(t *testing.T) {
	r := newRigWith(t, nulPanelErrorStub{})
	body := fmt.Appendf(nil, `{"title":"Shop","uid":"abc","schemaVersion":39,"panels":[
		{"id":1,"type":"timeseries","title":"Good","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":{"type":"prometheus","uid":"prom"},"targets":[{"refId":"A","expr":"up"}]},
		{"id":2,"type":"timeseries","title":"Broken","gridPos":{"x":12,"y":0,"w":12,"h":8},"datasource":{"type":"prometheus","uid":"prom"},"targets":[{"refId":"A","expr":%q}]}]}`, brokenQuery)
	got, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: body})
	if err != nil {
		t.Fatal(err)
	}
	want := "bad label a\uFFFDb"
	reasonsOf := func(report grafana.Report) []string {
		for _, p := range report.Panels {
			if p.Title == "Broken" {
				return p.Reasons
			}
		}
		return nil
	}
	check := func(where string, report grafana.Report) {
		t.Helper()
		reasons := reasonsOf(report)
		if len(reasons) != 1 || !strings.Contains(reasons[0], want) || strings.ContainsRune(reasons[0], 0) {
			t.Errorf("reasons %s = %q, want one holding %q", where, reasons, want)
		}
		if report.Counts.Unverified != 1 {
			t.Errorf("unverified %s = %d, want 1", where, report.Counts.Unverified)
		}
	}
	check("on the import", got.Record.Report)
	stored, err := r.imports.ForDashboard(t.Context(), uuid(t, got.Dashboard.ID))
	if err != nil {
		t.Fatal(err)
	}
	check("in the stored report", stored.Report)
}

func assertNothingSaved(t *testing.T, r *rig) {
	t.Helper()
	page, err := r.dashboard.List(t.Context(), dashboards.ListParams{Limit: 10})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("dashboards after a refused import: %d, %v", len(page.Items), err)
	}
	if metas, _, err := r.imports.List(t.Context(), 0, ""); err != nil || len(metas) != 0 {
		t.Fatalf("imports after a refused import: %d, %v", len(metas), err)
	}
}

func TestImportRefusesANULInTheGrafanaUID(t *testing.T) {
	r := newRig(t)
	body := bytes.Replace(grafanaJSON("Shop", "prom", 1), []byte(`"uid":"abc"`), []byte(`"uid":"a\u0000b"`), 1)
	_, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: body})
	var invalid *dashboards.InvalidSpecError
	if !errors.As(err, &invalid) || len(invalid.Issues) != 1 || invalid.Issues[0].Path != "/summary/uid" {
		t.Fatalf("err = %v, want InvalidSpecError at /summary/uid", err)
	}
	assertNothingSaved(t, r)
}

func TestImportRefusesANULInADatasourceName(t *testing.T) {
	r := newRig(t)
	body := []byte(`{"title":"Shop","uid":"abc","schemaVersion":39,
		"__inputs":[{"name":"DS_PROM","label":"Pro\u0000m","pluginId":"prometheus","type":"datasource"}],
		"panels":[{"id":1,"type":"timeseries","title":"One","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":{"type":"prometheus","uid":"${DS_PROM}"},"targets":[{"refId":"A","expr":"up"}]}]}`)
	_, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: body, Mapping: map[string]string{"${DS_PROM}": "prom"}})
	var invalid *dashboards.InvalidSpecError
	if !errors.As(err, &invalid) || len(invalid.Issues) != 1 || invalid.Issues[0].Path != "/datasources/0/name" {
		t.Fatalf("err = %v, want InvalidSpecError at /datasources/0/name", err)
	}
	assertNothingSaved(t, r)
}

func TestImportEnforcesTheBodyAndMappingLimits(t *testing.T) {
	r := newRig(t)
	var input *grafana.InputError
	_, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: bytes.Repeat([]byte("x"), imports.MaxBodyBytes+1)})
	if !errors.As(err, &input) || input.Code != "too_large" {
		t.Fatalf("large body: err = %v, want InputError too_large", err)
	}
	mapping := map[string]string{}
	for i := range imports.MaxMappingEntries + 1 {
		mapping[fmt.Sprintf("key%d", i)] = "prom"
	}
	_, err = r.imports.Import(t.Context(), ada, imports.Request{JSON: grafanaJSON("Shop", "prom", 1), Mapping: mapping})
	if !errors.As(err, &input) || input.Code != "invalid_mapping" {
		t.Fatalf("large mapping: err = %v, want InputError invalid_mapping", err)
	}
	assertNothingSaved(t, r)
}

func TestPreviewEnforcesTheBodyAndMappingLimits(t *testing.T) {
	r := newRig(t)
	var input *grafana.InputError
	_, err := r.imports.Preview(t.Context(), ada, imports.Request{JSON: bytes.Repeat([]byte("x"), imports.MaxBodyBytes+1)})
	if !errors.As(err, &input) || input.Code != "too_large" {
		t.Fatalf("large body: err = %v, want InputError too_large", err)
	}
	mapping := map[string]string{}
	for i := range imports.MaxMappingEntries + 1 {
		mapping[fmt.Sprintf("key%d", i)] = "prom"
	}
	_, err = r.imports.Preview(t.Context(), ada, imports.Request{JSON: grafanaJSON("Shop", "prom", 1), Mapping: mapping})
	if !errors.As(err, &input) || input.Code != "invalid_mapping" {
		t.Fatalf("large mapping: err = %v, want InputError invalid_mapping", err)
	}
}

func TestARetriedRequestReplaysWhateverItCarries(t *testing.T) {
	r := newRig(t)
	row, err := r.queue.SubmitObserve(t.Context(), ada, "import it")
	if err != nil {
		t.Fatal(err)
	}
	first, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: grafanaJSON("Shop", "prom", 1), RequestID: row.ID})
	if err != nil {
		t.Fatal(err)
	}
	for name, req := range map[string]imports.Request{
		"not json":            {JSON: []byte(`not json`), RequestID: row.ID},
		"unmatched sources":   {JSON: grafanaJSON("Shop", "elsewhere", 1), RequestID: row.ID},
		"different dashboard": {JSON: grafanaJSON("Other", "prom", 3), RequestID: row.ID},
	} {
		again, err := r.imports.Import(t.Context(), ada, req)
		if err != nil || !again.Replayed || again.Dashboard.ID != first.Dashboard.ID || again.Record.ID != first.Record.ID {
			t.Errorf("%s: replayed %t, err %v", name, again.Replayed, err)
		}
	}
}

func TestReplayOfADashboardTheAgentCreatedHasNoRecord(t *testing.T) {
	r := newRig(t)
	row, err := r.queue.SubmitObserve(t.Context(), ada, "build it")
	if err != nil {
		t.Fatal(err)
	}
	created, _, _, err := r.dashboard.CreateRequestDashboard(t.Context(), ada, specWith("up"), "create", row.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: grafanaJSON("Shop", "prom", 1), RequestID: row.ID})
	if err != nil || !got.Replayed || got.Dashboard.ID != created.ID || got.Record.ID != "" {
		t.Fatalf("replayed %t, dashboard %q, record %+v, err %v", got.Replayed, got.Dashboard.ID, got.Record, err)
	}
}

func TestSummaryKeepsItsFramingForTrimmedAndLongTitles(t *testing.T) {
	r := newRig(t)
	long := strings.Repeat("日", 200)
	for name, c := range map[string]struct{ title, want string }{
		"padded": {"  Shop  ", `Imported "Shop" from Grafana`},
		"long":   {long, ""},
	} {
		got, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: grafanaJSON(c.title, "prom", 1)})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		summary := got.Version.Summary
		if !strings.HasPrefix(summary, `Imported "`) || !strings.HasSuffix(summary, `" from Grafana`) || len(summary) > 500 || !utf8.ValidString(summary) {
			t.Errorf("%s: summary %q (%d bytes)", name, summary, len(summary))
		}
		if c.want != "" && summary != c.want {
			t.Errorf("%s: summary %q, want %q", name, summary, c.want)
		}
	}
}

func TestAFreshImportWithoutDatasourcesHasAnEmptyList(t *testing.T) {
	r := newRig(t)
	body := []byte(`{"title":"Notes","uid":"abc","schemaVersion":39,"panels":[{"id":1,"type":"text","title":"Hello","gridPos":{"x":0,"y":0,"w":12,"h":4},"options":{"mode":"markdown","content":"hi"}}]}`)
	got, err := r.imports.Import(t.Context(), ada, imports.Request{JSON: body})
	if err != nil {
		t.Fatal(err)
	}
	if got.Record.Datasources == nil || len(got.Record.Datasources) != 0 {
		t.Fatalf("datasources = %#v, want an empty non-nil slice", got.Record.Datasources)
	}
}
