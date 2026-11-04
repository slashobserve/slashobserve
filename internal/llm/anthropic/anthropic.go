package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	name = "anthropic"
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
	cfg      Config
	messages sdk.MessageService
}

func New(cfg Config) *Provider {
	opts := []option.RequestOption{
		option.WithoutEnvironmentDefaults(),
		option.WithAPIKey(cfg.APIKey),
		option.WithMaxRetries(0),
		option.WithMiddleware(llm.TraceAttempt),
	}
	if cfg.BaseURL != nil {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL.String()))
	}
	client := sdk.NewClient(opts...)
	return &Provider{cfg: cfg, messages: client.Messages}
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

		buildSaves: req.BuildSaves,
		pruner:     llm.NewPruner(req.Pruning),
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

	buildSaves int
	pruner     *llm.Pruner
}

func (s *session) Next(ctx context.Context, results []llm.ToolResult) (llm.Turn, error) {
	if err := ctx.Err(); err != nil {
		return llm.Turn{}, err
	}
	if err := llm.CheckResults(name, s.calls > 0, s.pending, results); err != nil {
		return llm.Turn{}, err
	}

	s.pruner.Note(results)
	messages := s.history[:len(s.history):len(s.history)]
	if len(results) > 0 {
		blocks := make([]sdk.ContentBlockParamUnion, 0, len(results))
		for _, r := range results {
			blocks = append(blocks, sdk.NewToolResultBlock(r.CallID, r.Content, r.IsError))
		}
		messages = append(messages, sdk.NewUserMessage(blocks...))
	}

	callCtx, cancel := llm.CallContext(ctx, s.provider.cfg.RequestTimeout)
	defer cancel()
	configured := llm.Tuning{Effort: s.provider.cfg.Effort, MaxTokens: s.provider.cfg.MaxTokens}
	tuning := configured.ForCall(s.calls == 0 && s.buildSaves == 0)
	sent := s.pruned(messages)
	resp, turn, err := s.exchange(ctx, callCtx, sent, tuning)
	if err == nil && configured.CutOffEarly(tuning, turn.Stop) {
		cutOff := turn.Usage
		resp, turn, err = s.exchange(ctx, callCtx, sent, configured)
		turn.Usage = cutOff.Add(turn.Usage)
		turn.Usage.Calls = 1
	}
	if err != nil {
		return llm.Turn{}, err
	}
	if turn.Stop == llm.StopMaxTokens {
		return turn, nil
	}

	messages = append(messages, resp.ToParam())
	s.history = messages
	s.calls++
	s.pending = llm.PendingIDs(turn.ToolCalls)
	return turn, nil
}

func (s *session) Compact(c llm.Compaction) error {
	rest := s.history[1:]
	weighed := make([]llm.Weighed, len(rest))
	for i, m := range rest {
		weighed[i] = llm.Weighed{Assistant: m.Role == sdk.MessageParamRoleAssistant, Bytes: messageBytes(m, c.ResultBytes)}
	}
	kept := rest[llm.TailStart(weighed, c.TailTurns, c.TailBytes):]
	history := make([]sdk.MessageParam, 0, 1+len(kept))
	history = append(history, sdk.NewUserMessage(sdk.NewTextBlock(c.Message)))
	for i, m := range kept {
		switch {
		case m.Role == sdk.MessageParamRoleUser:
			m = cutToolResults(m, c.ResultBytes)
		case i < len(kept)-1:
			m = withoutThinking(m)
		}
		history = append(history, m)
	}
	s.history = history
	s.pruner.Reset()
	return nil
}

func (s *session) pruned(messages []sdk.MessageParam) []sdk.MessageParam {
	view := make([]llm.PruneMessage, len(messages))
	for i, m := range messages {
		view[i].Assistant = m.Role == sdk.MessageParamRoleAssistant
		for _, b := range m.Content {
			if b.OfToolResult != nil {
				view[i].Results = append(view[i].Results, llm.PruneResult{ID: b.OfToolResult.ToolUseID, Bytes: messageBytes(sdk.MessageParam{Content: []sdk.ContentBlockParamUnion{b}}, 0)})
			}
		}
	}
	boundary := s.pruner.Boundary(view)
	if boundary == 0 {
		return messages
	}
	out := slices.Clone(messages)
	for i := range boundary {
		out[i] = s.withStubs(out[i])
	}
	return out
}

func (s *session) withStubs(m sdk.MessageParam) sdk.MessageParam {
	if !slices.ContainsFunc(m.Content, func(b sdk.ContentBlockParamUnion) bool {
		return b.OfToolResult != nil && s.pruner.Prunable(b.OfToolResult.ToolUseID)
	}) {
		return m
	}
	blocks := slices.Clone(m.Content)
	for i, b := range blocks {
		if b.OfToolResult == nil || !s.pruner.Prunable(b.OfToolResult.ToolUseID) {
			continue
		}
		stub := sdk.NewToolResultBlock(b.OfToolResult.ToolUseID, llm.PrunedResult, b.OfToolResult.IsError.Value)
		blocks[i] = stub
	}
	m.Content = blocks
	return m
}

