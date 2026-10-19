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
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxPromptRunes  = 4000
	maxRepairRounds = 3
	maxModelCalls   = 20
	maxRestarts     = 1
)

var (
	ErrNoSave          = errors.New("the model finished without saving a dashboard")
	ErrRepairExhausted = fmt.Errorf("the dashboard still failed validation after %d repair rounds", maxRepairRounds)
	ErrTurnLimit       = errors.New("the agent reached its turn limit without saving")
	ErrRefused         = errors.New("the model declined the request")
	ErrConflict        = errors.New("the dashboard changed twice while the agent was revising it")
	ErrInvalidTask     = errors.New("invalid agent task")
	ErrTooLarge        = errors.New("the dashboard is too large for the agent to revise")
	ErrNoSuggestion    = errors.New("the model finished without proposing a panel")
	ErrDraftInvalid    = errors.New("the draft panel is not valid")
	errModelCall       = errors.New("model call failed")
)

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

type Config struct {
	Provider    llm.Provider
	Tools       ToolSet
	Dashboards  DashboardReader
	Datasources DatasourceHealth
	Logger      *slog.Logger
}

type Task struct {
	Prompt      string
	DashboardID string
	PanelID     string
	Source      revisions.Source
	RequestID   string
	Principal   auth.Principal
	Suggest     bool
	BaseVersion int
	Draft       json.RawMessage
	Progress    func(Step)
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
	DashboardID  string
	Version      int
	Suggestion   json.RawMessage
	Replayed     bool
	RepairRounds int
	Usage        llm.Usage
	Provider     string
	Model        string
}

type Agent struct {
	provider    llm.Provider
	tools       ToolSet
	dashboards  DashboardReader
	datasources DatasourceHealth
	logger      *slog.Logger
	system      string
	toolSpecs   []llm.ToolSpec
}

