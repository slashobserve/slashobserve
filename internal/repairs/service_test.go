package repairs_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/repairs"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

func TestListIncludesUnverifiedPanelsVariablesAndPlaceholders(t *testing.T) {
	r := newRig(t)
	panel := r.failing(t, r.shared, "Panel")
	variable := r.dashboard(t, r.shared, "Variable")
	r.setUnverified(t, variable, `{"panels":[],"variables":[{"name":"pod","issues":[]}]}`)
	top := r.dashboard(t, r.shared, "Top placeholder")
	r.setPanels(t, top, `[{"id":"p1","type":"placeholder"},{"id":"p2","type":"placeholder"},{"id":"ok","type":"timeseries"}]`)
	inRow := r.dashboard(t, r.shared, "Row placeholder")
	r.setPanels(t, inRow, `[{"id":"r","type":"row","panels":[{"id":"p","type":"placeholder"},{"id":"ok","type":"stat"}]}]`)
	clean := r.dashboard(t, r.shared, "Clean")

	got := byID(r.list(t, r.editor, repairs.ListParams{}).Items)
	if _, ok := got[clean.String()]; ok {
		t.Errorf("clean dashboard is listed")
	}
	want := map[string][3]int{
		panel.String():    {1, 0, 0},
		variable.String(): {0, 1, 0},
		top.String():      {0, 0, 2},
		inRow.String():    {0, 0, 1},
	}
	if len(got) != len(want) {
		t.Fatalf("listed %d dashboards, want %d", len(got), len(want))
	}
	for id, counts := range want {
		it := got[id]
		if c := [3]int{it.UnverifiedPanels, it.UnverifiedVariables, it.PlaceholderPanels}; c != counts {
			t.Errorf("%s counts %v, want %v", it.Title, c, counts)
		}
	}
	it := got[panel.String()]
	if it.URL != "https://observe.example.com/d/"+panel.String()+"/panel" || it.Folder.Title != "Imported" || it.Folder.ID != r.shared || it.Repair != nil {
		t.Errorf("item %+v", it)
	}
}

func TestListHandlesNullLists(t *testing.T) {
	r := newRig(t)
	placeholder := r.dashboard(t, r.shared, "Placeholder")
	r.setPanels(t, placeholder, `[{"id":"p","type":"placeholder"}]`)
	nullPanels := r.dashboard(t, r.shared, "Null panels")
	r.setUnverified(t, nullPanels, `{"panels":null,"variables":[{"name":"pod","issues":[]}]}`)

	got := byID(r.list(t, r.editor, repairs.ListParams{}).Items)
	if it := got[placeholder.String()]; it.UnverifiedPanels != 0 || it.UnverifiedVariables != 0 || it.PlaceholderPanels != 1 {
		t.Errorf("placeholder dashboard %+v", it)
	}
	if it := got[nullPanels.String()]; it.UnverifiedPanels != 0 || it.UnverifiedVariables != 1 {
		t.Errorf("null panels dashboard %+v", it)
	}
}

func TestListHidesFoldersTheCallerCannotSee(t *testing.T) {
	r := newRig(t)
	secret := r.folder(t, "Secret")
	hidden := r.failing(t, secret, "Hidden")
	r.restrict(t, secret)
	visible := r.failing(t, r.shared, "Visible")

	got := byID(r.list(t, r.other, repairs.ListParams{}).Items)
	if _, ok := got[hidden.String()]; ok {
		t.Errorf("dashboard in a restricted folder is listed")
	}
	if _, ok := got[visible.String()]; !ok {
		t.Errorf("visible dashboard is missing")
	}
	if _, err := r.svc.List(t.Context(), r.other, repairs.ListParams{FolderID: secret, Limit: 10}); !errors.Is(err, repairs.ErrFolderNotFound) {
		t.Errorf("hidden folder filter error %v, want ErrFolderNotFound", err)
	}
	if _, err := r.svc.List(t.Context(), r.other, repairs.ListParams{Cursor: "!!", Limit: 10}); !errors.Is(err, repairs.ErrInvalidCursor) {
		t.Errorf("bad cursor error %v, want ErrInvalidCursor", err)
	}
}

