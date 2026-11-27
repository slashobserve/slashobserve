package datasource

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
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
	healthTTL       = 10 * time.Second
	recheckInterval = 2 * time.Second
	healthTimeout   = 5 * time.Second
	closeGrace      = 30 * time.Second
)

var ErrServerOwned = errors.New("a datasource from the datasources file or built into the server already has this uid")

const UIDPattern = `^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$`

var uidPattern = regexp.MustCompile(UIDPattern)

func ValidUID(uid string) bool { return uidPattern.MatchString(uid) }

const (
	SourceFile    = "file"
	SourceApp     = "app"
	SourceBuiltin = "builtin"
)

type Instance struct {
	UID         string
	Name        string
	GrafanaUIDs []string
	Datasource  Datasource
	Source      string
	Audience    Audience

	generation uint64
}

type Status struct {
	UID         string    `json:"uid"`
	Type        string    `json:"type"`
	Name        string    `json:"name"`
	Source      string    `json:"source"`
	Index       string    `json:"index,omitempty"`
	TimeField   string    `json:"time_field,omitempty"`
	Spans       string    `json:"spans,omitempty"`
	TraceSearch bool      `json:"trace_search,omitempty"`
	Healthy     bool      `json:"healthy"`
	Message     string    `json:"message,omitempty"`
	Detail      string    `json:"detail,omitempty"`
	CheckedAt   time.Time `json:"checked_at"`

	HealthState HealthState `json:"health_state"`
	FailureKind FailureKind `json:"failure_kind,omitempty"`
}

func statusFor(in Instance) Status {
	s := Status{UID: in.UID, Type: in.Datasource.Type(), Name: in.Name, Source: in.Source, HealthState: HealthUnchecked}
	if indexed, ok := in.Datasource.(IndexSource); ok {
		s.Index, s.TimeField = indexed.Index()
	}
	spans, isSpanIndex := in.Datasource.(SpanIndexSource)
	if isSpanIndex {
		s.Spans = spans.SpanFormat()
	}
	_, reads := in.Datasource.(TraceSource)
	s.TraceSearch = reads && !isSpanIndex
	return s
}

type Registry struct {
	logger *slog.Logger
	group  singleflight.Group

	instancesMu sync.RWMutex
	instances   map[string]Instance
	generations uint64
	retired     map[*retired]struct{}

	mu     sync.Mutex
	cached map[string]cacheEntry
	now    func() time.Time
}

type retired struct {
	datasource Datasource
	timer      *time.Timer
}

type cacheEntry struct {
	status   Status
	at       time.Time
	probeErr error
}

func NewRegistry(logger *slog.Logger, instances ...Instance) (*Registry, error) {
	if logger == nil {
		logger = slog.Default()
	}
	r := &Registry{
		logger:    logger,
		instances: make(map[string]Instance, len(instances)),
		cached:    make(map[string]cacheEntry, len(instances)),
		retired:   map[*retired]struct{}{},
		now:       time.Now,
	}
	for _, in := range instances {
		in, err := normalized(in)
		if err != nil {
			return nil, err
		}
		if _, dup := r.instances[in.UID]; dup {
			return nil, fmt.Errorf("duplicate datasource uid %q", in.UID)
		}
		in.generation = r.nextGenerationLocked()
		r.instances[in.UID] = in
	}
	return r, nil
}

func normalized(in Instance) (Instance, error) {
	switch {
	case in.UID == "":
		return Instance{}, errors.New("datasource uid is required")
	case !ValidUID(in.UID):
		return Instance{}, fmt.Errorf("datasource uid %q must match %s", in.UID, UIDPattern)
	case in.Datasource == nil:
		return Instance{}, fmt.Errorf("datasource %q has no implementation", in.UID)
	}
	if strings.TrimSpace(in.Name) == "" {
		in.Name = in.UID
	}
	if in.Source == "" {
		in.Source = SourceFile
	}
	switch in.Audience {
	case "":
		in.Audience = AudienceEveryone
	case AudienceEveryone, AudienceAdmins:
	default:
		return Instance{}, fmt.Errorf("datasource %q audience must be %s or %s", in.UID, AudienceEveryone, AudienceAdmins)
	}
	in.GrafanaUIDs = slices.Clone(in.GrafanaUIDs)
	return in, nil
}

