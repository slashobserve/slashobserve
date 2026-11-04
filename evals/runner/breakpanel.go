package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/grafana"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	brokenSuffix     = "_broken_by_eval"
	maxBreakAttempts = 20

	fieldExpr   = "expr"
	fieldRawSQL = "rawSql"
	fieldQuery  = "query"
)

const sqlName = `(?:[A-Za-z_][A-Za-z0-9_]*|\$\{[^}]*\}|\$[A-Za-z_][A-Za-z0-9_]*)`

var (
	groupingWords        = map[string]bool{"by": true, "without": true, "on": true, "ignoring": true, "group_left": true, "group_right": true}
	aggregationModifiers = []string{"by", "without"}
	keywordWords         = map[string]bool{"and": true, "or": true, "unless": true, "offset": true, "bool": true, "inf": true, "nan": true, "atan2": true}
	queryFields          = []string{fieldExpr, fieldRawSQL, fieldQuery}
	sqlStatement         = regexp.MustCompile(`(?i)^\s*(select|with)\b`)
	fromTable            = regexp.MustCompile(`(?i)\bfrom\s+((?:` + sqlName + `\.)?` + sqlName + `)`)
)

func init() { panelBreaker = breakPanel }

type querySite struct {
	target map[string]any
	field  string
}

var hostIndependentMetrics = map[string]bool{
	"node_uname_info":                   true,
	"node_boot_time_seconds":            true,
	"node_time_seconds":                 true,
	"node_load1":                        true,
	"node_load5":                        true,
	"node_load15":                       true,
	"node_cpu_seconds_total":            true,
	"node_context_switches_total":       true,
	"node_forks_total":                  true,
	"node_intr_total":                   true,
	"node_memory_MemTotal_bytes":        true,
	"node_memory_MemFree_bytes":         true,
	"node_memory_MemAvailable_bytes":    true,
	"node_filesystem_size_bytes":        true,
	"node_filesystem_avail_bytes":       true,
	"node_filesystem_free_bytes":        true,
	"node_network_receive_bytes_total":  true,
	"node_network_transmit_bytes_total": true,
}

func (s querySite) servedByEveryHost() bool {
	text, _ := s.target[s.field].(string)
	if s.field == fieldRawSQL {
		return false
	}
	end, ok := firstMetricEnd(text)
	return ok && hostIndependentMetrics[text[skipBackWhile(text, end, isIdentByte):end]]
}

func skipBackWhile(s string, end int, keep func(byte) bool) int {
	i := end
	for i > 0 && keep(s[i-1]) {
		i--
	}
	return i
}

func breakPanel(source []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(source))
	dec.UseNumber()
	var root map[string]any
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("decode the dashboard: %w", err)
	}
	opts := stackOptions()
	pristine, err := convertSource(source, opts)
	if err != nil {
		return nil, err
	}
	tree := root
	if inner, ok := root["dashboard"].(map[string]any); ok {
		tree = inner
	}
	sites := querySites(tree)
	slices.SortStableFunc(sites, func(a, b querySite) int {
		switch {
		case a.servedByEveryHost() == b.servedByEveryHost():
			return 0
		case a.servedByEveryHost():
			return -1
		}
		return 1
	})
	for _, site := range sites[:min(len(sites), maxBreakAttempts)] {
		original, _ := site.target[site.field].(string)
		renamed, ok := renameSubject(site.field, original)
		if !ok {
			continue
		}
		site.target[site.field] = renamed
		broken, err := encodeTree(root)
		site.target[site.field] = original
		if err != nil {
			return nil, err
		}
		if breaksOnePanel(pristine, broken, opts) {
			return broken, nil
		}
	}
	return nil, errors.New("no query of the dashboard can be broken so that exactly one data panel changes")
}

func breaksOnePanel(pristine *spec.Dashboard, broken []byte, opts grafana.Options) bool {
	changed, err := convertSource(broken, opts)
	if err != nil {
		return false
	}
	id, err := changedPanel(spec.Diff(pristine, changed))
	if err != nil {
		return false
	}
	_, ok := changed.FindDataPanel(id)
	return ok
}

