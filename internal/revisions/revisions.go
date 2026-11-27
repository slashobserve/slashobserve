package revisions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/anchor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/usertext"
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
	MaxActiveSuggest                 = 10
	MaxDraftBytes                    = 64 << 10
	MaxSteps                         = 1000
	MaxStepMessageRunes              = 200
	BuildRetentionDays               = 7
	UnclaimedFor                     = 10 * time.Minute
	ReplyRetryFor                    = 24 * time.Hour
	ReplyRetryBatch                  = 50

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
	KindAsk         = "ask"
	KindRevise      = "revise"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

var (
	ErrUnavailable       = errors.New("no agent is configured")
	ErrTooManyRevisions  = errors.New("too many revisions are queued or running for this dashboard")
	ErrNotFound          = errors.New("revision not found")
	ErrLeaseLost         = errors.New("the agent request no longer belongs to this attempt")
	ErrInvalidCursor     = errors.New("invalid cursor")
	ErrAnchoredNeedsMine = errors.New("anchored needs scope mine")
	ErrTooManyRequests   = fmt.Errorf("you already have %d new dashboard requests queued or running", MaxActiveObservePerPrincipal)
	ErrAgentBusy         = errors.New("the agent is busy; try again in a few minutes")
	ErrRequestNotFound   = errors.New("agent request not found")
	ErrUnknownPanel      = errors.New("panel does not exist in the latest version of the dashboard")
	ErrCommentNotFound   = errors.New("comment not found")
	errNoThreads         = errors.New("comment threads are not configured")
	ErrRequestFinished   = errors.New("the request already finished")
	ErrNotContinuable    = errors.New("the request has no stopped build to continue")

	ErrAssigneeUnavailable = errors.New("the agent this run was assigned to cannot take it again: its connection was revoked, expired or is not yours, or the service account no longer takes runs")

	ErrTooManyInvestigations = tooManyInvestigationsError{}

	ErrTooManySuggestions = fmt.Errorf("you already have %d suggestions queued or running", MaxActiveSuggestPerPrincipal)
	ErrDraftTooLarge      = fmt.Errorf("the draft panel is over %d bytes as compact JSON", spec.MaxSuggestedPanelBytes)
	ErrDraftInvalid       = errors.New("the draft panel is not valid")
	ErrNotSuggestible     = notSuggestibleError{}
	ErrDraftBodyTooLarge  = draftBodyTooLargeError{}

	ErrDashboardNotFound        = errors.New("dashboard not found")
	ErrObservationNotFound      = errors.New("observation not found")
	ErrChatNotFound             = errors.New("chat not found")
	ErrChatReadOnly             = errors.New("this chat is read-only for you: only its owner can ask in it or archive it")
	ErrDashboardVersionNotFound = errors.New("dashboard version not found")
)

const (
	CodeChatReadOnly    = "chat_read_only"
	CodeRequestFinished = "request_finished"
	CodeNotContinuable  = "not_continuable"
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
	RunEnded(ctx context.Context, q *dbsqlc.Queries, run dbsqlc.AgentRequest) error
}

type RunObservationOpen struct {
	Run           dbsqlc.AgentRequest
	By            auth.Principal
	ObservationID pgtype.UUID
	Strict        bool
}

type RunObservations interface {
	OpenForRun(ctx context.Context, q *dbsqlc.Queries, in RunObservationOpen) (pgtype.UUID, error)
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

func WithObservations(o RunObservations) Option {
	return func(s *Service) { s.observations = o }
}

func WithThreads(t Threads) Option {
	return func(s *Service) { s.threads = t }
}

func WithBuiltInAgent() Option {
	return func(s *Service) { s.builtInAgent = true }
}

type Metrics interface {
	AgentRun(kind, outcome string)
	RequestClaimed(source string, waited time.Duration)
}

type noMetrics struct{}

func (noMetrics) AgentRun(string, string)              {}
func (noMetrics) RequestClaimed(string, time.Duration) {}

func WithMetrics(m Metrics) Option {
	return func(s *Service) { s.metrics = m }
}

type Request struct {
	ID            string
	DashboardID   string
	Intent        Intent
	Source        Source
	Investigation bool

	ObservationID        string
	SubjectObservationID string
	ChatID               string
	Prompt               string
	PanelID              string
	BaseVersion          int
	Draft                json.RawMessage
	Attempt              int
	ClaimEpoch           int
	RequestedBy          auth.Principal
	Origin               origin.Origin
	Assignee             AssigneeKind

	Continues    bool
	ContinueNote string

	ThreadContext string
	ThreadAsk     bool

	Anchor  *anchor.Anchor
	BlockID string

	From, To *time.Time

	TimeZone string

	AsOf *time.Time
}

func (r Request) Investigates() bool {
	return r.Intent == IntentAsk && r.Source != SourceObserve
}

func (r Request) WriteOrigin(outside origin.Origin) origin.Origin {
	if r.Assignee == AssignBuiltin {
		return r.Origin
	}
	return outside
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

	Hypotheses  []Hypothesis `json:"hypotheses,omitempty"`
	CallLimit   int          `json:"call_limit,omitempty"`
	StageAtSave string       `json:"stage_at_save,omitempty"`
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

type Revision struct {
	ID            string
	DashboardID   string
	Intent        Intent
	Investigation bool
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

	audience runAudience
}

func CheckPrompt(field, s string) (string, error) {
	return usertext.Clean(field, s, usertext.Text(MaxPromptRunes))
}

func CleanPrompt(s string) (string, bool) {
	cleaned, err := CheckPrompt("prompt", s)
	return cleaned, err == nil
}

func Kind(source Source) string {
	if source == SourceInvestigate {
		return KindInvestigate
	}
	return KindBuild
}

func DefaultTitle(prompt string) string {
	if title := usertext.ShortTitle(prompt); title != "" {
		return title
	}
	line, _, _ := strings.Cut(strings.TrimSpace(prompt), "\n")
	return strings.TrimSpace(line)
}

func CheckTitle(s string) (string, error) {
	return usertext.Clean("title", s, usertext.Line(MaxTitleRunes))
}

func CheckQuery(s string) (string, error) {
	q := strings.TrimSpace(s)
	if n := utf8.RuneCountInString(q); n < 1 || n > maxQueryRunes || !utf8.ValidString(q) || strings.ContainsRune(q, 0) {
		return "", fmt.Errorf("q must be 1 to %d characters", maxQueryRunes)
	}
	return q, nil
}

func CheckContinueNote(note *string) (string, error) {
	if note == nil {
		return "", nil
	}
	return CheckPrompt("note", *note)
}

func ValidPanelID(panelID string) error {
	if n := utf8.RuneCountInString(panelID); n < 1 || n > MaxPanelIDRunes || !utf8.ValidString(panelID) || strings.ContainsRune(panelID, 0) {
		return fmt.Errorf("panel_id must be 1 to %d characters without NUL", MaxPanelIDRunes)
	}
	return nil
}

func truncateError(message string) string {
	message = strings.ToValidUTF8(strings.ReplaceAll(message, "\x00", ""), "")
	if r := []rune(message); len(r) > MaxErrorRunes {
		return string(r[:MaxErrorRunes])
	}
	return message
}
