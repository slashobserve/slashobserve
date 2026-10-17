package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	maxContentBytes = 16 << 10
	listBudget      = 14 << 10

	codeUnknownTool    = "unknown_tool"
	codeInvalidRequest = "invalid_request"
	codeTooLarge       = "too_large"
)

type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	ReadOnly    bool
}

type Result struct {
	Content any
	IsError bool
	Step    string
}

type Deps struct {
	Datasources *datasource.Registry
	Runner      *queryproxy.Runner
	Dashboards  *dashboards.Service
	PublicURL   *url.URL
	Logger      *slog.Logger
}

type Mode string

const (
	ModeCreate Mode = "create"
	ModeRevise Mode = "revise"
)

type Run struct {
	Mode          Mode
	DashboardID   string
	ParentVersion int
	RequestID     string
}

type runKey struct{}

func WithRun(ctx context.Context, run Run) context.Context {
	return context.WithValue(ctx, runKey{}, run)
}

func RunFrom(ctx context.Context) (Run, bool) {
	run, ok := ctx.Value(runKey{}).(Run)
	return run, ok
}

func DashboardURL(base *url.URL, id string) string {
	if base == nil {
		return "/dashboards/" + url.PathEscape(id)
	}
	return base.JoinPath("dashboards", id).String()
}

type handler func(ctx context.Context, input json.RawMessage) (Result, error)

type entry struct {
	tool   Tool
	handle handler
}

type Registry struct {
	deps    Deps
	entries map[string]entry
	tools   []Tool
}

func New(d Deps) *Registry {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	r := &Registry{deps: d, entries: map[string]entry{}}
	var all []entry
	all = append(all, r.discoveryEntries()...)
	all = append(all, r.queryEntries()...)
	all = append(all, r.dashboardEntries()...)
	all = append(all, r.guideEntries()...)
	for _, e := range all {
		r.entries[e.tool.Name] = e
		r.tools = append(r.tools, e.tool)
	}
	slices.SortFunc(r.tools, func(a, b Tool) int { return strings.Compare(a.Name, b.Name) })
	return r
}

func (r *Registry) List() []Tool {
	out := make([]Tool, len(r.tools))
	for i, t := range r.tools {
		t.InputSchema = bytes.Clone(t.InputSchema)
		out[i] = t
	}
	return out
}

func (r *Registry) Call(ctx context.Context, name string, input json.RawMessage) (res Result, err error) {
	e, ok := r.entries[name]
	if !ok {
		return toolError(codeUnknownTool, fmt.Sprintf("unknown tool %q", name)), nil
	}
	defer func() {
		if p := recover(); p != nil {
			r.deps.Logger.ErrorContext(ctx, "agent tool panicked",
				slog.String("tool", name),
				slog.String("panic_type", fmt.Sprintf("%T", p)),
				slog.String("stack", string(debug.Stack())))
			res, err = Result{}, fmt.Errorf("tool %s panicked", name)
		}
	}()
	res, err = e.handle(ctx, input)
	if err != nil {
		return Result{}, fmt.Errorf("tool %s: %w", name, err)
	}
	body, err := json.Marshal(res.Content)
	if err != nil {
		return Result{}, fmt.Errorf("tool %s: marshal result: %w", name, err)
	}
	if len(body) > maxContentBytes {
		r.deps.Logger.ErrorContext(ctx, "agent tool result too large", slog.String("tool", name), slog.Int("bytes", len(body)))
		return Result{}, fmt.Errorf("tool %s: result is %d bytes, over the %d byte limit", name, len(body), maxContentBytes)
	}
	return res, nil
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

func success(step string, content any) Result {
	return Result{Content: content, Step: step}
}

func failure(step string, content any) Result {
	return Result{Content: content, IsError: true, Step: step}
}

func toolError(code, message string) Result {
	return failure("", errorBody{Code: code, Message: message})
}

func invalid(format string, args ...any) Result {
	return toolError(codeInvalidRequest, fmt.Sprintf(format, args...))
}

func decodeInput(input json.RawMessage, dst any) (Result, bool) {
	raw := bytes.TrimSpace(input)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		raw = []byte("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return invalid("%s", inputMessage(err)), false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return invalid("input must be a single JSON object"), false
	}
	return Result{}, true
}

func inputMessage(err error) string {
	var typeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError
	switch {
	case errors.As(err, &typeErr):
		if typeErr.Field == "" {
			return "input must be a JSON object"
		}
		return fmt.Sprintf("field %q has the wrong type", typeErr.Field)
	case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF):
		return "input is not valid JSON"
	}
	if msg, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		return "unknown field " + msg
	}
	return "input is not valid"
}

func fit[T any](items []T, budget int) ([]T, bool) {
	size := 0
	for i, item := range items {
		b, err := json.Marshal(item)
		if err != nil {
			return items[:i], true
		}
		size += len(b) + 1
		if size > budget {
			return items[:i], true
		}
	}
	return items, false
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !isRuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
