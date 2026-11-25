package prompts

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	systemTextBudget     = 24947
	fullSystemTextBudget = 29669
)

var demoTypes = []spec.DatasourceType{spec.Prometheus, spec.ClickHouse}

func TestBuildHoldsTheRulesTheGuideAndTheExampleButNotTheSchema(t *testing.T) {
	system := Build(demoTypes)
	for _, want := range []string{
		"You are SlashObserve's dashboard agent.",
		"\n\n# Authoring guide\n",
		"\n\n# Example Spec (format only)\n```json\n",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("system text lacks %q", want)
		}
	}
	if strings.Contains(system, "# ObserveSpec v0 JSON Schema") || strings.Contains(system, `"$defs"`) {
		t.Fatal("system text carries the JSON Schema")
	}
	if len(system) > systemTextBudget {
		t.Fatalf("system text is %d bytes, over the %d byte budget", len(system), systemTextBudget)
	}
	if !strings.Contains(system, "- Never create a placeholder panel. Keep existing placeholders unchanged apart from their position, and delete one only when the request asks.") {
		t.Fatal("system prompt lacks the placeholder rule")
	}
}

func TestBuildForEveryTypeStaysWithinBudget(t *testing.T) {
	system := Build([]spec.DatasourceType{spec.Prometheus, spec.ClickHouse, spec.Loki, spec.Tempo, spec.Elasticsearch, spec.OpenSearch})
	if len(system) > fullSystemTextBudget {
		t.Fatalf("system text for every datasource type is %d bytes, over the %d byte budget", len(system), fullSystemTextBudget)
	}
}

func TestBuildCarriesOnlyTheSectionsOfItsTypes(t *testing.T) {
	system := Build([]spec.DatasourceType{spec.Loki})
	if !strings.Contains(system, "## Loki (LogQL)") {
		t.Fatal("system text lacks the Loki section")
	}
	for _, heading := range []string{"## Prometheus (PromQL)", "## ClickHouse (OpenTelemetry exporter tables)", "## Tempo (TraceQL)"} {
		if strings.Contains(system, heading) {
			t.Errorf("system text for loki holds %q", heading)
		}
	}
}

func TestBothSystemTextsCarryTheScopeOnceBeforeTheGuide(t *testing.T) {
	scope := "\n\n# Scope\n\nEvery answer uses only the datasources in the context, the dashboards and observations on this server, and names you discovered in this run.\nWhen the data does not cover the question, say so in the finding; only a short definition of an observability term comes from general knowledge.\nFor help, say you answer questions and build new dashboards, and that an existing dashboard is changed from its own chat.\n\n# Authoring guide\n"
	for name, system := range map[string]string{"build": Build(demoTypes), "investigate": Investigate()} {
		if strings.Count(system, scope) != 1 || strings.Count(system, "Every answer uses only") != 1 {
			t.Errorf("%s system text does not carry the scope once before the guide", name)
		}
	}
}

