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
	"github.com/slashobserve/slashobserve/internal/origin"
)

const (
	MaxPromptRunes  = 4000
	MaxActive       = 5
	MaxAttempts     = 2
	MaxErrorRunes   = 500
	MaxPanelIDRunes = 64

	MaxActiveObservePerPrincipal = 3
	MaxActiveObserve             = 20
	MaxSteps                     = 100
	MaxStepMessageRunes          = 200

	WakeChannel      = "slashobserve_agent_requests"
	ExhaustedMessage = "agent run did not finish"
)

type Source string

const (
	SourceComment Source = "comment"
	SourcePrompt  Source = "prompt"
	SourceObserve Source = "observe"
	SourceSuggest Source = "suggest"
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
	ErrInvalidCursor    = errors.New("invalid cursor")
	ErrTooManyRequests  = fmt.Errorf("you already have %d new dashboard requests queued or running", MaxActiveObservePerPrincipal)
	ErrAgentBusy        = errors.New("the agent is busy; try again in a few minutes")
	ErrRequestNotFound  = errors.New("agent request not found")
	ErrUnknownPanel     = errors.New("panel does not exist in the latest version of the dashboard")
	ErrCommentNotFound  = errors.New("comment not found")

	ErrDashboardNotFound        = errors.New("dashboard not found")
	ErrDashboardVersionNotFound = errors.New("dashboard version not found")
)

type PanelChecker interface {
	HasPanel(ctx context.Context, dashboardID pgtype.UUID, version int, panelID string) (bool, error)
}

type Option func(*Service)

func WithPanels(p PanelChecker) Option {
	return func(s *Service) { s.panels = p }
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
}

type Result struct {
	Version    int
	Suggestion json.RawMessage
	Usage      *Usage
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
	Attempts      int
	ResultVersion *int
	Error         string
	RequestedBy   auth.Principal
	Origin        string
	OriginClient  string
	CreatedAt     time.Time
	StartedAt     *time.Time
	FinishedAt    *time.Time
}

func CleanPrompt(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if n := utf8.RuneCountInString(s); n < 1 || n > MaxPromptRunes || !utf8.ValidString(s) || strings.ContainsFunc(s, forbiddenRune) {
		return "", false
	}
	return s, true
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
