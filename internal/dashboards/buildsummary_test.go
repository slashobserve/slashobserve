package dashboards

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func parsed(t *testing.T, raw []byte) *spec.Dashboard {
	t.Helper()
	d, err := spec.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestCreationSummaryNamesEveryPanelUpToAFewAndCountsTheRest(t *testing.T) {
	for name, c := range map[string]struct {
		ids  []string
		want string
	}{
		"one":  {[]string{"a"}, `Built the dashboard with 1 panel: "a".`},
		"two":  {[]string{"a", "b"}, `Built the dashboard with 2 panels: "a" and "b".`},
		"four": {[]string{"a", "b", "c", "d"}, `Built the dashboard with 4 panels: "a", "b", "c" and "d".`},
		"six":  {[]string{"a", "b", "c", "d", "e", "f"}, `Built the dashboard with 6 panels: "a", "b", "c", "d" and 2 more.`},
	} {
		if got := creationSummary(parsed(t, panelsSpec("t", c.ids...))); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

func TestChangeSentenceNamesEveryKindOfChangeInOneSave(t *testing.T) {
	before := parsed(t, panelsSpec("t", "a", "b", "c"))
	after := parsed(t, []byte(strings.NewReplacer(
		`"title":"a"`, `"title":"A renamed"`,
		`"title":"t"`, `"title":"New title"`,
	).Replace(string(panelsSpec("t", "a", "b", "d")))))
	after.Description = "now described"

	got := changeSentence(changeParts(before, after))
	want := `Added "d"; removed "c"; renamed "a" to "A renamed"; renamed the dashboard to "New title"; changed the description.`
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestChangeSentenceTellsAMoveFromAChange(t *testing.T) {
	before := parsed(t, panelsSpec("t", "a", "b"))
	after := parsed(t, panelsSpec("t", "a", "b"))
	after.Panels[1].GridPos.X = 12
	if got := changeSentence(changeParts(before, after)); got != `Moved "b".` {
		t.Fatalf("got %q", got)
	}
	after.Panels[1].Queries[0].Query = "down"
	if got := changeSentence(changeParts(before, after)); got != `Changed "b".` {
		t.Fatalf("got %q", got)
	}
}

func TestNoChangeProducesNoParts(t *testing.T) {
	d := parsed(t, panelsSpec("t", "a"))
	if parts := changeParts(d, parsed(t, panelsSpec("t", "a"))); len(parts) != 0 {
		t.Fatalf("parts = %v", parts)
	}
}

func TestSummaryStaysWithinTheLimit(t *testing.T) {
	before := parsed(t, panelsSpec("t", "a"))
	after := parsed(t, panelsSpec("t", "a", "b", "c", "d"))
	for i := range after.Panels {
		after.Panels[i].Title = strings.Repeat("long title ", 30)
	}
	if got := changeSentence(changeParts(before, after)); utf8.RuneCountInString(got) > maxSummaryLength {
		t.Fatalf("summary is %d characters", utf8.RuneCountInString(got))
	}
}

func TestRestoreSentenceStaysWithinTheLimit(t *testing.T) {
	parts := []string{
		"removed " + strings.Repeat("a long title ", 20),
		"added " + strings.Repeat("another long title ", 20),
	}
	got := restoreSentence(3, parts)
	if n := utf8.RuneCountInString(got); n > maxSummaryLength {
		t.Fatalf("summary is %d characters", n)
	}
	if !strings.HasPrefix(got, "Restored version 3 (") {
		t.Fatalf("summary = %q", got)
	}
}

func TestACommittedCreateBuildSummarisesAllItsPanelsNotTheLastSave(t *testing.T) {
	r, _ := newProbeRig(t)
	id := r.runningRequest(t, pgtype.UUID{})
	r.saveBuild(t, id, pgtype.UUID{}, 0, "a")
	r.saveBuild(t, id, pgtype.UUID{}, 0, "a", "b")

	commit, err := r.service.CommitBuild(reading(t.Context()), testUser, id, buildEpoch)
	if err != nil {
		t.Fatal(err)
	}
	if want := `Built the dashboard with 2 panels: "a" and "b".`; commit.Version.Summary != want {
		t.Fatalf("summary = %q, want %q", commit.Version.Summary, want)
	}
}

func TestACommittedRevisionBuildThatUndoesAVersionIsMarkedRestored(t *testing.T) {
	r, _ := newProbeRig(t)
	dash := r.createDashboard(t, "rps")
	if _, err := r.service.Save(reading(t.Context()), testUser, SaveInput{DashboardID: dash, Parent: 1, Spec: panelsSpec("t", "rps", "p99"), Summary: "add p99"}); err != nil {
		t.Fatal(err)
	}
	id := r.runningRequest(t, dash)
	r.saveBuild(t, id, dash, 2, "rps")

	commit, err := r.service.CommitBuild(reading(t.Context()), testUser, id, buildEpoch)
	if err != nil {
		t.Fatal(err)
	}
	ver := commit.Version
	if ver.RestoredFrom == nil || *ver.RestoredFrom != 1 {
		t.Fatalf("restored_from = %v, want 1", ver.RestoredFrom)
	}
	if want := `Restored version 1 (removed "p99").`; ver.Summary != want {
		t.Fatalf("summary = %q, want %q", ver.Summary, want)
	}
}

func TestACommittedRevisionBuildThatChangesSomethingNewIsNotMarkedRestored(t *testing.T) {
	r, _ := newProbeRig(t)
	dash := r.createDashboard(t, "rps")
	id := r.runningRequest(t, dash)
	r.saveBuild(t, id, dash, 1, "rps", "p99")

	commit, err := r.service.CommitBuild(reading(t.Context()), testUser, id, buildEpoch)
	if err != nil {
		t.Fatal(err)
	}
	if commit.Version.RestoredFrom != nil || commit.Version.Summary != `Added "p99".` {
		t.Fatalf("restored_from = %v, summary = %q", commit.Version.RestoredFrom, commit.Version.Summary)
	}
}
