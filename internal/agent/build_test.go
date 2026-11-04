package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func buildWithPanels(t *testing.T, built, planned int) *dashboards.Build {
	t.Helper()
	rows := make([]string, built)
	for i := range rows {
		id := fmt.Sprintf("b%d", i+1)
		rows[i] = fmt.Sprintf(`{"id":%q,"type":"row","title":"Built %d","gridPos":{"x":0,"y":%d,"w":24,"h":1},"panels":[{"id":"%s_p","type":"stat","title":"p","gridPos":{"x":0,"y":%d,"w":6,"h":4},"datasource":"prom","queries":[{"query":"up"}]}]}`, id, i+1, i*5, id, i*5+1)
	}
	d, err := spec.Parse([]byte(`{"specVersion":"0","title":"t","time":{"from":"now-15m","to":"now"},"panels":[` + strings.Join(rows, ",") + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	return &dashboards.Build{Spec: d, PlannedPanels: planned, Saves: 1}
}

func TestWriteBuildStatesProgressAndTheReasonForEachNewSession(t *testing.T) {
	sentences := map[seedReason]string{
		seedRestart:     "Why this is a new session: The server restarted while you worked.\n",
		seedOutputLimit: "Why your conversation was shortened: Your last answer was cut off at the output limit. Save one panel per call, and check only what that panel needs.\n",
		seedPending:     "Why this is a new session: You answered without a tool call, but the build has fewer panels than you planned. Add the rest one per save, or resend planned_panels with the number you built, then finish.\n",
		seedContinue:    "Why this is a new session: Your earlier run stopped before it finished, and the person asked you to continue it from the build above.\n",
		seedRenewed:     "Why your conversation was shortened: Your session was renewed to keep it short; the build above holds every panel you saved.\n",
		seedCallTimeout: "Why your conversation was shortened: Your last call ran past the time limit for one model call. Keep each answer short: save one panel per call, and check only what that panel needs.\n",
		seedConflict:    "Why this is a new session: Someone saved a new version while you worked; your build was discarded. Make the change again on the dashboard above.\n",
	}
	for reason, sentence := range sentences {
		t.Run(string(reason), func(t *testing.T) {
			var b strings.Builder
			writeBuild(&b, seed{build: buildWithPanels(t, 2, 4), reason: reason})
			want := "\n<build>\n" +
				"You are building this dashboard. Your saves so far are in the build above; do not redo them.\n" +
				"Panels built: 2 of 4.\n" +
				sentence +
				"Continue with the next panel, one panel per save, and set finish: true on the last one. If nothing is missing, answer with one short sentence and no tool call.\n" +
				"</build>"
			if got := b.String(); got != want {
				t.Fatalf("section:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestWriteBuildHasNoReasonLineForAFreshSession(t *testing.T) {
	var b strings.Builder
	writeBuild(&b, seed{build: buildWithPanels(t, 2, 2)})
	if strings.Contains(b.String(), "Why this is a new session") {
		t.Fatalf("section = %s", b.String())
	}
}

func TestWriteBuildWithoutAPlannedCountStatesTheBuiltPanels(t *testing.T) {
	var b strings.Builder
	writeBuild(&b, seed{build: buildWithPanels(t, 12, 0), reason: seedRestart})
	if !strings.Contains(b.String(), "The build holds 12 panels.\n") || strings.Contains(b.String(), "Panels built") {
		t.Fatalf("section = %s", b.String())
	}
}

func TestWriteBuildStatesBuiltAboveThePlannedCount(t *testing.T) {
	var b strings.Builder
	writeBuild(&b, seed{build: buildWithPanels(t, 5, 3)})
	if !strings.Contains(b.String(), "Panels built: 5 of 3.\n") {
		t.Fatalf("section = %s", b.String())
	}
}

func TestBuildContextOmitsTheBuildSectionWithoutABuildOrReason(t *testing.T) {
	got, err := buildContext(nil, tools.ModeCreate, revisions.SourcePrompt, "", "p", nil, seed{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Configured datasources:\n\nCreate a new dashboard for the request." {
		t.Fatalf("context = %q", got)
	}
}

func TestBuildContextCarriesTheReasonWhenNothingIsBuiltYet(t *testing.T) {
	got, err := buildContext(nil, tools.ModeCreate, revisions.SourcePrompt, "", "p", nil, seed{reason: seedOutputLimit})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "\nWhy your conversation was shortened: "+seedSentences[seedOutputLimit]) || strings.Contains(got, "<build>") {
		t.Fatalf("context = %q", got)
	}
}

func buildOf(t *testing.T) (*dashboards.Build, *revision) {
	t.Helper()
	raw := `{"specVersion":"0","title":"t","time":{"from":"now-15m","to":"now"},"panels":[{"id":"r1","type":"row","title":"R1","gridPos":{"x":0,"y":0,"w":24,"h":1},"panels":[{"id":"a","type":"stat","title":"A","gridPos":{"x":0,"y":1,"w":6,"h":4},"datasource":"prom","queries":[{"query":"up"}]}]}]}`
	d, err := spec.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	build := &dashboards.Build{Spec: d, PlannedPanels: 2, Saves: 1}
	return build, &revision{spec: d}
}

func TestBuildContextShowsTheBuildAsTheNewDashboardForACreate(t *testing.T) {
	build, view := buildOf(t)
	got, err := buildContext(nil, tools.ModeCreate, revisions.SourcePrompt, "", "p", view, seed{build: build, reason: seedRestart})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"This is your build of the new dashboard, not saved as a version yet.",
		`"id":"r1"`,
		"Panels built: 1 of 2.",
		"The server restarted while you worked.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("context lacks %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "```json") > strings.Index(got, "<build>") {
		t.Fatal("the build section must come after the Spec")
	}
}

func TestBuildContextShowsTheBuildAsTheRevisionForARevise(t *testing.T) {
	build, view := buildOf(t)
	view.dashboardID, view.number = "d1", 3
	got, err := buildContext(nil, tools.ModeRevise, revisions.SourcePrompt, "", "p", view, seed{build: build, reason: seedPending})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "You are revising dashboard d1, currently version 3. This is your build of the revision, not saved as a version yet.") ||
		!strings.Contains(got, "<build>") {
		t.Fatalf("context = %s", got)
	}
}

func TestResumeReasonIsContinueOnlyForTheFirstAttemptOfAContinuation(t *testing.T) {
	for _, tc := range []struct {
		name string
		task Task
		want seedReason
	}{
		{"first attempt of a continuation", Task{Continues: true, Attempt: 1}, seedContinue},
		{"second attempt of a continuation", Task{Continues: true, Attempt: 2}, seedRestart},
		{"first attempt of an ordinary request", Task{Attempt: 1}, seedRestart},
		{"second attempt of an ordinary request", Task{Attempt: 2}, seedRestart},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resumeReason(tc.task); got != tc.want {
				t.Fatalf("resumeReason = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSeedsCarryTheContinueNoteOnceAfterTheSeedSentenceAndNeverWhenEmpty(t *testing.T) {
	const noteLine = "The person's note for this run: add a latency row"
	for _, reason := range []seedReason{seedContinue, seedRestart, seedConflict} {
		t.Run(string(reason), func(t *testing.T) {
			sentence := "Why this is a new session: " + seedSentences[reason]
			for name, write := range map[string]func(*strings.Builder, seed){
				"build": writeBuild,
				"note":  writeSeedNote,
			} {
				sd := seed{build: buildWithPanels(t, 2, 3), reason: reason, note: "add a latency row"}
				var with, without strings.Builder
				write(&with, sd)
				sd.note = ""
				write(&without, sd)
				got := with.String()
				if strings.Count(got, noteLine) != 1 || strings.Index(got, noteLine) < strings.Index(got, sentence) {
					t.Fatalf("%s with note:\n%s", name, got)
				}
				if strings.Contains(without.String(), "The person's note") {
					t.Fatalf("%s without note:\n%s", name, without.String())
				}
			}
		})
	}
}

func TestWriteSeedNoteWritesTheContinueSentenceThenTheNote(t *testing.T) {
	var b strings.Builder
	writeSeedNote(&b, seed{reason: seedContinue, note: "keep it small"})
	want := "\nWhy this is a new session: Your earlier run stopped before it finished, and the person asked you to continue it from the build above." +
		"\nThe person's note for this run: keep it small\n"
	if b.String() != want {
		t.Fatalf("note = %q, want %q", b.String(), want)
	}
}

func TestBuildContextPutsTheContinueNoteInTheBuildSection(t *testing.T) {
	build, view := buildOf(t)
	got, err := buildContext(nil, tools.ModeCreate, revisions.SourcePrompt, "", "p", view, seed{build: build, reason: seedContinue, note: "only the CPU rows"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(got, "The person's note for this run: only the CPU rows\n") != 1 || !strings.Contains(got, seedSentences[seedContinue]) {
		t.Fatalf("context:\n%s", got)
	}
}
