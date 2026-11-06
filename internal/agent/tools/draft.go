package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

//go:embed draft_prompt.md
var draftSystem string

const (
	nameDraftQuery      = "draft_query"
	nameSubmitQuery     = "submit_query"
	nameCannotTranslate = "cannot_translate"

	CodeCannotTranslate = "cannot_translate"

	maxQuestionRunes = 2000
	maxReasonRunes   = 300
	draftDeadline    = 60 * time.Second
	lastTurnNote     = "\nThis is your last turn: call submit_query with a fixed query, or cannot_translate."
	stillFails       = "The drafted query still fails after one correction: "

	draftUnavailableMessage = "No model is configured on this server, so Ask is off."

	reasonNoQuery  = "The model did not return a query."
	reasonDeclined = "The model declined this question."
	reasonCutOff   = "The model's answer was cut off."
	reasonNoReplay = "No replay fixture matches this question."

	descSubmitQuery     = "Answer with the query or trace filters for the question, and one sentence saying what they match."
	descCannotTranslate = "Answer that this datasource cannot answer the question, with one sentence saying why."

	descDraftQuery = "Write a logs query, or trace filters for the trace list, for one Loki, ClickHouse or Tempo datasource from a plain-language question. " +
		"The server first reads the datasource's labels or columns, then checks the answer with the lint check and one run over the last 15 minutes of the range; it never runs the full query. " +
		"It spends at most two of this server's model calls per question, the second only to fix a query that failed its check, and stops after 60 seconds; a key may have 3 questions in flight. " +
		"Returns query and language (logql on Loki, clickhouse_sql on ClickHouse logs, traceql on Tempo), or trace_filters with language filters (ClickHouse traces always, Tempo when the filters are enough), with a one-sentence explanation and warnings. " +
		"The code cannot_translate means the datasource cannot answer the question, with the reason as the message. Run a returned query with query_instant."

	schemaDraftQuery = `{"type":"object","properties":{` +
		`"datasource_uid":{"type":"string","minLength":1,"description":"Uid of a Loki, ClickHouse or Tempo datasource, from list_datasources."},` +
		`"question":{"type":"string","minLength":1,"maxLength":2000,"description":"The question in plain language, such as error logs from checkout with a trace id."},` +
		`"from":{"type":"string","description":"Start of the range the answer is for: a relative time such as now-1h, epoch milliseconds or an RFC 3339 time. Default now-1h."},` +
		`"to":{"type":"string","description":"End of the range, in the same forms. Default now."},` +
		`"signal":{"type":"string","enum":["logs","traces"],"description":"Which data the question is about. Loki defaults to logs and Tempo to traces; ClickHouse needs it."}},` +
		`"required":["datasource_uid","question"],"additionalProperties":false}`
)

var (
	ErrDraftUnavailable = errors.New("no model is configured")
	ErrDraftInvalid     = errors.New("invalid draft request")
	ErrDraftNotFound    = errors.New("datasource not found")
	ErrDraftTooMany     = fmt.Errorf("you already have %d questions in flight; wait for one to finish", maxDraftsPerCaller)
	ErrDraftBusy        = fmt.Errorf("the server is answering %d questions; try again shortly", maxDraftsInFlight)
	ErrDraftProvider    = errors.New("the model provider failed")
	ErrDraftTimeout     = fmt.Errorf("the draft did not finish within %d seconds", int(draftDeadline.Seconds()))
)

var draftCodes = []struct {
	err  error
	code string
}{
	{ErrDraftInvalid, codeInvalidRequest},
	{ErrDraftNotFound, "not_found"},
	{ErrDraftTooMany, "too_many_requests"},
	{ErrDraftBusy, "agent_busy"},
	{ErrDraftProvider, "provider_error"},
	{ErrDraftTimeout, "draft_timeout"},
}

type DraftInput struct {
	DatasourceUID string
	Question      string
	Signal        string
	From          string
	To            string
}

type TraceFilters struct {
	Service string `json:"service,omitempty"`
	Span    string `json:"span,omitempty"`
	Min     string `json:"min,omitempty"`
	Max     string `json:"max,omitempty"`
	Errors  bool   `json:"errors,omitempty"`
}

type Draft struct {
	Query        string        `json:"query"`
	Language     string        `json:"language"`
	TraceFilters *TraceFilters `json:"trace_filters"`
	Explanation  string        `json:"explanation"`
	Warnings     []string      `json:"warnings"`
}

type DatasourceError struct {
	Code    string
	Message string
}

func (e *DatasourceError) Error() string { return e.Code + ": " + e.Message }

type CannotTranslate struct{ Reason string }

func (e *CannotTranslate) Error() string { return e.Reason }

type invalidDraft struct{ rule string }

func (e *invalidDraft) Error() string { return e.rule }

func (e *invalidDraft) Is(target error) bool { return target == ErrDraftInvalid }

