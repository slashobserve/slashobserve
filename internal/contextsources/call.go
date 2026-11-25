package contextsources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	DefaultCallTimeout    = 20 * time.Second
	DefaultCallsInFlight  = 4
	DefaultResultBytes    = 12 << 10
	MaxArgumentBytes      = 8 << 10
	maxOfferedDescription = 200
	maxOfferedArguments   = 8
	maxNamedTools         = 20

	reasonToolError = "tool_error"

	actionCall audit.Action = "context_source.call"
)

type Limits struct {
	Timeout     time.Duration
	InFlight    int
	ResultBytes int
}

func (l Limits) filled() Limits {
	if l.Timeout <= 0 {
		l.Timeout = DefaultCallTimeout
	}
	if l.InFlight <= 0 {
		l.InFlight = DefaultCallsInFlight
	}
	if l.ResultBytes <= 0 {
		l.ResultBytes = DefaultResultBytes
	}
	return l
}

type gate struct {
	mu   sync.Mutex
	held map[string]int
}

func (g *gate) acquire(uid string, limit int) (release func(), ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.held[uid] >= limit {
		return nil, false
	}
	if g.held == nil {
		g.held = map[string]int{}
	}
	g.held[uid]++
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			if g.held[uid]--; g.held[uid] == 0 {
				delete(g.held, uid)
			}
		})
	}, true
}

func (s *Service) Offers(ctx context.Context) ([]tools.ContextOffer, error) {
	access, ok := datasource.AccessFrom(ctx)
	if !ok {
		return nil, datasource.ErrNoAccess
	}
	sources, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	return offered(ctx, access, sources, func(uid string) ([]Tool, error) { return s.Tools(ctx, uid) })
}

func offered(ctx context.Context, access datasource.Access, sources []Source, toolsOf func(uid string) ([]Tool, error)) ([]tools.ContextOffer, error) {
	var out []tools.ContextOffer
	for _, source := range sources {
		if source.Tools.Approved == 0 || !access.Reads(source.Audience) {
			continue
		}
		stored, err := toolsOf(source.UID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		datasource.NoteRead(ctx, source.Audience)
		out = append(out, offerOf(source, stored))
	}
	return out, nil
}

func offerOf(source Source, stored []Tool) tools.ContextOffer {
	offer := tools.ContextOffer{UID: source.UID, Name: source.Name, Host: hostOfURL(source.URL), Description: source.Description, Unreachable: source.State == StateUnreachable, Failure: source.Failure}
	for _, tool := range stored {
		if tool.State == ToolApproved {
			offer.Tools = append(offer.Tools, tools.ContextOfferedTool{Name: tool.Name, Description: firstLine(tool.Pinned.Description), Arguments: argumentsOf(tool.Pinned.InputSchema)})
		}
	}
	return offer
}

func firstLine(description string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(description), "\n")
	return queryproxy.ClipBytes(strings.TrimSpace(line), maxOfferedDescription)
}

