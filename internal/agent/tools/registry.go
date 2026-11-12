package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/health"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/proposals"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/savedsearches"
	"github.com/slashobserve/slashobserve/internal/search"
	"github.com/slashobserve/slashobserve/internal/threads"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxContentBytes = 16 << 10
	listBudget      = 14 << 10

	codeUnknownTool    = "unknown_tool"
	codeInvalidRequest = "invalid_request"
	codeTooLarge       = "too_large"
	codeForbiddenRole  = "forbidden_role"

	MaxModelSpecBytes = 128 << 10
	maxSuggestResult  = 64 << 10
	maxBriefBytes     = 128 << 10
)

var (
	ErrResultTooLarge = errors.New("result too large")
	ErrPanicked       = errors.New("panicked")
	ErrInvariant      = errors.New("tool invariant broken")
)

const (
	FailureToolFailed     = "tool_failed"
	FailureResultTooLarge = "result_too_large"
	FailurePanicked       = "tool_panicked"
)

// FailureCode names why Call returned an error, without any of the error's text.
func FailureCode(err error) string {
	switch {
	case errors.Is(err, ErrResultTooLarge):
		return FailureResultTooLarge
	case errors.Is(err, ErrPanicked):
		return FailurePanicked
	}
	return FailureToolFailed
}

var resultLimits = map[string]int{nameGetDashboard: MaxModelSpecBytes, nameGetSpecGuide: 96 << 10, nameUpdateDashboard: maxSuggestResult, nameGetPanels: MaxModelSpecBytes, nameUpdatePanels: maxSuggestResult, nameGetRunBrief: maxBriefBytes}

const defaultNarrowing = "narrow the request"

var narrowing = map[string]string{
	nameListMetrics:     "pass search, or a smaller limit and the next_offset as offset",
	nameListLabels:      "pass metric or table, search, or a smaller limit and the next_offset as offset",
	nameListLabelValues: "pass metric or table, search, or a smaller limit and the next_offset as offset",
	nameGetSchema:       "pass table, or page on with the next_offset as offset",
	nameSampleRows:      "pass a smaller limit",
	nameQueryRange:      "shorten from and to, lower max_data_points, or reduce the series with a label selector or an aggregation",
	nameQueryInstant:    "shorten from and to, or reduce the series with a label selector or an aggregation",
	nameGetDashboard:    "use get_panels with the panel ids you need",
	nameGetPanels:       "ask for fewer panel ids",
	nameListDashboards:  "pass a smaller limit and page on with next_cursor",
}

const datasourceSilent = "the datasource did not answer"

type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	ReadOnly    bool
	MCPOnly     bool
	AgentOnly   bool
	Claim       bool
	MinRole     auth.Role

	InvestigateOnly bool
	Answers         bool

	AgentDescription string
	AgentInputSchema json.RawMessage
}

type Refusal string

const RefusalOnePanel Refusal = "one_panel"

type Result struct {
	Content    any
	IsError    bool
	Step       string
	Discovered *Discovery
	Refusal    Refusal
	Edits      Edits
}

type Edits struct {
	Panels   []string
	Added    []string
	Deleted  []string
	Settings bool
}

type Discovery struct {
	Datasource string
	Of         string
	Names      []string
}

type Deps struct {
	Datasources   *datasource.Registry
	Runner        *queryproxy.Runner
	Dashboards    *dashboards.Service
	Threads       *threads.Service
	Proposals     *proposals.Service
	Imports       *imports.Service
	Folders       *folders.Access
	Findings      *findings.Service
	Observations  *observations.Service
	SavedSearches *savedsearches.Service
	Health        *health.Service
	Annotations   *annotations.Service
	Activity      *activity.Store
	Runs          RunReader
	Claims        ClaimService
	RunSessions   RunSessions
	LiveText      LiveText
	Search        *search.Service
	Provider      llm.Provider
	PublicURL     *url.URL
	Logger        *slog.Logger
}

type Mode string

const (
	ModeCreate      Mode = "create"
	ModeRevise      Mode = "revise"
	ModeSuggest     Mode = "suggest"
	ModeInvestigate Mode = "investigate"
)

