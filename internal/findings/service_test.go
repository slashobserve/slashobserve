package findings_test

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbtest"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

const payrollGroup = "payroll-team"

var (
	editor      = auth.Principal{Kind: auth.KindUser, ID: "11111111-1111-4111-8111-111111111111", Name: "Eve", Role: auth.RoleEditor}
	otherEditor = auth.Principal{Kind: auth.KindUser, ID: "22222222-2222-4222-8222-222222222222", Name: "Ed", Role: auth.RoleEditor}
	viewer      = auth.Principal{Kind: auth.KindUser, ID: "33333333-3333-4333-8333-333333333333", Name: "Vic", Role: auth.RoleViewer}
	admin       = auth.Principal{Kind: auth.KindUser, ID: "55555555-5555-4555-8555-555555555555", Name: "Root", Role: auth.RoleAdmin}
)

var windowTo = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

type rig struct {
	db      *dbtest.Database
	service *findings.Service
	open    pgtype.UUID
}

func newRig(t *testing.T) rig {
	t.Helper()
	d := dbtest.New(t)
	access := folders.NewAccess(d.DB)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := rig{db: d, service: findings.New(d.DB, access, annotations.New(d.DB, access), nil, logger)}
	r.open = r.dashboard(t, "Checkout")
	return r
}

