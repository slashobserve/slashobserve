package llm

import (
	"context"
	"encoding/json"
	"errors"
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
	CallID  string
	Content string
	IsError bool
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
type Session interface {
	Next(ctx context.Context, results []ToolResult) (Turn, error)
}

var (
	ErrProvider  = errors.New("llm provider error")
	ErrNoFixture = errors.New("no replay fixture")
)