func (t Tool) OfferedIn(mode Mode) bool {
	switch {
	case t.MCPOnly:
		return false
	case t.Answers:
		return mode != ModeSuggest
	case mode == ModeInvestigate:
		return t.ReadOnly || t.InvestigateOnly
	default:
		return !t.InvestigateOnly
	}
}

type Run struct {
	Mode          Mode
	DashboardID   string
	ParentVersion int
	RequestID     string
	PanelID       string
	Prompt        string
	Base          *spec.Dashboard
	Build         bool
	Attempt       int
	From, To      time.Time
}

type runKey struct{}

func WithRun(ctx context.Context, run Run) context.Context {
	return context.WithValue(ctx, runKey{}, run)
}

func RunFrom(ctx context.Context) (Run, bool) {
	run, ok := ctx.Value(runKey{}).(Run)
	return run, ok
}

type handler func(ctx context.Context, input json.RawMessage) (Result, error)

type entry struct {
	tool   Tool
	handle handler
}

type surfaceFields struct{ mcp, agent []string }

type Registry struct {
	deps    Deps
	entries map[string]entry
	fields  map[string]surfaceFields
	tools   []Tool
	drafts  draftLimiter
}

const (
	maxDraftsPerCaller = 3
	maxDraftsInFlight  = 10
)

type draftLimiter struct {
	mu       sync.Mutex
	total    int
	byCaller map[string]int
}

func (r *Registry) acquireDraft(p auth.Principal) (release func(), err error) {
	key := string(p.Kind) + ":" + p.ID
	l := &r.drafts
	l.mu.Lock()
	defer l.mu.Unlock()
	switch {
	case l.byCaller[key] >= maxDraftsPerCaller:
		return nil, ErrDraftTooMany
	case l.total >= maxDraftsInFlight:
		return nil, ErrDraftBusy
	}
	if l.byCaller == nil {
		l.byCaller = map[string]int{}
	}
	l.byCaller[key]++
	l.total++
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.total--
			if l.byCaller[key]--; l.byCaller[key] == 0 {
				delete(l.byCaller, key)
			}
		})
	}, nil
}

func New(d Deps) *Registry {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	r := &Registry{deps: d, entries: map[string]entry{}, fields: map[string]surfaceFields{}}
	all := r.baseEntries()
	for _, e := range all {
		if e.tool.MinRole == "" {
			panic("tools: " + e.tool.Name + " has no MinRole")
		}
		if _, dup := r.entries[e.tool.Name]; dup {
			panic("tools: " + e.tool.Name + " registered twice")
		}
		e.tool.AgentDescription, e.tool.AgentInputSchema = agentWording.render(e.tool)
		e.tool.Description, e.tool.InputSchema = mcpWording.render(e.tool)
		r.fields[e.tool.Name] = surfaceFields{mcp: propertyNames(e.tool.InputSchema), agent: propertyNames(e.tool.AgentInputSchema)}
		r.entries[e.tool.Name] = e
		r.tools = append(r.tools, e.tool)
	}
	slices.SortFunc(r.tools, func(a, b Tool) int { return strings.Compare(a.Name, b.Name) })
	return r
}

var entryBuilders []func(*Registry) []entry

func registerEntries(build func(*Registry) []entry) {
	entryBuilders = append(entryBuilders, build)
}

func (r *Registry) baseEntries() []entry {
	var all []entry
	for _, build := range entryBuilders {
		all = append(all, build(r)...)
	}
	return all
}

func (r *Registry) List() []Tool {
	out := make([]Tool, len(r.tools))
	for i, t := range r.tools {
		t.InputSchema = bytes.Clone(t.InputSchema)
		t.AgentInputSchema = bytes.Clone(t.AgentInputSchema)
		out[i] = t
	}
	return out
}

func roleRefusal(p auth.Principal, tool string) string {
	if p.Kind == auth.KindUser {
		return fmt.Sprintf("your role (%s) cannot use %s; an admin can change your role", p.Role, tool)
	}
	return fmt.Sprintf("this key's role (%s) cannot use %s; an admin can change the service account's role", p.Role, tool)
}

func (r *Registry) Call(ctx context.Context, name string, input json.RawMessage) (Result, error) {
	return r.call(ctx, name, input, false)
}

