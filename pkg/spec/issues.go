package spec

import (
	"fmt"
	"strings"
)

type IssueCode string

const (
	CodeMalformedJSON         IssueCode = "malformed_json"
	CodeTooLarge              IssueCode = "too_large"
	CodeDuplicateKey          IssueCode = "duplicate_key"
	CodeSchema                IssueCode = "schema"
	CodeUnknownUnit           IssueCode = "unknown_unit"
	CodeDuplicatePanelID      IssueCode = "duplicate_panel_id"
	CodeDuplicateVariableName IssueCode = "duplicate_variable_name"
	CodeGridOverflow          IssueCode = "grid_overflow"
	CodeGridOverlap           IssueCode = "grid_overlap"
	CodeUnknownVariable       IssueCode = "unknown_variable"
	CodeVariableOrder         IssueCode = "variable_order"
	CodeUnknownMacro          IssueCode = "unknown_macro"
	CodeInvalidReference      IssueCode = "invalid_reference"
	CodeThresholdsUnordered   IssueCode = "thresholds_unordered"
	CodeMinMaxOrder           IssueCode = "min_max_order"
	CodeTimeRangeOrder        IssueCode = "time_range_order"
	CodeTooManyIssues         IssueCode = "too_many_issues"
)

const maxIssues = 50

type Issue struct {
	Path    string    `json:"path"`
	Code    IssueCode `json:"code"`
	Message string    `json:"message"`
}

type ValidationError struct {
	Issues []Issue
}

func (e *ValidationError) Error() string {
	if len(e.Issues) == 0 {
		return "spec: 0 issue(s)"
	}
	first := e.Issues[0]
	if first.Path == "" {
		return fmt.Sprintf("spec: %d issue(s): %s", len(e.Issues), first.Message)
	}
	return fmt.Sprintf("spec: %d issue(s): %s: %s", len(e.Issues), first.Path, first.Message)
}

func newValidationError(issues []Issue) *ValidationError {
	if len(issues) > maxIssues {
		omitted := len(issues) - maxIssues + 1
		issues = append(issues[:maxIssues-1:maxIssues-1], Issue{
			Code:    CodeTooManyIssues,
			Message: fmt.Sprintf("%d more issues were omitted; fix the reported ones and validate again", omitted),
		})
	}
	return &ValidationError{Issues: issues}
}

var pointerEscaper = strings.NewReplacer("~", "~0", "/", "~1")

func pointer(tokens ...string) string {
	var b strings.Builder
	for _, t := range tokens {
		b.WriteByte('/')
		b.WriteString(pointerEscaper.Replace(t))
	}
	return b.String()
}