func (r *Registry) AddBuiltIn(in Instance) error {
	in.Audience = AudienceAdmins
	in.Source = SourceBuiltin
	in, err := normalized(in)
	if err != nil {
		return err
	}
	r.instancesMu.Lock()
	defer r.instancesMu.Unlock()
	if _, dup := r.instances[in.UID]; dup {
		return fmt.Errorf("duplicate datasource uid %q", in.UID)
	}
	in.generation = r.nextGenerationLocked()
	r.instances[in.UID] = in
	return nil
}

func (r *Registry) Put(in Instance) error {
	in.Source = SourceApp
	in, err := normalized(in)
	if err != nil {
		return err
	}
	r.instancesMu.Lock()
	old, exists := r.instances[in.UID]
	if exists && old.Source != SourceApp {
		r.instancesMu.Unlock()
		return ErrServerOwned
	}
	in.generation = r.nextGenerationLocked()
	r.instances[in.UID] = in
	r.instancesMu.Unlock()
	r.dropCached(in.UID)
	if exists {
		r.retire(old.Datasource)
	}
	return nil
}

func (r *Registry) nextGenerationLocked() uint64 {
	r.generations++
	return r.generations
}

func (r *Registry) Generation() uint64 {
	r.instancesMu.RLock()
	defer r.instancesMu.RUnlock()
	return r.generations
}

func (r *Registry) current(in Instance) bool {
	r.instancesMu.RLock()
	defer r.instancesMu.RUnlock()
	latest, ok := r.instances[in.UID]
	return ok && latest.generation == in.generation
}

func (r *Registry) Remove(uid string) bool {
	r.instancesMu.Lock()
	old, exists := r.instances[uid]
	if !exists || old.Source != SourceApp {
		r.instancesMu.Unlock()
		return false
	}
	delete(r.instances, uid)
	r.instancesMu.Unlock()
	r.dropCached(uid)
	r.retire(old.Datasource)
	return true
}

func (r *Registry) dropCached(uid string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cached, uid)
}

func (r *Registry) retire(ds Datasource) {
	entry := &retired{datasource: ds}
	r.instancesMu.Lock()
	entry.timer = time.AfterFunc(closeGrace, func() {
		r.instancesMu.Lock()
		delete(r.retired, entry)
		r.instancesMu.Unlock()
		_ = ds.Close()
	})
	r.retired[entry] = struct{}{}
	r.instancesMu.Unlock()
}

func (r *Registry) For(ctx context.Context, uid string) (Instance, error) {
	in, err := r.Describe(ctx, uid)
	if err != nil {
		return Instance{}, err
	}
	NoteRead(ctx, in.Audience)
	return in, nil
}

func (r *Registry) Describe(ctx context.Context, uid string) (Instance, error) {
	access, ok := AccessFrom(ctx)
	if !ok {
		return Instance{}, ErrNoAccess
	}
	r.instancesMu.RLock()
	in, ok := r.instances[uid]
	r.instancesMu.RUnlock()
	if !ok {
		return Instance{}, ErrNotVisible
	}
	if !access.Reads(in.Audience) {
		return Instance{}, access.Refusal(in.Audience)
	}
	in.GrafanaUIDs = slices.Clone(in.GrafanaUIDs)
	return in, nil
}

func (r *Registry) Visible(ctx context.Context) []Instance {
	access, ok := AccessFrom(ctx)
	if !ok {
		return nil
	}
	return slices.DeleteFunc(r.all(), func(in Instance) bool { return !access.Reads(in.Audience) })
}

func (r *Registry) VisibleToEveryone(ctx context.Context) []Instance {
	return slices.DeleteFunc(r.Visible(ctx), func(in Instance) bool { return in.Audience == AudienceAdmins })
}

func (r *Registry) AdminsOnly(uid string) bool {
	r.instancesMu.RLock()
	defer r.instancesMu.RUnlock()
	return r.instances[uid].Audience == AudienceAdmins
}

func (r *Registry) AdminsOnlyUIDs() []string {
	r.instancesMu.RLock()
	defer r.instancesMu.RUnlock()
	var uids []string
	for uid, in := range r.instances {
		if in.Audience == AudienceAdmins {
			uids = append(uids, uid)
		}
	}
	slices.Sort(uids)
	return uids
}