func (r *Registry) CallClaim(ctx context.Context, name string, input json.RawMessage) (Result, error) {
	return r.call(ctx, name, input, true)
}

func (r *Registry) call(ctx context.Context, name string, input json.RawMessage, claimOnly bool) (Result, error) {
	e, ok := r.entries[name]
	if !ok || (claimOnly && !e.tool.Claim) {
		res := toolError(codeUnknownTool, fmt.Sprintf("unknown tool %q", clip(name, maxStepNameBytes)))
		res.Step = "Rejected an unknown tool"
		return res, nil
	}
	run, inRun := RunFrom(ctx)
	principal, hasPrincipal := auth.FromContext(ctx)
	if inRun && !hasPrincipal {
		return Result{}, fmt.Errorf("tool %s: %w: a run without a principal", name, ErrInvariant)
	}
	if !claimOnly && !allowed(principal, inRun, e.tool.MinRole) {
		res := toolError(codeForbiddenRole, roleRefusal(principal, name))
		res.Step = "Refused " + name + ": " + codeForbiddenRole
		return res, nil
	}
	if res, ok := offered(e.tool, run, inRun); !ok {
		return res, nil
	}
	res, err := r.handle(ctx, e, name, input)
	if err != nil {
		return Result{}, err
	}
	res = r.namingFields(name, inRun, res)
	if res, err = r.capped(ctx, name, res); err != nil {
		return Result{}, err
	}
	if eb, ok := res.Content.(errorBody); ok && res.IsError && res.Step == "" {
		res.Step = callErrorStep(name, eb)
	}
	return res, nil
}

func (r *Registry) namingFields(name string, inRun bool, res Result) Result {
	eb, ok := res.Content.(errorBody)
	if !ok || !eb.unknownField {
		return res
	}
	fields := r.fields[name].mcp
	if inRun {
		fields = r.fields[name].agent
	}
	eb.Message += "; " + name + " takes " + fieldList(fields)
	res.Content = eb
	return res
}

func (r *Registry) handle(ctx context.Context, e entry, name string, input json.RawMessage) (res Result, err error) {
	defer func() {
		if p := recover(); p != nil {
			r.deps.Logger.ErrorContext(ctx, "agent tool panicked",
				slog.String("tool", name),
				slog.String("panic_type", fmt.Sprintf("%T", p)))
			res, err = Result{}, fmt.Errorf("tool %s: %w: %w", name, ErrInvariant, ErrPanicked)
		}
	}()
	res, err = e.handle(ctx, input)
	switch {
	case err == nil:
		return res, nil
	case endsRun(ctx, err):
		return Result{}, fmt.Errorf("tool %s: %w: %w", name, ErrInvariant, err)
	}
	r.deps.Logger.WarnContext(ctx, "agent tool failed", logattrs.Failure(FailureToolFailed, err, slog.String("tool", name), runRequestAttr(ctx))...)
	return handlerFailed(name, err), nil
}

func (r *Registry) capped(ctx context.Context, name string, res Result) (Result, error) {
	body, err := json.Marshal(res.Content)
	if err != nil {
		return Result{}, fmt.Errorf("tool %s: %w: marshal result: %w", name, ErrInvariant, err)
	}
	limit := maxContentBytes
	if l, ok := resultLimits[name]; ok {
		limit = l
	}
	if len(body) <= limit {
		return res, nil
	}
	if trimmed, ok := trimmedSave(res.Content, limit); ok {
		res.Content = trimmed
		return res, nil
	}
	r.deps.Logger.ErrorContext(ctx, "agent tool result too large", slog.String("tool", name), slog.Int("bytes", len(body)))
	return tooLarge(name, len(body), limit)
}

