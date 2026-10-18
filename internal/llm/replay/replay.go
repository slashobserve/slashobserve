package replay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/llm"
)

const (
	name              = "replay"
	promptHashLength  = 12
	fixtureFileSuffix = ".json"
)

type Provider struct {
	fixtures map[string]*fixture
	logger   *slog.Logger
}

func New(dir string, logger *slog.Logger) (*Provider, error) {
	if dir == "" {
		return nil, errors.New("replay fixture directory is not set")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read replay fixture directory: %w", err)
	}

	p := &Provider{fixtures: map[string]*fixture{}, logger: logger}
	var errs []error
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), fixtureFileSuffix) {
			continue
		}
		f, err := loadFixture(filepath.Join(dir, entry.Name()), entry.Name())
		if err != nil {
			errs = append(errs, err)
			continue
		}
		key := promptKey(f.Prompt)
		if other, ok := p.fixtures[key]; ok {
			errs = append(errs, fmt.Errorf("replay fixtures %s and %s have the same prompt", other.File, f.File))
			continue
		}
		p.fixtures[key] = f
		logger.Debug("replay fixture loaded", slog.String("fixture", f.File), slog.String("prompt_key_sha256", promptKeyHash(f.Prompt)))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	logger.Info("replay fixtures loaded", slog.Int("count", len(p.fixtures)))
	logger.Warn("replay LLM provider answers only from recorded fixtures; use it for tests only")
	return p, nil
}

func loadFixture(path, file string) (*fixture, error) {
	content, err := os.ReadFile(path) //nolint:gosec // the directory is operator configuration
	if err != nil {
		return nil, fmt.Errorf("replay fixture %s: %w", file, err)
	}
	return parseFixture(file, content)
}

func (p *Provider) Name() string  { return name }
func (p *Provider) Model() string { return name }

func (p *Provider) Start(ctx context.Context, req llm.Request) (llm.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, ok := p.fixtures[promptKey(req.Prompt)]
	if !ok {
		p.logger.DebugContext(ctx, "replay found no fixture for the prompt", slog.Int("prompt_length", len(req.Prompt)), slog.String("prompt_key_sha256", promptKeyHash(req.Prompt)))
		return nil, llm.ErrNoFixture
	}
	return &session{fixture: f, logger: p.logger}, nil
}

type session struct {
	fixture  *fixture
	logger   *slog.Logger
	next     int
	previous []llm.ToolCall
}

func (s *session) Next(ctx context.Context, results []llm.ToolResult) (llm.Turn, error) {
	if err := ctx.Err(); err != nil {
		return llm.Turn{}, err
	}
	index := s.next
	if index >= len(s.fixture.Turns) {
		return llm.Turn{}, fmt.Errorf("replay fixture %s: no turn %d", s.fixture.File, index)
	}
	if err := s.checkResults(ctx, index, results); err != nil {
		return llm.Turn{}, err
	}

	t := s.fixture.Turns[index]
	out := llm.Turn{Text: t.Text, Stop: llm.StopEndTurn, Usage: llm.Usage{Calls: 1}}
	for i, c := range t.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
			ID:    fmt.Sprintf("replay_%d_%d", index, i),
			Name:  c.Name,
			Input: slices.Clone(c.Input),
		})
	}
	if len(out.ToolCalls) > 0 {
		out.Stop = llm.StopToolUse
	}
	s.next = index + 1
	s.previous = out.ToolCalls
	return out, nil
}

func (s *session) checkResults(ctx context.Context, index int, results []llm.ToolResult) error {
	if len(results) != len(s.previous) {
		return fmt.Errorf("replay fixture %s: turn %d: got %d tool results for %d tool calls", s.fixture.File, index, len(results), len(s.previous))
	}
	for i, r := range results {
		if r.CallID != s.previous[i].ID {
			return fmt.Errorf("replay fixture %s: turn %d: tool result %d answers %q, want %q", s.fixture.File, index, i, r.CallID, s.previous[i].ID)
		}
	}
	expect := s.fixture.Turns[index].Expect
	for i, r := range results {
		if err := s.checkResult(ctx, index, i, r, expect); err != nil {
			return err
		}
	}
	return nil
}

func (s *session) checkResult(ctx context.Context, index, i int, r llm.ToolResult, expect []expectation) error {
	call := s.previous[i]
	where := fmt.Sprintf("replay fixture %s: turn %d: tool call %d (%s)", s.fixture.File, index, i, call.Name)
	wantError := false
	if expect != nil {
		wantError = *expect[i].IsError
	}
	if r.IsError != wantError {
		s.logExpectationFailure(ctx, index, i, call.Name, r, wantError)
		return fmt.Errorf("%s: is_error was %t, expected %t", where, r.IsError, wantError)
	}
	if expect != nil && !strings.Contains(r.Content, expect[i].Contains) {
		s.logExpectationFailure(ctx, index, i, call.Name, r, wantError)
		return fmt.Errorf("%s: result does not contain %q", where, expect[i].Contains)
	}
	return nil
}

func (s *session) logExpectationFailure(ctx context.Context, turn, call int, tool string, r llm.ToolResult, wantError bool) {
	attrs := []any{
		slog.String("fixture", s.fixture.File),
		slog.Int("turn", turn),
		slog.Int("call", call),
		slog.String("tool", tool),
		slog.Bool("is_error", r.IsError),
		slog.Bool("expected_is_error", wantError),
		slog.Int("result_length", len(r.Content)),
	}
	if code := toolErrorCode(r); code != "" {
		attrs = append(attrs, slog.String("tool_error_code", code))
	}
	s.logger.DebugContext(ctx, "replay expectation failed", attrs...)
}

func toolErrorCode(r llm.ToolResult) string {
	var body struct {
		Code string `json:"code"`
	}
	if !r.IsError || json.Unmarshal([]byte(r.Content), &body) != nil || !llm.IsPlainToken(body.Code) {
		return ""
	}
	return body.Code
}

func promptKeyHash(prompt string) string {
	sum := sha256.Sum256([]byte(promptKey(prompt)))
	return hex.EncodeToString(sum[:])[:promptHashLength]
}