func (r *Registry) all() []Instance {
	r.instancesMu.RLock()
	out := make([]Instance, 0, len(r.instances))
	for _, in := range r.instances {
		in.GrafanaUIDs = slices.Clone(in.GrafanaUIDs)
		out = append(out, in)
	}
	r.instancesMu.RUnlock()
	slices.SortFunc(out, func(a, b Instance) int { return strings.Compare(a.UID, b.UID) })
	return out
}

func (r *Registry) Health(ctx context.Context) []Status {
	list := r.Visible(ctx)
	out := make([]Status, len(list))
	var wg sync.WaitGroup
	for i, in := range list {
		wg.Go(func() {
			out[i] = r.listed(ctx, in)
		})
	}
	wg.Wait()
	return out
}

func (r *Registry) Recheck(ctx context.Context, uid string) (Status, bool) {
	in, err := r.For(ctx, uid)
	if err != nil {
		return Status{}, false
	}
	return r.status(ctx, in, recheckInterval), true
}

func (r *Registry) Close() error {
	var errs []error
	for _, in := range r.all() {
		if err := in.Datasource.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close datasource %q: %w", in.UID, err))
		}
	}
	r.instancesMu.Lock()
	pending := slices.Collect(maps.Keys(r.retired))
	clear(r.retired)
	r.instancesMu.Unlock()
	for _, entry := range pending {
		if entry.timer.Stop() {
			_ = entry.datasource.Close()
		}
	}
	return errors.Join(errs...)
}

func (r *Registry) listed(ctx context.Context, in Instance) Status {
	if e, ok := r.entry(in.UID); ok && !e.status.Healthy {
		if r.now().Sub(e.at) >= recheckInterval {
			r.refreshInBackground(in)
			e.status.HealthState = HealthRechecking
		}
		return e.status
	}
	return r.status(ctx, in, healthTTL)
}

func (r *Registry) refreshInBackground(in Instance) {
	r.group.DoChan(fmt.Sprintf("%s#%d", in.UID, in.generation), func() (any, error) {
		return r.check(context.Background(), in), nil
	})
}

func (r *Registry) entry(uid string) (cacheEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.cached[uid]
	return e, ok
}

func (r *Registry) status(ctx context.Context, in Instance, maxAge time.Duration) Status {
	if s, ok := r.fresh(in.UID, maxAge); ok {
		return s
	}
	ch := r.group.DoChan(fmt.Sprintf("%s#%d", in.UID, in.generation), func() (any, error) {
		if s, ok := r.fresh(in.UID, maxAge); ok {
			return s, nil
		}
		return r.check(ctx, in), nil
	})
	select {
	case res := <-ch:
		return res.Val.(Status)
	case <-ctx.Done():
		s := statusFor(in)
		s.Message = verdictOn(ctx.Err()).text
		s.CheckedAt = r.now().UTC()
		return s
	}
}

func (r *Registry) fresh(uid string, maxAge time.Duration) (Status, bool) {
	e, ok := r.entry(uid)
	if !ok || r.now().Sub(e.at) >= maxAge {
		return Status{}, false
	}
	return e.status, true
}

var transientKinds = []error{ErrUnavailable, ErrTimeout, ErrLimitExceeded}

func transient(err error) bool {
	return slices.ContainsFunc(transientKinds, func(kind error) bool { return errors.Is(err, kind) })
}

func (r *Registry) Answered(uid string) {
	r.instancesMu.RLock()
	in, ok := r.instances[uid]
	r.instancesMu.RUnlock()
	if !ok {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	e, cached := r.cached[uid]
	if !cached || e.status.Healthy || !transient(e.probeErr) || !r.current(in) {
		return
	}
	now := r.now()
	r.cached[uid] = cacheEntry{status: healthyStatus(in, now), at: now, probeErr: e.probeErr}
}

func healthyStatus(in Instance, at time.Time) Status {
	s := statusFor(in)
	s.Healthy = true
	s.HealthState = HealthHealthy
	s.CheckedAt = at.UTC()
	return s
}

func checked(in Instance, err error, at time.Time) (s Status, conclusive bool) {
	if err == nil {
		return healthyStatus(in, at), true
	}
	verdict := verdictOn(err)
	s = statusFor(in)
	s.CheckedAt = at.UTC()
	s.Message = verdict.text
	s.Detail = healthDetail(err)
	if !verdict.blamesTheDatasource() {
		return s, false
	}
	s.HealthState = HealthFailing
	s.FailureKind = verdict.failure
	return s, true
}

func (r *Registry) check(ctx context.Context, in Instance) Status {
	started := r.now()
	checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), healthTimeout)
	defer cancel()
	err := r.safeCheckHealth(checkCtx, in)

	s, conclusive := checked(in, err, r.now())

	r.mu.Lock()
	prev, seen := r.cached[in.UID]
	answeredMeanwhile := err != nil && seen && prev.status.Healthy && prev.at.After(started)
	if r.current(in) && conclusive && !answeredMeanwhile {
		r.cached[in.UID] = cacheEntry{status: s, at: r.now(), probeErr: err}
	}
	r.mu.Unlock()

	if answeredMeanwhile {
		return prev.status
	}
	if err != nil && conclusive && (!seen || prev.probeErr == nil) {
		r.logger.Warn("datasource unhealthy", logattrs.Failure("datasource_unhealthy", err, slog.String("uid", in.UID), slog.String("type", s.Type))...)
	}
	return s
}

