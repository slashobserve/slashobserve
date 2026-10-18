package events

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	TypeResync = "resync"

	subscriptionBuffer = 32
	minBackoff         = time.Second
	maxBackoff         = 30 * time.Second
)

var ErrClosed = errors.New("event broker is closed")

type Listener func(ctx context.Context, channel string, onListening func(), handle func(payload string)) error

type Broker struct {
	listen     Listener
	logger     *slog.Logger
	minBackoff time.Duration
	maxBackoff time.Duration

	mu     sync.Mutex
	closed bool
	done   chan struct{}
	byID   map[string]map[*Subscription]struct{}
}

func NewBroker(listen Listener, logger *slog.Logger) *Broker {
	return &Broker{
		listen:     listen,
		logger:     logger,
		minBackoff: minBackoff,
		maxBackoff: maxBackoff,
		done:       make(chan struct{}),
		byID:       map[string]map[*Subscription]struct{}{},
	}
}

func (b *Broker) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-b.done:
			cancel()
		case <-ctx.Done():
		}
	}()

	backoff := b.minBackoff
	for ctx.Err() == nil {
		established := false
		onListening := func() {
			established = true
			b.listening()
		}
		err := b.listen(ctx, Channel, onListening, b.dispatch)
		if ctx.Err() != nil {
			return
		}
		b.logger.Warn("event listener failed", logattrs.Failure("event_listener_failed", err, slog.Duration("retry_in", backoff))...)
		if established {
			backoff = b.minBackoff
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, b.maxBackoff)
	}
}

func (b *Broker) Subscribe(dashboardID string) (*Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrClosed
	}
	s := &Subscription{
		broker:      b,
		dashboardID: dashboardID,
		events:      make(chan Event, subscriptionBuffer),
		done:        make(chan struct{}),
	}
	subs := b.byID[dashboardID]
	if subs == nil {
		subs = map[*Subscription]struct{}{}
		b.byID[dashboardID] = subs
	}
	subs[s] = struct{}{}
	return s, nil
}

func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	close(b.done)
	for id, subs := range b.byID {
		for s := range subs {
			s.finish()
		}
		delete(b.byID, id)
	}
}

func (b *Broker) listening() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, subs := range b.byID {
		for s := range subs {
			b.deliverLocked(s, Event{Type: TypeResync, DashboardID: id})
		}
	}
}

func (b *Broker) dispatch(payload string) {
	var ev Event
	if err := json.Unmarshal([]byte(payload), &ev); err != nil || ev.Type == "" || ev.DashboardID == "" {
		b.logger.Warn("skipping malformed event payload", slog.String("reason", malformedReason(err)))
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.byID[ev.DashboardID] {
		b.deliverLocked(s, ev)
	}
}

func malformedReason(err error) string {
	if err != nil {
		return "not an event object"
	}
	return "missing type or dashboard_id"
}

func (b *Broker) deliverLocked(s *Subscription, ev Event) {
	select {
	case s.events <- ev:
	default:
		b.logger.Debug("dropping slow event subscriber", slog.String("dashboard_id", s.dashboardID))
		b.removeLocked(s)
		s.finish()
	}
}

func (b *Broker) removeLocked(s *Subscription) {
	subs := b.byID[s.dashboardID]
	delete(subs, s)
	if len(subs) == 0 {
		delete(b.byID, s.dashboardID)
	}
}

type Subscription struct {
	broker      *Broker
	dashboardID string
	events      chan Event
	done        chan struct{}
	finishOnce  sync.Once
}

func (s *Subscription) Events() <-chan Event { return s.events }

func (s *Subscription) Done() <-chan struct{} { return s.done }

func (s *Subscription) Close() {
	s.broker.mu.Lock()
	s.broker.removeLocked(s)
	s.broker.mu.Unlock()
	s.finish()
}

func (s *Subscription) finish() {
	s.finishOnce.Do(func() { close(s.done) })
}
