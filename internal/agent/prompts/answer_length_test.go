package prompts

import (
	"strings"
	"testing"
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
