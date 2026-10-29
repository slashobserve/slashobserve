package grafana

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const ImporterVersion = 1

type Format string

const (
	formatClassic Format = "classic"
	formatExport  Format = "export"
	formatAPI     Format = "api"
)

type Datasource struct {
	UID     string
	Type    string
	Aliases []string
}

type Options struct {
	Datasources []Datasource
	Mapping     map[string]string
}

type Summary struct {
	Format        Format `json:"format"`
	Title         string `json:"title"`
	UID           string `json:"uid"`
	SchemaVersion int    `json:"schema_version"`
	Bytes         int    `json:"bytes"`
	Panels        int    `json:"panels"`
	Rows          int    `json:"rows"`
	Variables     int    `json:"variables"`
	FolderTitle   string `json:"-"`
}

type SourceDatasource struct {
	Key        string   `json:"key"`
	Name       *string  `json:"name"`
	Type       *string  `json:"type"`
	Kind       *string  `json:"kind"`
	Panels     int      `json:"panels"`
	Match      *string  `json:"match"`
	MatchedBy  *string  `json:"matched_by"`
	Candidates []string `json:"candidates"`
}

type Status string

const (
	statusImported    Status = "imported"
	statusPartial     Status = "partial"
	statusPlaceholder Status = "placeholder"
	statusUnverified  Status = "unverified"
)

type QueryLine struct {
	RefID      string   `json:"ref_id"`
	Ref        *string  `json:"ref"`
	Datasource *string  `json:"datasource"`
	Rewrites   []string `json:"rewrites"`
	Dropped    *string  `json:"dropped"`
}

type PanelLine struct {
	PanelID     string      `json:"panel_id"`
	GrafanaID   *int        `json:"grafana_id"`
	GrafanaPath string      `json:"grafana_path"`
	Title       string      `json:"title"`
	Type        string      `json:"type"`
	SourceType  string      `json:"source_type"`
	Row         *string     `json:"row"`
	Status      Status      `json:"status"`
	Reasons     []string    `json:"reasons"`
	Paths       []string    `json:"paths"`
	Queries     []QueryLine `json:"queries"`
}

type Note struct {
	Subject string   `json:"subject"`
	Status  Status   `json:"status"`
	Reason  string   `json:"reason"`
	Paths   []string `json:"paths"`
}

type Counts struct {
	Total       int `json:"total"`
	Imported    int `json:"imported"`
	Partial     int `json:"partial"`
	Placeholder int `json:"placeholder"`
	Unverified  int `json:"unverified"`
}

type Report struct {
	ImporterVersion int         `json:"importer_version"`
	Verified        bool        `json:"verified"`
	Counts          Counts      `json:"counts"`
	Panels          []PanelLine `json:"panels"`
	Notes           []Note      `json:"notes"`
}

type Result struct {
	Summary     Summary
	Datasources []SourceDatasource
	Spec        *spec.Dashboard
	Report      Report

	ready        bool
	variablePath []string
}

func (r *Result) Ready() bool {
	return r.ready
}

type InputError struct {
	Code    string
	Message string
}

func (e *InputError) Error() string {
	return e.Message
}

const (
	codeInvalidJSON       = "invalid_json"
	codeNotADashboard     = "not_a_dashboard"
	codeUnsupportedFormat = "unsupported_format"
	codeTooManyPanels     = "too_many_panels"
	codeInvalidMapping    = "invalid_mapping"
)

func inputErrorf(code, format string, args ...any) *InputError {
	return &InputError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func asObject(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func asArray(v any) ([]any, bool) {
	a, ok := v.([]any)
	return a, ok
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	case float64:
		return n, true
	}
	return 0, false
}

func asInt(v any) (int, bool) {
	f, ok := asFloat(v)
	if !ok || f != math.Trunc(f) || math.Abs(f) > 1<<53 {
		return 0, false
	}
	return int(f), true
}

func truthy(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case json.Number:
		f, ok := asFloat(b)
		return ok && f != 0
	}
	return false
}

func present(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	case bool:
		return x
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	return true
}

func cutRunes(s string, limit int) (string, bool) {
	if utf8.RuneCountInString(s) <= limit {
		return s, false
	}
	runes := []rune(s)
	return string(runes[:limit]), true
}

func stringList(v any) []string {
	items, _ := asArray(v)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func ptr[T any](v T) *T {
	return &v
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return strings.ToUpper(string(r)) + s[size:]
}

func sortedKeys(m map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
}