func trimmedSave(content any, limit int) (any, bool) {
	switch c := content.(type) {
	case savedOut:
		panels, unverified := c.Panels, c.UnverifiedPanels
		c.Panels, c.UnverifiedPanels, c.Truncated = []panelOut{}, []string{}, true
		c.UnverifiedPanels, _ = fit(unverified, limit-jsonSize(c)-len(`,"unverified_panels":[]`))
		c.Panels, _ = fit(panels, limit-jsonSize(c))
		return c, jsonSize(c) <= limit
	case buildOut:
		changed := c.ChangedPanels
		c.ChangedPanels = []panelOut{}
		if jsonSize(c) > limit {
			c.Layout = nil
		}
		c.ChangedPanels, _ = fit(changed, limit-jsonSize(c))
		return c, jsonSize(c) <= limit
	}
	return nil, false
}

func jsonSize(v any) int {
	raw, err := json.Marshal(v)
	if err != nil {
		return math.MaxInt
	}
	return len(raw)
}

var runEnding = []error{
	context.Canceled,
	context.DeadlineExceeded,
	dashboards.ErrBuildLost,
	ErrPanicked,
	ErrInvariant,
	errInvalidRun,
	errCommentsNotConfigured,
	errFoldersNotConfigured,
	errImportsNotConfigured,
	errFindingsNotConfigured,
	errSavedSearchesNotConfigured,
	errHealthNotConfigured,
	errAnnotationsNotConfigured,
	errClaimsNotConfigured,
}

func endsRun(ctx context.Context, err error) bool {
	return ctx.Err() != nil || slices.ContainsFunc(runEnding, func(target error) bool { return errors.Is(err, target) })
}

func runRequestAttr(ctx context.Context) slog.Attr {
	run, _ := RunFrom(ctx)
	return slog.String("agent_request_id", run.RequestID)
}

func handlerFailed(name string, err error) Result {
	return toolError(FailureToolFailed, name+" failed: "+safeText(err))
}

func safeText(err error) string {
	var parts []string
	for _, a := range logattrs.Safe(err) {
		attr, ok := a.(slog.Attr)
		if !ok || attr.Key == "error_type" {
			continue
		}
		parts = append(parts, attr.Key+" "+attr.Value.String())
	}
	if len(parts) == 0 {
		return datasourceSilent
	}
	return strings.Join(parts, ", ")
}

func tooLarge(name string, size, limit int) (Result, error) {
	hint, ok := narrowing[name]
	if !ok {
		hint = defaultNarrowing
	}
	res := failure("Result too large: "+name, errorBody{
		Code:    FailureResultTooLarge,
		Message: fmt.Sprintf("the %s result is %d bytes, over the %d byte limit; %s", name, size, limit, hint),
	})
	if _, err := json.Marshal(res.Content); err != nil {
		return Result{}, fmt.Errorf("tool %s: %w: %w: %d bytes, over the %d byte limit", name, ErrInvariant, ErrResultTooLarge, size, limit)
	}
	return res, nil
}

func offered(t Tool, run Run, inRun bool) (Result, bool) {
	var message string
	switch {
	case t.MCPOnly:
		return Result{}, true
	case inRun && run.Mode == ModeInvestigate && !t.OfferedIn(run.Mode):
		message = "an investigate run only reads data and ends with " + nameSaveFinding + "; it cannot use " + t.Name
	case inRun && t.Answers && !t.OfferedIn(run.Mode):
		message = t.Name + " cannot end a suggest run; propose the panel instead"
	case inRun && !t.OfferedIn(run.Mode):
		message = t.Name + " is only for investigate runs"
	case !inRun && t.AgentOnly:
		message = nameSaveFinding + " runs inside the agent; MCP clients use " + nameCreateFinding
	default:
		return Result{}, true
	}
	res := toolError(codeNotInThisRun, message)
	res.Step = "Refused " + t.Name + ": " + codeNotInThisRun
	return res, false
}

func allowed(p auth.Principal, inRun bool, floor auth.Role) bool {
	return inRun || p.Role.AtLeast(floor)
}

func errorStep(tool, code string) string {
	if code == codeInvalidRequest {
		return "Rejected " + tool + " input: " + code
	}
	return "Could not run " + tool + ": " + code
}

