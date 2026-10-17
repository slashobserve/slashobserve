package datasource

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	healthTTL     = 10 * time.Second
	healthTimeout = 5 * time.Second
)

var uidPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$`)

type Instance struct {
	UID        string
	Name       string
	Datasource Datasource
}

type Status struct {
	UID       string    `json:"uid"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	Healthy   bool      `json:"healthy"`
	Message   string    `json:"message,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
}

type Registry struct {
	logger    *slog.Logger
	instances map[string]Instance
	group     singleflight.Group

	mu     sync.Mutex
	cached map[string]Status
}

func NewRegistry(logger *slog.Logger, instances ...Instance) (*Registry, error) {
	if logger == nil {
		logger = slog.Default()
	}
	r := &Registry{
		logger:    logger,
		instances: make(map[string]Instance, len(instances)),
		cached:    make(map[string]Status, len(instances)),
	}
	for _, in := range instances {
		switch {
		case in.UID == "":
			return nil, errors.New("datasource uid is required")
		case !uidPattern.MatchString(in.UID):
			return nil, fmt.Errorf("datasource uid %q must match %s", in.UID, uidPattern)
		case in.Datasource == nil:
			return nil, fmt.Errorf("datasource %q has no implementation", in.UID)
		}
		if _, dup := r.instances[in.UID]; dup {
			return nil, fmt.Errorf("duplicate datasource uid %q", in.UID)
		}
		if strings.TrimSpace(in.Name) == "" {
			in.Name = in.UID
		}
		r.instances[in.UID] = in
	}
	return r, nil
}

func (r *Registry) Get(uid string) (Instance, bool) {
	in, ok := r.instances[uid]
	return in, ok
}

func (r *Registry) List() []Instance {
	out := make([]Instance, 0, len(r.instances))
	for _, in := range r.instances {
		out = append(out, in)
	}
	slices.SortFunc(out, func(a, b Instance) int { return strings.Compare(a.UID, b.UID) })
	return out
}

func (r *Registry) Health(ctx context.Context) []Status {
	list := r.List()
	out := make([]Status, len(list))
	var wg sync.WaitGroup
	for i, in := range list {
		wg.Go(func() {
			out[i] = r.status(ctx, in)
		})
	}
	wg.Wait()
	return out
}

func (r *Registry) Close() error {
	var errs []error
	for _, in := range r.List() {
		if err := in.Datasource.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close datasource %q: %w", in.UID, err))
		}
	}
	return errors.Join(errs...)
}

func (r *Registry) status(ctx context.Context, in Instance) Status {
	if s, ok := r.fresh(in.UID); ok {
		return s
	}
	v, _, _ := r.group.Do(in.UID, func() (any, error) {
		if s, ok := r.fresh(in.UID); ok {
			return s, nil
		}
		return r.check(ctx, in), nil
	})
	return v.(Status)
}

func (r *Registry) fresh(uid string) (Status, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.cached[uid]
	if !ok || time.Since(s.CheckedAt) >= healthTTL {
		return Status{}, false
	}
	return s, true
}

func (r *Registry) check(ctx context.Context, in Instance) Status {
	checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), healthTimeout)
	defer cancel()
	err := in.Datasource.CheckHealth(checkCtx)

	s := Status{
		UID:       in.UID,
		Type:      in.Datasource.Type(),
		Name:      in.Name,
		Healthy:   err == nil,
		CheckedAt: time.Now().UTC(),
	}
	if err != nil {
		s.Message = err.Error()
	}

	r.mu.Lock()
	prev, seen := r.cached[in.UID]
	r.cached[in.UID] = s
	r.mu.Unlock()

	if err != nil && (!seen || prev.Healthy) {
		r.logger.Warn("datasource unhealthy", "uid", in.UID, "type", s.Type)
	}
	return s
}