func TestListPagesWithTheCursor(t *testing.T) {
	r := newRig(t)
	for _, title := range []string{"A", "B", "C"} {
		r.failing(t, r.shared, title)
	}
	first := r.list(t, r.editor, repairs.ListParams{Limit: 2})
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page %d items, cursor %q", len(first.Items), first.NextCursor)
	}
	second := r.list(t, r.editor, repairs.ListParams{Limit: 2, Cursor: first.NextCursor})
	if len(second.Items) != 1 || second.NextCursor != "" {
		t.Fatalf("second page %d items, cursor %q", len(second.Items), second.NextCursor)
	}
	seen := map[string]bool{}
	for _, it := range append(first.Items, second.Items...) {
		if seen[it.ID] {
			t.Errorf("%s listed twice", it.Title)
		}
		seen[it.ID] = true
	}
}

func TestListShowsTheLatestRepair(t *testing.T) {
	r := newRig(t)
	id := r.failing(t, r.shared, "Board")
	out := r.request(t, repairs.Target{DashboardIDs: []pgtype.UUID{id}})
	if len(out.Queued) != 1 {
		t.Fatalf("outcome %+v", out)
	}
	r.db.Exec(t, "UPDATE agent_requests SET status = 'failed', finished_at = now() WHERE id = $1", out.Queued[0].RequestID)
	if _, err := r.revisions.Submit(t.Context(), revisions.EnqueueParams{DashboardID: id, Source: revisions.SourcePrompt, Prompt: "add a panel", BaseVersion: 1, By: r.editor}); err != nil {
		t.Fatal(err)
	}
	it := byID(r.list(t, r.editor, repairs.ListParams{}).Items)[id.String()]
	if it.Repair == nil || it.Repair.ID != out.Queued[0].RequestID || it.Repair.Status != "failed" || it.Repair.FinishedAt == nil {
		t.Fatalf("repair %+v, want the failed repair %s", it.Repair, out.Queued[0].RequestID)
	}
}

func TestListShowsTheQueuedRepair(t *testing.T) {
	r := newRig(t)
	id := r.failing(t, r.shared, "Board")
	out := r.request(t, repairs.Target{DashboardIDs: []pgtype.UUID{id}})
	it := byID(r.list(t, r.editor, repairs.ListParams{}).Items)[id.String()]
	if it.Repair == nil || it.Repair.ID != out.Queued[0].RequestID || it.Repair.Status != "queued" || it.Repair.FinishedAt != nil {
		t.Fatalf("repair %+v", it.Repair)
	}
}

func TestListShowsLastImportAt(t *testing.T) {
	r := newRig(t)
	imported := r.failing(t, r.shared, "Imported")
	copied := r.failing(t, r.shared, "Copy")
	r.db.Exec(t, `INSERT INTO imports (dashboard_id, version, source, importer_version, original, mapping, summary, datasources, report,
		title, created_by_kind, created_by_id, created_by_name)
		VALUES ($1, 1, 'grafana', 1, '\x7b7d', '{}', '{}', '[]', '{}', 'Imported', 'user', $2, 'eda')`, imported, r.editor.ID)
	var versionAt time.Time
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), "SELECT created_at FROM dashboard_versions WHERE dashboard_id = $1 AND version = 1", imported.String()).Scan(&versionAt); err != nil {
		t.Fatal(err)
	}
	got := byID(r.list(t, r.editor, repairs.ListParams{}).Items)
	if at := got[imported.String()].LastImportAt; at == nil || !at.Equal(versionAt) {
		t.Errorf("imported LastImportAt %v, want %v", at, versionAt)
	}
	if at := got[copied.String()].LastImportAt; at != nil {
		t.Errorf("copy LastImportAt %v, want nil", at)
	}
}

