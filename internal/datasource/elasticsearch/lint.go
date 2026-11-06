package elasticsearch

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/url"
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/macros"
	"github.com/slashobserve/slashobserve/internal/kql"
)

const (
	variableLintPrefix = "could not lint around a variable: "
	maxExplainBytes    = 200
)

type finding struct {
	severity   datasource.Severity
	message    string
	start, end int
}

func (d *Datasource) Lint(ctx context.Context, query string) ([]datasource.Diagnostic, error) {
	diags := []datasource.Diagnostic{}
	exp, err := macros.ExpandForLint(d.dialect(), query)
	if err != nil {
		var ue *datasource.UpstreamError
		message := err.Error()
		if errors.As(err, &ue) {
			message = ue.Message
		}
		return append(diags, datasource.Diagnostic{Severity: datasource.SeverityError, Message: message}), nil
	}
	ctx, cancel := bound(ctx, d.opts.Limits.Timeout)
	defer cancel()
	form, body, offset := classify(exp.Text, d.opts.QueryLanguage)
	var findings []finding
	switch form {
	case FormKQL:
		findings, err = d.lintKQL(ctx, body, offset)
	case FormLucene:
		findings, err = d.lintLucene(ctx, body, offset)
	default:
		findings, err = d.lintDSL(ctx, body, offset)
	}
	if err != nil {
		return nil, err
	}
	for _, f := range findings {
		diags = append(diags, place(exp, query, f))
	}
	return diags, nil
}

func place(exp macros.Expansion, query string, f finding) datasource.Diagnostic {
	start, end, inVariable := exp.Locate(clamp(f.start, len(exp.Text)), clamp(f.end, len(exp.Text)))
	start = clamp(start, len(query))
	diag := datasource.Diagnostic{Severity: f.severity, Message: f.message, Start: start, End: max(start, clamp(end, len(query)))}
	if inVariable {
		diag.Severity = datasource.SeverityWarning
		diag.Message = variableLintPrefix + diag.Message
	}
	return diag
}

func clamp(v, upper int) int { return max(0, min(v, upper)) }

func (d *Datasource) lintKQL(ctx context.Context, body string, offset int) ([]finding, error) {
	m, err := d.Mapping(ctx)
	if err != nil {
		return nil, err
	}
	compiled, _ := kql.Compile(body, kqlFields(m))
	findings := make([]finding, len(compiled.Diagnostics))
	for i, diag := range compiled.Diagnostics {
		findings[i] = finding{severity: datasource.Severity(diag.Severity), message: diag.Message, start: offset + diag.Start, end: offset + diag.End}
	}
	return findings, nil
}

func (d *Datasource) lintLucene(ctx context.Context, body string, offset int) ([]finding, error) {
	if body == "" {
		return nil, nil
	}
	message, err := d.validate(ctx, luceneQuery(body))
	if err != nil || message == "" {
		return nil, err
	}
	return []finding{{severity: datasource.SeverityError, message: message, start: offset, end: offset + len(body)}}, nil
}

func (d *Datasource) lintDSL(ctx context.Context, body string, offset int) ([]finding, error) {
	obj, problems := parseDSL(body)
	if len(problems) > 0 {
		findings := make([]finding, len(problems))
		for i, p := range problems {
			findings[i] = finding{severity: datasource.SeverityError, message: p.message, start: offset + p.start, end: offset + p.end}
		}
		return findings, nil
	}
	clause, ok := obj["query"].(map[string]any)
	if !ok {
		return nil, nil
	}
	message, err := d.validate(ctx, clause)
	if err != nil || message == "" {
		return nil, err
	}
	return []finding{{severity: datasource.SeverityError, message: message, start: offset, end: offset + len(body)}}, nil
}

// A lint never runs _search: _validate/query parses the query on the shards without executing it.
func (d *Datasource) validate(ctx context.Context, clause map[string]any) (string, error) {
	params := url.Values{"explain": {"true"}}
	maps.Copy(params, lenientIndices)
	raw, err := d.call(ctx, http.MethodPost, d.indexPath("/_validate/query"), params, map[string]any{"query": clause})
	if err != nil {
		return "", err
	}
	var resp struct {
		Valid        bool `json:"valid"`
		Explanations []struct {
			Error string `json:"error"`
		} `json:"explanations"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", unreadable()
	}
	if resp.Valid {
		return "", nil
	}
	message := "the query is not valid"
	if len(resp.Explanations) > 0 && resp.Explanations[0].Error != "" {
		message = firstLine(resp.Explanations[0].Error)
	}
	return message, nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	line = strings.TrimSpace(line)
	if len(line) > maxExplainBytes {
		line = line[:maxExplainBytes]
	}
	return strings.ToValidUTF8(line, "")
}
