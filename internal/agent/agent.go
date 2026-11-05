package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/imports"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxPromptRunes  = 4000
	maxRestarts     = 1
	maxLoggedIssues = 20
)

var (
	ErrNoSave          = errors.New("the model finished without saving a dashboard")
	ErrRepairExhausted = fmt.Errorf("the dashboard still failed validation after %d rejected saves in a row", maxRejectionsInRow)
	ErrRefused         = errors.New("the model declined the request")
	ErrConflict        = errors.New("the dashboard changed twice while the agent was revising it")
	ErrInvalidTask     = errors.New("invalid agent task")
	ErrNoSuggestion    = errors.New("the model finished without proposing a panel")
	ErrNoFinding       = errors.New("the model finished without saving a finding")
	ErrDraftInvalid    = errors.New("the draft panel is not valid")
	errModelCall       = errors.New("model call failed")

	ErrSuggestRepairExhausted error = &variantError{
		message: fmt.Sprintf("the proposed panel still failed validation after %d rejected saves in a row", maxRejectionsInRow),
		base:    ErrRepairExhausted,
	}
)

type variantError struct {
	message string
	base    error
}

func (e *variantError) Error() string { return e.message }
func (e *variantError) Unwrap() error { return e.base }

var panelIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

type ToolSet interface {
	List() []tools.Tool
	Call(ctx context.Context, name string, input json.RawMessage) (tools.Result, error)
}

type DashboardReader interface {
	Get(ctx context.Context, id pgtype.UUID) (dashboards.Dashboard, dashboards.Version, error)
	GetVersion(ctx context.Context, id pgtype.UUID, version int) (dashboards.Dashboard, dashboards.Version, error)
}

type DatasourceHealth interface {
	Health(ctx context.Context) []datasource.Status
}

type ImportReader interface {
	ForDashboard(ctx context.Context, id pgtype.UUID) (imports.Record, error)
}

type BuildStore interface {
	Build(ctx context.Context, requestID pgtype.UUID) (dashboards.Build, bool, error)
	CommitBuild(ctx context.Context, by auth.Principal, requestID pgtype.UUID, attempt int) (dashboards.Dashboard, dashboards.Version, bool, error)
	DiscardBuild(ctx context.Context, requestID pgtype.UUID, attempt int) error
	RebaseBuild(ctx context.Context, by auth.Principal, requestID pgtype.UUID, attempt int) (int, bool, error)
}

type Config struct {
	Provider          llm.Provider
	Tools             ToolSet
	Dashboards        DashboardReader
	Builds            BuildStore
	Datasources       DatasourceHealth
	Imports           ImportReader
	Logger            *slog.Logger
	SessionRenewBytes int
	LogToolInputs     bool
}

type Task struct {
	Prompt       string
	DashboardID  string
	PanelID      string
	Source       revisions.Source
	RequestID    string
	Attempt      int
	Continues    bool
	ContinueNote string
	Principal    auth.Principal
	Suggest      bool
	BaseVersion  int
	Draft        json.RawMessage
	Investigate  bool
	From, To     *time.Time
	Progress     func(Step)
}

type Step struct {
	At         time.Time `json:"at"`
	Phase      string    `json:"phase"`
	Tool       string    `json:"tool"`
	Status     string    `json:"status"`
	Message    string    `json:"message"`
	DurationMS int64     `json:"duration_ms"`
}

type Outcome struct {
	DashboardID         string
	Version             int
	Suggestion          json.RawMessage
	FindingID           string
	Replayed            bool
	RepairRounds        int
	Usage               llm.Usage
	Provider            string
	Model               string
	ToolCalls           int
	FailedCalls         int
	Retries             int
	ModelTime           time.Duration
	ToolTime            time.Duration
	MaxSaveOutputTokens int64
	BuildSaves          int
	Renewals            int
	Compactions         int
	Nudges              int
	SaveNowRefusals     int
	StalledTurns        int
	OnePanelRefusals    int
}

type Agent struct {
	provider          llm.Provider
	tools             ToolSet
	dashboards        DashboardReader
	builds            BuildStore
	datasources       DatasourceHealth
	imports           ImportReader
	logger            *slog.Logger
	investigateSystem string
	toolSpecs         map[tools.Mode][]llm.ToolSpec
	sessionRenewBytes int
	logToolInputs     bool
}

