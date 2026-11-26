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
		"when the ratios it asks for are near 1, say that factor does not explain the change",
		"state the reading you used and give the other in one line; never headline two figures for one quantity",
		"reading each one's slow tail or error set separately, never one combined top-N that one of them can crowd out",
		"at its native step around the event, not a coarse step",
		"from the whole window the question covers, not a sub-window",
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
