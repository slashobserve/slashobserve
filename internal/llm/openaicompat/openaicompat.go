package openaicompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"

	"github.com/slashobserve/slashobserve/internal/llm"
)

const (
	name             = "openai"
	maxRetries       = 2
	errorResultLabel = "ERROR: "
)

type Config struct {
	APIKey         string
	Model          string
	BaseURL        *url.URL
	MaxTokens      int
	RequestTimeout time.Duration
}

type Provider struct {
	cfg    Config
	chat   openai.ChatCompletionService
	logger *slog.Logger
}

func New(cfg Config, logger *slog.Logger) *Provider {
	opts := []option.RequestOption{
		option.WithEnvironmentProduction(),
		option.WithRequestTimeout(cfg.RequestTimeout),
		option.WithMaxRetries(maxRetries),
	}
	if cfg.BaseURL != nil {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL.String()))
	}
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	return &Provider{cfg: cfg, chat: openai.NewChatCompletionService(opts...), logger: logger}
}

func (p *Provider) Name() string  { return name }
func (p *Provider) Model() string { return p.cfg.Model }

func (p *Provider) Start(ctx context.Context, req llm.Request) (llm.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tools := make([]openai.ChatCompletionToolUnionParam, 0, len(req.Tools))
	for _, spec := range req.Tools {
		tool, err := toolParam(spec)
		if err != nil {
			return nil, err
		}
		tools = append(tools, tool)
	}
	return &session{
		provider: p,
		tools:    tools,
		history: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(req.System),
			openai.UserMessage(req.UserMessage()),
		},
	}, nil
}

func toolParam(spec llm.ToolSpec) (openai.ChatCompletionToolUnionParam, error) {
	schema, err := llm.ParseToolSchema(spec)
	if err != nil {
		return openai.ChatCompletionToolUnionParam{}, err
	}
	def := shared.FunctionDefinitionParam{
		Name:       spec.Name,
		Parameters: shared.FunctionParameters(schema.Full),
	}
	if spec.Description != "" {
		def.Description = openai.String(spec.Description)
	}
	return openai.ChatCompletionFunctionTool(def), nil
}

type session struct {
	provider *Provider
	tools    []openai.ChatCompletionToolUnionParam
	history  []openai.ChatCompletionMessageParamUnion
	calls    int
	pending  []string
}

func (s *session) Next(ctx context.Context, results []llm.ToolResult) (llm.Turn, error) {
	if err := ctx.Err(); err != nil {
		return llm.Turn{}, err
	}
	if err := s.checkResults(results); err != nil {
		return llm.Turn{}, err
	}

	messages := s.history[:len(s.history):len(s.history)]
	for _, r := range results {
		content := r.Content
		if r.IsError {
			content = errorResultLabel + content
		}
		messages = append(messages, openai.ToolMessage(content, r.CallID))
	}

	resp, err := s.provider.chat.New(ctx, openai.ChatCompletionNewParams{
		Model:               s.provider.cfg.Model,
		MaxCompletionTokens: openai.Int(int64(s.provider.cfg.MaxTokens)),
		Messages:            messages,
		Tools:               s.tools,
	})
	if err != nil {
		return llm.Turn{}, s.provider.sanitize(ctx, err)
	}
	if len(resp.Choices) == 0 {
		return llm.Turn{}, fmt.Errorf("%w: openai returned no choices", llm.ErrProvider)
	}
	choice := resp.Choices[0]

	turn, err := parseChoice(choice)
	if err != nil {
		return llm.Turn{}, err
	}
	turn.Usage = usage(resp.Usage)

	messages = append(messages, choice.Message.ToParam())
	s.history = messages
	s.calls++
	s.pending = nil
	for _, c := range turn.ToolCalls {
		s.pending = append(s.pending, c.ID)
	}
	return turn, nil
}

func (s *session) checkResults(results []llm.ToolResult) error {
	if s.calls > 0 && len(s.pending) == 0 {
		return errors.New("openai session: the previous turn made no tool calls, so the session has ended")
	}
	if len(results) != len(s.pending) {
		return fmt.Errorf("openai session: got %d tool results for %d tool calls", len(results), len(s.pending))
	}
	for i, r := range results {
		if r.CallID != s.pending[i] {
			return fmt.Errorf("openai session: tool result %d answers %q, want %q", i, r.CallID, s.pending[i])
		}
	}
	return nil
}

func parseChoice(choice openai.ChatCompletionChoice) (llm.Turn, error) {
	stop, err := stopReason(choice.FinishReason)
	if err != nil {
		return llm.Turn{}, err
	}
	turn := llm.Turn{Text: choice.Message.Content, Stop: stop}
	for _, call := range choice.Message.ToolCalls {
		fn, ok := call.AsAny().(openai.ChatCompletionMessageFunctionToolCall)
		if !ok {
			return llm.Turn{}, fmt.Errorf("%w: openai returned unsupported tool call type %q", llm.ErrProvider, call.Type)
		}
		turn.ToolCalls = append(turn.ToolCalls, llm.ToolCall{
			ID:    fn.ID,
			Name:  fn.Function.Name,
			Input: json.RawMessage(fn.Function.Arguments),
		})
	}
	return turn, nil
}

func stopReason(reason string) (llm.StopReason, error) {
	switch reason {
	case "tool_calls":
		return llm.StopToolUse, nil
	case "stop":
		return llm.StopEndTurn, nil
	case "length":
		return llm.StopMaxTokens, nil
	case "content_filter":
		return llm.StopRefusal, nil
	default:
		return "", fmt.Errorf("%w: openai returned unexpected finish reason %q", llm.ErrProvider, reason)
	}
}

func usage(u openai.CompletionUsage) llm.Usage {
	return llm.Usage{
		InputTokens:     u.PromptTokens,
		OutputTokens:    u.CompletionTokens,
		CacheReadTokens: u.PromptTokensDetails.CachedTokens,
		Calls:           1,
	}
}

func (p *Provider) sanitize(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		p.logger.Warn("llm request failed", slog.String("provider", name), slog.Int("status", apiErr.StatusCode))
		return fmt.Errorf("%w: openai returned status %d", llm.ErrProvider, apiErr.StatusCode)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		p.logger.Warn("llm request timed out", slog.String("provider", name))
		return fmt.Errorf("%w: openai request timed out", llm.ErrProvider)
	}
	p.logger.Warn("llm request failed", slog.String("provider", name))
	return fmt.Errorf("%w: openai request failed", llm.ErrProvider)
}
