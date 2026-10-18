package revisions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/auth"
)

const (
	MaxPromptRunes = 4000
	MaxActive      = 5
	MaxAttempts    = 2
	MaxErrorRunes  = 500

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

	ErrDashboardNotFound        = errors.New("dashboard not found")
	ErrDashboardVersionNotFound = errors.New("dashboard version not found")
)

type Request struct {
	ID          string
	DashboardID string
	Source      Source
	Prompt      string
	PanelID     string
	BaseVersion int
	Attempt     int
	RequestedBy auth.Principal
}

type Result struct {
	Version int
	Usage   *Usage
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
	BaseVersion   int
	Status        Status
	Attempts      int
	ResultVersion *int
	Error         string
	RequestedBy   auth.Principal
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
