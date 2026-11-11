package findings_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/folders/foldertest"
)

func (r rig) saveErr(t *testing.T, by auth.Principal, in findings.SaveInput) error {
	t.Helper()
	in.Title, in.Markdown, in.From, in.To = "t", "m", windowTo.Add(-1), windowTo
	_, _, err := r.service.Save(t.Context(), by, in)
	return err
}

func TestAFindingWithoutADashboardIsFiledInTheSaversMyDashboards(t *testing.T) {
	r := newRig(t)
	saved := r.save(t, editor, findings.SaveInput{})
	personal := r.personalFolder(t, editor)
	if saved.FolderID != personal.String() {
		t.Fatalf("folder = %q, want My Dashboards %s", saved.FolderID, personal)
	}
	got, err := r.service.Get(t.Context(), editor, mustUUID(t, saved.ID))
	if err != nil {
		t.Fatal(err)
	}
	if got.Folder == nil || !got.Folder.Personal || got.Folder.ID != personal || got.Abilities != folders.AbilitiesFor(folders.LevelEdit) {
		t.Fatalf("folder %+v, abilities %+v", got.Folder, got.Abilities)
	}
}

func TestAFindingOnADashboardKeepsNoFolderOfItsOwn(t *testing.T) {
	r := newRig(t)
	saved := r.save(t, editor, findings.SaveInput{DashboardID: r.open})
	got, err := r.service.Get(t.Context(), viewer, mustUUID(t, saved.ID))
	if err != nil {
		t.Fatal(err)
	}
	if saved.FolderID != "" || got.Folder == nil || got.Folder.General != true || got.Abilities != folders.AbilitiesFor(folders.LevelView) {
		t.Fatalf("own folder %q, effective folder %+v, abilities %+v", saved.FolderID, got.Folder, got.Abilities)
	}
}

func TestAFindingFromAnAccountThatCannotCreateAnywhereIsUnfiled(t *testing.T) {
	r := newRig(t)
	saved := r.save(t, pipeline, findings.SaveInput{})
	got, err := r.service.Get(t.Context(), pipeline, mustUUID(t, saved.ID))
	if err != nil {
		t.Fatal(err)
	}
	if saved.FolderID != "" || got.Folder != nil || got.Abilities != folders.AbilitiesFor(folders.LevelEdit) {
		t.Fatalf("folder %q and %+v, abilities %+v", saved.FolderID, got.Folder, got.Abilities)
	}
}

func TestAnEditorKeyWithNoMyDashboardsFilesInGeneral(t *testing.T) {
	r := newRig(t)
	key := auth.Principal{Kind: auth.KindServiceAccount, ID: "77777777-7777-4777-8777-777777777777", Name: "ci", Role: auth.RoleEditor}
	r.db.Exec(t, `INSERT INTO service_accounts (id, name) VALUES ($1::uuid, 'ci')`, key.ID)
	saved := r.save(t, key, findings.SaveInput{})
	got, err := r.service.Get(t.Context(), viewer, mustUUID(t, saved.ID))
	if err != nil || got.Folder == nil || !got.Folder.General || got.Abilities != folders.AbilitiesFor(folders.LevelView) {
		t.Fatalf("a viewer reads a key's finding in General: folder %+v, abilities %+v, %v", got.Folder, got.Abilities, err)
	}
}

