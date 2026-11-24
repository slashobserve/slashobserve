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
	UID       string    `json:"uid"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	Source    string    `json:"source"`
	Index     string    `json:"index,omitempty"`
	TimeField string    `json:"time_field,omitempty"`
	Healthy   bool      `json:"healthy"`
	Message   string    `json:"message,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

func statusFor(in Instance) Status {
	s := Status{UID: in.UID, Type: in.Datasource.Type(), Name: in.Name, Source: in.Source}
	if indexed, ok := in.Datasource.(IndexSource); ok {
		s.Index, s.TimeField = indexed.Index()
	}
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
}

type retired struct {
	datasource Datasource
	timer      *time.Timer
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
		retired:   map[*retired]struct{}{},
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
		return Instance{}, access.refusal(in.Audience)
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
			out[i] = r.status(ctx, in, healthTTL)
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
		s.Message = healthMessage(ctx.Err())
		s.CheckedAt = time.Now().UTC()
		return s
	}
}

func (r *Registry) fresh(uid string, maxAge time.Duration) (Status, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.cached[uid]
	if !ok || time.Since(e.at) >= maxAge {
		return Status{}, false
	}
	return e.status, true
}

func (r *Registry) check(ctx context.Context, in Instance) Status {
	checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), healthTimeout)
	defer cancel()
	err := r.safeCheckHealth(checkCtx, in)

	s := statusFor(in)
	s.Healthy = err == nil
	s.CheckedAt = time.Now().UTC()
	if err != nil {
		s.Message = healthMessage(err)
		s.Detail = healthDetail(err)
	}

	r.mu.Lock()
	prev, seen := r.cached[in.UID]
	if r.current(in) {
		r.cached[in.UID] = cacheEntry{status: s, at: time.Now()}
	}
	r.mu.Unlock()

	if err != nil && (!seen || prev.status.Healthy) {
		r.logger.Warn("datasource unhealthy", logattrs.Failure("datasource_unhealthy", err, slog.String("uid", in.UID), slog.String("type", s.Type))...)
	}
	return s
}

// Status messages reach browsers and the agent, so an error's own text, which can echo an address or a user, never does.
var healthTexts = []struct {
	kind error
	text string
}{
	{ErrTimeout, "Timed out waiting for the datasource"},
	{context.DeadlineExceeded, "Timed out waiting for the datasource"},
	{ErrAuthFailed, "The datasource refused the credentials"},
	{ErrHostNotAllowed, "The ClickHouse user may not connect from this server's address"},
	{ErrNoSecretKey, "This datasource's credentials need SLASHOBSERVE_SECRET_KEY on the server"},
	{ErrUnreadable, "The saved credentials cannot be read with the server's secret key; set them again"},
	{ErrNotLoadable, "The saved definition cannot be used; open the datasource and save it again"},
	{ErrOutboundBlocked, "The address is not allowed by the outbound allowlist"},
	{ErrUnavailable, "The datasource is not answering"},
	{ErrNotReadOnly, "The ClickHouse user must be readonly = 2"},
	{ErrUnsafeGrant, "The ClickHouse user holds a grant that reaches outside systems; revoke it"},
	{ErrForbiddenQuery, "The datasource refused the health check"},
	{ErrLimitExceeded, "The health check hit a datasource limit"},
	{ErrIsOpenSearch, "The server is OpenSearch, configure it as type opensearch"},
	{ErrIsElasticsearch, "The server is Elasticsearch, configure it as type elasticsearch"},
	{ErrUnsupportedVersion, "The datasource version is not supported: Elasticsearch 8.x or 9.x, OpenSearch 2.x or 3.x"},
	{ErrNoIndex, "The index pattern matches no index"},
	{ErrNoTimeField, "The time field is missing or is not a date"},
	{ErrNoRoleField, "A configured field is missing from the mapping"},
	{ErrInvalidQuery, "The datasource rejected the health check"},
	{context.Canceled, "The health check was cancelled"},
}

func Reason(err error) (message, detail string) {
	return healthMessage(err), healthDetail(err)
}

func healthMessage(err error) string {
	for _, h := range healthTexts {
		if errors.Is(err, h.kind) {
			return h.text
		}
	}
	return "The health check failed"
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
