package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"

	"github.com/slashobserve/slashobserve/internal/llm"
)

const (
	name             = "openai"
	errorResultLabel = "ERROR: "
)

type Config struct {
	APIKey         string
	Model          string
	Effort         string
	BaseURL        *url.URL
	MaxTokens      int
	RequestTimeout time.Duration
	IdleTimeout    time.Duration

	LogProviderErrors bool
}

type Provider struct {
	cfg    Config
	chat   openai.ChatCompletionService
	logger *slog.Logger
}

func New(cfg Config, logger *slog.Logger) *Provider {
	opts := []option.RequestOption{
		option.WithEnvironmentProduction(),
		option.WithMaxRetries(0),
		option.WithMiddleware(llm.TraceAttempt),
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
	return &session{provider: p, tools: tools, history: history, buildSaves: req.BuildSaves, pruner: llm.NewPruner(req.Pruning)}, nil
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
	ended    bool

	buildSaves int
	pruner     *llm.Pruner
}

func (s *session) Next(ctx context.Context, results []llm.ToolResult) (llm.Turn, error) {
	if err := ctx.Err(); err != nil {
		return llm.Turn{}, err
	}
	if err := llm.CheckResults(name, s.ended, s.pending, results); err != nil {
		return llm.Turn{}, err
	}

	s.pruner.Note(results)
	messages := s.history[:len(s.history):len(s.history)]
	for _, r := range results {
		content := r.Content
		if r.IsError {
			content = errorResultLabel + content
		}
		messages = append(messages, openai.ToolMessage(content, r.CallID))
	}

	callCtx, cancel := llm.CallContext(ctx, s.provider.cfg.RequestTimeout)
	defer cancel()
	configured := llm.Tuning{Effort: s.provider.cfg.Effort, MaxTokens: s.provider.cfg.MaxTokens}
	tuning := configured.ForCall(s.calls == 0 && s.buildSaves == 0)
	sent := s.pruned(messages)
	message, turn, err := s.exchange(ctx, callCtx, sent, tuning)
	if err == nil && configured.CutOffEarly(tuning, turn.Stop) {
		cutOff := turn.Usage
		message, turn, err = s.exchange(ctx, callCtx, sent, configured)
		turn.Usage = cutOff.Add(turn.Usage)
		turn.Usage.Calls = 1
	}
	if err != nil {
		return llm.Turn{}, err
	}
	if turn.Stop == llm.StopMaxTokens {
		return turn, nil
	}

	messages = append(messages, assistantMessage(message, turn.ToolCalls))
	s.history = messages
	s.calls++
	s.pending = llm.PendingIDs(turn.ToolCalls)
	s.ended = len(s.pending) == 0
	return turn, nil
}

func endsInPlainAnswer(history []openai.ChatCompletionMessageParamUnion) bool {
	if len(history) == 0 {
		return false
	}
	last := history[len(history)-1].OfAssistant
	return last != nil && len(last.ToolCalls) == 0
}

func (s *session) Compact(c llm.Compaction) error {
	lead := 1
	if s.history[0].OfSystem != nil {
		lead = 2
	}
	rest := s.history[lead:]
	if endsInPlainAnswer(rest) {
		rest = rest[:len(rest)-1]
		s.ended = false
	}
	weighed := make([]llm.Weighed, len(rest))
	for i, m := range rest {
		weighed[i] = llm.Weighed{Assistant: m.OfAssistant != nil, Bytes: messageBytes(m, c.ResultBytes)}
	}
	kept := rest[llm.TailStart(weighed, c.TailTurns, c.TailBytes):]
	history := make([]openai.ChatCompletionMessageParamUnion, 0, lead+len(kept))
	history = append(history, s.history[:lead-1]...)
	history = append(history, openai.UserMessage(c.Message))
	for _, m := range kept {
		if m.OfTool != nil && m.OfTool.Content.OfString.Valid() {
			m = openai.ToolMessage(llm.CutResult(m.OfTool.Content.OfString.Value, c.ResultBytes), m.OfTool.ToolCallID)
		}
		history = append(history, m)
	}
	s.history = history
	s.pruner.Reset()
	return nil
}

func (s *session) pruned(messages []openai.ChatCompletionMessageParamUnion) []openai.ChatCompletionMessageParamUnion {
	view := make([]llm.PruneMessage, len(messages))
	for i, m := range messages {
		view[i].Assistant = m.OfAssistant != nil
		if m.OfTool != nil {
			view[i].Results = []llm.PruneResult{{ID: m.OfTool.ToolCallID, Bytes: messageBytes(m, 0)}}
		}
	}
	boundary := s.pruner.Boundary(view)
	if boundary == 0 {
		return messages
	}
	out := slices.Clone(messages)
	for i := range boundary {
		if m := out[i].OfTool; m != nil && s.pruner.Prunable(m.ToolCallID) {
			stub := llm.PrunedResult
			if strings.HasPrefix(m.Content.OfString.Value, errorResultLabel) {
				stub = errorResultLabel + stub
			}
			out[i] = openai.ToolMessage(stub, m.ToolCallID)
		}
	}
	return out
}

func messageBytes(m openai.ChatCompletionMessageParamUnion, resultBytes int) int {
	switch {
	case m.OfTool != nil:
		return llm.KeptBytes(len(m.OfTool.Content.OfString.Value), resultBytes)
	case m.OfAssistant != nil:
		total := len(m.OfAssistant.Content.OfString.Value)
		for _, call := range m.OfAssistant.ToolCalls {
			if call.OfFunction != nil {
				total += len(call.OfFunction.Function.Arguments)
			}
		}
		return total
	}
	return 0
}

func (s *session) exchange(ctx, callCtx context.Context, messages []openai.ChatCompletionMessageParamUnion, tuning llm.Tuning) (openai.ChatCompletionMessage, llm.Turn, error) {
	resp, used, err := s.provider.stream(ctx, callCtx, openai.ChatCompletionNewParams{
		Model:               s.provider.cfg.Model,
		MaxCompletionTokens: openai.Int(int64(tuning.MaxTokens)),
		ReasoningEffort:     shared.ReasoningEffort(tuning.Effort),
		Messages:            messages,
		Tools:               s.tools,
		StreamOptions:       openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)},
	})
	if err != nil {
		return openai.ChatCompletionMessage{}, llm.Turn{}, err
	}
	if len(resp.Choices) == 0 {
		return openai.ChatCompletionMessage{}, llm.Turn{}, fmt.Errorf("%w: openai returned no choices", llm.ErrProvider)
	}
	choice := resp.Choices[0]
	turn, err := s.provider.parseChoice(ctx, choice)
	if err != nil {
		return openai.ChatCompletionMessage{}, llm.Turn{}, err
	}
	turn.Usage = usage(used)
	return choice.Message, turn, nil
}