func TestSavingInARequestedFolderNeedsCreateThere(t *testing.T) {
	r := newRig(t)
	shared := r.folder(t, "Incidents")
	hidden := foldertest.Restricted(t, r.db, "Payroll", payrollGroup)

	filed := r.save(t, editor, findings.SaveInput{FolderID: shared})
	if filed.FolderID != shared.String() {
		t.Fatalf("folder = %q, want %s", filed.FolderID, shared)
	}
	cases := []struct {
		name string
		by   auth.Principal
		in   findings.SaveInput
		want error
	}{
		{"a viewer of the folder", viewer, findings.SaveInput{FolderID: shared}, folders.ErrForbidden},
		{"a folder that does not exist", editor, findings.SaveInput{FolderID: pgtype.UUID{Bytes: [16]byte{15: 9}, Valid: true}}, findings.ErrFolderNotFound},
		{"a folder the caller cannot see", otherEditor, findings.SaveInput{FolderID: hidden}, findings.ErrFolderNotFound},
		{"another person's My Dashboards", otherEditor, findings.SaveInput{FolderID: r.personalFolder(t, editor)}, findings.ErrFolderNotFound},
		{"a dashboard and a folder", editor, findings.SaveInput{DashboardID: r.open, FolderID: shared}, findings.ErrPlacement},
	}
	for _, c := range cases {
		if err := r.saveErr(t, c.by, c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
}

func TestListingByFolderShowsOnlyFindingsFiledThere(t *testing.T) {
	r := newRig(t)
	shared := r.folder(t, "Incidents")
	hidden := foldertest.Restricted(t, r.db, "Payroll", payrollGroup)
	filed := r.save(t, editor, findings.SaveInput{FolderID: shared})
	r.save(t, editor, findings.SaveInput{})
	r.save(t, editor, findings.SaveInput{DashboardID: r.open})
	r.save(t, otherEditor, findings.SaveInput{FolderID: shared})

	rows, _, err := r.service.List(t.Context(), viewer, findings.ListFilter{FolderID: shared}, 10, "")
	if err != nil || len(rows) != 2 || rows[1].ID != filed.ID {
		t.Fatalf("folder list: %+v, %v", rows, err)
	}
	if _, _, err := r.service.List(t.Context(), viewer, findings.ListFilter{FolderID: hidden}, 10, ""); !errors.Is(err, findings.ErrNotFound) {
		t.Errorf("a folder the caller cannot see: %v", err)
	}
	if _, _, err := r.service.List(t.Context(), viewer, findings.ListFilter{FolderID: shared, DashboardID: r.open}, 10, ""); !errors.Is(err, findings.ErrPlacement) {
		t.Errorf("both filters: %v", err)
	}
	own, _, err := r.service.List(t.Context(), editor, findings.ListFilter{}, 10, "")
	if err != nil || len(own) != 3 {
		t.Errorf("own findings: %d, %v, want the editor's 3 and none of the other editor's", len(own), err)
	}
}

func TestListingOwnFindingsDropsOnesFiledInAFolderThatTurnedHidden(t *testing.T) {
	r := newRig(t)
	folder := r.folder(t, "Incidents")
	r.save(t, editor, findings.SaveInput{FolderID: folder})
	kept := r.save(t, editor, findings.SaveInput{})
	r.db.Exec(t, `INSERT INTO folder_permissions (folder_id, group_name, level) VALUES ($1, 'oncall', 'view')`, folder)

	rows, _, err := r.service.List(t.Context(), otherEditor, findings.ListFilter{}, 10, "")
	if err != nil || len(rows) != 0 {
		t.Fatalf("another editor's own list: %+v, %v", rows, err)
	}
	rows, _, err = r.service.List(t.Context(), editor, findings.ListFilter{}, 10, "")
	if err != nil || len(rows) != 1 || rows[0].ID != kept.ID {
		t.Fatalf("own list: %+v, %v, want only the finding in My Dashboards", rows, err)
	}
}

func TestFindingEnvelopeNamesTheFolderAndWhatTheCallerMayDo(t *testing.T) {
	d := newDocumented(t)
	shared := d.folder(t, "Incidents")
	filed := d.save(t, editor, findings.SaveInput{FolderID: shared})
	onDashboard := d.save(t, editor, findings.SaveInput{DashboardID: d.open})
	unfiled := d.save(t, pipeline, findings.SaveInput{})
	personal := d.save(t, editor, findings.SaveInput{})

	type envelope struct {
		Finding struct {
			FolderID    *string `json:"folder_id"`
			DashboardID *string `json:"dashboard_id"`
		} `json:"finding"`
		Folder *struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			General  bool   `json:"general"`
			Personal bool   `json:"personal"`
		} `json:"folder"`
		Permissions struct {
			Comment bool `json:"comment"`
			Edit    bool `json:"edit"`
		} `json:"permissions"`
	}
	read := func(by auth.Principal, id string) envelope {
		var got envelope
		rec := d.call(t, by, http.MethodGet, "/api/v1/findings/"+id, http.StatusOK)
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("%s: %v", rec.Body, err)
		}
		return got
	}

	if got := read(viewer, filed.ID); got.Finding.FolderID == nil || *got.Finding.FolderID != shared.String() || got.Folder == nil || got.Folder.Title != "Incidents" || got.Permissions.Edit {
		t.Errorf("a viewer of a shared folder: %+v", got)
	}
	if got := read(editor, filed.ID); !got.Permissions.Edit || !got.Permissions.Comment {
		t.Errorf("the creator: %+v", got)
	}
	if got := read(viewer, onDashboard.ID); got.Finding.FolderID != nil || got.Finding.DashboardID == nil || got.Folder == nil || !got.Folder.General {
		t.Errorf("a finding on a dashboard: %+v", got)
	}
	if got := read(admin, unfiled.ID); got.Folder != nil || got.Finding.FolderID != nil || !got.Permissions.Edit {
		t.Errorf("an unfiled finding read by an admin: %+v", got)
	}
	if got := read(editor, personal.ID); got.Folder == nil || !got.Folder.Personal {
		t.Errorf("a finding in My Dashboards: %+v", got)
	}
	d.call(t, admin, http.MethodGet, "/api/v1/findings/"+personal.ID, http.StatusNotFound)
	d.call(t, otherEditor, http.MethodGet, "/api/v1/findings/"+unfiled.ID, http.StatusNotFound)
}

func TestListingFindingsByFolderOverHTTP(t *testing.T) {
	d := newDocumented(t)
	shared := d.folder(t, "Incidents")
	hidden := foldertest.Restricted(t, d.db, "Payroll", payrollGroup)
	filed := d.save(t, editor, findings.SaveInput{FolderID: shared})
	d.save(t, editor, findings.SaveInput{})
	d.save(t, editor, findings.SaveInput{DashboardID: d.open})

	rec := d.call(t, viewer, http.MethodGet, "/api/v1/findings?folder_id="+shared.String(), http.StatusOK)
	var list struct {
		Findings []struct {
			ID string `json:"id"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Findings) != 1 || list.Findings[0].ID != filed.ID {
		t.Fatalf("list %s, %v", rec.Body, err)
	}
	refusals := []struct {
		name string
		path string
		want int
		code string
	}{
		{"a folder id that is not a UUID", "/api/v1/findings?folder_id=nope", http.StatusBadRequest, "invalid_request"},
		{"a folder and a dashboard together", "/api/v1/findings?folder_id=" + shared.String() + "&dashboard_id=" + d.open.String(), http.StatusBadRequest, "invalid_request"},
		{"a folder the caller cannot see", "/api/v1/findings?folder_id=" + hidden.String(), http.StatusNotFound, "not_found"},
	}
	for _, c := range refusals {
		t.Run(c.name, func(t *testing.T) {
			if code := errorCode(t, d.call(t, viewer, http.MethodGet, c.path, c.want)); code != c.code {
				t.Fatalf("code %q, want %q", code, c.code)
			}
		})
	}
}
