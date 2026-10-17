package openaicompat

import (
	"bytes"
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
	var tools []openai.ChatCompletionToolUnionParam
	for _, spec := range req.Tools {
		tool, err := toolParam(spec)
		if err != nil {
			return nil, err
		}
		tools = append(tools, tool)
	}
	var history []openai.ChatCompletionMessageParamUnion
	if req.System != "" {
		history = append(history, openai.SystemMessage(req.System))
	}
	history = append(history, openai.UserMessage(req.UserMessage()))
	return &session{provider: p, tools: tools, history: history}, nil
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
	if err := llm.CheckResults(name, s.calls > 0, s.pending, results); err != nil {
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

	callCtx, cancel := context.WithTimeout(ctx, s.provider.cfg.RequestTimeout)
	defer cancel()
	resp, err := s.provider.chat.New(callCtx, openai.ChatCompletionNewParams{
		Model:               s.provider.cfg.Model,
		MaxCompletionTokens: openai.Int(int64(s.provider.cfg.MaxTokens)),
		Messages:            messages,
		Tools:               s.tools,
	})
	if err != nil {
		return llm.Turn{}, s.provider.sanitize(ctx, callCtx, err)
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

	messages = append(messages, assistantMessage(choice.Message, turn.ToolCalls))
	s.history = messages
	s.calls++
	s.pending = llm.PendingIDs(turn.ToolCalls)
	return turn, nil
}

func assistantMessage(msg openai.ChatCompletionMessage, calls []llm.ToolCall) openai.ChatCompletionMessageParamUnion {
	var asst openai.ChatCompletionAssistantMessageParam
	if msg.Content != "" {
		asst.Content.OfString = openai.String(msg.Content)
	}
	if msg.Refusal != "" {
		asst.Refusal = openai.String(msg.Refusal)
	}
	for _, c := range calls {
		asst.ToolCalls = append(asst.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID:       c.ID,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{Name: c.Name, Arguments: string(c.Input)},
			},
		})
	}
	return openai.ChatCompletionMessageParamUnion{OfAssistant: &asst}
}

func parseChoice(choice openai.ChatCompletionChoice) (llm.Turn, error) {
	turn := llm.Turn{Text: choice.Message.Content}
	for _, call := range choice.Message.ToolCalls {
		if call.Type != "" && call.Type != "function" {
			return llm.Turn{}, fmt.Errorf("%w: openai returned unsupported tool call type %q", llm.ErrProvider, call.Type)
		}
		args := json.RawMessage(call.Function.Arguments)
		if len(bytes.TrimSpace(args)) == 0 {
			args = json.RawMessage(`{}`)
		}
		turn.ToolCalls = append(turn.ToolCalls, llm.ToolCall{ID: call.ID, Name: call.Function.Name, Input: args})
	}
	stop, err := stopReason(choice.FinishReason, len(turn.ToolCalls) > 0, choice.Message.Refusal != "")
	if err != nil {
		return llm.Turn{}, err
	}
	turn.Stop = stop
	return turn, nil
}

func stopReason(reason string, hasToolCalls, refused bool) (llm.StopReason, error) {
	if hasToolCalls {
		return llm.StopToolUse, nil
	}
	if refused {
		return llm.StopRefusal, nil
	}
	switch reason {
	case "stop", "tool_calls":
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
		InputTokens:     max(0, u.PromptTokens-u.PromptTokensDetails.CachedTokens),
		OutputTokens:    u.CompletionTokens,
		CacheReadTokens: u.PromptTokensDetails.CachedTokens,
		Calls:           1,
	}
}

func (p *Provider) sanitize(ctx, callCtx context.Context, err error) error {
	var api *llm.APIError
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		api = &llm.APIError{Status: apiErr.StatusCode, Type: apiErr.Type}
		if apiErr.Response != nil {
			api.RequestID = apiErr.Response.Header.Get("x-request-id")
		}
	}
	return llm.SanitizeError(ctx, callCtx, p.logger, name, err, api)
}