func TestBuildStatesTheRulesTheLoopEnforces(t *testing.T) {
	system := Build(demoTypes)
	for _, want := range []string{
		"from the sixth model call (the twelfth for a new chat message before its first panel) without an accepted save only a save runs, and reads in the same call as a save still run",
		"the next call may run reads, such as `query_range` on a failing query, to see why",
		"Every rejection of a save lists the panels in `failing_panels`",
		"call `get_spec_guide` for the JSON Schema",
		"Set `planned_panels` once from the request",
		"names a dashboard to rebuild",
		"A third identical call is refused, and a `<note>` at the end of a tool result is an instruction from the server",
		"when `truncated` is true, pass `next_offset` to page on",
		"make its widths add up to exactly 24",
		"out of the panels you save: the server sizes a new panel",
		"## Panel type by data shape",
		"as wide as its line's recipe whatever its series count",
		"with `row` naming its row",
		"First decide what the request asks for, then do only that.",
		"Every answer keeps to the scope below.",
		"never call `save_finding` after you saved a panel",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
	for _, gone := range []string{
		"two to eight panels unless the request asks for more",
		"after an `update_panels` rejection resend only",
		"The schema here leaves out",
		"Pick widths in a row that add up to 24",
		"Use `stat` or `table` for",
		"10 to 20 series: a full-width",
		"The only result that matters is a saved dashboard",
		"list_findings",
	} {
		if strings.Contains(system, gone) {
			t.Errorf("system text still holds %q", gone)
		}
	}
}

func TestBuildAnswersATermAndNamesWhatTheServerCannotDo(t *testing.T) {
	system := Build(demoTypes)
	for _, want := range []string{
		`"what does p99 mean": answer with ` + "`save_finding`" + ` in two or three lines`,
		"alerts, notifications and schedules",
		"keep panel ids, positions, filters, variables",
		"naming each repaired query and what changed",
		"keeping its metric and meaning",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
}

func TestInvestigateStatesHowToWriteAndWhatToCheck(t *testing.T) {
	system := Investigate()
	for _, want := range []string{
		"write every time in the time zone the context names, with its label, or with the word UTC when it names none",
		"When several pods or services tie, lead with the one with the most errors, then the first by name",
		"Call `list_annotations`",
		"read that observation first with `get_observation`, because \"this\" means that page",
		"read it with `get_observation`, and cite it as a Markdown link with its title",
		"A request to summarize, explain or rewrite the page is answered from `get_observation` and the discussion with no new queries, and it still ends with `save_finding`",
		"Before you say a signal does not exist, list its metrics with scope set to a selector for the service",
		"a read that failed or timed out could not check anything",
		"say in the finding what you did not read",
		"Write the first sentence in plain words",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("investigate text lacks %q", want)
		}
	}
}

func TestBothSystemTextsSayLiveTextIsShownButNotKept(t *testing.T) {
	for name, system := range map[string]string{"build": Build(demoTypes), "investigate": Investigate()} {
		for _, want := range []string{
			"is shown live to the person watching the run and to an agent polling it, but it is not kept once the run ends",
			"write one short plain sentence with no Markdown that says what you are about to do and why",
			"do not write one per call, do not restate tool output",
		} {
			if !strings.Contains(system, want) {
				t.Errorf("%s system text lacks %q", name, want)
			}
		}
		for _, stale := range []string{"not shown to anyone", "is not shown"} {
			if strings.Contains(system, stale) {
				t.Errorf("%s system text still says %q", name, stale)
			}
		}
	}
}

func TestBuildLetsTextPanelsHoldMarkdownButNotHTML(t *testing.T) {
	system := Build(demoTypes)
	for _, want := range []string{
		"- Never write HTML or JavaScript; Markdown (bold, lists, tables) goes only in a text panel's `content`, so write an asked-for HTML table in Markdown.",
		"- Add a `text` panel only when the request asks for notes, as Markdown in `content`.",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
	for _, refusal := range []string{"Markdown anywhere in a Spec", "plain sentences in `content`", "all text is plain"} {
		if strings.Contains(system, refusal) {
			t.Errorf("system text still forbids Markdown: %q", refusal)
		}
	}
}

func TestInvestigateAsksForNoSuggestedChangeWhenNoneIsNeededAndNoRestatement(t *testing.T) {
	system := Investigate()
	for _, want := range []string{
		"Leave `suggested_change` out when no change is needed",
		"never write it into the markdown",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("investigate system text lacks %q", want)
		}
	}
}

func TestAskRunTextNeverRefusesAChangeAndNeverNamesAButton(t *testing.T) {
	for name, system := range map[string]string{"build": Build(demoTypes), "investigate": Investigate()} {
		if strings.Contains(system, "cannot change") || strings.Contains(system, "never change") {
			t.Errorf("%s system text reads as a refusal to change a dashboard", name)
		}
		if strings.Contains(system, "Revise the dashboard") {
			t.Errorf("%s system text names a button the requester may not have", name)
		}
	}
	investigate := Investigate()
	for _, want := range []string{
		"set `suggested_change` to the change the person asked for, in their terms",
		"\"remove the empty panels\" gives a change that removes them",
		"Never answer that you cannot make a change; for an unsafe or impossible one, say why in one sentence and set `suggested_change` to the nearest safe change, or leave it out when none exists",
	} {
		if !strings.Contains(investigate, want) {
			t.Errorf("investigate text lacks %q", want)
		}
	}
}

func TestHelpSaysAnExistingDashboardIsChangedFromItsOwnChat(t *testing.T) {
	want := "For help, say you answer questions and build new dashboards, and that an existing dashboard is changed from its own chat."
	for name, system := range map[string]string{"build": Build(demoTypes), "investigate": Investigate()} {
		if strings.Count(system, want) != 1 {
			t.Errorf("%s system text lacks the help sentence", name)
		}
	}
}

func TestInvestigateTreatsAnEmptyFilterAsMatchingNothing(t *testing.T) {
	system := Investigate()
	for _, want := range []string{
		"A filter that returns nothing matched nothing",
		"a ClickHouse span status, which is `Error`",
		"look for the service's span-metrics series",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
}

func TestTheClickHouseGuideNamesTheErrorStatus(t *testing.T) {
	guide := GuideFor([]spec.DatasourceType{spec.ClickHouse})
	if !strings.Contains(guide, "failed spans are `StatusCode = 'Error'`") {
		t.Fatal("the ClickHouse guide does not name the error status value")
	}
}

func TestInvestigateRanksAndGroupsErrorsBeforeItNamesACause(t *testing.T) {
	system := Investigate()
	for _, want := range []string{
		"rank the services by their error counts before you read one trace",
		"group the error spans by status message over time",
		"Give the time of the last error span before you call a bucket partial",
		"Make the first sentence match the numbers under it",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
}

func TestInvestigateNamesTheWindowItQueriedFromTheContextTime(t *testing.T) {
	system := Investigate()
	for _, want := range []string{
		"Name the window you queried, from its start to its end",
		"never add an offset to a time you wrote in UTC",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
}

func TestInvestigateAsksForEvidencePanelsWhenTheAnswerGivesCounts(t *testing.T) {
	system := Investigate()
	for _, want := range []string{
		"When the answer gives counts, add 1 to 4 evidence panels",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
}

func TestInvestigateKeepsTheTitleWithinWhatTheTextConfirms(t *testing.T) {
	system := Investigate()
	for _, want := range []string{
		"never claim a cause there that the markdown calls likely or unconfirmed",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
}

func TestBuildClosesWithTheSummaryOfItsLastSave(t *testing.T) {
	system := Build(demoTypes)
	for _, want := range []string{
		"Your last save's `summary` is the closing line the person reads",
		"how many panels are still unverified or empty",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
}

func TestBuildReadsGridPosBeforeAMove(t *testing.T) {
	system := Build(demoTypes)
	for _, want := range []string{
		"read their `gridPos` with `get_panels` first",
		"name in `summary` where it went",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
}

func TestBuildKeepsAPanelChangeSmall(t *testing.T) {
	system := Build(demoTypes)
	for _, want := range []string{
		"changes only that: keep its title, queries and `format`",
		"Say in `summary` when a change stops a panel using a dashboard variable",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
}

func TestInvestigateSuggestsTheOneOptionItRecommends(t *testing.T) {
	system := Investigate()
	for _, want := range []string{
		"the one option the markdown recommends, the smallest that does what was asked",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
}
