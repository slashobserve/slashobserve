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

	followers map[*Follower]struct{}
}

func NewBroker(listen Listener, logger *slog.Logger) *Broker {
	return &Broker{
		listen:     listen,
		logger:     logger,
		minBackoff: minBackoff,
		maxBackoff: maxBackoff,
		done:       make(chan struct{}),
		byID:       map[string]map[*Subscription]struct{}{},
		followers:  map[*Follower]struct{}{},
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

func (b *Broker) Follow() (*Follower, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrClosed
	}
	f := &Follower{
		broker: b,
		wakes:  make(chan struct{}, 1),
		done:   make(chan struct{}),
	}
	b.followers[f] = struct{}{}
	return f, nil
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
	for f := range b.followers {
		f.finish()
		delete(b.followers, f)
	}
}

func (b *Broker) listening() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.wakeFollowersLocked()
	for id, subs := range b.byID {
		for s := range subs {
			b.deliverLocked(s, Event{Type: TypeResync, DashboardID: id})
		}
	}
}

func (b *Broker) dispatch(payload string) {
	var n notice
	err := json.Unmarshal([]byte(payload), &n)
	live := n.Event != nil && n.Type != "" && n.DashboardID != ""
	if err != nil || (n.Seq <= 0 && !live) {
		b.logger.Warn("skipping malformed event payload", slog.String("reason", malformedReason(err)))
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if n.Seq > 0 {
		b.wakeFollowersLocked()
	}
	if !live {
		return
	}
	for s := range b.byID[n.DashboardID] {
		b.deliverLocked(s, *n.Event)
	}
}

func (b *Broker) wakeFollowersLocked() {
	for f := range b.followers {
		select {
		case f.wakes <- struct{}{}:
		default:
		}
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

type Follower struct {
	broker     *Broker
	wakes      chan struct{}
	done       chan struct{}
	finishOnce sync.Once
}

func (f *Follower) Wakes() <-chan struct{} { return f.wakes }

func (f *Follower) Done() <-chan struct{} { return f.done }

func (f *Follower) Close() {
	f.broker.mu.Lock()
	delete(f.broker.followers, f)
	f.broker.mu.Unlock()
	f.finish()
}

func (f *Follower) finish() {
	f.finishOnce.Do(func() { close(f.done) })
}
