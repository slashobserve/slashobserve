package drafts

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/opaquetoken"
)

// A harness that polled within listenWindow is taken to be listening: its claim_draft waits at most 30 seconds.
const listenWindow = 40 * time.Second

var (
	errOutOfTurn = errors.New("this draft is not waiting for a model turn")
	errNoModel   = errors.New("no model is configured")
)

type Config struct {
	Model   func() *claims.ModelSettings
	Timeout time.Duration
	Now     func() time.Time
}

type Queue struct {
	cfg Config

	mu       sync.Mutex
	waiting  []*job
	jobs     map[string]*job
	wake     chan struct{}
	polling  int
	lastPoll time.Time
}

type answer struct {
	turn tools.DraftTurn
	err  error
}

type job struct {
	id      string
	req     tools.DraftRequest
	turns   chan answer
	replies chan []tools.DraftResult
	ended   chan struct{}
	once    sync.Once
	claimed bool
	err     error
}

func (j *job) end(err error) {
	j.once.Do(func() {
		j.err = err
		close(j.ended)
	})
}

func New(cfg Config) *Queue {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Queue{cfg: cfg, jobs: map[string]*job{}, wake: make(chan struct{})}
}

func (q *Queue) Available() bool {
	if q.cfg.Model == nil {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.polling > 0 || (!q.lastPoll.IsZero() && q.cfg.Now().Sub(q.lastPoll) < listenWindow)
}

func (q *Queue) Start(ctx context.Context, req tools.DraftRequest) (tools.DraftSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !q.Available() {
		return nil, tools.ErrDraftUnavailable
	}
	id, _, err := opaquetoken.New("draft_")
	if err != nil {
		return nil, err
	}
	j := &job{id: id, req: req, turns: make(chan answer, 1), replies: make(chan []tools.DraftResult), ended: make(chan struct{})}
	return &session{queue: q, job: j}, nil
}

func (q *Queue) enqueue(j *job) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs[j.id] = j
	q.waiting = append(q.waiting, j)
	close(q.wake)
	q.wake = make(chan struct{})
}

func (q *Queue) remove(j *job) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.jobs, j.id)
	for i, w := range q.waiting {
		if w == j {
			q.waiting = append(q.waiting[:i], q.waiting[i+1:]...)
			break
		}
	}
}

// Lost ends every draft: the harness that held or would take them stopped.
func (q *Queue) Lost() {
	q.mu.Lock()
	jobs := make([]*job, 0, len(q.jobs))
	for _, j := range q.jobs {
		jobs = append(jobs, j)
	}
	q.jobs = map[string]*job{}
	q.waiting = nil
	q.lastPoll = time.Time{}
	q.mu.Unlock()
	for _, j := range jobs {
		j.end(tools.ErrDraftStopped)
	}
}

func (q *Queue) Claim(ctx context.Context, wait time.Duration) (*tools.DraftJob, error) {
	if q.cfg.Model == nil {
		return nil, errNoModel
	}
	q.mu.Lock()
	q.polling++
	q.mu.Unlock()
	defer func() {
		q.mu.Lock()
		q.polling--
		q.lastPoll = q.cfg.Now()
		q.mu.Unlock()
	}()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		q.mu.Lock()
		if len(q.waiting) > 0 {
			j := q.waiting[0]
			q.waiting = q.waiting[1:]
			j.claimed = true
			q.mu.Unlock()
			return q.offer(j), nil
		}
		wake := q.wake
		q.mu.Unlock()
		select {
		case <-wake:
		case <-timer.C:
			return nil, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (q *Queue) offer(j *job) *tools.DraftJob {
	return &tools.DraftJob{
		ID:             j.id,
		System:         j.req.System,
		Prompt:         j.req.Prompt,
		Context:        j.req.Context,
		Tools:          j.req.Tools,
		Model:          q.cfg.Model(),
		TimeoutSeconds: int(q.cfg.Timeout.Seconds()),
	}
}

func (q *Queue) Finish(ctx context.Context, report tools.DraftReport) (tools.DraftReply, error) {
	q.mu.Lock()
	j, ok := q.jobs[report.ID]
	claimed := ok && j.claimed
	q.mu.Unlock()
	if !claimed {
		return tools.DraftReply{}, tools.ErrDraftEnded
	}
	a := answer{}
	if report.Turn != nil {
		a.turn = *report.Turn
	} else {
		a.err = &tools.DraftModelError{Failure: report.Failure}
	}
	select {
	case j.turns <- a:
	default:
		return tools.DraftReply{}, errOutOfTurn
	}
	select {
	case results := <-j.replies:
		return tools.DraftReply{Results: results}, nil
	case <-j.ended:
		return tools.DraftReply{Done: true}, nil
	case <-ctx.Done():
		return tools.DraftReply{}, ctx.Err()
	}
}

type session struct {
	queue   *Queue
	job     *job
	started bool
}

func (s *session) Next(ctx context.Context, results []tools.DraftResult) (tools.DraftTurn, error) {
	j := s.job
	if !s.started {
		s.started = true
		s.queue.enqueue(j)
	} else {
		select {
		case j.replies <- results:
		case <-j.ended:
			return tools.DraftTurn{}, j.err
		case <-ctx.Done():
			return tools.DraftTurn{}, ctx.Err()
		}
	}
	select {
	case a := <-j.turns:
		return a.turn, a.err
	case <-j.ended:
		return tools.DraftTurn{}, j.err
	case <-ctx.Done():
		return tools.DraftTurn{}, ctx.Err()
	}
}

func (s *session) Close() {
	s.queue.remove(s.job)
	s.job.end(nil)
}
