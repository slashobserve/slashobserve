package prompts

import (
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/route"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestInvestigateAnswersInTheLengthTheRequestNames(t *testing.T) {
	system := Investigate()
	for _, want := range []string{
		"then, unless the request names a length or a form, gives the evidence in short paragraphs or bullets",
		"When the request names a length or a form, such as two sentences or a list, the markdown is exactly that: that many sentences, or only the list, with no extra evidence prose",
		"no link to the page the request was asked on",
		"A read that was refused, failed or came back empty still gets one short clause in it",
		"The evidence panels are not part of that limit",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
	if strings.Contains(system, "then gives the evidence in short paragraphs or bullets") {
		t.Error("the format line still demands evidence after the first sentence")
	}
}

func TestInvestigateCitesObservationsAsTitledLinksNeverBarePaths(t *testing.T) {
	system := Investigate()
	for _, want := range []string{
		"cite it as a Markdown link with its title, `[title](/o/<id>)`, never as a bare path",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
}

func TestBuildAnswersHomeQuestionsInTheLengthTheRequestNames(t *testing.T) {
	system := Build(demoTypes)
	for _, want := range []string{
		"the markdown's first line answers in one sentence, then, unless the request names a length or a form, the names with their counts",
		"When the request names a length or a form, such as one sentence or a list, the markdown is exactly that, with no intro, group labels, Source, Not checked or other extra sections",
		"a refused, failed or empty read still gets one short clause",
		"Evidence panels are outside that limit",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
}

func TestInvestigateAsksForOneValuePerQuantityAndAVerdictFirst(t *testing.T) {
	core := []string{
		"Give one value per quantity, copied at the precision the tool returned and never rounded or truncated",
		"never a total or a neighbouring quantity in its place",
		"make one more read that settles it and state only that figure",
		"never report both candidates or which one you trust",
		"The window you were given is a default, not a limit of the question",
		"start from its candidates and test each with your own reads, because it is not evidence",
		"take the next read yourself instead of suggesting it",
		"name it as a stand-in",
		"Never state that something does not exist, is not happening, is not recurring or was not deployed unless a query checked it, say what was checked, and when nothing checked it, say it is unchecked; before you say a signal does not exist, list its metrics",
		"An empty result that says why",
	}
	byTask := map[route.Task][]string{
		route.Measure: {
			"Answer at the grain the question names, its window, its aggregation and whether it is per request or a total",
			"a short spike or peak inside the window comes later as a separate observation, never as the headline",
			"decide from the ratio of each over the same windows and state the ratios",
			"one within about 10% of 1, or one that moves against the change, explains nothing",
			"a mean over another span or a short spike does not overturn them",
			"A current or right-now rate is a short window ending now, such as `rate(x[5m])`",
			"the same read with PromQL's `offset 1h`",
			"never compute one as a longer increase divided by its length",
			"use a wide window only when the question asks for an average over that span",
			"give every member the same metric, window and unit, including the ones that look fine",
			"a total across members is a sum, and a ratio or percentile across a set is computed over the set, never summed",
			"the headline gives one figure for the reading you used",
			"Saved evidence panels keep `$__rate_interval` as the authoring guide says",
			"from the question's whole window without the event's own span, never from a sub-window",
			"`measure_window`", "`measure_aggregate`",
		},
		route.Traces: {
			"call `trace_error_origins`, with `by_route` when routes matter",
			"the leaders by its count and share are the origin, and the services in `relayed_by` are secondary",
			"answer each one from its own figures",
			"several services, routes or operations",
			"Slowness is decided by the slow requests, not a percentile alone",
			"call `trace_slow_path` on the route or service, with the question's threshold when it gives one",
			"its example trace with the one exact duration given, even when its p99 looks normal",
			"every service in its slow traces with its count",
			"never one combined top-N that one of them can crowd out",
		},
		route.Incident: {
			"The first sentence is a plain verdict from the comparison with the one number that decides it",
			"name the origin by service and operation in the headline and the body alike, with self time and downstream symptoms kept as separate points",
			"when you found an onset and they show no deploy, search the involved services' logs at every level for deploy, rollout, version or restart lines in a short window before the onset",
			"List every cause the evidence supports in `causes`, most confident first, and never add one to be safe.",
			"A second service's errors are a cause of their own only with their own onset or trigger",
			"Name a trigger by its key exactly as the system spells it",
			"never call background errors an independent failure without that before-onset rate",
			"at the series' native step in a short range around the event",
			"rank the services by their error counts before you read one trace",
			"group the error spans or lines by message over time",
			"Give the time of the last error span before you call a bucket partial",
			"Call `list_annotations`",
		},
		route.Logs: {
			"A filter that returns nothing matched nothing",
			"`log_patterns`", "`measure_aggregate`",
		},
	}
	full := Investigate()
	for _, want := range core {
		for _, task := range append([]route.Task{route.Full}, route.Tasks...) {
			if !strings.Contains(InvestigateFor(Variant{Task: task}, allTypes), want) {
				t.Errorf("%s text lacks the core rule %q", task.Name(), want)
			}
		}
	}
	for task, wants := range byTask {
		own := InvestigateFor(Variant{Task: task}, allTypes)
		for _, want := range wants {
			if !strings.Contains(own, want) {
				t.Errorf("%s text lacks %q", task.Name(), want)
			}
			if !strings.Contains(full, want) {
				t.Errorf("the full text lacks %q", want)
			}
		}
	}
	for _, gone := range []string{"say which one you trust in one clause", "lead with the verdict and the one number that decides it", "Never state that something is not happening, not recurring or not deployed", "is the same expression and window with `offset`;", "a ClickHouse span status, which is `Error`"} {
		if strings.Contains(full, gone) {
			t.Errorf("system text still has %q", gone)
		}
	}
}

func TestPrometheusGuideAsksExtremesAndDurationsOfTheDatasource(t *testing.T) {
	system := Build([]spec.DatasourceType{spec.Prometheus})
	for _, want := range []string{"`measure_window`", "`first_time` with `where`", "Read a store's label names before selecting by a service name"} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
}