func invalidDraftf(format string, args ...any) error {
	return &invalidDraft{rule: fmt.Sprintf(format, args...)}
}

func DraftFailure(err error) (code, message string, ok bool) {
	var cannot *CannotTranslate
	var dsErr *DatasourceError
	switch {
	case errors.As(err, &cannot):
		return CodeCannotTranslate, cannot.Reason, true
	case errors.As(err, &dsErr):
		return dsErr.Code, dsErr.Message, true
	case errors.Is(err, ErrDraftUnavailable):
		return "agent_unavailable", draftUnavailableMessage, true
	}
	for _, c := range draftCodes {
		if errors.Is(err, c.err) {
			return c.code, err.Error(), true
		}
	}
	return "", "", false
}

func init() { registerEntries((*Registry).draftEntries) }

func (r *Registry) draftEntries() []entry {
	return []entry{
		{Tool{Name: nameDraftQuery, Description: descDraftQuery, InputSchema: json.RawMessage(schemaDraftQuery), ReadOnly: true, MCPOnly: true, MinRole: auth.RoleViewer}, r.draftQuery},
	}
}

func (r *Registry) draftQuery(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		DatasourceUID string `json:"datasource_uid"`
		Question      string `json:"question"`
		Signal        string `json:"signal"`
		From          string `json:"from"`
		To            string `json:"to"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	draft, err := r.DraftQuery(ctx, DraftInput(in))
	if err != nil {
		code, message, ok := DraftFailure(err)
		if !ok {
			return Result{}, err
		}
		return toolError(code, message), nil
	}
	return success("Wrote a "+draft.Language+" draft", draft), nil
}

func (r *Registry) DraftQuery(ctx context.Context, in DraftInput) (Draft, error) {
	if r.deps.Provider == nil {
		return Draft{}, ErrDraftUnavailable
	}
	run, question, err := r.draftRequest(in)
	if err != nil {
		return Draft{}, err
	}
	principal, _ := auth.FromContext(ctx)
	release, err := r.acquireDraft(principal)
	if err != nil {
		return Draft{}, err
	}
	defer release()
	draftCtx, cancel := context.WithTimeout(ctx, draftDeadline)
	defer cancel()
	started := time.Now()
	draft, stats, err := r.draft(draftCtx, run, question)
	err = timedOut(ctx, draftCtx, err)
	r.logDraft(ctx, run, stats, time.Since(started), err)
	return draft, err
}

func timedOut(caller, draftCtx context.Context, err error) error {
	if err != nil && caller.Err() == nil && errors.Is(draftCtx.Err(), context.DeadlineExceeded) {
		return ErrDraftTimeout
	}
	return err
}

func (r *Registry) draftRequest(in DraftInput) (draftRun, string, error) {
	question, ok := revisions.CleanPrompt(in.Question)
	if !ok || utf8.RuneCountInString(question) > maxQuestionRunes {
		return draftRun{}, "", invalidDraftf("question must be 1 to %d characters of text without control characters", maxQuestionRunes)
	}
	if in.Signal != "" && in.Signal != signalLogs && in.Signal != signalTraces {
		return draftRun{}, "", invalidDraftf("signal must be logs or traces")
	}
	if in.DatasourceUID == "" {
		return draftRun{}, "", invalidDraftf("datasource_uid is required")
	}
	inst, found := r.deps.Datasources.Get(in.DatasourceUID)
	if !found {
		return draftRun{}, "", ErrDraftNotFound
	}
	dsType := typeOf(inst)
	signal, err := draftSignal(dsType, inst.UID, in.Signal)
	if err != nil {
		return draftRun{}, "", err
	}
	rng, res, ok := resolveRange(in.From, in.To, time.Now())
	if !ok {
		return draftRun{}, "", invalidDraftf("%s", resultMessage(res))
	}
	return draftRun{inst: inst, dsType: dsType, signal: signal, rng: rng}, question, nil
}

func draftSignal(dsType spec.DatasourceType, uid, signal string) (string, error) {
	switch dsType {
	case spec.Loki:
		if signal == "" || signal == signalLogs {
			return signalLogs, nil
		}
		return "", invalidDraftf("%s is a Loki datasource, which holds logs; signal must be logs", clip(uid, maxStepNameBytes))
	case spec.Tempo:
		if signal == "" || signal == signalTraces {
			return signalTraces, nil
		}
		return "", invalidDraftf("%s is a Tempo datasource, which holds traces; signal must be traces", clip(uid, maxStepNameBytes))
	case spec.ClickHouse:
		if signal == "" {
			return "", invalidDraftf("signal is required on a ClickHouse datasource: logs or traces")
		}
		return signal, nil
	}
	return "", invalidDraftf("draft_query works on Loki, ClickHouse and Tempo datasources; %s is a %s datasource", clip(uid, maxStepNameBytes), dsType)
}

type draftStats struct {
	calls int
	reads int
	usage llm.Usage
}

func (r *Registry) draft(ctx context.Context, run draftRun, question string) (Draft, draftStats, error) {
	var stats draftStats
	facts, err := r.draftContext(ctx, run.inst, run.signal, run.rng)
	stats.reads = facts.Reads
	if err != nil {
		return Draft{}, stats, err
	}
	run.facts = facts
	session, err := r.deps.Provider.Start(ctx, llm.Request{System: draftSystem, Tools: draftTools(run), Prompt: question, Context: facts.Text})
	if err != nil {
		return Draft{}, stats, modelFailure(err)
	}
	next := func(results []llm.ToolResult) (llm.Turn, error) {
		stats.calls++
		turn, err := session.Next(ctx, results)
		stats.usage = stats.usage.Add(turn.Usage)
		return turn, modelFailure(err)
	}
	turn, err := next(nil)
	if err != nil {
		return Draft{}, stats, err
	}
	draft, callID, err := r.readTurn(ctx, run, turn)
	var fix *checkFailure
	if !errors.As(err, &fix) {
		return draft, stats, err
	}
	turn, err = next([]llm.ToolResult{{CallID: callID, IsError: true, Content: fix.text + lastTurnNote}})
	if err != nil {
		return Draft{}, stats, err
	}
	draft, _, err = r.readTurn(ctx, run, turn)
	if errors.As(err, &fix) {
		return Draft{}, stats, &CannotTranslate{Reason: stillFails + fix.text + "."}
	}
	return draft, stats, err
}

func draftTools(run draftRun) []llm.ToolSpec {
	return []llm.ToolSpec{
		{Name: nameSubmitQuery, Description: descSubmitQuery, InputSchema: submitSchema(run.dsType, run.signal)},
		{Name: nameCannotTranslate, Description: descCannotTranslate, InputSchema: json.RawMessage(schemaCannotTranslate)},
	}
}

func (r *Registry) readTurn(ctx context.Context, run draftRun, turn llm.Turn) (Draft, string, error) {
	switch {
	case turn.Stop == llm.StopRefusal:
		return Draft{}, "", &CannotTranslate{Reason: reasonDeclined}
	case turn.Stop == llm.StopMaxTokens:
		return Draft{}, "", &CannotTranslate{Reason: reasonCutOff}
	case len(turn.ToolCalls) != 1:
		return Draft{}, "", &CannotTranslate{Reason: reasonNoQuery}
	}
	call := turn.ToolCalls[0]
	switch call.Name {
	case nameSubmitQuery:
		draft, err := r.checkSubmit(ctx, run, call.Input)
		return draft, call.ID, err
	case nameCannotTranslate:
		return Draft{}, "", cannotTranslateFrom(call.Input)
	}
	return Draft{}, "", &CannotTranslate{Reason: reasonNoQuery}
}

func cannotTranslateFrom(input json.RawMessage) error {
	var in struct {
		Reason string `json:"reason"`
	}
	if _, ok := decodeInput(input, &in); !ok {
		return &CannotTranslate{Reason: reasonNoQuery}
	}
	reason, ok := revisions.CleanPrompt(in.Reason)
	if !ok || utf8.RuneCountInString(reason) > maxReasonRunes {
		return &CannotTranslate{Reason: reasonNoQuery}
	}
	return &CannotTranslate{Reason: reason}
}

func modelFailure(err error) error {
	var callErr *llm.CallError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, llm.ErrNoFixture):
		return &CannotTranslate{Reason: reasonNoReplay}
	case errors.As(err, &callErr) && callErr.Failure == llm.RateLimited:
		return fmt.Errorf("%w: rate limited; try again shortly", ErrDraftProvider)
	case errors.Is(err, llm.ErrProvider):
		return ErrDraftProvider
	}
	return err
}

func (r *Registry) logDraft(ctx context.Context, run draftRun, stats draftStats, elapsed time.Duration, err error) {
	outcome := "drafted"
	if err != nil {
		code, _, ok := DraftFailure(err)
		switch {
		case ok:
			outcome = code
		case errors.Is(err, context.Canceled):
			outcome = "canceled"
		default:
			outcome = "internal"
		}
	}
	r.deps.Logger.InfoContext(ctx, "draft_query finished",
		slog.String("datasource_uid", run.inst.UID),
		slog.String("datasource_type", string(run.dsType)),
		slog.String("signal", run.signal),
		slog.Int("model_calls", stats.calls),
		slog.Int64("input_tokens", stats.usage.InputTokens),
		slog.Int64("output_tokens", stats.usage.OutputTokens),
		slog.Int("discovery_calls", stats.reads),
		slog.Int64("duration_ms", elapsed.Milliseconds()),
		slog.String("outcome", outcome))
}