// Status messages reach browsers and the agent, so an error's own text, which can echo an address or a user, never does.
var healthTexts = []struct {
	kind    error
	text    string
	failure FailureKind
}{
	{ErrTimeout, "Timed out waiting for the datasource", FailureTransient},
	{context.DeadlineExceeded, "Timed out waiting for the datasource", FailureTransient},
	{ErrAuthFailed, "The datasource refused the credentials", FailureCredentials},
	{ErrHostNotAllowed, "The ClickHouse user may not connect from this server's address", FailureConfiguration},
	{ErrNoSecretKey, "This datasource's credentials need SLASHOBSERVE_SECRET_KEY on the server", FailureConfiguration},
	{ErrUnreadable, "The saved credentials cannot be read with the server's secret key; set them again", FailureCredentials},
	{ErrNotLoadable, "The saved definition cannot be used; open the datasource and save it again", FailureConfiguration},
	{ErrOutboundBlocked, "The address is not allowed by the outbound allowlist", FailureConfiguration},
	{ErrUnavailable, "The datasource is not answering", FailureTransient},
	{ErrNotReadOnly, "The ClickHouse user must be readonly = 2", FailureConfiguration},
	{ErrUnsafeGrant, "The ClickHouse user holds a grant that reaches outside systems; revoke it", FailureConfiguration},
	{ErrForbiddenQuery, "The datasource refused the health check", FailureConfiguration},
	{ErrLimitExceeded, "The health check hit a datasource limit", FailureTransient},
	{ErrIsOpenSearch, "The server is OpenSearch, configure it as type opensearch", FailureConfiguration},
	{ErrIsElasticsearch, "The server is Elasticsearch, configure it as type elasticsearch", FailureConfiguration},
	{ErrUnsupportedVersion, "The datasource version is not supported: Elasticsearch 8.x or 9.x, OpenSearch 2.x or 3.x", FailureUnsupported},
	{ErrNoIndex, "The index pattern matches no index", FailureConfiguration},
	{ErrNoTimeField, "The time field is missing or is not a date", FailureConfiguration},
	{ErrNoRoleField, "A configured field is missing from the mapping", FailureConfiguration},
	{ErrInvalidQuery, "The datasource rejected the health check", FailureConfiguration},
	{context.Canceled, "The health check was cancelled", notTheDatasource},
}

type verdict struct {
	text    string
	failure FailureKind
}

func (v verdict) blamesTheDatasource() bool { return v.failure != notTheDatasource }

func verdictOn(err error) verdict {
	for _, h := range healthTexts {
		if errors.Is(err, h.kind) {
			return verdict{text: h.text, failure: h.failure}
		}
	}
	return verdict{text: "The health check failed", failure: FailureTransient}
}

func Reason(err error) (message, detail string, kind FailureKind) {
	v := verdictOn(err)
	return v.text, healthDetail(err), v.failure
}

const connectionFailedMessage = "connection failed"

func healthDetail(err error) string {
	var upstream *UpstreamError
	switch {
	case !errors.As(err, &upstream):
		return ""
	case upstream.Status > 0:
		return fmt.Sprintf("The datasource answered HTTP %d", upstream.Status)
	case upstream.Message == connectionFailedMessage:
		return "The server could not connect to the datasource's address"
	}
	return ""
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