func TestRequestSkipsADashboardWithAQueuedOrRunningRevision(t *testing.T) {
	r := newRig(t)
	queued := r.failing(t, r.shared, "Queued")
	running := r.failing(t, r.shared, "Running")
	if _, err := r.revisions.Submit(t.Context(), revisions.EnqueueParams{DashboardID: queued, Source: revisions.SourcePrompt, Prompt: "revise", BaseVersion: 1, By: r.other}); err != nil {
		t.Fatal(err)
	}
	rev, err := r.revisions.Submit(t.Context(), revisions.EnqueueParams{DashboardID: running, Source: revisions.SourcePrompt, Prompt: "revise", BaseVersion: 1, By: r.other})
	if err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, "UPDATE agent_requests SET status = 'running', started_at = now() WHERE id = $1", rev.ID)

	out := r.request(t, repairs.Target{DashboardIDs: []pgtype.UUID{queued, running}})
	if len(out.Queued) != 0 {
		t.Errorf("queued %+v, want none", out.Queued)
	}
	got := reasons(out)
	if got[queued.String()] != repairs.ReasonActive || got[running.String()] != repairs.ReasonActive {
		t.Errorf("skipped %+v, want both active", out.Skipped)
	}
}

func TestRequestSkipsPlaceholderOnlyDashboards(t *testing.T) {
	r := newRig(t)
	id := r.dashboard(t, r.shared, "Placeholders")
	r.setPanels(t, id, `[{"id":"p","type":"placeholder"}]`)
	r.setUnverified(t, id, `{"panels":null,"variables":null}`)
	out := r.request(t, repairs.Target{DashboardIDs: []pgtype.UUID{id}})
	if len(out.Queued) != 0 || reasons(out)[id.String()] != repairs.ReasonNothingToRepair {
		t.Errorf("outcome %+v, want nothing_to_repair", out)
	}
}

func TestRequestSkipsMissingAndHiddenDashboards(t *testing.T) {
	r := newRig(t)
	secret := r.folder(t, "Secret")
	hidden := r.failing(t, secret, "Hidden")
	r.restrict(t, secret)
	missing := pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true}
	out, err := r.svc.Request(t.Context(), r.other, repairs.Target{DashboardIDs: []pgtype.UUID{missing, hidden}})
	if err != nil {
		t.Fatal(err)
	}
	got := reasons(out)
	if len(out.Queued) != 0 || got[missing.String()] != repairs.ReasonNotFound || got[hidden.String()] != repairs.ReasonNotFound {
		t.Errorf("outcome %+v, want both not_found", out)
	}
}