func messageBytes(m sdk.MessageParam, resultBytes int) int {
	total := 0
	for _, b := range m.Content {
		switch {
		case b.OfText != nil:
			total += len(b.OfText.Text)
		case b.OfToolUse != nil:
			total += inputBytes(b.OfToolUse.Input)
		case b.OfToolResult != nil:
			for _, part := range b.OfToolResult.Content {
				if part.OfText != nil {
					total += llm.KeptBytes(len(part.OfText.Text), resultBytes)
				}
			}
		}
	}
	return total
}

func inputBytes(input any) int {
	if raw, ok := input.(json.RawMessage); ok {
		return len(raw)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return 0
	}
	return len(encoded)
}

func cutToolResults(m sdk.MessageParam, n int) sdk.MessageParam {
	blocks := slices.Clone(m.Content)
	for i, b := range blocks {
		if b.OfToolResult == nil {
			continue
		}
		result := *b.OfToolResult
		result.Content = slices.Clone(result.Content)
		for j, part := range result.Content {
			if part.OfText != nil {
				text := *part.OfText
				text.Text = llm.CutResult(text.Text, n)
				result.Content[j].OfText = &text
			}
		}
		blocks[i].OfToolResult = &result
	}
	m.Content = blocks
	return m
}

func withoutThinking(m sdk.MessageParam) sdk.MessageParam {
	m.Content = slices.DeleteFunc(slices.Clone(m.Content), func(b sdk.ContentBlockParamUnion) bool {
		return b.OfThinking != nil || b.OfRedactedThinking != nil
	})
	return m
}

func (s *session) exchange(ctx, callCtx context.Context, messages []sdk.MessageParam, tuning llm.Tuning) (sdk.Message, llm.Turn, error) {
	resp, err := s.provider.stream(ctx, callCtx, sdk.MessageNewParams{
		Model:     s.provider.cfg.Model,
		MaxTokens: int64(tuning.MaxTokens),
		System:    s.system,
		Tools:     s.tools,
		Messages:  withCacheBreakpoint(messages),
		OutputConfig: sdk.OutputConfigParam{
			Effort: sdk.OutputConfigEffort(tuning.Effort),
		},
	})
	if err != nil {
		return sdk.Message{}, llm.Turn{}, err
	}
	turn, err := parseResponse(&resp)
	return resp, turn, err
}

func (p *Provider) stream(ctx, callCtx context.Context, params sdk.MessageNewParams) (sdk.Message, error) {
	msg, err := llm.Retry(callCtx, func(attemptCtx context.Context) (sdk.Message, error) {
		return p.streamOnce(ctx, attemptCtx, params)
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return sdk.Message{}, ctxErr
		}
		return sdk.Message{}, err
	}
	return msg, nil
}

func (p *Provider) streamOnce(ctx, callCtx context.Context, params sdk.MessageNewParams) (sdk.Message, error) {
	watch := llm.NewIdleWatch(p.cfg.IdleTimeout)
	stream := p.messages.NewStreaming(callCtx, params, option.WithMiddleware(watch.Middleware))
	defer func() { _ = stream.Close() }()
	var msg sdk.Message
	for stream.Next() {
		if err := msg.Accumulate(stream.Current()); err != nil {
			return sdk.Message{}, p.sanitize(ctx, callCtx, watch, err)
		}
	}
	if err := stream.Err(); err != nil {
		return sdk.Message{}, p.sanitize(ctx, callCtx, watch, err)
	}
	return msg, nil
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

func (p *Provider) sanitize(ctx, callCtx context.Context, watch *llm.IdleWatch, err error) error {
	if idle := watch.Failure(ctx, callCtx, name); idle != nil {
		return idle
	}
	var api *llm.APIError
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		api = &llm.APIError{Status: apiErr.StatusCode, Type: string(apiErr.Type()), RequestID: apiErr.RequestID, Message: errorMessage(apiErr)}
		if apiErr.Response != nil {
			api.RetryAfter = llm.ParseRetryAfter(apiErr.Response.Header.Get("Retry-After"))
		}
	}
	return llm.SanitizeError(ctx, callCtx, name, err, api, llm.MessageLog{Enabled: p.cfg.LogProviderErrors, APIKey: p.cfg.APIKey})
}

func errorMessage(apiErr *sdk.Error) string {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(apiErr.RawJSON()), &envelope) != nil {
		return ""
	}
	return envelope.Error.Message
}
