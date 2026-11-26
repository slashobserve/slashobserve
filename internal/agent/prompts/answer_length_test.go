package prompts

import (
	"strings"
	"testing"

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
	system := Investigate()
	for _, want := range []string{
		"Give one value per quantity, copied at the precision the tool returned and never rounded or truncated",
		"never a total or a neighbouring quantity in its place",
		"make one more read that settles it and state only that figure",
		"never report both candidates or which one you trust",
		"Answer at the grain the question names, its window, its aggregation and whether it is per request or a total",
		"a short spike or peak inside the window comes later as a separate observation, never as the headline",
		"the first sentence is a plain verdict from that comparison with the one number that decides it",
		"That window is a default, not a limit of the question",
		"take the next read yourself instead of suggesting it",
		"the deepest erroring span is the origin",
		"answer each one, with one trace read per route",
		"name it as a stand-in",
		"except the one-line note on another reading of a term below",
		"several services, routes or operations",
		"decide from the ratio of each over the same windows and state the ratios in the answer",
		"when the ratios the question asks for are within about 10% of 1, or move against the change, say that factor does not explain it",
		"a mean over another span or a short spike does not overturn the windowed ratios",
		"A current or right-now rate is a short window ending now, such as `rate(x[5m])`",
		"the same expression and window with `offset`",
		"never compute a current rate as a longer increase divided by its length",
		"use a wide window such as 1h or 24h only when the question asks for an average over that span",
		"give every member the same metric, window and unit, including the ones that look fine",
		"classify them as knock-on from the incident (on its call path, with the onset aligned), pre-existing background (present before the onset at a similar rate), or a separate incident (its own onset or burst)",
		"never call background errors an independent failure without that before-onset rate",
		"Never state that something is not happening, not recurring or not deployed unless a query checked it, and say what was checked",
		"the headline gives one figure, for the reading you used; name the other reading in words, with its figure only in a clearly separate one-line note",
		"reading each one's slow tail or error set separately, never one combined top-N that one of them can crowd out",
		"at the series' native step in a short range around the event, not a coarse step",
		"from the question's whole window without the event's own span, with an aggregate or a coarser step so the query stays under the point limit, never from a sub-window",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
	for _, gone := range []string{"say which one you trust in one clause", "lead with the verdict and the one number that decides it"} {
		if strings.Contains(system, gone) {
			t.Errorf("system text still has %q", gone)
		}
	}
}

func TestPrometheusGuideAsksExtremesAndDurationsOfTheDatasource(t *testing.T) {
	system := Build([]spec.DatasourceType{spec.Prometheus})
	for _, want := range []string{"`max_over_time(<series>[<window>])`", "`timestamp()` of the extreme", "Read a store's label names before selecting by a service name"} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
	}
}