func TestRequestSkipsDashboardsTheCallerCannotEdit(t *testing.T) {
	r := newRig(t)
	id := r.failing(t, r.shared, "Board")
	out, err := r.svc.Request(t.Context(), r.viewer, repairs.Target{DashboardIDs: []pgtype.UUID{id}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Queued) != 0 || reasons(out)[id.String()] != repairs.ReasonForbidden {
		t.Errorf("outcome %+v, want forbidden_role", out)
	}
}

func TestRequestDeduplicatesIDs(t *testing.T) {
	r := newRig(t)
	id := r.failing(t, r.shared, "Board")
	out := r.request(t, repairs.Target{DashboardIDs: []pgtype.UUID{id, id}})
	if len(out.Queued) != 1 || len(out.Skipped) != 0 {
		t.Errorf("outcome %+v, want one queued", out)
	}
}

func TestRequestAllQueuesEveryListedDashboardOnce(t *testing.T) {
	r := newRig(t)
	ids := []pgtype.UUID{r.failing(t, r.shared, "A"), r.failing(t, r.shared, "B"), r.failing(t, r.shared, "C"), r.failing(t, r.shared, "D")}
	r.dashboard(t, r.shared, "Clean")
	if _, err := r.revisions.Submit(t.Context(), revisions.EnqueueParams{DashboardID: ids[0], Source: revisions.SourcePrompt, Prompt: "revise", BaseVersion: 1, By: r.other}); err != nil {
		t.Fatal(err)
	}
	first := r.request(t, repairs.Target{All: true})
	if len(first.Queued) != 3 || len(first.Skipped) != 1 || reasons(first)[ids[0].String()] != repairs.ReasonActive {
		t.Fatalf("first outcome %+v, want three queued and A skipped", first)
	}
	again := r.request(t, repairs.Target{All: true})
	if len(again.Queued) != 0 || len(again.Skipped) != 4 {
		t.Fatalf("second outcome %+v, want four skipped", again)
	}
	for _, s := range again.Skipped {
		if s.Reason != repairs.ReasonActive {
			t.Errorf("%s skipped as %s, want active", s.DashboardID, s.Reason)
		}
	}
}

func TestRequestAllHonoursTheFolder(t *testing.T) {
	r := newRig(t)
	ops := r.folder(t, "Ops")
	inOps := r.failing(t, ops, "In ops")
	r.failing(t, r.shared, "Elsewhere")
	out := r.request(t, repairs.Target{All: true, FolderID: ops})
	if len(out.Queued) != 1 || out.Queued[0].DashboardID != inOps.String() || len(out.Skipped) != 0 {
		t.Errorf("outcome %+v, want only the Ops dashboard", out)
	}
	secret := r.folder(t, "Secret")
	r.restrict(t, secret)
	if _, err := r.svc.Request(t.Context(), r.other, repairs.Target{All: true, FolderID: secret}); !errors.Is(err, repairs.ErrFolderNotFound) {
		t.Errorf("hidden folder error %v, want ErrFolderNotFound", err)
	}
}

func TestRequestQueuesWithTheFixedPromptAndLatestVersion(t *testing.T) {
	r := newRig(t)
	id := r.dashboard(t, r.shared, "Board")
	if _, _, _, err := r.dashboards.AddVersion(t.Context(), r.editor, id, 1, boardSpec("Board v2"), "edit"); err != nil {
		t.Fatal(err)
	}
	r.setUnverified(t, id, onePanelUnverified)
	out := r.request(t, repairs.Target{DashboardIDs: []pgtype.UUID{id}})
	if len(out.Queued) != 1 {
		t.Fatalf("outcome %+v", out)
	}
	var source, prompt, requester, status string
	var base int
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), "SELECT source, prompt, base_version, requested_by_id::text, status FROM agent_requests WHERE id = $1", out.Queued[0].RequestID).
		Scan(&source, &prompt, &base, &requester, &status)
	if err != nil {
		t.Fatal(err)
	}
	if source != "prompt" || prompt != repairs.Prompt || base != 2 || requester != r.editor.ID || status != "queued" {
		t.Errorf("stored request %s %q %d %s %s", source, prompt, base, requester, status)
	}
}

func TestRequestStopsAtTheRequesterCap(t *testing.T) {
	r := newRig(t)
	busy := r.dashboard(t, r.shared, "Busy")
	r.db.Exec(t, `INSERT INTO agent_requests (dashboard_id, source, prompt, base_version, requested_by_kind, requested_by_id, requested_by_name)
		SELECT $1, 'prompt', $2, 1, 'user', $3, 'eda' FROM generate_series(1, $4::integer)`, busy, repairs.Prompt, r.editor.ID, repairs.MaxActivePerRequester)
	id := r.failing(t, r.shared, "Board")
	out := r.request(t, repairs.Target{DashboardIDs: []pgtype.UUID{id}})
	if len(out.Queued) != 0 || reasons(out)[id.String()] != repairs.ReasonQueueFull {
		t.Fatalf("outcome %+v, want queue_full", out)
	}
	theirs, err := r.svc.Request(t.Context(), r.other, repairs.Target{DashboardIDs: []pgtype.UUID{id}})
	if err != nil {
		t.Fatal(err)
	}
	if len(theirs.Queued) != 1 {
		t.Errorf("another requester's outcome %+v, want queued", theirs)
	}
}

func TestRequestWithoutAnAgentIsUnavailable(t *testing.T) {
	r := newRig(t)
	id := r.failing(t, r.shared, "Board")
	if _, err := r.noAgent.Request(t.Context(), r.editor, repairs.Target{DashboardIDs: []pgtype.UUID{id}}); !errors.Is(err, revisions.ErrUnavailable) {
		t.Errorf("error %v, want ErrUnavailable", err)
	}
}
