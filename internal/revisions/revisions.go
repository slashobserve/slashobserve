package revisions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	MaxPromptRunes  = 4000
	MaxActive       = 5
	MaxAttempts     = 2
	LeaseTTL        = 60 * time.Second
	MaxErrorRunes   = 500
	MaxPanelIDRunes = 64

	MaxActiveObservePerPrincipal     = 3
	MaxActiveObserve                 = 20
	MaxActiveSuggestPerPrincipal     = 2
	MaxActiveInvestigatePerPrincipal = 3
	MaxActiveInvestigate             = 10
	MaxTitleRunes                    = 120
	DefaultTitleRunes                = 80
	MaxActiveSuggest                 = 10
	MaxDraftBytes                    = 64 << 10
	MaxSteps                         = 1000
	MaxStepMessageRunes              = 200
	BuildRetentionDays               = 7
	UnclaimedFor                     = 10 * time.Minute

	WakeChannel      = "slashobserve_agent_requests"
	ExhaustedMessage = "agent run did not finish"
	CanceledMessage  = "the run was canceled"
	UnclaimedMessage = "no agent took this run within 10 minutes"
)

type Source string

const (
	SourceComment     Source = "comment"
	SourcePrompt      Source = "prompt"
	SourceObserve     Source = "observe"
	SourceSuggest     Source = "suggest"
	SourceInvestigate Source = "investigate"
)

