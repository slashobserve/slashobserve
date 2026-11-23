package jaeger

import (
	"context"
	"errors"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

const variableLintPrefix = "could not lint around a variable: "

func (d *Datasource) Lint(_ context.Context, query string) ([]datasource.Diagnostic, error) {
	diags := []datasource.Diagnostic{}
	exp, err := macros.ExpandForLint(macros.Jaeger, query)
	if err != nil {
		return append(diags, datasource.Diagnostic{Severity: datasource.SeverityError, Message: messageOf(err)}), nil
	}
	if _, ok := traceIDOf(exp.Text); ok {
		return diags, nil
	}
	_, bad := parseSearch(exp.Text)
	if bad == nil {
		return diags, nil
	}
	start, end, inVariable := exp.Locate(clamp(bad.start, len(exp.Text)), clamp(bad.end, len(exp.Text)))
	diag := datasource.Diagnostic{
		Severity: datasource.SeverityError,
		Message:  bad.message,
		Start:    clamp(start, len(query)),
		End:      max(clamp(start, len(query)), clamp(end, len(query))),
	}
	if inVariable {
		diag.Severity = datasource.SeverityWarning
		diag.Message = variableLintPrefix + diag.Message
	}
	return append(diags, diag), nil
}

func clamp(v, upper int) int {
	return max(0, min(v, upper))
}

func messageOf(err error) string {
	var ue *datasource.UpstreamError
	if errors.As(err, &ue) {
		return ue.Message
	}
	return err.Error()
}