func (r rig) dashboard(t *testing.T, title string) pgtype.UUID {
	t.Helper()
	var raw string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO dashboards (title, latest_version, created_by_kind, created_by_id, created_by_name, created_via) VALUES ($1, 1, 'user', gen_random_uuid(), 'Seed', 'direct') RETURNING id::text`, title).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return mustUUID(t, raw)
}

func (r rig) request(t *testing.T, by auth.Principal, dashboardID pgtype.UUID) pgtype.UUID {
	t.Helper()
	var raw string
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), `INSERT INTO agent_requests (source, prompt, requested_by_kind, requested_by_id, requested_by_name, dashboard_id, context_from, context_to)
VALUES ('investigate', 'why', $1, $2::uuid, $3, $4, $5::timestamptz - interval '1 hour', $5::timestamptz) RETURNING id::text`,
		string(by.Kind), by.ID, by.Name, dashboardID, windowTo).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return mustUUID(t, raw)
}

func (r rig) save(t *testing.T, by auth.Principal, in findings.SaveInput) findings.Finding {
	t.Helper()
	if in.Markdown == "" {
		in.Markdown = "## Checkout errors come from checkout-0\nThe 5xx rate rose at 09:40."
	}
	if in.Title == "" {
		in.Title = "why"
	}
	if in.To.IsZero() {
		in.From, in.To = windowTo.Add(-time.Hour), windowTo
	}
	f, replayed, err := r.service.Save(t.Context(), by, in)
	if err != nil || replayed {
		t.Fatalf("save: replayed %v, %v", replayed, err)
	}
	return f
}

func mustUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	id, err := db.ParseUUID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestFindingsAreReadableByWhoMayViewThem(t *testing.T) {
	r := newRig(t)
	onDashboard := r.save(t, editor, findings.SaveInput{DashboardID: r.open})
	hidden := r.dashboard(t, "Payroll")
	foldertest.Move(t, r.db, hidden, foldertest.Restricted(t, r.db, "Payroll", payrollGroup))
	onHidden := r.save(t, admin, findings.SaveInput{DashboardID: hidden})
	private := r.save(t, editor, findings.SaveInput{})
	cases := []struct {
		name    string
		by      auth.Principal
		finding findings.Finding
		visible bool
	}{
		{"a viewer of the dashboard", viewer, onDashboard, true},
		{"a viewer without access to the dashboard", viewer, onHidden, false},
		{"the creator of a finding without a dashboard", editor, private, true},
		{"an admin", admin, private, true},
		{"another editor", otherEditor, private, false},
	}
	for _, c := range cases {
		_, err := r.service.Get(t.Context(), c.by, mustUUID(t, c.finding.ID))
		if c.visible && err != nil || !c.visible && !errors.Is(err, findings.ErrNotFound) {
			t.Errorf("%s: %v, want visible %v", c.name, err, c.visible)
		}
	}
}

func TestSavingTwiceForOneRequestReturnsTheFirstFinding(t *testing.T) {
	r := newRig(t)
	request := r.request(t, editor, r.open)
	first := r.save(t, editor, findings.SaveInput{AgentRequestID: request, DashboardID: r.open, Markdown: "first"})
	again, replayed, err := r.service.Save(t.Context(), editor, findings.SaveInput{
		AgentRequestID: request, DashboardID: r.open, Title: "why", Markdown: "second", From: windowTo.Add(-time.Hour), To: windowTo,
	})
	if err != nil || !replayed || again.ID != first.ID || again.Markdown != "first" {
		t.Fatalf("second save: %+v replayed %v, %v", again, replayed, err)
	}
}

func TestSavingOnAHiddenDashboardIsNotFound(t *testing.T) {
	r := newRig(t)
	hidden := r.dashboard(t, "Payroll")
	foldertest.Move(t, r.db, hidden, foldertest.Restricted(t, r.db, "Payroll", payrollGroup))
	_, _, err := r.service.Save(t.Context(), editor, findings.SaveInput{DashboardID: hidden, Title: "t", Markdown: "m", From: windowTo.Add(-time.Hour), To: windowTo})
	if !errors.Is(err, findings.ErrNotFound) {
		t.Fatalf("save on a hidden dashboard: %v", err)
	}
}

func TestListingOwnFindingsPagesAndDropsHiddenDashboards(t *testing.T) {
	r := newRig(t)
	other := r.dashboard(t, "Orders")
	var made []string
	for _, dash := range []pgtype.UUID{{}, r.open, other, {}} {
		made = append(made, r.save(t, editor, findings.SaveInput{DashboardID: dash}).ID)
	}
	r.save(t, otherEditor, findings.SaveInput{})
	foldertest.Move(t, r.db, other, foldertest.Restricted(t, r.db, "Orders", payrollGroup))

	var walked []string
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		page, next, err := r.service.List(t.Context(), editor, pgtype.UUID{}, 1, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range page {
			walked = append(walked, f.ID)
		}
		if next == "" {
			break
		}
		cursor = next
	}
	want := []string{made[3], made[1], made[0]}
	if !slices.Equal(walked, want) {
		t.Fatalf("walked %v, want %v", walked, want)
	}
	if _, _, err := r.service.List(t.Context(), editor, pgtype.UUID{}, 1, "not a cursor"); !errors.Is(err, findings.ErrInvalidCursor) {
		t.Errorf("a bad cursor: %v", err)
	}
	onDashboard, _, err := r.service.List(t.Context(), viewer, r.open, 10, "")
	if err != nil || len(onDashboard) != 1 || onDashboard[0].ID != made[1] {
		t.Errorf("dashboard list: %v, %v", onDashboard, err)
	}
}

func TestPinningCreatesOneAnnotationAtTheWindowEnd(t *testing.T) {
	r := newRig(t)
	f := r.save(t, editor, findings.SaveInput{AgentRequestID: r.request(t, editor, r.open), DashboardID: r.open})
	id := mustUUID(t, f.ID)
	a, created, err := r.service.Pin(t.Context(), editor, id)
	if err != nil || !created {
		t.Fatalf("pin: created %v, %v", created, err)
	}
	if !slices.Equal(a.Tags, []string{findings.PinTag}) || !a.From.Equal(windowTo) || a.DashboardID != r.open.String() {
		t.Errorf("annotation = %+v", a)
	}
	if want := "Checkout errors come from checkout-0 /findings/" + f.ID; a.Text != want {
		t.Errorf("text = %q, want %q", a.Text, want)
	}
	again, created, err := r.service.Pin(t.Context(), viewer, id)
	if err != nil || created || again.ID != a.ID {
		t.Fatalf("second pin: %+v created %v, %v", again, created, err)
	}
	var count int
	if err := r.db.SQLDB(t).QueryRow(`SELECT count(*) FROM annotations`).Scan(&count); err != nil || count != 1 {
		t.Errorf("annotations = %d, %v", count, err)
	}
}

func TestPinningNeedsADashboardAndEdit(t *testing.T) {
	r := newRig(t)
	private := r.save(t, editor, findings.SaveInput{})
	if _, _, err := r.service.Pin(t.Context(), editor, mustUUID(t, private.ID)); !errors.Is(err, findings.ErrNoDashboard) {
		t.Errorf("pin without a dashboard: %v", err)
	}
	shared := r.save(t, editor, findings.SaveInput{DashboardID: r.open})
	if _, _, err := r.service.Pin(t.Context(), viewer, mustUUID(t, shared.ID)); !errors.Is(err, annotations.ErrForbidden) {
		t.Errorf("pin by a viewer: %v", err)
	}
}

func TestSavingAFindingWithoutEvidenceAsADashboardIsRefused(t *testing.T) {
	r := newRig(t)
	f := r.save(t, editor, findings.SaveInput{Evidence: []spec.Panel{}})
	_, _, err := r.service.SaveAsDashboard(t.Context(), editor, mustUUID(t, f.ID))
	if !errors.Is(err, findings.ErrNoEvidence) {
		t.Fatalf("save without evidence: %v", err)
	}
}
