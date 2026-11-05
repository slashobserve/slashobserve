package queue

import (
	"context"
	"sync"
	"time"
)

const pausePoll = 20 * time.Millisecond

type Job[T any] struct {
	Value    T
	Enqueued time.Time
}

type Queue[T any] struct {
	workers   int
	capacity  int
	pausedPct func() int
	process   func(Job[T])

	mu     sync.Mutex
	ready  *sync.Cond
	jobs   []Job[T]
	closed bool
}

func New[T any](workers, capacity int, pausedPct func() int, process func(Job[T])) *Queue[T] {
	q := &Queue[T]{workers: workers, capacity: capacity, pausedPct: pausedPct, process: process}
	q.ready = sync.NewCond(&q.mu)
	return q
}

func (q *Queue[T]) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for i := range q.workers {
		wg.Go(func() { q.work(ctx, i) })
	}
	<-ctx.Done()
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.ready.Broadcast()
	wg.Wait()
}

func (q *Queue[T]) Push(value T) bool {
	q.mu.Lock()
	if q.closed || len(q.jobs) >= q.capacity {
		q.mu.Unlock()
		return false
	}
	q.jobs = append(q.jobs, Job[T]{Value: value, Enqueued: time.Now()})
	q.mu.Unlock()
	q.ready.Broadcast()
	return true
}

func (q *Queue[T]) Depth() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.jobs)
}

func (q *Queue[T]) OldestAge() time.Duration {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.jobs) == 0 {
		return 0
	}
	return time.Since(q.jobs[0].Enqueued)
}

func (q *Queue[T]) PausedWorkers() int {
	return (q.workers*q.pausedPct() + 99) / 100
}

func (q *Queue[T]) work(ctx context.Context, index int) {
	for {
		q.mu.Lock()
		for len(q.jobs) == 0 && !q.closed {
			q.ready.Wait()
		}
		if q.closed {
			q.mu.Unlock()
			return
		}
		if index < q.PausedWorkers() {
			q.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-time.After(pausePoll):
			}
			continue
		}
		job := q.jobs[0]
		q.jobs[0] = Job[T]{}
		q.jobs = q.jobs[1:]
		q.mu.Unlock()
		q.process(job)
	}
}