type streamed struct {
	completion openai.ChatCompletion
	usage      openai.CompletionUsage
}

func (p *Provider) stream(ctx, callCtx context.Context, params openai.ChatCompletionNewParams) (openai.ChatCompletion, openai.CompletionUsage, error) {
	got, err := llm.Retry(callCtx, func(attemptCtx context.Context) (streamed, error) {
		return p.streamOnce(ctx, attemptCtx, params)
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return openai.ChatCompletion{}, openai.CompletionUsage{}, ctxErr
		}
		return openai.ChatCompletion{}, openai.CompletionUsage{}, err
	}
	return got.completion, got.usage, nil
}

func (p *Provider) streamOnce(ctx, callCtx context.Context, params openai.ChatCompletionNewParams) (streamed, error) {
	watch := llm.NewIdleWatch(p.cfg.IdleTimeout)
	stream := p.chat.NewStreaming(callCtx, params, option.WithMiddleware(watch.Middleware))
	defer func() { _ = stream.Close() }()
	var acc openai.ChatCompletionAccumulator
	var used openai.CompletionUsage
	var finishReason string
	for stream.Next() {
		chunk := stream.Current()
		if !acc.AddChunk(chunk) {
			return streamed{}, fmt.Errorf("%w: openai returned a stream chunk that does not belong to the completion", llm.ErrProvider)
		}
		for _, choice := range chunk.Choices {
			if choice.Index == 0 && choice.FinishReason != "" {
				finishReason = choice.FinishReason
			}
		}
		if chunk.Usage.TotalTokens > 0 || chunk.Usage.CompletionTokens > 0 {
			used = chunk.Usage
		}
	}
	if err := stream.Err(); err != nil {
		return streamed{}, p.sanitize(ctx, callCtx, watch, err)
	}
	completion := acc.ChatCompletion
	if finishReason != "" && len(completion.Choices) > 0 {
		completion.Choices[0].FinishReason = finishReason
	}
	return streamed{completion: completion, usage: used}, nil
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

func (p *Provider) parseChoice(ctx context.Context, choice openai.ChatCompletionChoice) (llm.Turn, error) {
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
	turn.Stop = p.stopReason(ctx, choice.FinishReason, len(turn.ToolCalls) > 0, choice.Message.Refusal != "")
	return turn, nil
}

func (p *Provider) stopReason(ctx context.Context, reason string, hasToolCalls, refused bool) llm.StopReason {
	if hasToolCalls && reason != "length" {
		return llm.StopToolUse
	}
	if refused {
		return llm.StopRefusal
	}
	switch reason {
	case "stop", "tool_calls":
		return llm.StopEndTurn
	case "length":
		return llm.StopMaxTokens
	case "content_filter":
		return llm.StopRefusal
	default:
		attrs := []any{slog.String("provider", name)}
		if llm.IsPlainToken(reason) {
			attrs = append(attrs, slog.String("finish_reason", reason))
		}
		p.logger.WarnContext(ctx, "llm finish reason treated as end of turn", attrs...)
		return llm.StopEndTurn
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

func (p *Provider) sanitize(ctx, callCtx context.Context, watch *llm.IdleWatch, err error) error {
	if idle := watch.Failure(ctx, callCtx, name); idle != nil {
		return idle
	}
	var api *llm.APIError
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		api = &llm.APIError{Status: apiErr.StatusCode, Type: apiErr.Type, Code: apiErr.Code, Param: apiErr.Param, Message: apiErr.Message}
		if apiErr.Response != nil {
			api.RequestID = apiErr.Response.Header.Get("x-request-id")
			api.RetryAfter = llm.ParseRetryAfter(apiErr.Response.Header.Get("Retry-After"))
		}
	}
	return llm.SanitizeError(ctx, callCtx, name, err, api, llm.MessageLog{Enabled: p.cfg.LogProviderErrors, APIKey: p.cfg.APIKey})
}