func New(c Config) *Agent {
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	specs := make([]llm.ToolSpec, 0)
	for _, t := range c.Tools.List() {
		if t.MCPOnly {
			continue
		}
		specs = append(specs, llm.ToolSpec{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	return &Agent{
		provider:    c.Provider,
		tools:       c.Tools,
		dashboards:  c.Dashboards,
		datasources: c.Datasources,
		logger:      c.Logger,
		system:      buildSystem(),
		toolSpecs:   specs,
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

func validateTask(t Task) (prompt string, id pgtype.UUID, err error) {
	prompt = strings.TrimSpace(t.Prompt)
	switch {
	case prompt == "":
		return "", id, fmt.Errorf("%w: the prompt is empty", ErrInvalidTask)
	case utf8.RuneCountInString(prompt) > maxPromptRunes:
		return "", id, fmt.Errorf("%w: the prompt is longer than %d characters", ErrInvalidTask, maxPromptRunes)
	case t.Principal.ID == "" || (t.Principal.Kind != auth.KindUser && t.Principal.Kind != auth.KindServiceAccount):
		return "", id, fmt.Errorf("%w: the principal is not valid", ErrInvalidTask)
	case t.DashboardID == "" && t.PanelID != "":
		return "", id, fmt.Errorf("%w: a panel needs a dashboard", ErrInvalidTask)
	case t.PanelID != "" && !panelIDPattern.MatchString(t.PanelID):
		return "", id, fmt.Errorf("%w: the panel id is not valid", ErrInvalidTask)
	case incompleteSuggestion(t):
		return "", id, fmt.Errorf("%w: a suggestion needs a dashboard, a panel and a base version", ErrInvalidTask)
	}
	if t.DashboardID != "" {
		if id, err = db.ParseUUID(t.DashboardID); err != nil {
			return "", id, fmt.Errorf("%w: the dashboard id is not a UUID", ErrInvalidTask)
		}
	}
	return prompt, id, nil
}

func (a *Agent) run(ctx context.Context, t Task, out *Outcome) error {
	prompt, dashboardUUID, err := validateTask(t)
	if err != nil {
		return err
	}
	mode := modeOf(t)
	ctx = auth.WithPrincipal(ctx, t.Principal)
	statuses := a.datasources.Health(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	for restarts := 0; ; restarts++ {
		var current *revision
		switch mode {
		case tools.ModeRevise:
			if current, err = a.loadLatest(ctx, dashboardUUID, t.RequestID); err != nil {
				return err
			}
			out.DashboardID = current.dashboardID
		case tools.ModeSuggest:
			if current, err = a.loadBase(ctx, dashboardUUID, t); err != nil {
				return err
			}
			out.DashboardID = current.dashboardID
		}
		conflict, err := a.session(ctx, t, prompt, mode, statuses, current, out)
		if err != nil {
			return err
		}
		if !conflict {
			return nil
		}
		if restarts >= maxRestarts {
			return ErrConflict
		}
	}
}

func (a *Agent) loadLatest(ctx context.Context, id pgtype.UUID, requestID string) (*revision, error) {
	dash, ver, err := a.dashboards.Get(ctx, id)
	return a.loaded(ctx, dash, ver, err, requestID, "latest version has no spec")
}

func (a *Agent) loadBase(ctx context.Context, id pgtype.UUID, t Task) (*revision, error) {
	dash, ver, err := a.dashboards.GetVersion(ctx, id, t.BaseVersion)
	rev, err := a.loaded(ctx, dash, ver, err, t.RequestID, "base version has no spec")
	if err != nil {
		return nil, err
	}
	if len(t.Draft) > 0 {
		if rev.spec, err = spec.WithPanel(rev.spec, t.PanelID, t.Draft); err != nil {
			return nil, ErrDraftInvalid
		}
	}
	if _, ok := rev.spec.FindPanel(t.PanelID); !ok {
		return nil, ErrDraftInvalid
	}
	return rev, nil
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
			slog.String("error_code", codeLoadFailed),
			slog.String("agent_request_id", requestID),
		}, logattrs.Safe(err)...)...)
		return nil, &safeError{message: "could not load the dashboard", code: codeLoadFailed, err: err}
	case ver.Spec == nil:
		return nil, &safeError{message: "could not load the dashboard", code: codeLoadFailed, err: errors.New(noSpec)}
	}
	return &revision{dashboardID: dash.ID, number: ver.Number, spec: ver.Spec}, nil
}

func (a *Agent) session(ctx context.Context, t Task, prompt string, mode tools.Mode, statuses []datasource.Status, current *revision, out *Outcome) (bool, error) {
	contextText, err := buildContext(statuses, mode, t.Source, t.PanelID, current)
	if err != nil {
		return false, contextError(err)
	}
	run := runScope(t, mode, current)
	ctx = tools.WithRun(ctx, run)
	ctx = llm.WithAgentRequestID(ctx, t.RequestID)
	state := &runState{tools: a.tools, progress: t.Progress, logger: a.logger, mode: mode, parentVersion: run.ParentVersion}
	defer func() { out.RepairRounds += state.repairs }()

	chat, err := a.provider.Start(ctx, llm.Request{System: a.system, Tools: a.toolSpecs, Prompt: prompt, Context: contextText})
	if err != nil {
		return false, providerError(ctx, err)
	}
	noResult := noResultError(mode)
	var results []llm.ToolResult
	for modelCall := 1; modelCall <= maxModelCalls; modelCall++ {
		turn, err := chat.Next(ctx, results)
		if err != nil {
			return false, providerError(ctx, err)
		}
		out.Usage = out.Usage.Add(turn.Usage)
		if turn.Stop == llm.StopRefusal {
			return false, ErrRefused
		}
		if turn.Stop == llm.StopMaxTokens {
			return false, fmt.Errorf("%w after reaching max tokens", noResult)
		}
		if len(turn.ToolCalls) == 0 {
			return false, noResult
		}
		var outcome turnOutcome
		if results, outcome, err = state.executeTurn(ctx, turn.ToolCalls, modelCall == maxModelCalls); err != nil {
			return false, err
		}
		switch {
		case outcome.suggestion != nil:
			out.Suggestion = outcome.suggestion
			return false, nil
		case outcome.saved != nil:
			out.DashboardID = outcome.saved.dashboardID
			out.Version = outcome.saved.version
			out.Replayed = outcome.saved.replayed
			return false, nil
		case outcome.conflict:
			return true, nil
		case outcome.notFound:
			return false, dashboards.ErrNotFound
		case outcome.rejected && state.rejections > maxRepairRounds:
			return false, repairExhausted(outcome.issueCodes)
		}
	}
	return false, ErrTurnLimit
}

func runScope(t Task, mode tools.Mode, current *revision) tools.Run {
	run := tools.Run{Mode: mode, RequestID: t.RequestID}
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
	if mode == tools.ModeSuggest {
		return ErrNoSuggestion
	}
	return ErrNoSave
}

func contextError(err error) error {
	if errors.Is(err, ErrTooLarge) {
		return err
	}
	return &safeError{message: "could not build the request context", code: codeContextFailed, err: err}
}

func providerError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return fmt.Errorf("%w: %w", errModelCall, err)
}

func repairExhausted(codes []string) error {
	if len(codes) == 0 {
		return ErrRepairExhausted
	}
	return fmt.Errorf("%w (last issues: %s)", ErrRepairExhausted, strings.Join(codes, ", "))
}

func (a *Agent) logFinish(ctx context.Context, mode tools.Mode, requestID string, out Outcome, took time.Duration, err error) {
	attrs := []any{
		slog.String("agent_request_id", requestID),
		slog.String("mode", string(mode)),
		slog.Int("version", out.Version),
		slog.Int("model_calls", out.Usage.Calls),
		slog.Int("repair_rounds", out.RepairRounds),
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
	if err != nil {
		attrs = append(attrs, slog.String("error_code", errorKind(err)))
		var safe *safeError
		if errors.As(err, &safe) || errors.Is(err, errModelCall) {
			attrs = append(attrs, logattrs.Safe(err)...)
		}
	}
	a.logger.InfoContext(ctx, "agent run finished", attrs...)
}

func errorKind(err error) string {
	var safe *safeError
	switch {
	case errors.As(err, &safe):
		return safe.code
	case errors.Is(err, errModelCall):
		return "model_failed"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, ErrNoSave):
		return "no_save"
	case errors.Is(err, ErrNoSuggestion):
		return "no_suggestion"
	case errors.Is(err, ErrDraftInvalid):
		return "draft_invalid"
	case errors.Is(err, ErrRepairExhausted):
		return "repair_exhausted"
	case errors.Is(err, ErrTurnLimit):
		return "turn_limit"
	case errors.Is(err, ErrRefused):
		return "refused"
	case errors.Is(err, ErrConflict):
		return "conflict"
	case errors.Is(err, ErrInvalidTask):
		return "invalid_task"
	case errors.Is(err, ErrTooLarge):
		return "too_large"
	case errors.Is(err, dashboards.ErrNotFound):
		return "not_found"
	}
	return codeInternal
}
