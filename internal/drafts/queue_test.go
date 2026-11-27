package drafts

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/claims"
)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func newQueue(c *clock) *Queue {
	return New(Config{
		Model:   func() *claims.ModelSettings { return &claims.ModelSettings{Provider: "anthropic", Model: "m"} },
		Timeout: time.Minute,
		Now:     c.Now,
	})
}

func listening(t *testing.T, q *Queue) {
	t.Helper()
	if job, err := q.Claim(t.Context(), 0); err != nil || job != nil {
		t.Fatalf("idle claim = %v, %v", job, err)
	}
}

func claimOne(t *testing.T, q *Queue) *tools.DraftJob {
	t.Helper()
	job, err := q.Claim(t.Context(), 5*time.Second)
	if err != nil || job == nil {
		t.Fatalf("claim = %v, %v", job, err)
	}
	return job
}

type next struct {
	turn tools.DraftTurn
	err  error
}

func nextAsync(ctx context.Context, s tools.DraftSession, results []tools.DraftResult) chan next {
	out := make(chan next, 1)
	go func() {
		turn, err := s.Next(ctx, results)
		out <- next{turn, err}
	}()
	return out
}

func TestADraftIsUnavailableUntilTheHarnessListens(t *testing.T) {
	c := &clock{now: time.Unix(1000, 0)}
	q := newQueue(c)
	if q.Available() {
		t.Fatal("available before any claim_draft")
	}
	if _, err := q.Start(t.Context(), tools.DraftRequest{}); !errors.Is(err, tools.ErrDraftUnavailable) {
		t.Fatalf("start = %v, want ErrDraftUnavailable", err)
	}
	listening(t, q)
	if !q.Available() {
		t.Fatal("unavailable right after a claim_draft")
	}
	c.now = c.now.Add(listenWindow)
	if q.Available() {
		t.Fatal("available after the listen window")
	}
	if New(Config{}).Available() {
		t.Fatal("a queue without model settings is available")
	}
}

func TestADraftGoesToTheHarnessTurnByTurnAndEndsWhenTheServerCloses(t *testing.T) {
	q := newQueue(&clock{now: time.Unix(1000, 0)})
	listening(t, q)
	req := tools.DraftRequest{System: "sys", Prompt: "errors from checkout", Context: "facts", Tools: []tools.DraftToolSpec{{Name: "submit_query", InputSchema: json.RawMessage(`{}`)}}}
	s, err := q.Start(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	first := nextAsync(t.Context(), s, nil)
	job := claimOne(t, q)
	if job.System != "sys" || job.Prompt != "errors from checkout" || job.Context != "facts" || len(job.Tools) != 1 || job.Model == nil || job.TimeoutSeconds != 60 {
		t.Fatalf("job = %+v", job)
	}

	call := tools.DraftCall{ID: "c1", Name: "submit_query", Input: json.RawMessage(`{"query":"x"}`)}
	replied := make(chan tools.DraftReply, 1)
	go func() {
		reply, err := q.Finish(t.Context(), tools.DraftReport{ID: job.ID, Turn: &tools.DraftTurn{Stop: tools.DraftStopToolUse, ToolCalls: []tools.DraftCall{call}}})
		if err != nil {
			t.Error(err)
		}
		replied <- reply
	}()
	got := <-first
	if got.err != nil || len(got.turn.ToolCalls) != 1 || got.turn.ToolCalls[0].ID != "c1" {
		t.Fatalf("first turn = %+v", got)
	}

	second := nextAsync(t.Context(), s, []tools.DraftResult{{CallID: "c1", IsError: true, Content: "fix it"}})
	if reply := <-replied; reply.Done || len(reply.Results) != 1 || reply.Results[0].Content != "fix it" {
		t.Fatalf("reply to the first turn = %+v", reply)
	}
	go func() {
		reply, err := q.Finish(t.Context(), tools.DraftReport{ID: job.ID, Turn: &tools.DraftTurn{Stop: tools.DraftStopToolUse, ToolCalls: []tools.DraftCall{call}}})
		if err != nil {
			t.Error(err)
		}
		replied <- reply
	}()
	if got := <-second; got.err != nil || len(got.turn.ToolCalls) != 1 {
		t.Fatalf("second turn = %+v", got)
	}
	s.Close()
	if reply := <-replied; !reply.Done {
		t.Fatalf("reply after the server closed = %+v, want done", reply)
	}
	if _, err := q.Finish(t.Context(), tools.DraftReport{ID: job.ID, Failure: tools.DraftFailureError}); !errors.Is(err, tools.ErrDraftEnded) {
		t.Fatalf("finish after the end = %v, want ErrDraftEnded", err)
	}
}

func TestAModelFailureReachesTheServerAsADraftModelError(t *testing.T) {
	q := newQueue(&clock{now: time.Unix(1000, 0)})
	listening(t, q)
	s, err := q.Start(t.Context(), tools.DraftRequest{Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	first := nextAsync(t.Context(), s, nil)
	job := claimOne(t, q)
	go func() {
		_, _ = q.Finish(t.Context(), tools.DraftReport{ID: job.ID, Failure: tools.DraftFailureRateLimited})
	}()
	var modelErr *tools.DraftModelError
	if got := <-first; !errors.As(got.err, &modelErr) || modelErr.Failure != tools.DraftFailureRateLimited {
		t.Fatalf("next = %+v, want a rate_limited DraftModelError", got)
	}
}

func TestLostEndsEveryDraftWithAnError(t *testing.T) {
	q := newQueue(&clock{now: time.Unix(1000, 0)})
	listening(t, q)
	claimed, err := q.Start(t.Context(), tools.DraftRequest{Prompt: "claimed"})
	if err != nil {
		t.Fatal(err)
	}
	claimedNext := nextAsync(t.Context(), claimed, nil)
	claimOne(t, q)
	waiting, err := q.Start(t.Context(), tools.DraftRequest{Prompt: "waiting"})
	if err != nil {
		t.Fatal(err)
	}
	waitingNext := nextAsync(t.Context(), waiting, nil)
	for !queued(q, 1) {
		time.Sleep(time.Millisecond)
	}
	q.Lost()
	for name, ch := range map[string]chan next{"claimed": claimedNext, "waiting": waitingNext} {
		if got := <-ch; !errors.Is(got.err, tools.ErrDraftStopped) {
			t.Errorf("%s draft = %+v, want ErrDraftStopped", name, got)
		}
	}
	if q.Available() {
		t.Fatal("available after the harness was lost")
	}
}

func TestADraftClosedBeforeItsClaimIsNeverOffered(t *testing.T) {
	q := newQueue(&clock{now: time.Unix(1000, 0)})
	listening(t, q)
	ctx, cancel := context.WithCancel(t.Context())
	s, err := q.Start(ctx, tools.DraftRequest{Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	pending := nextAsync(ctx, s, nil)
	for !queued(q, 1) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if got := <-pending; !errors.Is(got.err, context.Canceled) {
		t.Fatalf("next = %+v, want canceled", got)
	}
	s.Close()
	if job, err := q.Claim(t.Context(), 0); err != nil || job != nil {
		t.Fatalf("claim after the close = %v, %v, want none", job, err)
	}
}

func queued(q *Queue, n int) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.waiting) == n
}
