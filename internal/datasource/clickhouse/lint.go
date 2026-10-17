package clickhouse

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
)

const (
	lintTimeout        = 5 * time.Second
	explainSyntax      = "EXPLAIN SYNTAX "
	explainPlan        = "EXPLAIN PLAN "
	syntaxErrorCode    = 62
	variableLintPrefix = "could not lint around a variable: "
)

var positionPattern = regexp.MustCompile(`position (\d+)`)

func (d *Datasource) Lint(ctx context.Context, query string) []datasource.Diagnostic {
	diagnostics := []datasource.Diagnostic{}
	exp, err := macros.ExpandForLint(macros.ClickHouse, query)
	if err != nil {
		return append(diagnostics, errorDiagnostic(messageOf(err), 0, 0))
	}
	text, err := guard(exp.Text)
	if err != nil {
		return append(diagnostics, errorDiagnostic(messageOf(err), 0, 0))
	}
	ctx, cancel := context.WithTimeout(ctx, lintTimeout)
	defer cancel()
	if err := d.readOnly(ctx); err != nil {
		return append(diagnostics, couldNotLint(err))
	}
	lim := d.limits
	lim.Timeout = lintTimeout

	if err := d.explain(ctx, explainSyntax+text, lim); err != nil {
		if exceptionCode(err) != syntaxErrorCode {
			return append(diagnostics, lintFailure(err))
		}
		return append(diagnostics, syntaxDiagnostic(err, exp, len(text)))
	}
	if err := d.explain(ctx, explainPlan+text, lim); err != nil {
		return append(diagnostics, lintFailure(err))
	}
	return diagnostics
}

func (d *Datasource) explain(ctx context.Context, sql string, lim datasource.Limits) error {
	_, err := d.fetch(ctx, sql, lim, nil)
	return err
}

func syntaxDiagnostic(err error, exp macros.Expansion, textLen int) datasource.Diagnostic {
	message := messageOf(mapError(err, lintTimeout))
	m := positionPattern.FindStringSubmatch(message)
	if m == nil {
		return errorDiagnostic(message, 0, 0)
	}
	pos, convErr := strconv.Atoi(m[1])
	if convErr != nil {
		return errorDiagnostic(message, 0, 0)
	}
	start := min(max(pos-1-len(explainSyntax), 0), textLen)
	origStart, origEnd, inVariable := exp.Locate(start, textLen)
	if inVariable {
		return datasource.Diagnostic{Severity: datasource.SeverityWarning, Message: variableLintPrefix + message, Start: origStart, End: origEnd}
	}
	return errorDiagnostic(message, origStart, origEnd)
}

func lintFailure(err error) datasource.Diagnostic {
	mapped := mapError(err, lintTimeout)
	if errors.Is(mapped, datasource.ErrUnavailable) || errors.Is(mapped, datasource.ErrTimeout) || errors.Is(mapped, context.Canceled) {
		return couldNotLint(mapped)
	}
	return errorDiagnostic(messageOf(mapped), 0, 0)
}

func couldNotLint(err error) datasource.Diagnostic {
	return datasource.Diagnostic{Severity: datasource.SeverityWarning, Message: "could not lint: " + messageOf(mapError(err, lintTimeout))}
}

func errorDiagnostic(message string, start, end int) datasource.Diagnostic {
	return datasource.Diagnostic{Severity: datasource.SeverityError, Message: message, Start: start, End: end}
}
