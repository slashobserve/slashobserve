package datasource

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	healthTTL     = 10 * time.Second
	healthTimeout = 5 * time.Second
)

const UIDPattern = `^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$`

var uidPattern = regexp.MustCompile(UIDPattern)

func ValidUID(uid string) bool { return uidPattern.MatchString(uid) }

type Instance struct {
	UID         string
	Name        string
	GrafanaUIDs []string
	Datasource  Datasource
}

type Status struct {
	UID       string    `json:"uid"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	Healthy   bool      `json:"healthy"`
	Message   string    `json:"message,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

type Registry struct {
	logger    *slog.Logger
	instances map[string]Instance
	group     singleflight.Group

	mu     sync.Mutex
	cached map[string]cacheEntry
}

type cacheEntry struct {
	status Status
	at     time.Time
}

func NewRegistry(logger *slog.Logger, instances ...Instance) (*Registry, error) {
	if logger == nil {
		logger = slog.Default()
	}
	r := &Registry{
		logger:    logger,
		instances: make(map[string]Instance, len(instances)),
		cached:    make(map[string]cacheEntry, len(instances)),
	}
	for _, in := range instances {
		switch {
		case in.UID == "":
			return nil, errors.New("datasource uid is required")
		case !ValidUID(in.UID):
			return nil, fmt.Errorf("datasource uid %q must match %s", in.UID, UIDPattern)
		case in.Datasource == nil:
			return nil, fmt.Errorf("datasource %q has no implementation", in.UID)
		}
		if _, dup := r.instances[in.UID]; dup {
			return nil, fmt.Errorf("duplicate datasource uid %q", in.UID)
		}
		if strings.TrimSpace(in.Name) == "" {
			in.Name = in.UID
		}
		in.GrafanaUIDs = slices.Clone(in.GrafanaUIDs)
		r.instances[in.UID] = in
	}
	return r, nil
}

func (r *Registry) Get(uid string) (Instance, bool) {
	in, ok := r.instances[uid]
	in.GrafanaUIDs = slices.Clone(in.GrafanaUIDs)
	return in, ok
}

func (r *Registry) List() []Instance {
	out := make([]Instance, 0, len(r.instances))
	for _, in := range r.instances {
		in.GrafanaUIDs = slices.Clone(in.GrafanaUIDs)
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
	ch := r.group.DoChan(in.UID, func() (any, error) {
		if s, ok := r.fresh(in.UID); ok {
			return s, nil
		}
		return r.check(ctx, in), nil
	})
	select {
	case res := <-ch:
		return res.Val.(Status)
	case <-ctx.Done():
		return Status{
			UID:       in.UID,
			Type:      in.Datasource.Type(),
			Name:      in.Name,
			Message:   healthMessage(ctx.Err()),
			CheckedAt: time.Now().UTC(),
		}
	}
}

func (r *Registry) fresh(uid string) (Status, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.cached[uid]
	if !ok || time.Since(e.at) >= healthTTL {
		return Status{}, false
	}
	return e.status, true
}

func (r *Registry) check(ctx context.Context, in Instance) Status {
	checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), healthTimeout)
	defer cancel()
	err := r.safeCheckHealth(checkCtx, in)

	s := Status{
		UID:       in.UID,
		Type:      in.Datasource.Type(),
		Name:      in.Name,
		Healthy:   err == nil,
		CheckedAt: time.Now().UTC(),
	}
	if err != nil {
		s.Message = healthMessage(err)
	}

	r.mu.Lock()
	prev, seen := r.cached[in.UID]
	r.cached[in.UID] = cacheEntry{status: s, at: time.Now()}
	r.mu.Unlock()

	if err != nil && (!seen || prev.status.Healthy) {
		r.logger.Warn("datasource unhealthy", logattrs.Failure("datasource_unhealthy", err, slog.String("uid", in.UID), slog.String("type", s.Type))...)
	}
	return s
}

// Status messages reach browsers and the agent, so an error's own text, which can echo an address or a user, never does.
func healthMessage(err error) string {
	switch {
	case errors.Is(err, ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return "Timed out waiting for the datasource"
	case errors.Is(err, ErrAuthFailed):
		return "The datasource refused the credentials"
	case errors.Is(err, ErrHostNotAllowed):
		return "The ClickHouse user may not connect from this server's address"
	case errors.Is(err, ErrUnavailable):
		return "The datasource is not answering"
	case errors.Is(err, ErrNotReadOnly):
		return "The ClickHouse user must be readonly = 2"
	case errors.Is(err, ErrUnsafeGrant):
		return "The ClickHouse user holds a grant that reaches outside systems; revoke it"
	case errors.Is(err, ErrForbiddenQuery):
		return "The datasource refused the health check"
	case errors.Is(err, ErrLimitExceeded):
		return "The health check hit a datasource limit"
	case errors.Is(err, ErrInvalidQuery):
		return "The datasource rejected the health check"
	case errors.Is(err, context.Canceled):
		return "The health check was cancelled"
	}
	return "The health check failed"
}

func (r *Registry) safeCheckHealth(ctx context.Context, in Instance) (err error) {
	defer func() {
		if p := recover(); p != nil {
			r.logger.ErrorContext(ctx, "datasource health check panicked",
				slog.String("uid", in.UID), slog.String("panic_type", fmt.Sprintf("%T", p)), slog.String("stack", string(debug.Stack())))
			err = errors.New("health check panicked")
		}
	}()
	return in.Datasource.CheckHealth(ctx)
}
