package anthropic

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/slashobserve/slashobserve/internal/llm"
)

const (
	name       = "anthropic"
	maxRetries = 2
)

type Config struct {
	APIKey         string
	Model          string
	Effort         string
	BaseURL        *url.URL
	MaxTokens      int
	RequestTimeout time.Duration
}

type Provider struct {
	cfg      Config
	messages sdk.MessageService
	logger   *slog.Logger
}

func New(cfg Config, logger *slog.Logger) *Provider {
	opts := []option.RequestOption{
		option.WithoutEnvironmentDefaults(),
		option.WithAPIKey(cfg.APIKey),
		option.WithRequestTimeout(cfg.RequestTimeout),
		option.WithMaxRetries(maxRetries),
	}
	if cfg.BaseURL != nil {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL.String()))
	}
	client := sdk.NewClient(opts...)
	return &Provider{cfg: cfg, messages: client.Messages, logger: logger}
}

func (p *Provider) Name() string  { return name }
func (p *Provider) Model() string { return p.cfg.Model }

func (p *Provider) Start(ctx context.Context, req llm.Request) (llm.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var tools []sdk.ToolUnionParam
	for _, spec := range req.Tools {
		tool, err := toolParam(spec)
		if err != nil {
			return nil, err
		}
		tools = append(tools, tool)
	}
	var system []sdk.TextBlockParam
	if req.System != "" {
		system = []sdk.TextBlockParam{{
			Text:         req.System,
			CacheControl: sdk.NewCacheControlEphemeralParam(),
		}}
	}
	return &session{
		provider: p,
		system:   system,
		tools:    tools,
		history:  []sdk.MessageParam{sdk.NewUserMessage(sdk.NewTextBlock(req.UserMessage()))},
	}, nil
}

func toolParam(spec llm.ToolSpec) (sdk.ToolUnionParam, error) {
	schema, err := llm.ParseToolSchema(spec)
	if err != nil {
		return sdk.ToolUnionParam{}, err
	}
	tool := sdk.ToolParam{
		Name:        spec.Name,
		InputSchema: param.Override[sdk.ToolInputSchemaParam](schema.Normalized),
	}
	if spec.Description != "" {
		tool.Description = sdk.String(spec.Description)
	}
	return sdk.ToolUnionParam{OfTool: &tool}, nil
}

type session struct {
	provider *Provider
	system   []sdk.TextBlockParam
	tools    []sdk.ToolUnionParam
	history  []sdk.MessageParam
	calls    int
	pending  []string
}

func (s *session) Next(ctx context.Context, results []llm.ToolResult) (llm.Turn, error) {
	if err := ctx.Err(); err != nil {
		return llm.Turn{}, err
	}
	if err := llm.CheckResults(name, s.calls > 0, s.pending, results); err != nil {
		return llm.Turn{}, err
	}

	messages := s.history[:len(s.history):len(s.history)]
	if len(results) > 0 {
		blocks := make([]sdk.ContentBlockParamUnion, 0, len(results))
		for _, r := range results {
			blocks = append(blocks, sdk.NewToolResultBlock(r.CallID, r.Content, r.IsError))
		}
		messages = append(messages, sdk.NewUserMessage(blocks...))
	}

	callCtx, cancel := context.WithTimeout(ctx, s.provider.cfg.RequestTimeout)
	defer cancel()
	resp, err := s.provider.messages.New(callCtx, sdk.MessageNewParams{
		Model:     s.provider.cfg.Model,
		MaxTokens: int64(s.provider.cfg.MaxTokens),
		System:    s.system,
		Tools:     s.tools,
		Messages:  withCacheBreakpoint(messages),
		OutputConfig: sdk.OutputConfigParam{
			Effort: sdk.OutputConfigEffort(s.provider.cfg.Effort),
		},
	})
	if err != nil {
		return llm.Turn{}, s.provider.sanitize(ctx, callCtx, err)
	}

	turn, err := parseResponse(resp)
	if err != nil {
		return llm.Turn{}, err
	}

	messages = append(messages, resp.ToParam())
	s.history = messages
	s.calls++
	s.pending = llm.PendingIDs(turn.ToolCalls)
	return turn, nil
}

func withCacheBreakpoint(messages []sdk.MessageParam) []sdk.MessageParam {
	last := len(messages) - 1
	msg := messages[last]
	blocks := slices.Clone(msg.Content)
	end := len(blocks) - 1
	switch {
	case blocks[end].OfText != nil:
		text := *blocks[end].OfText
		text.CacheControl = sdk.NewCacheControlEphemeralParam()
		blocks[end].OfText = &text
	case blocks[end].OfToolResult != nil:
		result := *blocks[end].OfToolResult
		result.CacheControl = sdk.NewCacheControlEphemeralParam()
		blocks[end].OfToolResult = &result
	default:
		return messages
	}
	msg.Content = blocks
	out := slices.Clone(messages)
	out[last] = msg
	return out
}

func parseResponse(resp *sdk.Message) (llm.Turn, error) {
	stop, err := stopReason(resp.StopReason)
	if err != nil {
		return llm.Turn{}, err
	}
	turn := llm.Turn{
		Stop: stop,
		Usage: llm.Usage{
			InputTokens:      resp.Usage.InputTokens,
			OutputTokens:     resp.Usage.OutputTokens,
			CacheReadTokens:  resp.Usage.CacheReadInputTokens,
			CacheWriteTokens: resp.Usage.CacheCreationInputTokens,
			Calls:            1,
		},
	}
	var text strings.Builder
	for _, block := range resp.Content {
		if block.AsAny() == nil {
			return llm.Turn{}, fmt.Errorf("%w: anthropic returned unsupported content block type %q", llm.ErrProvider, block.Type)
		}
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
		case "tool_use":
			turn.ToolCalls = append(turn.ToolCalls, llm.ToolCall{ID: block.ID, Name: block.Name, Input: block.Input})
		}
	}
	turn.Text = text.String()
	return turn, nil
}

func stopReason(reason sdk.StopReason) (llm.StopReason, error) {
	switch reason {
	case sdk.StopReasonToolUse:
		return llm.StopToolUse, nil
	case sdk.StopReasonEndTurn, sdk.StopReasonStopSequence:
		return llm.StopEndTurn, nil
	case sdk.StopReasonMaxTokens, sdk.StopReasonModelContextWindowExceeded:
		return llm.StopMaxTokens, nil
	case sdk.StopReasonRefusal:
		return llm.StopRefusal, nil
	default:
		return "", fmt.Errorf("%w: anthropic returned unexpected stop reason %q", llm.ErrProvider, reason)
	}
}

func (p *Provider) sanitize(ctx, callCtx context.Context, err error) error {
	var api *llm.APIError
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		api = &llm.APIError{Status: apiErr.StatusCode, Type: string(apiErr.Type()), RequestID: apiErr.RequestID}
	}
	return llm.SanitizeError(ctx, callCtx, p.logger, name, err, api)
}
