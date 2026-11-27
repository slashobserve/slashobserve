package jaeger

import (
	"context"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

const variableLintPrefix = "could not lint around a variable: "

func (d *Datasource) Lint(_ context.Context, query string) ([]datasource.Diagnostic, error) {
	diags := []datasource.Diagnostic{}
	exp, err := macros.ExpandForLint(macros.Jaeger, query)
	if err != nil {
		return append(diags, datasource.Diagnostic{Severity: datasource.SeverityError, Message: datasource.MessageOf(err)}), nil
	}
	if _, ok := traceIDOf(exp.Text); ok {
		return diags, nil
	}
	_, bad := parseSearch(exp.Text)
	if bad == nil {
		return diags, nil
	}
	start, end, inVariable := exp.Locate(datasource.Clamp(bad.start, len(exp.Text)), datasource.Clamp(bad.end, len(exp.Text)))
	diag := datasource.Diagnostic{
		Severity: datasource.SeverityError,
		Message:  bad.message,
		Start:    datasource.Clamp(start, len(query)),
		End:      max(datasource.Clamp(start, len(query)), datasource.Clamp(end, len(query))),
	}
	if inVariable {
		diag.Severity = datasource.SeverityWarning
		diag.Message = variableLintPrefix + diag.Message
	}
	return append(diags, diag), nil
}
