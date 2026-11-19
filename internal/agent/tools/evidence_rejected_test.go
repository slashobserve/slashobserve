package tools

import (
	"slices"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

var evidencePointers = map[string]string{"/panels/0": "evidence-1", "/panels/1": "evidence-2"}

func TestARejectedEvidencePanelIsNamedWithItsReason(t *testing.T) {
	res := evidenceRejected([]spec.Issue{{Path: "/panels/1/queries/0", Code: queryproxy.CodeQueryEmpty, Message: "query returned no data"}}, evidencePointers)
	body, ok := res.Content.(patchRejectedOut)
	if !res.IsError || !ok || body.Code != codeInvalidSpec {
		t.Fatalf("result = %#v", res)
	}
	if !slices.Equal(body.FailingPanels, []string{"evidence-2"}) {
		t.Errorf("failing_panels = %v", body.FailingPanels)
	}
	if !strings.Contains(body.Message, "evidence-N is the Nth panel you sent") || !strings.Contains(body.Message, "query_range or query_instant") {
		t.Errorf("message = %q", body.Message)
	}
	if res.Step != "Evidence rejected: 1 issue (query_empty)" {
		t.Errorf("step = %q", res.Step)
	}
}

func TestEvidenceThatTimedOutIsToldToBeLeftOutNotFixed(t *testing.T) {
	res := evidenceRejected([]spec.Issue{
		{Path: "/panels/0/queries/0", Code: queryproxy.CodeQueryTimeout, Message: "query timed out"},
		{Path: "/panels/1/queries/0", Code: queryproxy.CodeDatasourceDown, Message: "unavailable"},
	}, evidencePointers)
	body := res.Content.(patchRejectedOut)
	if !strings.Contains(body.Message, "without the panels in failing_panels") || strings.Contains(body.Message, "query_range") {
		t.Errorf("message = %q", body.Message)
	}
	if !slices.Equal(body.FailingPanels, []string{"evidence-1", "evidence-2"}) {
		t.Errorf("failing_panels = %v", body.FailingPanels)
	}
}

func TestOneFixableIssueAmongTimeoutsKeepsTheRepairAdvice(t *testing.T) {
	res := evidenceRejected([]spec.Issue{
		{Path: "/panels/0/queries/0", Code: queryproxy.CodeQueryTimeout},
		{Path: "/panels/1/queries/0", Code: queryproxy.CodeQueryInvalid},
	}, evidencePointers)
	if body := res.Content.(patchRejectedOut); !strings.Contains(body.Message, "query_range") {
		t.Errorf("message = %q", body.Message)
	}
}