func callErrorStep(tool string, eb errorBody) string {
	if eb.Code == codeInvalidRequest && eb.Message != "" {
		return "Rejected " + tool + " input: " + clip(eb.Message, maxStepReasonBytes)
	}
	return errorStep(tool, eb.Code)
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`

	unknownField bool
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
	var fields map[string]json.RawMessage
	if err := dec.Decode(&fields); err != nil {
		return invalid("%s", inputMessage(err)), false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return invalid("input must be a single JSON object"), false
	}
	known := jsonFields(reflect.TypeOf(dst).Elem())
	keys := slices.Sorted(maps.Keys(fields))
	var body bytes.Buffer
	body.WriteByte('{')
	for i, key := range keys {
		typ, ok := known[key]
		if !ok {
			return failure("", errorBody{Code: codeInvalidRequest, Message: fmt.Sprintf("unknown field %q", clip(key, maxStepNameBytes)), unknownField: true}), false
		}
		value := fields[key]
		if isIntegerType(typ) {
			n, ok := integralNumber(value)
			if !ok {
				return invalid("field %q must be an integer", key), false
			}
			value = n
		}
		if i > 0 {
			body.WriteByte(',')
		}
		name, _ := json.Marshal(key)
		body.Write(name)
		body.WriteByte(':')
		body.Write(value)
	}
	body.WriteByte('}')
	strict := json.NewDecoder(&body)
	strict.DisallowUnknownFields()
	if err := strict.Decode(dst); err != nil {
		return invalid("%s", inputMessage(err)), false
	}
	return Result{}, true
}

func jsonFields(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for i := range t.NumField() {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct {
			maps.Copy(out, jsonFields(f.Type))
			continue
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out[name] = f.Type
	}
	return out
}

func isIntegerType(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Kind() == reflect.Int
}

func integralNumber(raw json.RawMessage) (json.RawMessage, bool) {
	s := string(raw)
	if s == "" || (s[0] != '-' && (s[0] < '0' || s[0] > '9')) || !strings.ContainsAny(s, ".eE") {
		return raw, true
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f != math.Trunc(f) || math.Abs(f) > 1<<53 {
		return nil, false
	}
	return json.RawMessage(strconv.FormatInt(int64(f), 10)), true
}

func inputMessage(err error) string {
	var typeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError
	switch {
	case errors.As(err, &typeErr):
		if typeErr.Field == "" {
			return "input must be a JSON object"
		}
		if want, got := jsonTypeName(typeErr.Type), jsonValueName(typeErr.Value); want != "" && got != "" {
			return fmt.Sprintf("field %q must be %s, not %s", typeErr.Field, want, got)
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

func propertyNames(schema json.RawMessage) []string {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &parsed); err != nil {
		return nil
	}
	return slices.Sorted(maps.Keys(parsed.Properties))
}

func fieldList(names []string) string {
	switch len(names) {
	case 0:
		return "no fields"
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

func jsonTypeName(t reflect.Type) string {
	if t == nil {
		return ""
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		return "an array"
	case reflect.Map, reflect.Struct:
		return "an object"
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "a boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "an integer"
	case reflect.Float32, reflect.Float64:
		return "a number"
	}
	return ""
}

func jsonValueName(v string) string {
	word, _, _ := strings.Cut(v, " ")
	switch word {
	case "object":
		return "an object"
	case "array":
		return "an array"
	case "string":
		return "a string"
	case "number":
		return "a number"
	case "bool":
		return "a boolean"
	}
	return ""
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

type truncation struct {
	Truncated   bool   `json:"truncated"`
	TruncatedBy string `json:"truncated_by,omitempty"`
	NextOffset  *int   `json:"next_offset,omitempty"`
}

const (
	truncatedByBytes = "bytes"
	truncatedByLimit = "limit"
)

func page[T any](items []T, offset int, upstreamCut bool, budget int) ([]T, truncation) {
	kept, cut := fit(items[min(offset, len(items)):], budget)
	next := offset + len(kept)
	switch {
	case cut:
		return kept, truncation{Truncated: true, TruncatedBy: truncatedByBytes, NextOffset: &next}
	case upstreamCut:
		t := truncation{Truncated: true, TruncatedBy: truncatedByLimit}
		if next < queryproxy.MaxNamesLimit {
			t.NextOffset = &next
		}
		return kept, t
	}
	return kept, truncation{}
}

func clip(s string, n int) string { return queryproxy.ClipBytes(s, n) }

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
