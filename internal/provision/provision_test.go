package provision_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/provision"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

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

func (promStub) VariableValues(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
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

type rig struct {
	db          *dbtest.Database
	dashboards  *dashboards.Service
	dir         string
	provisioner *provision.Provisioner
}

func newRig(t *testing.T) *rig {
	t.Helper()
	database := dbtest.New(t)
	logger := slog.New(slog.DiscardHandler)
	registry, err := datasource.NewRegistry(logger, datasource.Instance{UID: "prom", Datasource: promStub{}})
	if err != nil {
		t.Fatal(err)
	}
	access := folders.NewAccess(database.DB)
	dash := dashboards.New(database.DB, queryproxy.NewRunner(registry, logger), access, logger)
	dir := t.TempDir()
	return &rig{db: database, dashboards: dash, dir: dir, provisioner: provision.New(logger, dash, access, database.DB, dir)}
}

func specJSON(title, query string) string {
	return fmt.Sprintf(`{"specVersion":"0","title":%q,"time":{"from":"now-15m","to":"now"},"panels":[{"id":"p","type":"timeseries","title":"p","gridPos":{"x":0,"y":0,"w":12,"h":8},"datasource":"prom","queries":[{"query":%q}]}]}`, title, query)
}

func (r *rig) write(t *testing.T, path, content string) {
	t.Helper()
	full := filepath.Join(r.dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) remove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(filepath.Join(r.dir, filepath.FromSlash(path))); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) run(t *testing.T) provision.Report {
	t.Helper()
	if err := r.provisioner.Run(t.Context()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return r.report(t)
}

func (r *rig) report(t *testing.T) provision.Report {
	t.Helper()
	report, err := r.provisioner.Report(reading(t.Context()))
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	return report
}

func entry(t *testing.T, report provision.Report, path string) provision.Provisioned {
	t.Helper()
	for _, d := range report.Dashboards {
		if d.Path == path {
			return d
		}
	}
	t.Fatalf("no provisioned dashboard for %s in %+v", path, report.Dashboards)
	return provision.Provisioned{}
}

func (r *rig) latest(t *testing.T, dashboardID string) (dashboards.Dashboard, dashboards.Version) {
	t.Helper()
	id, err := db.ParseUUID(dashboardID)
	if err != nil {
		t.Fatal(err)
	}
	dash, ver, err := r.dashboards.Get(reading(t.Context()), id)
	if err != nil {
		t.Fatalf("Get %s: %v", dashboardID, err)
	}
	return dash, ver
}

func (r *rig) versions(t *testing.T, dashboardID string) []dashboards.VersionMeta {
	t.Helper()
	id, err := db.ParseUUID(dashboardID)
	if err != nil {
		t.Fatal(err)
	}
	metas, _, err := r.dashboards.ListVersions(reading(t.Context()), id, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	return metas
}

func (r *rig) folderID(t *testing.T, title string) pgtype.UUID {
	t.Helper()
	if title == "General" {
		var id pgtype.UUID
		if err := r.db.SQLDB(t).QueryRowContext(reading(t.Context()), `SELECT id FROM folders WHERE general`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	id, err := r.db.Queries().FolderByTitle(reading(t.Context()), title)
	if err != nil {
		t.Fatalf("folder %q: %v", title, err)
	}
	return id
}

func TestRunCreatesOneDashboardPerSpecFileInTheFolderOfItsFirstDirectory(t *testing.T) {
	r := newRig(t)
	r.write(t, "root.json", specJSON("Root", "up"))
	r.write(t, "team/a.json", specJSON("Team A", "up"))
	r.write(t, "team/sub/b.json", specJSON("Team B", "up"))
	r.write(t, ".hidden/c.json", specJSON("Hidden", "up"))
	r.write(t, "team/.d.json", specJSON("Hidden file", "up"))
	r.write(t, "team/notes.txt", "not a spec")

	report := r.run(t)
	if report.Dir != r.dir || len(report.Errors) != 0 {
		t.Fatalf("dir %q, errors %+v", report.Dir, report.Errors)
	}
	want := []struct{ path, folder, title string }{
		{"root.json", "General", "Root"},
		{"team/a.json", "team", "Team A"},
		{"team/sub/b.json", "team", "Team B"},
	}
	if len(report.Dashboards) != len(want) {
		t.Fatalf("provisioned %+v, want %d dashboards", report.Dashboards, len(want))
	}
	for i, w := range want {
		got := report.Dashboards[i]
		if got.Path != w.path || got.Folder != w.folder || got.LastError != "" {
			t.Errorf("entry %d = %+v, want path %s in %s", i, got, w.path, w.folder)
		}
		dash, ver := r.latest(t, got.DashboardID)
		if dash.Title != w.title || ver.Number != 1 || dash.Folder.ID != r.folderID(t, w.folder) {
			t.Errorf("%s: title %q, version %d, folder %s", w.path, dash.Title, ver.Number, dash.Folder.ID.String())
		}
		metas := r.versions(t, got.DashboardID)
		if len(metas) != 1 || metas[0].Summary != "Provisioned from "+w.path || metas[0].Author.Name != "Provisioning" || metas[0].Author.Kind != "service_account" || metas[0].Origin != "api" {
			t.Errorf("%s: versions %+v", w.path, metas)
		}
	}
}

func TestRunIsIdempotentForUnchangedFiles(t *testing.T) {
	r := newRig(t)
	r.write(t, "team/a.json", specJSON("Team A", "up"))
	first := entry(t, r.run(t), "team/a.json")
	second := r.run(t)
	again := entry(t, second, "team/a.json")
	if len(second.Dashboards) != 1 || again.DashboardID != first.DashboardID || !again.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatalf("first %+v, second %+v", first, second.Dashboards)
	}
	if metas := r.versions(t, first.DashboardID); len(metas) != 1 {
		t.Fatalf("an unchanged file added versions: %+v", metas)
	}
}

func TestRunChangeAddsExactlyOneVersion(t *testing.T) {
	r := newRig(t)
	r.write(t, "team/a.json", specJSON("Team A", "up"))
	first := entry(t, r.run(t), "team/a.json")
	r.write(t, "team/a.json", specJSON("Team A renamed", "up"))
	changed := entry(t, r.run(t), "team/a.json")
	entry(t, r.run(t), "team/a.json")

	if changed.DashboardID != first.DashboardID || changed.Folder != "team" {
		t.Fatalf("changed entry %+v, first %+v", changed, first)
	}
	dash, ver := r.latest(t, first.DashboardID)
	if ver.Number != 2 || dash.Title != "Team A renamed" {
		t.Fatalf("version %d, title %q", ver.Number, dash.Title)
	}
	metas := r.versions(t, first.DashboardID)
	if len(metas) != 2 || metas[0].Summary != "Provisioned from team/a.json" {
		t.Fatalf("versions %+v", metas)
	}
}

func TestRunErrorRecordsTheFileAndTheOthersStillProvision(t *testing.T) {
	r := newRig(t)
	r.write(t, "a-broken.json", `{"specVersion":"0","title":`)
	r.write(t, "b-failing.json", specJSON("Failing", brokenQuery))
	r.write(t, "c-good.json", specJSON("Good", "up"))

	report := r.run(t)
	if len(report.Dashboards) != 1 || report.Dashboards[0].Path != "c-good.json" {
		t.Fatalf("dashboards %+v", report.Dashboards)
	}
	if len(report.Errors) != 2 {
		t.Fatalf("errors %+v", report.Errors)
	}
	for i, path := range []string{"a-broken.json", "b-failing.json"} {
		got := report.Errors[i]
		if got.Path != path || !strings.HasPrefix(got.Message, "the spec is invalid: ") || got.SeenAt.IsZero() {
			t.Errorf("error %d = %+v, want %s", i, got, path)
		}
	}
	if !strings.Contains(report.Errors[1].Message, "/panels/") {
		t.Errorf("the failing query is not named by its panel: %q", report.Errors[1].Message)
	}

	r.write(t, "b-failing.json", specJSON("Fixed", "up"))
	r.remove(t, "a-broken.json")
	report = r.run(t)
	if len(report.Errors) != 0 || len(report.Dashboards) != 2 {
		t.Fatalf("after the fix and removal: dashboards %+v, errors %+v", report.Dashboards, report.Errors)
	}
	if dash, _ := r.latest(t, entry(t, report, "b-failing.json").DashboardID); dash.Title != "Fixed" {
		t.Fatalf("fixed title %q", dash.Title)
	}
}

func TestRunErrorOnAChangedFileKeepsTheSavedDashboard(t *testing.T) {
	r := newRig(t)
	original := specJSON("Team A", "up")
	r.write(t, "team/a.json", original)
	saved := entry(t, r.run(t), "team/a.json")

	r.write(t, "team/a.json", specJSON("Team A", brokenQuery))
	report := r.run(t)
	failed := entry(t, report, "team/a.json")
	if !strings.HasPrefix(failed.LastError, "the spec is invalid: ") || len(report.Errors) != 0 {
		t.Fatalf("entry %+v, errors %+v", failed, report.Errors)
	}
	if _, ver := r.latest(t, saved.DashboardID); ver.Number != 1 {
		t.Fatalf("a failing save added version %d", ver.Number)
	}

	r.write(t, "team/a.json", original)
	restored := entry(t, r.run(t), "team/a.json")
	if restored.LastError != "" {
		t.Fatalf("the error stayed after the file went back to its saved content: %+v", restored)
	}
	if _, ver := r.latest(t, saved.DashboardID); ver.Number != 1 {
		t.Fatalf("going back to the saved content added version %d", ver.Number)
	}
}

func TestRunErrorMessageIsCappedAtWholeCharacters(t *testing.T) {
	r := newRig(t)
	panels := make([]string, 120)
	for i := range panels {
		panels[i] = fmt.Sprintf(`{"id":"p%d","type":"no-such-type-é","title":"é","gridPos":{"x":0,"y":%d,"w":12,"h":8}}`, i, i*8)
	}
	r.write(t, "many.json", `{"specVersion":"0","title":"Many","panels":[`+strings.Join(panels, ",")+`]}`)

	report := r.run(t)
	if len(report.Errors) != 1 {
		t.Fatalf("errors %+v", report.Errors)
	}
	message := report.Errors[0].Message
	if len(message) > 2048 || len(message) < 2040 || !utf8.ValidString(message) || !strings.HasPrefix(message, "the spec is invalid: ") {
		t.Fatalf("message of %d bytes, valid UTF-8 %t: %q", len(message), utf8.ValidString(message), message)
	}
}

func TestRunKeepsTheDashboardOfARemovedFileAndClearsItsError(t *testing.T) {
	r := newRig(t)
	r.write(t, "team/a.json", specJSON("Team A", "up"))
	saved := entry(t, r.run(t), "team/a.json")
	r.write(t, "team/a.json", specJSON("Team A", brokenQuery))
	if failed := entry(t, r.run(t), "team/a.json"); failed.LastError == "" {
		t.Fatalf("a failing change left no last_error: %+v", failed)
	}
	r.remove(t, "team/a.json")

	kept := entry(t, r.run(t), "team/a.json")
	if kept.DashboardID != saved.DashboardID || kept.LastError != "" {
		t.Fatalf("kept %+v, saved %+v; want the same dashboard with its error cleared", kept, saved)
	}
	if dash, _ := r.latest(t, saved.DashboardID); dash.Title != "Team A" {
		t.Fatalf("title %q", dash.Title)
	}
}

func TestRunErrorWhenOffOrTheDirectoryIsMissing(t *testing.T) {
	r := newRig(t)
	logger := slog.New(slog.DiscardHandler)
	off := provision.New(logger, r.dashboards, folders.NewAccess(r.db.DB), r.db.DB, "")
	if err := off.Run(t.Context()); !errors.Is(err, provision.ErrOff) {
		t.Fatalf("Run with no directory = %v, want ErrOff", err)
	}
	missing := provision.New(logger, r.dashboards, folders.NewAccess(r.db.DB), r.db.DB, filepath.Join(r.dir, "missing"))
	if err := missing.Run(t.Context()); !errors.Is(err, provision.ErrDirUnreadable) {
		t.Fatalf("Run on a missing directory = %v, want ErrDirUnreadable", err)
	}
	report, err := off.Report(reading(t.Context()))
	if err != nil || report.Dir != "" || len(report.Dashboards) != 0 || len(report.Errors) != 0 {
		t.Fatalf("report when off = %+v, %v", report, err)
	}
}

func TestWatchProvisionsWhenItStarts(t *testing.T) {
	r := newRig(t)
	r.write(t, "team/a.json", specJSON("Team A", "up"))
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.provisioner.Watch(ctx)
	}()
	deadline := time.Now().Add(10 * time.Second)
	for len(r.report(t).Dashboards) == 0 {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("Watch did not provision the directory at start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
}

func TestRunRecordsOneSystemEventPerSavedFileAndOneUnchangedEventWhenNothingChanged(t *testing.T) {
	r := newRig(t)
	r.write(t, "a.json", specJSON("A", "up"))
	r.write(t, "Shop/b.json", specJSON("B", "up"))
	report := r.run(t)

	got := r.db.AuditEvents(t, "provision.apply")
	if len(got) != 2 {
		t.Fatalf("events after the first run = %+v, want one per file", got)
	}
	for _, e := range got {
		want := dbtest.AuditEvent{Action: "provision.apply", Outcome: "success", ActorKind: "system", Initiator: "system", Origin: "job", TargetKind: "dashboard", TargetID: e.TargetID}
		if fmt.Sprint(e) != fmt.Sprint(want) || e.TargetID != entry(t, report, "a.json").DashboardID && e.TargetID != entry(t, report, "Shop/b.json").DashboardID {
			t.Errorf("event %+v, want %+v on a provisioned dashboard", e, want)
		}
	}

	r.run(t)
	got = r.db.AuditEvents(t, "provision.apply")
	last := got[len(got)-1]
	if len(got) != 3 || last.Outcome != "success" || last.Reason != "unchanged" || last.TargetKind != "dashboard" || last.TargetID != "" || last.ActorKind != "system" {
		t.Errorf("events after an unchanged run = %+v", got)
	}
}

func TestAWatchRunThatChangesNothingRecordsNothingWhileARequestedRunRecordsUnchanged(t *testing.T) {
	r := newRig(t)
	r.write(t, "a.json", specJSON("A", "up"))
	r.run(t)
	before := len(r.db.AuditEvents(t, "provision.apply"))

	if err := r.provisioner.RunAsWatch(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n := len(r.db.AuditEvents(t, "provision.apply")); n != before {
		t.Errorf("events after a watch run that changed nothing = %d, want %d", n, before)
	}

	r.run(t)
	got := r.db.AuditEvents(t, "provision.apply")
	if len(got) != before+1 || got[len(got)-1].Reason != "unchanged" {
		t.Errorf("events after a requested run that changed nothing = %+v, want one unchanged event more", got)
	}
}

func TestAWatchRunThatSavesAFileStillRecordsIt(t *testing.T) {
	r := newRig(t)
	r.write(t, "a.json", specJSON("A", "up"))
	if err := r.provisioner.RunAsWatch(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := r.db.AuditEvents(t, "provision.apply"); len(got) != 1 || got[0].Reason != "" {
		t.Errorf("events %+v, want one for the saved file", got)
	}
}

func TestRunByAPersonRecordsThatPerson(t *testing.T) {
	r := newRig(t)
	person := uuid.New()
	ctx := audit.Open(t.Context(), audit.Scope{
		RequestID: uuid.New(),
		Actor:     audit.Actor{Kind: audit.ActorUser, ID: uuid.NullUUID{UUID: person, Valid: true}, Name: "Ada", Role: "admin"},
		Initiator: audit.InitiatorPerson,
		Origin:    audit.Origin{Kind: audit.OriginAPI},
	})
	r.write(t, "a.json", specJSON("A", "up"))
	if err := r.provisioner.Run(ctx); err != nil {
		t.Fatal(err)
	}
	got := r.db.AuditEvents(t, "provision.apply")
	if len(got) != 1 || got[0].ActorID != person.String() || got[0].Initiator != "person" || got[0].Origin != "api" {
		t.Errorf("events %+v, want one by the person through the api", got)
	}
}

func TestAFileThatFailsToProvisionRecordsNoEvent(t *testing.T) {
	r := newRig(t)
	r.write(t, "broken.json", specJSON("Broken", brokenQuery))
	if report := r.run(t); len(report.Errors) != 1 {
		t.Fatalf("report %+v, want one file error", report)
	}
	if n := len(r.db.AuditEvents(t, "provision.apply")); n != 0 {
		t.Errorf("events = %d, want 0", n)
	}
}
