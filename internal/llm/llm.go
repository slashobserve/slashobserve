package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type ToolSpec struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

type ToolResult struct {
	CallID   string
	Content  string
	IsError  bool
	Prunable bool
}

type StopReason string

const (
	StopToolUse   StopReason = "tool_use"
	StopEndTurn   StopReason = "end_turn"
	StopMaxTokens StopReason = "max_tokens"
	StopRefusal   StopReason = "refusal"
)

type Usage struct {
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	Calls            int   `json:"model_calls"`
}

func (u Usage) Add(o Usage) Usage {
	return Usage{
		InputTokens:      u.InputTokens + o.InputTokens,
		OutputTokens:     u.OutputTokens + o.OutputTokens,
		CacheReadTokens:  u.CacheReadTokens + o.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens + o.CacheWriteTokens,
		Calls:            u.Calls + o.Calls,
	}
}

type Turn struct {
	Text      string
	ToolCalls []ToolCall
	Stop      StopReason
	Usage     Usage
}

type Request struct {
	System  string
	Tools   []ToolSpec
	Prompt  string
	Context string

	BuildSaves int
	Pruning    Pruning
}

type Pruning struct {
	KeepTurns int
	StepBytes int
}

type Compaction struct {
	Message     string
	TailTurns   int
	TailBytes   int
	ResultBytes int
}

func (r Request) UserMessage() string {
	if r.Context == "" {
		return r.Prompt
	}
	return r.Context + "\n\n<request>\n" + r.Prompt + "\n</request>"
}

type Provider interface {
	Name() string
	Model() string
	Start(ctx context.Context, req Request) (Session, error)
}

// The first Next call passes nil results; each later call passes exactly one
// result per tool call of the previous turn, in order, with matching CallIDs,
// and a session returns an error rather than panicking when that does not hold.
// Compact keeps the pending tool calls, so the next Next passes the results it would have passed.
// After a turn without tool calls it drops that answer and reopens the session for a Next with no results.
type Session interface {
	Next(ctx context.Context, results []ToolResult) (Turn, error)
	Compact(c Compaction) error
}

var (
	ErrProvider  = errors.New("llm provider error")
	ErrNoFixture = errors.New("no replay fixture")
)

type CallFailure string

const (
	CallTimeout   CallFailure = "call_timeout"
	RateLimited   CallFailure = "rate_limited"
	ProviderError CallFailure = "provider_error"
)

type CallError struct {
	Failure   CallFailure
	Provider  string
	Status    int
	Type      string
	Code      string
	Param     string
	RequestID string
	Idle      bool
	text      string

	RetryAfter time.Duration

	ProviderMessage string
}

func NewCallError(provider string, failure CallFailure, status int) *CallError {
	var detail string
	switch {
	case failure == CallTimeout:
		detail = fmt.Sprintf("%s request timed out", provider)
	case status > 0:
		detail = fmt.Sprintf("%s returned status %d", provider, status)
	default:
		detail = fmt.Sprintf("%s request failed", provider)
	}
	return &CallError{Failure: failure, Provider: provider, Status: status, text: ErrProvider.Error() + ": " + detail}
}

func IdleTimeoutError(provider string) error {
	err := NewCallError(provider, CallTimeout, 0)
	err.Idle = true
	return err
}

func (e *CallError) Error() string { return e.text }

func (e *CallError) Unwrap() error { return ErrProvider }
