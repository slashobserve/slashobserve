package clickhouse

import (
	"context"
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

func (d *Datasource) Lint(ctx context.Context, query string) ([]datasource.Diagnostic, error) {
	diagnostics := []datasource.Diagnostic{}
	exp, err := macros.ExpandForLint(macros.ClickHouse, query)
	if err != nil {
		return append(diagnostics, errorDiagnostic(messageOf(err), 0, 0)), nil
	}
	if _, err := d.guard(exp.Text); err != nil {
		return append(diagnostics, errorDiagnostic(messageOf(err), 0, 0)), nil
	}
	timeout := effectiveTimeout(ctx, lintTimeout)
	ctx, cancel := context.WithTimeout(ctx, lintTimeout)
	defer cancel()
	if err := d.readOnly(ctx); err != nil {
		mapped := mapError(ctx, err, timeout)
		if datasource.IsTransient(mapped) {
			return nil, mapped
		}
		return append(diagnostics, couldNotLint(mapped)), nil
	}
	text, err := d.guard(exp.Text)
	if err != nil {
		return append(diagnostics, errorDiagnostic(messageOf(err), 0, 0)), nil
	}
	clamped := d.clampTimeout(lintTimeout)
	timeout = effectiveTimeout(ctx, clamped)
	ctx, cancelClamped := context.WithTimeout(ctx, clamped)
	defer cancelClamped()
	lim := d.limits
	lim.Timeout = clamped

	if err := d.explain(ctx, explainSyntax+text, lim); err != nil {
		if exceptionCode(err) != syntaxErrorCode {
			return lintFailure(ctx, diagnostics, err, timeout)
		}
		return append(diagnostics, syntaxDiagnostic(ctx, err, exp, len(text), timeout)), nil
	}
	if err := d.explain(ctx, explainPlan+text, lim); err != nil {
		return lintFailure(ctx, diagnostics, err, timeout)
	}
	return diagnostics, nil
}

func (d *Datasource) explain(ctx context.Context, sql string, lim datasource.Limits) error {
	_, err := d.fetch(ctx, sql, lim, nil)
	return err
}

func syntaxDiagnostic(ctx context.Context, err error, exp macros.Expansion, textLen int, timeout time.Duration) datasource.Diagnostic {
	message := messageOf(mapError(ctx, err, timeout))
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

func lintFailure(ctx context.Context, diagnostics []datasource.Diagnostic, err error, timeout time.Duration) ([]datasource.Diagnostic, error) {
	mapped := mapError(ctx, err, timeout)
	if datasource.IsTransient(mapped) {
		return nil, mapped
	}
	return append(diagnostics, errorDiagnostic(messageOf(mapped), 0, 0)), nil
}

func couldNotLint(err error) datasource.Diagnostic {
	return datasource.Diagnostic{Severity: datasource.SeverityWarning, Message: "could not lint: " + messageOf(err)}
}

func errorDiagnostic(message string, start, end int) datasource.Diagnostic {
	return datasource.Diagnostic{Severity: datasource.SeverityError, Message: message, Start: start, End: end}
}