func New(c Config) *Agent {
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	specs := map[tools.Mode][]llm.ToolSpec{}
	for _, mode := range []tools.Mode{tools.ModeCreate, tools.ModeRevise, tools.ModeSuggest, tools.ModeInvestigate} {
		specs[mode] = make([]llm.ToolSpec, 0)
		for _, t := range c.Tools.List() {
			if !t.OfferedIn(mode) {
				continue
			}
			t = t.ForAgent()
			specs[mode] = append(specs[mode], llm.ToolSpec{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
		}
	}
	if c.SessionRenewBytes <= 0 {
		c.SessionRenewBytes = defaultSessionRenewBytes
	}
	return &Agent{
		provider:          c.Provider,
		tools:             c.Tools,
		dashboards:        c.Dashboards,
		builds:            c.Builds,
		datasources:       c.Datasources,
		imports:           c.Imports,
		logger:            c.Logger,
		investigateSystem: buildInvestigateSystem(),
		toolSpecs:         specs,
		sessionRenewBytes: c.SessionRenewBytes,
		logToolInputs:     c.LogToolInputs,
	}
}

func (a *Agent) Run(ctx context.Context, t Task) (Outcome, error) {
	start := time.Now()
	out := Outcome{Provider: a.provider.Name(), Model: a.provider.Model()}
	err := a.run(ctx, t, &out)
	a.logFinish(ctx, modeOf(t), t.RequestID, out, time.Since(start), err)
	return out, err
}

func modeOf(t Task) tools.Mode {
	if t.Investigate {
		return tools.ModeInvestigate
	}
	if t.Suggest {
		return tools.ModeSuggest
	}
	if t.DashboardID == "" {
		return tools.ModeCreate
	}
	return tools.ModeRevise
}

func incompleteSuggestion(t Task) bool {
	return t.Suggest && (t.DashboardID == "" || t.PanelID == "" || t.BaseVersion < 1)
}

func validateTask(t Task) (prompt string, id, requestID pgtype.UUID, err error) {
	prompt = strings.TrimSpace(t.Prompt)
	switch {
	case prompt == "":
		return "", id, requestID, fmt.Errorf("%w: the prompt is empty", ErrInvalidTask)
	case utf8.RuneCountInString(prompt) > maxPromptRunes:
		return "", id, requestID, fmt.Errorf("%w: the prompt is longer than %d characters", ErrInvalidTask, maxPromptRunes)
	case t.Principal.ID == "" || (t.Principal.Kind != auth.KindUser && t.Principal.Kind != auth.KindServiceAccount):
		return "", id, requestID, fmt.Errorf("%w: the principal is not valid", ErrInvalidTask)
	case t.DashboardID == "" && t.PanelID != "":
		return "", id, requestID, fmt.Errorf("%w: a panel needs a dashboard", ErrInvalidTask)
	case t.PanelID != "" && !panelIDPattern.MatchString(t.PanelID):
		return "", id, requestID, fmt.Errorf("%w: the panel id is not valid", ErrInvalidTask)
	case incompleteSuggestion(t):
		return "", id, requestID, fmt.Errorf("%w: a suggestion needs a dashboard, a panel and a base version", ErrInvalidTask)
	}
	if t.DashboardID != "" {
		if id, err = db.ParseUUID(t.DashboardID); err != nil {
			return "", id, requestID, fmt.Errorf("%w: the dashboard id is not a UUID", ErrInvalidTask)
		}
	}
	if !t.Suggest {
		if requestID, err = db.ParseUUID(t.RequestID); err != nil {
			return "", id, requestID, fmt.Errorf("%w: the request id is not a UUID", ErrInvalidTask)
		}
	}
	return prompt, id, requestID, nil
}

type runner struct {
	agent       *Agent
	task        Task
	prompt      string
	mode        tools.Mode
	statuses    []datasource.Status
	requestID   pgtype.UUID
	dashboardID pgtype.UUID
	state       *runState
	totals      *callTotals
	out         *Outcome
}

type sessionEnd struct {
	compact  seedReason
	conflict bool
	chat     llm.Session
	results  []llm.ToolResult
}

func compaction(reason seedReason, chat llm.Session, results []llm.ToolResult) sessionEnd {
	return sessionEnd{compact: reason, chat: chat, results: results}
}

func (a *Agent) run(ctx context.Context, t Task, out *Outcome) error {
	prompt, dashboardUUID, requestUUID, err := validateTask(t)
	if err != nil {
		return err
	}
	mode := modeOf(t)
	ctx = auth.WithPrincipal(ctx, t.Principal)
	statuses := a.datasources.Health(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	r := &runner{
		agent: a, task: t, prompt: prompt, mode: mode, statuses: statuses,
		requestID: requestUUID, dashboardID: dashboardUUID,
		state:  &runState{tools: a.tools, known: toolNames(a.tools.List()), progress: t.Progress, logger: a.logger, logInputs: a.logToolInputs, mode: mode},
		totals: &callTotals{},
		out:    out,
	}
	defer func() {
		out.RepairRounds += r.state.repairs
		out.ToolCalls += r.state.toolCalls
		out.ToolTime += r.state.toolTime
		out.FailedCalls += r.totals.failedCalls
		out.Retries += r.totals.retries
		out.ModelTime += r.totals.modelTime
		out.MaxSaveOutputTokens = max(out.MaxSaveOutputTokens, r.state.maxSaveOutput)
		out.BuildSaves += r.state.buildSaves
		out.Nudges += r.state.nudges
		out.SaveNowRefusals += r.state.saveNowRefusals
		out.StalledTurns += r.state.stalledTurns
		out.OnePanelRefusals += r.state.onePanelRefusals
	}()
	return r.loop(ctx)
}

func (r *runner) loop(ctx context.Context) error {
	reason := seedFresh
	restarts := 0
	var carry *sessionEnd
	for {
		current, build, restarted, err := r.loadOnLatest(ctx, &restarts)
		if err != nil {
			return err
		}
		if restarted {
			reason, carry = seedConflict, nil
		}
		if build != nil && reason == seedFresh {
			reason = resumeReason(r.task)
		}
		end, err := r.session(ctx, current, build, reason, carry)
		if err != nil {
			return err
		}
		carry = nil
		switch {
		case end.conflict:
			committed, err := r.rebaseAndCommit(ctx)
			if err != nil || committed {
				return err
			}
			if err := r.restartFromLatest(ctx, &restarts); err != nil {
				return err
			}
			reason = seedConflict
		case end.compact != seedFresh:
			carry, reason = &end, end.compact
		default:
			return nil
		}
	}
}

func (r *runner) loadOnLatest(ctx context.Context, restarts *int) (current *revision, build *dashboards.Build, restarted bool, err error) {
	for {
		if current, err = r.load(ctx); err != nil {
			return nil, nil, false, err
		}
		if build, err = r.loadBuild(ctx); err != nil {
			return nil, nil, false, err
		}
		if !r.buildIsStale(current, build) {
			return current, build, restarted, nil
		}
		rebased, err := r.rebase(ctx)
		if err != nil {
			return nil, nil, false, err
		}
		if !rebased {
			if err := r.restartFromLatest(ctx, restarts); err != nil {
				return nil, nil, false, err
			}
			restarted = true
		}
	}
}

func (r *runner) buildIsStale(current *revision, build *dashboards.Build) bool {
	return r.mode == tools.ModeRevise && build != nil && current != nil && build.BaseVersion != current.number
}

func (r *runner) rebase(ctx context.Context) (bool, error) {
	onto, rebased, err := r.agent.builds.RebaseBuild(ctx, r.task.Principal, r.requestID, r.task.Attempt)
	switch {
	case ctx.Err() != nil:
		return false, ctx.Err()
	case err != nil:
		return false, r.agent.buildStoreError(ctx, "could not rebase the build", err, r.task.RequestID)
	case rebased:
		r.state.note(fmt.Sprintf("Rebased onto version %d", onto))
	}
	return rebased, nil
}

func (r *runner) rebaseAndCommit(ctx context.Context) (bool, error) {
	rebased, err := r.rebase(ctx)
	if err != nil || !rebased {
		return false, err
	}
	end, err := r.commit(ctx)
	return err == nil && !end.conflict, err
}

func (r *runner) restartFromLatest(ctx context.Context, restarts *int) error {
	if err := r.discardBuild(ctx); err != nil {
		return err
	}
	if *restarts >= maxRestarts {
		r.state.stop("Stopped: the dashboard changed twice while the agent was revising it")
		return ErrConflict
	}
	*restarts++
	r.state.restart()
	return nil
}

func (r *runner) load(ctx context.Context) (*revision, error) {
	var current *revision
	var err error
	switch r.mode {
	case tools.ModeRevise:
		current, err = r.agent.loadLatest(ctx, r.dashboardID, r.task.RequestID)
	case tools.ModeSuggest:
		current, err = r.agent.loadBase(ctx, r.dashboardID, r.task)
	case tools.ModeInvestigate:
		if r.dashboardID.Valid {
			current, err = r.agent.loadLatest(ctx, r.dashboardID, r.task.RequestID)
		}
	}
	if err != nil {
		return nil, err
	}
	if current != nil {
		r.out.DashboardID = current.dashboardID
	}
	return current, nil
}

func (r *runner) loadBuild(ctx context.Context) (*dashboards.Build, error) {
	if r.mode == tools.ModeSuggest || r.mode == tools.ModeInvestigate {
		return nil, nil
	}
	build, found, err := r.agent.builds.Build(ctx, r.requestID)
	switch {
	case ctx.Err() != nil:
		return nil, ctx.Err()
	case err != nil:
		return nil, r.agent.buildStoreError(ctx, "could not load the build", err, r.task.RequestID)
	case !found:
		return nil, nil
	}
	return &build, nil
}

func (r *runner) discardBuild(ctx context.Context) error {
	err := r.agent.builds.DiscardBuild(ctx, r.requestID, r.task.Attempt)
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case err != nil:
		return r.agent.buildStoreError(ctx, "could not discard the build", err, r.task.RequestID)
	}
	return nil
}

func (a *Agent) buildStoreError(ctx context.Context, message string, err error, requestID string) error {
	if errors.Is(err, dashboards.ErrBuildLost) || errors.Is(err, dashboards.ErrNotFound) {
		return err
	}
	a.logger.ErrorContext(ctx, "agent could not use the build", append([]any{
		slog.String("error_code", string(codeInternal)),
		slog.String("agent_request_id", requestID),
	}, logattrs.Safe(err)...)...)
	return &safeError{message: message, code: codeInternal, err: err}
}

func (a *Agent) loadLatest(ctx context.Context, id pgtype.UUID, requestID string) (*revision, error) {
	dash, ver, err := a.dashboards.Get(ctx, id)
	rev, err := a.loaded(ctx, dash, ver, err, requestID, "latest version has no spec")
	if err != nil {
		return nil, err
	}
	rev.conversion = a.conversionNotes(ctx, id, rev.number, requestID)
	return rev, nil
}

func (a *Agent) loadBase(ctx context.Context, id pgtype.UUID, t Task) (*revision, error) {
	dash, ver, err := a.dashboards.GetVersion(ctx, id, t.BaseVersion)
	rev, err := a.loaded(ctx, dash, ver, err, t.RequestID, "base version has no spec")
	if err != nil {
		return nil, err
	}
	rev.conversion = a.conversionNotes(ctx, id, rev.number, t.RequestID)
	if len(t.Draft) == 0 {
		if _, ok := rev.spec.FindPanel(t.PanelID); !ok {
			return nil, fmt.Errorf("%w: the panel is not in that version of the dashboard", ErrInvalidTask)
		}
	} else if rev.spec, err = spec.WithPanel(rev.spec, t.PanelID, t.Draft); err != nil {
		return nil, a.draftInvalid(ctx, t.RequestID, err)
	}
	entry, ok := rev.spec.FindPanel(t.PanelID)
	switch {
	case !ok:
		return nil, a.draftInvalid(ctx, t.RequestID, ErrDraftInvalid)
	case !entry.Panel.Type.IsEditable():
		return nil, fmt.Errorf("%w: the panel is not a data, text or dashboard list panel", ErrInvalidTask)
	}
	return rev, nil
}

func (a *Agent) conversionNotes(ctx context.Context, id pgtype.UUID, number int, requestID string) []tools.Attention {
	if a.imports == nil {
		return nil
	}
	record, err := a.imports.ForDashboard(ctx, id)
	switch {
	case errors.Is(err, imports.ErrNotFound), errors.Is(err, dashboards.ErrNotFound):
		return nil
	case err != nil:
		if ctx.Err() == nil {
			a.logger.WarnContext(ctx, "agent could not read the import record", logattrs.Failure("import_read_failed", err, slog.String("agent_request_id", requestID))...)
		}
		return nil
	case record.Version != number:
		return nil
	}
	return tools.ConversionAttention(record.Report)
}

func (a *Agent) draftInvalid(ctx context.Context, requestID string, err error) error {
	attrs := logattrs.Failure("draft_invalid", err, slog.String("agent_request_id", requestID))
	var invalid *spec.ValidationError
	if errors.As(err, &invalid) {
		issues := invalid.Issues[:min(len(invalid.Issues), maxLoggedIssues)]
		codes := make([]string, len(issues))
		paths := make([]string, len(issues))
		for i, issue := range issues {
			codes[i], paths[i] = string(issue.Code), issue.Path
		}
		attrs = append(attrs, slog.Int("issue_count", len(invalid.Issues)), slog.Any("issue_codes", codes), slog.Any("issue_paths", paths))
	}
	a.logger.WarnContext(ctx, "agent draft panel is not valid", attrs...)
	return ErrDraftInvalid
}

func (a *Agent) loaded(ctx context.Context, dash dashboards.Dashboard, ver dashboards.Version, err error, requestID, noSpec string) (*revision, error) {
	switch {
	case errors.Is(err, dashboards.ErrNotFound):
		return nil, dashboards.ErrNotFound
	case err != nil:
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		a.logger.ErrorContext(ctx, "agent could not load the dashboard", append([]any{
			slog.String("error_code", string(codeLoadFailed)),
			slog.String("agent_request_id", requestID),
		}, logattrs.Safe(err)...)...)
		return nil, &safeError{message: "could not load the dashboard", code: codeLoadFailed, err: err}
	case ver.Spec == nil:
		return nil, &safeError{message: "could not load the dashboard", code: codeLoadFailed, err: errors.New(noSpec)}
	}
	return &revision{dashboardID: dash.ID, number: ver.Number, spec: ver.Spec, unverified: ver.Unverified}, nil
}

func (r *runner) prepareSession(ctx context.Context, current *revision, build *dashboards.Build, reason seedReason) (context.Context, llm.Request, error) {
	contextText, system, err := r.sessionText(current, build, reason)
	if err != nil {
		return ctx, llm.Request{}, contextError(err)
	}
	if found := r.state.digest.text(); found != "" && r.mode != tools.ModeSuggest {
		contextText += "\n\n" + found
	}
	run := runScope(r.task, r.prompt, r.mode, current)
	ctx = tools.WithRun(ctx, run)
	r.state.parentVersion = run.ParentVersion
	r.state.guards.resetReads()
	r.state.window = saveWindow{}
	r.state.sessionBytes = 0

	request := llm.Request{
		System: system, Tools: r.agent.toolSpecs[r.mode], Prompt: r.prompt, Context: contextText,
		Pruning: llm.Pruning{KeepTurns: pruneKeepTurns, StepBytes: pruneStepBytes},
	}
	if build != nil {
		request.BuildSaves = build.Saves
	}
	return ctx, request, nil
}

func (r *runner) sessionText(current *revision, build *dashboards.Build, reason seedReason) (contextText, system string, err error) {
	sd := seed{build: build, reason: reason, note: r.task.ContinueNote}
	if r.mode == tools.ModeInvestigate {
		return investigateContext(r.statuses, r.task, current, sd), r.agent.investigateSystem, nil
	}
	view := current
	if build != nil {
		view = buildView(current, build)
	}
	contextText, err = buildContext(r.statuses, r.mode, r.task.Source, r.task.PanelID, r.prompt, view, sd)
	return contextText, systemFor(r.statuses), err
}

func (r *runner) startSession(ctx context.Context, current *revision, build *dashboards.Build, reason seedReason) (context.Context, llm.Session, error) {
	ctx, request, err := r.prepareSession(ctx, current, build, reason)
	if err != nil {
		return ctx, nil, err
	}
	chat, err := r.agent.provider.Start(ctx, request)
	if err != nil {
		return ctx, nil, providerError(ctx, err)
	}
	return ctx, chat, nil
}

func (r *runner) compact(ctx context.Context, chat llm.Session, current *revision, build *dashboards.Build, reason seedReason) (context.Context, error) {
	ctx, request, err := r.prepareSession(ctx, current, build, reason)
	if err != nil {
		return ctx, err
	}
	err = chat.Compact(llm.Compaction{
		Message:     request.UserMessage(),
		TailTurns:   compactTailTurns,
		TailBytes:   compactTailBytes,
		ResultBytes: compactResultBytes,
	})
	if err != nil {
		return ctx, providerError(ctx, err)
	}
	r.out.Compactions++
	if reason == seedRenewed {
		r.out.Renewals++
	}
	return ctx, nil
}

func (r *runner) session(ctx context.Context, current *revision, build *dashboards.Build, reason seedReason, carry *sessionEnd) (sessionEnd, error) {
	var chat llm.Session
	var results []llm.ToolResult
	var err error
	if carry == nil {
		ctx, chat, err = r.startSession(ctx, current, build, reason)
	} else {
		chat, results = carry.chat, carry.results
		ctx, err = r.compact(ctx, chat, current, build, reason)
	}
	if err != nil {
		return sessionEnd{}, err
	}
	for {
		r.state.modelCalls++
		r.state.window.calls++
		turn, err := r.agent.modelCall(ctx, chat, results, r.totals)
		if err != nil {
			if r.cutOffByTimeout(ctx, err) {
				return compaction(seedCallTimeout, chat, results), nil
			}
			return sessionEnd{}, providerError(ctx, err)
		}
		r.out.Usage = r.out.Usage.Add(turn.Usage)
		r.state.noteSavingTurn(turn)
		if end, done, err := r.endOfTurn(ctx, turn, chat, results); done || err != nil {
			return end, err
		}
		var outcome turnOutcome
		readGuard := readRefusal(r.mode, r.state.window, containsSave(turn.ToolCalls))
		if results, outcome, err = r.state.executeTurn(ctx, turn.ToolCalls, readGuard); err != nil {
			return sessionEnd{}, err
		}
		if err := r.state.endStall(outcome); err != nil {
			return sessionEnd{}, err
		}
		r.state.nudge(results, outcome)
		r.state.sessionBytes += sizeOfTurn(turn, results)
		if end, done, err := r.settle(outcome); done || err != nil {
			return end, err
		}
		if outcome.built && r.state.sessionBytes > r.agent.sessionRenewBytes {
			return compaction(seedRenewed, chat, results), nil
		}
	}
}

func sizeOfTurn(turn llm.Turn, results []llm.ToolResult) int {
	n := len(turn.Text)
	for _, c := range turn.ToolCalls {
		n += len(c.Input)
	}
	for _, res := range results {
		n += len(res.Content)
	}
	return n
}

func (r *runner) cutOffByTimeout(ctx context.Context, err error) bool {
	var callErr *llm.CallError
	if ctx.Err() != nil || !errors.As(err, &callErr) || callErr.Failure != llm.CallTimeout {
		return false
	}
	return !r.cutOff("a model call ran past the time limit")
}

func (r *runner) cutOff(what string) (final bool) {
	r.state.cutOffs++
	switch {
	case r.mode == tools.ModeSuggest:
		r.state.stop("Stopped: " + what)
		return true
	case r.state.cutOffs >= maxCutOffsInRow:
		r.state.stop(fmt.Sprintf("Stopped: %s %d times in a row", what, maxCutOffsInRow))
		return true
	}
	return false
}

func (r *runner) endOfTurn(ctx context.Context, turn llm.Turn, chat llm.Session, results []llm.ToolResult) (sessionEnd, bool, error) {
	switch {
	case turn.Stop == llm.StopRefusal:
		return sessionEnd{}, true, ErrRefused
	case turn.Stop == llm.StopMaxTokens:
		if r.cutOff("the model's answer was cut off at the output limit") {
			return sessionEnd{}, true, &outputLimitError{base: noResultError(r.mode)}
		}
		return compaction(seedOutputLimit, chat, results), true, nil
	case len(turn.ToolCalls) == 0:
		end, err := r.finishWithoutCall(ctx, chat, noResultError(r.mode))
		return end, true, err
	}
	return sessionEnd{}, false, nil
}

func (r *runner) settle(outcome turnOutcome) (sessionEnd, bool, error) {
	switch {
	case outcome.finding != "":
		r.out.FindingID = outcome.finding
		return sessionEnd{}, true, nil
	case outcome.suggestion != nil:
		r.out.Suggestion = outcome.suggestion
		return sessionEnd{}, true, nil
	case outcome.saved != nil:
		r.out.DashboardID = outcome.saved.dashboardID
		r.out.Version = outcome.saved.version
		r.out.Replayed = outcome.saved.replayed
		return sessionEnd{}, true, nil
	case outcome.conflict:
		return sessionEnd{conflict: true}, true, nil
	case outcome.notFound:
		return sessionEnd{}, true, dashboards.ErrNotFound
	case outcome.rejected && r.state.rejections >= maxRejectionsInRow:
		r.state.stop(fmt.Sprintf("Stopped: %d %s in a row were rejected", maxRejectionsInRow, rejectedThing(r.mode)))
		return sessionEnd{}, true, repairExhausted(r.mode, outcome.issueCodes)
	}
	return sessionEnd{}, false, nil
}

func rejectedThing(mode tools.Mode) string {
	switch mode {
	case tools.ModeSuggest:
		return "proposals"
	case tools.ModeInvestigate:
		return "findings"
	}
	return "saves"
}

func (r *runner) finishWithoutCall(ctx context.Context, chat llm.Session, noResult error) (sessionEnd, error) {
	switch {
	case r.mode == tools.ModeSuggest:
		return sessionEnd{}, noResult
	case r.mode == tools.ModeInvestigate && !r.state.findingNudged:
		r.state.findingNudged = true
		return compaction(seedNoFinding, chat, nil), nil
	case r.mode == tools.ModeInvestigate:
		return sessionEnd{}, noResult
	}
	build, found, err := r.agent.builds.Build(ctx, r.requestID)
	switch {
	case ctx.Err() != nil:
		return sessionEnd{}, ctx.Err()
	case err != nil:
		return sessionEnd{}, r.agent.buildStoreError(ctx, "could not load the build", err, r.task.RequestID)
	case !found:
		return sessionEnd{}, noResult
	}
	if build.Progress().Pending() > 0 && !r.state.pendingNudged {
		r.state.pendingNudged = true
		return compaction(seedPending, chat, nil), nil
	}
	return r.commit(ctx)
}

func (r *runner) commit(ctx context.Context) (sessionEnd, error) {
	dash, ver, replayed, err := r.agent.builds.CommitBuild(ctx, r.task.Principal, r.requestID, r.task.Attempt)
	var conflict *dashboards.ConflictError
	switch {
	case errors.As(err, &conflict):
		return sessionEnd{conflict: true}, nil
	case ctx.Err() != nil:
		return sessionEnd{}, ctx.Err()
	case err != nil:
		return sessionEnd{}, r.agent.buildStoreError(ctx, "could not save the build", err, r.task.RequestID)
	}
	r.out.DashboardID = dash.ID
	r.out.Version = ver.Number
	r.out.Replayed = replayed
	return sessionEnd{}, nil
}

func buildView(current *revision, build *dashboards.Build) *revision {
	view := &revision{spec: build.Spec, unverified: build.Unverified}
	if current != nil {
		view.dashboardID, view.number, view.conversion = current.dashboardID, current.number, current.conversion
	}
	return view
}

func runScope(t Task, prompt string, mode tools.Mode, current *revision) tools.Run {
	run := tools.Run{Mode: mode, RequestID: t.RequestID}
	switch mode {
	case tools.ModeCreate:
		run.Build, run.Attempt, run.Prompt = true, t.Attempt, prompt
	case tools.ModeRevise:
		run.Build, run.Attempt = true, t.Attempt
	case tools.ModeInvestigate:
		run.Attempt, run.Prompt, run.PanelID = t.Attempt, prompt, t.PanelID
		if t.From != nil && t.To != nil {
			run.From, run.To = *t.From, *t.To
		}
	}
	if current == nil {
		return run
	}
	run.DashboardID = current.dashboardID
	run.ParentVersion = current.number
	if mode == tools.ModeSuggest {
		run.PanelID = t.PanelID
		run.Base = current.spec
	}
	return run
}

func noResultError(mode tools.Mode) error {
	switch mode {
	case tools.ModeSuggest:
		return ErrNoSuggestion
	case tools.ModeInvestigate:
		return ErrNoFinding
	}
	return ErrNoSave
}

func contextError(err error) error {
	return &safeError{message: "could not build the request context", code: codeContextFailed, err: err}
}

func providerError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return fmt.Errorf("%w: %w", errModelCall, err)
}

type outputLimitError struct{ base error }

func (e *outputLimitError) Error() string { return e.base.Error() + " after reaching max tokens" }
func (e *outputLimitError) Unwrap() error { return e.base }

func repairExhausted(mode tools.Mode, codes []string) error {
	exhausted := ErrRepairExhausted
	if mode == tools.ModeSuggest {
		exhausted = ErrSuggestRepairExhausted
	}
	if len(codes) == 0 {
		return exhausted
	}
	return fmt.Errorf("%w (last issues: %s)", exhausted, strings.Join(codes, ", "))
}

func (a *Agent) logFinish(ctx context.Context, mode tools.Mode, requestID string, out Outcome, took time.Duration, err error) {
	attrs := []any{
		slog.String("agent_request_id", requestID),
		slog.String("mode", string(mode)),
		slog.Int("version", out.Version),
		slog.Int("model_calls", out.Usage.Calls),
		slog.Int("repair_rounds", out.RepairRounds),
		slog.Int("build_saves", out.BuildSaves),
		slog.Int("renewals", out.Renewals),
		slog.Int("compactions", out.Compactions),
		slog.Int("nudges", out.Nudges),
		slog.Int("save_now_refusals", out.SaveNowRefusals),
		slog.Int("stalled_turns", out.StalledTurns),
		slog.Int("one_panel_refusals", out.OnePanelRefusals),
		slog.Int64("input_tokens", out.Usage.InputTokens),
		slog.Int64("output_tokens", out.Usage.OutputTokens),
		slog.Int64("cache_read_tokens", out.Usage.CacheReadTokens),
		slog.Int64("cache_write_tokens", out.Usage.CacheWriteTokens),
		slog.Duration("duration", took),
		slog.String("provider", out.Provider),
		slog.String("model", out.Model),
	}
	if out.DashboardID != "" {
		attrs = append(attrs, slog.String("dashboard_id", out.DashboardID))
	}
	if out.FindingID != "" {
		attrs = append(attrs, slog.String("finding_id", out.FindingID))
	}
	if err != nil {
		attrs = append(attrs, slog.String("error_code", string(failureCode(err))))
		var safe *safeError
		if errors.As(err, &safe) || errors.Is(err, errModelCall) {
			attrs = append(attrs, logattrs.Safe(err)...)
		}
	}
	a.logger.Log(ctx, finishLevel(err), "agent run finished", attrs...)
}

func finishLevel(err error) slog.Level {
	var safe *safeError
	switch {
	case err == nil, failureCode(err) == revisions.FailureCanceled:
		return slog.LevelInfo
	case errors.As(err, &safe), failureCode(err) == codeInternal:
		return slog.LevelError
	default:
		return slog.LevelWarn
	}
}

var sentinelFailures = []struct {
	sentinel error
	code     revisions.FailureCode
}{
	{llm.ErrNoFixture, revisions.FailureNoFixture},
	{errModelCall, revisions.FailureProviderError},
	{context.Canceled, revisions.FailureCanceled},
	{context.DeadlineExceeded, revisions.FailureRunTimeout},
	{ErrNoSave, revisions.FailureNoToolCall},
	{ErrNoSuggestion, revisions.FailureNoToolCall},
	{ErrNoFinding, revisions.FailureNoFinding},
	{ErrDraftInvalid, revisions.FailureDraftInvalid},
	{ErrRepairExhausted, revisions.FailureValidationExhausted},
	{ErrNoProgress, revisions.FailureNoProgress},
	{dashboards.ErrBuildLost, revisions.FailureLeaseExpired},
	{ErrRefused, revisions.FailureRefused},
	{ErrConflict, revisions.FailureConflict},
	{ErrInvalidTask, revisions.FailureInvalidTask},
	{dashboards.ErrNotFound, revisions.FailureNotFound},
}

func failureCode(err error) revisions.FailureCode {
	var safe *safeError
	var limit *outputLimitError
	var callErr *llm.CallError
	switch {
	case errors.As(err, &limit):
		return revisions.FailureOutputLimit
	case errors.As(err, &safe):
		return safe.code
	case errors.As(err, &callErr):
		return revisions.FailureCode(callErr.Failure)
	}
	for _, f := range sentinelFailures {
		if errors.Is(err, f.sentinel) {
			return f.code
		}
	}
	return codeInternal
}
