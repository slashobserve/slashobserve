package revisions

import (
	"context"
	"errors"
	"math"
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

	WakeChannel      = "slashobserve_agent_requests"
	ExhaustedMessage = "agent run did not finish"
)

type Source string

const (
	SourceComment Source = "comment"
	SourcePrompt  Source = "prompt"
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
)

type Request struct {
	ID          string
	DashboardID string
	Source      Source
	Prompt      string
	PanelID     string
	BaseVersion int
	RequestedBy auth.Principal
}

type Result struct{ Version int }

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

func toInt32(n int) int32 {
	if n < math.MinInt32 || n > math.MaxInt32 {
		panic("revisions: integer outside the int32 range")
	}
	return int32(n)
}

func truncateError(message string) string {
	message = strings.ToValidUTF8(strings.ReplaceAll(message, "\x00", ""), "")
	if r := []rune(message); len(r) > MaxErrorRunes {
		return string(r[:MaxErrorRunes])
	}
	return message
}