const (
	KindBuild       = "build"
	KindInvestigate = "investigate"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

var (
	ErrUnavailable      = errors.New("no agent is configured")
	ErrTooManyRevisions = errors.New("too many revisions are queued or running for this dashboard")
	ErrNotFound         = errors.New("revision not found")
	ErrLeaseLost        = errors.New("the agent request no longer belongs to this attempt")
	ErrInvalidCursor    = errors.New("invalid cursor")
	ErrTooManyRequests  = fmt.Errorf("you already have %d new dashboard requests queued or running", MaxActiveObservePerPrincipal)
	ErrAgentBusy        = errors.New("the agent is busy; try again in a few minutes")
	ErrRequestNotFound  = errors.New("agent request not found")
	ErrUnknownPanel     = errors.New("panel does not exist in the latest version of the dashboard")
	ErrCommentNotFound  = errors.New("comment not found")
	errNoThreads        = errors.New("comment threads are not configured")
	ErrRequestFinished  = errors.New("the request already finished")
	ErrNotContinuable   = errors.New("the request has no stopped build to continue")

	ErrTooManyInvestigations = tooManyInvestigationsError{}

	ErrTooManySuggestions = fmt.Errorf("you already have %d suggestions queued or running", MaxActiveSuggestPerPrincipal)
	ErrDraftTooLarge      = fmt.Errorf("the draft panel is over %d bytes as compact JSON", spec.MaxSuggestedPanelBytes)
	ErrDraftInvalid       = errors.New("the draft panel is not valid")
	ErrNotSuggestible     = notSuggestibleError{}
	ErrDraftBodyTooLarge  = draftBodyTooLargeError{}

	ErrDashboardNotFound        = errors.New("dashboard not found")
	ErrDashboardVersionNotFound = errors.New("dashboard version not found")
)

type PanelChecker interface {
	HasPanel(ctx context.Context, dashboardID pgtype.UUID, version int, panelID string) (bool, error)
}

type SpecReader interface {
	VersionSpec(ctx context.Context, id pgtype.UUID, version int) (*spec.Dashboard, error)
}

type DraftError struct {
	Issues []spec.Issue
}

func (e *DraftError) Error() string { return ErrDraftInvalid.Error() }

func (e *DraftError) Unwrap() error { return ErrDraftInvalid }

type notSuggestibleError struct{}

func (notSuggestibleError) Error() string {
	return "the panel is not a data, text or dashboard list panel of that version"
}

func (notSuggestibleError) Unwrap() error { return ErrUnknownPanel }

type tooManyInvestigationsError struct{}

func (tooManyInvestigationsError) Error() string {
	return fmt.Sprintf("you already have %d investigations queued or running", MaxActiveInvestigatePerPrincipal)
}

func (tooManyInvestigationsError) Unwrap() error { return ErrTooManyRequests }

type draftBodyTooLargeError struct{}

func (draftBodyTooLargeError) Error() string {
	return fmt.Sprintf("the draft panel is over %d bytes", MaxDraftBytes)
}

func (draftBodyTooLargeError) Unwrap() error { return ErrDraftTooLarge }

type Threads interface {
	ForRun(ctx context.Context, dashboardID, commentID pgtype.UUID) (threadID pgtype.UUID, panelID string, err error)
	ResolveForRun(ctx context.Context, q *dbsqlc.Queries, run dbsqlc.AgentRequest) error
}

type Principals interface {
	Principal(ctx context.Context, kind auth.PrincipalKind, id string) (auth.Principal, error)
}

type Option func(*Service)

func WithPrincipals(p Principals) Option {
	return func(s *Service) { s.principals = p }
}

func WithAccess(a *folders.Access) Option {
	return func(s *Service) { s.access = a }
}

func WithPanels(p PanelChecker) Option {
	return func(s *Service) { s.panels = p }
}

func WithSpecs(r SpecReader) Option {
	return func(s *Service) { s.specs = r }
}

func WithBuilds(b Builds) Option {
	return func(s *Service) { s.builds = b }
}

func WithThreads(t Threads) Option {
	return func(s *Service) { s.threads = t }
}

type Request struct {
	ID          string
	DashboardID string
	Source      Source
	Prompt      string
	PanelID     string
	BaseVersion int
	Draft       json.RawMessage
	Attempt     int
	RequestedBy auth.Principal
	Origin      origin.Origin

	Continues    bool
	ContinueNote string

	From, To *time.Time
}

type Result struct {
	Version     int
	Suggestion  json.RawMessage
	FindingID   string
	Usage       *Usage
	FailureCode FailureCode
}

type Usage struct {
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	ModelCalls       int    `json:"model_calls"`
	RepairRounds     int    `json:"repair_rounds"`

	ToolCalls           int   `json:"tool_calls"`
	FailedCalls         int   `json:"failed_calls"`
	Retries             int   `json:"retries"`
	ModelMS             int64 `json:"model_ms"`
	ToolMS              int64 `json:"tool_ms"`
	MaxSaveOutputTokens int64 `json:"max_save_output_tokens"`
	BuildSaves          int   `json:"build_saves"`
	Renewals            int   `json:"renewals"`
	Compactions         int   `json:"compactions"`
	Nudges              int   `json:"nudges"`
	SaveNowRefusals     int   `json:"save_now_refusals"`
	StalledTurns        int   `json:"stalled_turns"`
	OnePanelRefusals    int   `json:"one_panel_refusals"`

	CostUSD *float64 `json:"cost_usd"`
}

type Step struct {
	At         time.Time `json:"at"`
	Attempt    int       `json:"attempt"`
	Phase      string    `json:"phase"`
	Tool       string    `json:"tool"`
	Status     string    `json:"status"`
	Message    string    `json:"message"`
	DurationMS int64     `json:"duration_ms"`
}

type StepRecorder interface {
	RecordStep(ctx context.Context, req Request, s Step)
}

type Reviser interface {
	Revise(ctx context.Context, req Request) (Result, error)
}

type Revision struct {
	ID            string
	DashboardID   string
	Source        Source
	CommentID     string
	PanelID       string
	Prompt        string
	BaseVersion   *int
	Status        Status
	State         State
	ResultKind    ResultKind
	Attempts      int
	ResultVersion *int
	Error         string
	RequestedBy   auth.Principal
	Origin        string
	OriginClient  string
	CreatedAt     time.Time
	StartedAt     *time.Time
	FinishedAt    *time.Time

	FailureCode  string
	BuildSavedAt *time.Time
	ContinuesID  string
	ContinueNote string
	AssignedKind AssigneeKind
	AssignedID   string
}

func CleanPrompt(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if n := utf8.RuneCountInString(s); n < 1 || n > MaxPromptRunes || !utf8.ValidString(s) || strings.ContainsFunc(s, forbiddenRune) {
		return "", false
	}
	return s, true
}

func Kind(source Source) string {
	if source == SourceInvestigate {
		return KindInvestigate
	}
	return KindBuild
}

func DefaultTitle(prompt string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(prompt), "\n")
	if r := []rune(line); len(r) > DefaultTitleRunes {
		line = string(r[:DefaultTitleRunes])
	}
	return strings.TrimSpace(line)
}

func CleanTitle(s string) (string, bool) {
	title, ok := CleanPrompt(s)
	if !ok || strings.ContainsAny(title, "\n\r") || utf8.RuneCountInString(title) > MaxTitleRunes {
		return "", false
	}
	return title, true
}

func CleanContinueNote(note *string) (string, bool) {
	if note == nil {
		return "", true
	}
	return CleanPrompt(*note)
}

func ValidPanelID(panelID string) error {
	if n := utf8.RuneCountInString(panelID); n < 1 || n > MaxPanelIDRunes || !utf8.ValidString(panelID) || strings.ContainsRune(panelID, 0) {
		return fmt.Errorf("panel_id must be 1 to %d characters without NUL", MaxPanelIDRunes)
	}
	return nil
}

func forbiddenRune(r rune) bool {
	switch {
	case r == '\n', r == '\r', r == '\t':
		return false
	case unicode.IsControl(r):
		return true
	default:
		return r >= '\u202a' && r <= '\u202e' || r >= '\u2066' && r <= '\u2069'
	}
}

func truncateError(message string) string {
	message = strings.ToValidUTF8(strings.ReplaceAll(message, "\x00", ""), "")
	if r := []rune(message); len(r) > MaxErrorRunes {
		return string(r[:MaxErrorRunes])
	}
	return message
}
