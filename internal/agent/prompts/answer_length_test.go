package prompts

import (
	"strings"
	"testing"
)

func TestInvestigateAnswersInTheLengthTheRequestNames(t *testing.T) {
	system := Investigate()
	for _, want := range []string{
		"When the request names a length or a form, such as one sentence or a list, the markdown is exactly that",
		"the first line is the whole answer, with no evidence bullets",
		"no link to the page the request was asked on",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system text lacks %q", want)
		}
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