func argumentsOf(schema json.RawMessage) string {
	var parsed struct {
		Properties map[string]struct {
			Type any `json:"type"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if json.Unmarshal(schema, &parsed) != nil || len(parsed.Properties) == 0 {
		return ""
	}
	names := make([]string, 0, len(parsed.Properties))
	for name := range parsed.Properties {
		names = append(names, name)
	}
	slices.SortFunc(names, func(a, b string) int {
		ra, rb := slices.Contains(parsed.Required, a), slices.Contains(parsed.Required, b)
		switch {
		case ra && !rb:
			return -1
		case rb && !ra:
			return 1
		}
		return strings.Compare(a, b)
	})
	more := len(names) - maxOfferedArguments
	if more > 0 {
		names = names[:maxOfferedArguments]
	}
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = name
		if t, ok := parsed.Properties[name].Type.(string); ok {
			parts[i] += " (" + t
			if slices.Contains(parsed.Required, name) {
				parts[i] += ", required"
			}
			parts[i] += ")"
		} else if slices.Contains(parsed.Required, name) {
			parts[i] += " (required)"
		}
	}
	if more > 0 {
		parts = append(parts, fmt.Sprintf("and %d more", more))
	}
	return strings.Join(parts, ", ")
}

type callAttempt struct {
	uid       string
	tool      string
	row       dbsqlc.ContextSource
	known     bool
	arguments map[string]any
	result    tools.ContextResult
}

func (s *Service) Call(ctx context.Context, uid, tool string, arguments json.RawMessage) (tools.ContextResult, error) {
	start := time.Now()
	attempt := callAttempt{uid: uid, tool: tool}
	result, err := s.call(ctx, &attempt, arguments)
	s.recordCall(ctx, attempt, err, time.Since(start))
	return result, err
}

func (s *Service) call(ctx context.Context, attempt *callAttempt, arguments json.RawMessage) (tools.ContextResult, error) {
	access, ok := datasource.AccessFrom(ctx)
	if !ok {
		return tools.ContextResult{}, datasource.ErrNoAccess
	}
	row, err := s.row(ctx, attempt.uid)
	if errors.Is(err, ErrNotFound) {
		return tools.ContextResult{}, notFound(attempt.uid)
	}
	if err != nil {
		return tools.ContextResult{}, err
	}
	attempt.row, attempt.known = row, true
	rows, err := s.db.Queries().ListContextSourceTools(ctx, row.ID)
	if err != nil {
		return tools.ContextResult{}, err
	}
	stored := make([]Tool, len(rows))
	for i, r := range rows {
		stored[i] = toolOf(r)
	}
	return s.dispatch(ctx, access, attempt, stored, arguments)
}

func (s *Service) dispatch(ctx context.Context, access datasource.Access, attempt *callAttempt, stored []Tool, arguments json.RawMessage) (tools.ContextResult, error) {
	row := attempt.row
	audience := datasource.Audience(row.Audience)
	if !access.Reads(audience) {
		if refusal := access.Refusal(audience); errors.Is(refusal, datasource.ErrWithheld) {
			return tools.ContextResult{}, refusal
		}
		return tools.ContextResult{}, notFound(attempt.uid)
	}
	pinned, err := approvedTool(row.Uid, stored, attempt.tool)
	if err != nil {
		return tools.ContextResult{}, err
	}
	args, err := argumentsFor(pinned, arguments)
	if err != nil {
		return tools.ContextResult{}, err
	}
	attempt.arguments = loggedArguments(pinned.Pinned.InputSchema, arguments)
	release, ok := s.gate.acquire(row.Uid, s.limits.InFlight)
	if !ok {
		return tools.ContextResult{}, &tools.ContextFailure{Code: tools.ContextCodeSourceBusy, Message: fmt.Sprintf("the context source %s already has %d calls in flight from this server; wait for them or go on without it", row.Uid, s.limits.InFlight)}
	}
	defer release()
	secret, failure := s.open(row)
	if failure != "" {
		return tools.ContextResult{}, down(row.Uid, failure, s.limits.Timeout)
	}
	callCtx, cancel := context.WithTimeout(ctx, s.limits.Timeout)
	defer cancel()
	called, err := s.connector.Call(callCtx, connectTarget(row, secret), attempt.tool, args)
	if ctx.Err() != nil {
		return tools.ContextResult{}, ctx.Err()
	}
	if err != nil {
		return tools.ContextResult{}, down(row.Uid, failureCode(err), s.limits.Timeout)
	}
	datasource.NoteRead(ctx, audience)
	text, cut := cutText(called.Text, s.limits.ResultBytes)
	attempt.result = tools.ContextResult{Text: text, Bytes: len(called.Text), Cut: cut, IsError: called.IsError}
	return attempt.result, nil
}

func notFound(uid string) error {
	return &tools.ContextFailure{Code: tools.ContextCodeSourceNotFound, Message: fmt.Sprintf("no context source %q; call list_context_sources", queryproxy.ClipBytes(uid, 80))}
}

func approvedTool(uid string, stored []Tool, name string) (Tool, error) {
	i := slices.IndexFunc(stored, func(t Tool) bool { return t.Name == name })
	if i < 0 {
		return Tool{}, unknownTool(uid, stored, name)
	}
	if tool := stored[i]; tool.State != ToolApproved {
		return Tool{}, &tools.ContextFailure{Code: tools.ContextCodeToolNotApproved, Message: fmt.Sprintf("the tool %s of %s is not approved: %s", name, uid, notApprovedWording[tool.State](tool))}
	}
	return stored[i], nil
}

var notApprovedWording = map[ToolState]func(Tool) string{
	ToolOffered: func(Tool) string { return "an Admin has not approved it" },
	ToolChanged: func(Tool) string {
		return "the server describes it differently from what an Admin approved, so it is not served until an Admin approves it again"
	},
	ToolGone:     func(Tool) string { return "the server no longer lists it" },
	ToolRefused:  func(t Tool) string { return "it is refused: " + t.Refusal },
	ToolApproved: func(Tool) string { return "" },
}

func unknownTool(uid string, stored []Tool, name string) error {
	message := fmt.Sprintf("the source %s has no approved tool %q", uid, queryproxy.ClipBytes(name, 80))
	var approved []string
	for _, t := range stored {
		if t.State == ToolApproved {
			approved = append(approved, t.Name)
		}
	}
	if len(approved) > maxNamedTools {
		approved = approved[:maxNamedTools]
	}
	if len(approved) > 0 {
		message += "; approved tools are " + strings.Join(approved, ", ")
	}
	return &tools.ContextFailure{Code: tools.ContextCodeToolNotApproved, Message: message}
}

func argumentsFor(tool Tool, raw json.RawMessage) (map[string]any, error) {
	body := bytes.TrimSpace(raw)
	if len(body) == 0 || bytes.Equal(body, []byte("null")) {
		body = []byte("{}")
	}
	if len(body) > MaxArgumentBytes {
		return nil, &tools.ContextFailure{Code: tools.ContextCodeInvalidArguments, Message: fmt.Sprintf("arguments are over the %d byte limit", MaxArgumentBytes)}
	}
	if err := checkArguments(tool.Pinned.InputSchema, body); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var args map[string]any
	if err := decoder.Decode(&args); err != nil {
		return nil, &tools.ContextFailure{Code: tools.ContextCodeInvalidArguments, Message: "arguments must be a JSON object"}
	}
	return args, nil
}

var downWording = map[string]string{
	FailureNotAllowed:   "its address is not on the outbound domain list",
	FailureUnauthorized: "it refused the stored secret",
	FailureTimeout:      "it did not answer in time",
	FailureNotMCP:       "it did not answer as an MCP server",
	FailureUnreachable:  "it could not be reached",
	FailureNoSecretKey:  "this server cannot read its secret",
	FailureUnreadable:   "this server cannot read its secret",
}

func down(uid, reason string, timeout time.Duration) error {
	why := downWording[reason]
	if why == "" {
		why = "it could not be reached"
	}
	if reason == FailureTimeout {
		why = fmt.Sprintf("it did not answer within %d seconds", int(timeout/time.Second))
	}
	return &tools.ContextFailure{Code: tools.ContextCodeSourceDown, Reason: reason, Message: fmt.Sprintf("the context source %s is not available: %s. Say in your answer that it could not be read, and go on without it", uid, why)}
}

func cutText(text string, limit int) (string, bool) {
	escaped := 0
	for at := 0; at < len(text); {
		_, width := utf8.DecodeRuneInString(text[at:])
		escaped += escapedSize(text[at : at+width])
		if escaped > limit {
			return text[:at], true
		}
		at += width
	}
	return text, false
}

func escapedSize(piece string) int {
	encoded, err := json.Marshal(piece)
	if err != nil {
		return len(piece) * 6
	}
	return len(encoded) - 2
}

func (s *Service) recordCall(ctx context.Context, attempt callAttempt, err error, took time.Duration) {
	if recorded := audit.RecordNow(context.WithoutCancel(ctx), callEvent(attempt, err, took)); recorded != nil && !errors.Is(recorded, audit.ErrNotInstalled) {
		s.logger.WarnContext(ctx, "context source call not recorded", logattrs.Failure("audit_record_failed", recorded, slog.String("uid", attempt.uid))...)
	}
}

func callEvent(attempt callAttempt, err error, took time.Duration) audit.Event {
	e := audit.Event{Action: actionCall, Outcome: audit.OutcomeSuccess, Duration: took, Target: audit.Target{Kind: targetKind, ID: attempt.uid}}
	data := map[string]any{"tool": queryproxy.ClipBytes(attempt.tool, maxToolNameBytes)}
	if attempt.known {
		e.Target = targetOf(attempt.row)
		data["host"] = hostOfURL(attempt.row.Url)
		if attempt.row.AllowPrivate {
			data["private"] = true
		}
	}
	if len(attempt.arguments) > 0 {
		data["arguments"] = attempt.arguments
	}
	switch {
	case err != nil:
		e.Outcome, e.Reason = outcomeOf(err)
	case attempt.result.IsError:
		e.Outcome, e.Reason = audit.OutcomeFailed, reasonToolError
	}
	if attempt.result.Cut {
		data["truncated"] = true
	}
	e.Data = data
	return e
}

func outcomeOf(err error) (audit.Outcome, string) {
	var failure *tools.ContextFailure
	switch {
	case errors.Is(err, datasource.ErrWithheld):
		return audit.OutcomeDenied, "withheld"
	case errors.As(err, &failure):
		reason := failure.Reason
		if reason == "" {
			reason = failure.Code
		}
		if failure.Code == tools.ContextCodeSourceNotFound {
			return audit.OutcomeDenied, reason
		}
		return audit.OutcomeFailed, reason
	}
	return audit.OutcomeFailed, "call_failed"
}
