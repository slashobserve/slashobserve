package queryproxy

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/ttlcache"
)

const (
	DefaultServicesWindow = time.Hour
	MinServicesWindow     = time.Minute
	MaxServicesWindow     = 6 * time.Hour
	MaxServices           = 200

	servicesTTL       = time.Minute
	servicesTimeout   = 5 * time.Second
	servicesMaxCost   = 4 << 20
	servicesEntryCost = 64
)

type ServicesRequest struct {
	Datasource string
	Signal     string
	Window     time.Duration
}

type Services struct {
	Datasource string
	Signal     string
	Field      string
	Window     time.Duration
	Services   []string
	Truncated  bool
}

type servicesKey struct {
	datasource, signal string
	window             time.Duration
	asOf               int64
}

type servicesCache struct {
	entries *ttlcache.Cache[servicesKey, Services]
	flights *ttlcache.Group[Services]
}

func newServicesCache(logger *slog.Logger, timeout time.Duration) *servicesCache {
	return &servicesCache{
		entries: ttlcache.New[servicesKey, Services](servicesTTL, servicesMaxCost),
		flights: ttlcache.NewGroup[Services](timeout, logger),
	}
}

func WithServicesTimeout(d time.Duration) RunnerOption {
	return func(r *Runner) { r.services = newServicesCache(r.logger, d) }
}

func (req ServicesRequest) window() (time.Duration, error) {
	switch {
	case req.Window == 0:
		return DefaultServicesWindow, nil
	case req.Window < MinServicesWindow || req.Window > MaxServicesWindow:
		return 0, invalidNamesf("window must be between %s and %s", MinServicesWindow, MaxServicesWindow)
	}
	return req.Window, nil
}

func (r *Runner) Services(ctx context.Context, req ServicesRequest) (Services, error) {
	window, err := req.window()
	if err != nil {
		return Services{}, err
	}
	inst, err := r.instance(ctx, req.Datasource)
	if err != nil {
		return Services{}, err
	}
	source, ok := inst.Datasource.(datasource.ServiceSource)
	if !ok {
		return Services{}, invalidNamesf("services are read on Loki, Tempo, Jaeger, ClickHouse, Elasticsearch and OpenSearch datasources; %s is a %s datasource", inst.UID, inst.Datasource.Type())
	}
	now := datasource.Now(ctx)
	key := servicesKey{datasource: inst.UID, signal: req.Signal, window: window, asOf: pinnedUnixNano(ctx)}
	if cached, _, hit := r.services.entries.Get(key); hit {
		return cached, nil
	}
	return r.services.flights.Do(ctx, fmt.Sprint(key), func(ctx context.Context) (Services, error) {
		if cached, _, hit := r.services.entries.Get(key); hit {
			return cached, nil
		}
		out, err := r.readServices(ctx, inst.UID, source, req.Signal, window, now)
		if err != nil {
			return Services{}, err
		}
		r.services.entries.Put(key, out, servicesEntryCost+int64(len(out.Services))*servicesEntryCost)
		return out, nil
	})
}

func (r *Runner) readServices(ctx context.Context, uid string, source datasource.ServiceSource, signal string, window time.Duration, now time.Time) (Services, error) {
	release, err := r.acquireUpstreamBounded(ctx, uid)
	if err != nil {
		return Services{}, err
	}
	defer release()
	list, err := source.Services(ctx, datasource.ServiceRequest{
		Signal: signal,
		Range:  datasource.TimeRange{From: now.Add(-window), To: now},
		Limit:  MaxServices + 1,
	})
	if err != nil {
		return Services{}, err
	}
	values, truncated := truncate(append([]string{}, list.Values...), MaxServices, false)
	return Services{Datasource: uid, Signal: list.Signal, Field: list.Field, Window: window, Services: values, Truncated: truncated}, nil
}

func pinnedUnixNano(ctx context.Context) int64 {
	if at, ok := datasource.PinnedNow(ctx); ok {
		return at.UnixNano()
	}
	return 0
}
