package search

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/savedsearches"
	"github.com/slashobserve/slashobserve/internal/threads"
)

type fixture struct {
	name   string
	kind   Kind
	id     string
	target threads.Target
}

type world struct {
	*rig
	fixtures []fixture
}

func (w *world) add(name string, kind Kind, id string, target threads.Target) {
	w.fixtures = append(w.fixtures, fixture{name: name, kind: kind, id: id, target: target})
}

func buildWorld(t *testing.T) *world {
	t.Helper()
	w := &world{rig: newRig(t)}
	shared := w.folder(t, "Incidents")
	hidden := w.restricted(t, "Payroll")
	mine := w.personal(t, ada)

	dashboard := func(name string, folder pgtype.UUID) string {
		id := w.dashboard(t, "zebra dashboard "+name, folder, "")
		w.add("dashboard "+name, KindDashboard, id, threads.Target{})
		return id
	}
	dGeneral := dashboard("general", pgtype.UUID{})
	dRestricted := dashboard("restricted", hidden)
	dMine := dashboard("mine", mine)

	converted := func(name string, in findingIn) string {
		in.title, in.markdown = "zebra conclusion "+name, "zebra evidence in "+name
		id := w.finding(t, in)
		w.add("observation "+name, KindObservation, id, threads.Target{})
		return id
	}
	oGeneral := converted("on general", findingIn{dashboard: dGeneral})
	oRestricted := converted("on restricted", findingIn{dashboard: dRestricted})
	oMine := converted("on mine", findingIn{dashboard: dMine, by: ada})
	oShared := converted("in shared", findingIn{folder: shared})
	oPersonal := converted("in personal", findingIn{folder: mine, by: ada})
	oKeyed := converted("saved by a key", findingIn{by: keyS})

	aGlobal := w.annotation(t, "", "zebra annotation global")
	w.add("annotation global", KindAnnotation, aGlobal, threads.Target{})
	aRestricted := w.annotation(t, dRestricted, "zebra annotation restricted")
	w.add("annotation restricted", KindAnnotation, aRestricted, threads.Target{})

	comment := func(name string, kind threads.Kind, target string) {
		_, message := w.comment(t, Kind(kind), target, "zebra comment on "+name)
		w.add("comment on "+name, KindComment, message, threads.Target{Kind: kind, ID: mustUUID(t, target)})
	}
	comment("dashboard general", threads.KindDashboard, dGeneral)
	comment("dashboard restricted", threads.KindDashboard, dRestricted)
	comment("dashboard mine", threads.KindDashboard, dMine)
	comment("observation general", threads.KindObservation, oGeneral)
	comment("observation restricted", threads.KindObservation, oRestricted)
	comment("observation mine", threads.KindObservation, oMine)
	comment("observation shared", threads.KindObservation, oShared)
	comment("observation personal", threads.KindObservation, oPersonal)
	comment("observation saved by a key", threads.KindObservation, oKeyed)
	comment("annotation global", threads.KindAnnotation, aGlobal)
	comment("annotation restricted", threads.KindAnnotation, aRestricted)

	logs := `{app="billing"} |= "zebra"`
	w.add("logs search", KindSavedSearch, w.savedSearch(t, "logs", "zebra logs", &logs, shared), threads.Target{})
	w.add("traces search", KindSavedSearch, w.savedSearch(t, "traces", "zebra traces", nil, mine), threads.Target{})
	return w
}

func (w *world) openable(t *testing.T, by auth.Principal, f fixture, savedByReader map[string]bool) bool {
	t.Helper()
	id := mustUUID(t, f.id)
	switch f.kind {
	case KindDashboard:
		got, err := w.access.Dashboard(t.Context(), by, id)
		if err != nil && !isHidden(err) {
			t.Fatalf("%s: %v", f.name, err)
		}
		return err == nil && got.Abilities.View
	case KindObservation:
		_, err := w.observations.Get(t.Context(), by, id)
		if err != nil && !isHidden(err) {
			t.Fatalf("%s: %v", f.name, err)
		}
		return err == nil
	case KindAnnotation:
		_, abilities, err := w.annotations.Access(t.Context(), by, id)
		if err != nil && !isHidden(err) {
			t.Fatalf("%s: %v", f.name, err)
		}
		return err == nil && abilities.View
	case KindComment:
		abilities, err := w.threads.Abilities(t.Context(), by, f.target)
		if err != nil && !isHidden(err) {
			t.Fatalf("%s: %v", f.name, err)
		}
		return err == nil && abilities.View
	}
	return savedByReader[f.id]
}

func (w *world) savedSearchesOf(t *testing.T, by auth.Principal) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	rows, _, err := w.saved.List(t.Context(), by, savedsearches.ListFilter{}, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range rows {
		out[s.ID] = true
	}
	return out
}

func TestSearchShowsOnlyWhatTheReaderCanOpen(t *testing.T) {
	w := buildWorld(t)
	readers := []struct {
		name  string
		by    auth.Principal
		count int
	}{
		{"an admin", admin, 17},
		{"an editor with no grant on the restricted folder", editor, 11},
		{"a viewer granted the restricted folder", payroll, 17},
		{"the owner of My Dashboards", ada, 18},
		{"the key that saved an observation", keyS, 11},
		{"another key", keyT, 11},
	}
	for _, reader := range readers {
		t.Run(reader.name, func(t *testing.T) {
			found := ids(w.search(t, reader.by, "zebra"))
			saved := w.savedSearchesOf(t, reader.by)
			want := map[string]bool{}
			for _, f := range w.fixtures {
				if w.openable(t, reader.by, f, saved) {
					want[string(f.kind)+":"+f.id] = true
				}
			}
			for key := range found {
				if !want[key] {
					t.Errorf("found %s, which the reader cannot open", key)
				}
			}
			for _, f := range w.fixtures {
				key := string(f.kind) + ":" + f.id
				if want[key] && !found[key] {
					t.Errorf("missed %s, which the reader can open", f.name)
				}
			}
			if len(found) != reader.count || len(want) != reader.count {
				t.Errorf("found %d, openable %d, want %d", len(found), len(want), reader.count)
			}
		})
	}
}

func TestSearchRefusesAPrincipalWithoutAUsableID(t *testing.T) {
	r := newRig(t)
	_, err := r.service.Search(t.Context(), auth.Principal{Kind: auth.KindUser, ID: "not-a-uuid", Role: auth.RoleAdmin}, Request{Q: "zebra"})
	if !errors.Is(err, folders.ErrForbidden) {
		t.Fatalf("got %v, want folders.ErrForbidden", err)
	}
}