func encodeTree(root map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(root); err != nil {
		return nil, fmt.Errorf("encode the broken dashboard: %w", err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func querySites(tree map[string]any) []querySite {
	sites := appendPanelSites(nil, tree["panels"])
	rows, _ := tree["rows"].([]any)
	for _, raw := range rows {
		if row, ok := raw.(map[string]any); ok {
			sites = appendPanelSites(sites, row["panels"])
		}
	}
	return sites
}

func appendPanelSites(sites []querySite, node any) []querySite {
	panels, _ := node.([]any)
	for _, raw := range panels {
		panel, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		targets, _ := panel["targets"].([]any)
		for _, rawTarget := range targets {
			target, ok := rawTarget.(map[string]any)
			if !ok {
				continue
			}
			if hidden, _ := target["hide"].(bool); hidden {
				continue
			}
			if field := queryField(target); field != "" {
				sites = append(sites, querySite{target: target, field: field})
			}
		}
		sites = appendPanelSites(sites, panel["panels"])
	}
	return sites
}

func queryField(target map[string]any) string {
	for _, field := range queryFields {
		if text, _ := target[field].(string); !blank(text) {
			return field
		}
	}
	return ""
}

func renameSubject(field, text string) (string, bool) {
	end, ok := 0, false
	switch field {
	case fieldExpr:
		end, ok = firstMetricEnd(text)
	case fieldRawSQL:
		end, ok = firstTableEnd(text)
	default:
		if end, ok = firstTableEnd(text); !ok && !sqlStatement.MatchString(text) {
			end, ok = firstMetricEnd(text)
		}
	}
	if !ok {
		return "", false
	}
	return text[:end] + brokenSuffix + text[end:], true
}

func firstTableEnd(sql string) (int, bool) {
	variableEnd := -1
	for _, m := range fromTable.FindAllStringSubmatchIndex(sql, -1) {
		token := sql[m[2]:m[3]]
		if !strings.HasPrefix(token[strings.LastIndex(token, ".")+1:], "$") {
			return m[3], true
		}
		if variableEnd < 0 {
			variableEnd = m[3]
		}
	}
	return variableEnd, variableEnd >= 0
}

func firstMetricEnd(expr string) (int, bool) {
	for i := 0; i < len(expr); {
		c := expr[i]
		switch {
		case isQuote(c):
			i = skipQuoted(expr, i)
		case c == '{':
			i = skipBalanced(expr, i, '{', '}')
		case c == '[':
			i = skipBalanced(expr, i, '[', ']')
		case c == '$':
			i = skipVariable(expr, i)
		case isDigit(c):
			i = skipWhile(expr, i, isWordByte)
		case isIdentStart(c):
			end := skipWhile(expr, i, isIdentByte)
			word := expr[i:end]
			switch {
			case groupingWords[word]:
				i = skipGroup(expr, end)
			case keywordWords[word] || callOrGrouping(expr, end):
				i = end
			default:
				return end, true
			}
		default:
			i++
		}
	}
	return 0, false
}

func callOrGrouping(expr string, from int) bool {
	rest := strings.TrimLeft(expr[from:], " \t\r\n")
	if strings.HasPrefix(rest, "(") {
		return true
	}
	for _, word := range aggregationModifiers {
		if after, ok := strings.CutPrefix(rest, word); ok && (after == "" || !isIdentByte(after[0])) {
			return true
		}
	}
	return false
}

func skipGroup(expr string, from int) int {
	i := skipWhile(expr, from, func(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' })
	if i < len(expr) && expr[i] == '(' {
		return skipBalanced(expr, i, '(', ')')
	}
	return from
}

func skipVariable(s string, from int) int {
	if from+1 < len(s) && s[from+1] == '{' {
		return skipBalanced(s, from+1, '{', '}')
	}
	return skipWhile(s, from+1, isWordByte)
}

func skipBalanced(s string, from int, open, closing byte) int {
	depth := 0
	for i := from; i < len(s); {
		switch c := s[i]; {
		case isQuote(c):
			i = skipQuoted(s, i)
			continue
		case c == open:
			depth++
		case c == closing:
			depth--
			if depth == 0 {
				return i + 1
			}
		}
		i++
	}
	return len(s)
}

func skipQuoted(s string, from int) int {
	quote := s[from]
	for i := from + 1; i < len(s); i++ {
		switch {
		case s[i] == '\\' && quote != '`':
			i++
		case s[i] == quote:
			return i + 1
		}
	}
	return len(s)
}

func skipWhile(s string, from int, keep func(byte) bool) int {
	i := from
	for i < len(s) && keep(s[i]) {
		i++
	}
	return i
}

func isQuote(c byte) bool { return c == '"' || c == '\'' || c == '`' }

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentByte(c byte) bool { return isIdentStart(c) || isDigit(c) || c == ':' }

func isWordByte(c byte) bool { return isIdentStart(c) || isDigit(c) || c == '.' }
