package datasourceadmin

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/provision"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/egress/allowlist"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	MaxAppDatasources = 100
	testTimeout       = 10 * time.Second
	uniqueViolation   = "23505"
	maxHeaders        = 20
	maxSecretBytes    = 4096
	maxSecretsBytes   = 60000
)

var (
	ErrNotFound        = errors.New("datasource not found")
	ErrReadOnly        = errors.New("this datasource is defined in the datasources file on the server and cannot be changed here")
	ErrBuiltIn         = errors.New("this datasource is built into the server and cannot be changed or deleted")
	ErrUIDTaken        = errors.New("a datasource with this uid already exists")
	ErrNoSecretKey     = errors.New("SLASHOBSERVE_SECRET_KEY must be set to store credentials")
	ErrTooMany         = errors.New("too many datasources were added in the app")
	ErrAddressRefused  = errors.New("the outbound allowlist is set by configuration and cannot list an address like this; use a domain name that is on the list, or define the datasource in the datasources file")
	ErrAllowlistFull   = errors.New("the outbound allowlist is full")
	ErrIdentityChanged = errors.New("uid and type cannot be changed; add a new datasource instead")
	ErrSecretRequired  = errors.New("changing the address or username needs every stored credential sent again, or cleared; stored credentials are not sent to a new address")
	ErrChanged         = errors.New("the datasource changed while this change was being prepared; read it again and retry")
)

type InvalidError struct {
	Fields []config.FieldError
}

func (e *InvalidError) Error() string {
	parts := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		parts[i] = f.Error()
	}
	return strings.Join(parts, "; ")
}

func invalid(field, message string) error {
	return &InvalidError{Fields: []config.FieldError{{Field: field, Message: message}}}
}

type User struct {
	Kind  string
	ID    string
	Title string
}

type InUseError struct {
	UID    string
	Users  []User
	Hidden int
}

func (e *InUseError) Error() string {
	n := len(e.Users) + e.Hidden
	return fmt.Sprintf("datasource %q is used by %d %s", e.UID, n, folders.Agree(n, "dashboard or saved search", "dashboards and saved searches"))
}

type Record struct {
	Definition Definition
	SecretsSet []string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type SecretsChange struct {
	Set   Secrets
	Clear []string
}

func (c SecretsChange) empty() bool { return c.Set.Empty() && len(c.Clear) == 0 }

func (c SecretsChange) apply(base Secrets) (Secrets, error) {
	out := Secrets{Password: base.Password, BearerToken: base.BearerToken, APIKey: base.APIKey}
	for name, value := range base.Headers {
		if out.Headers == nil {
			out.Headers = map[string]string{}
		}
		out.Headers[name] = value
	}
	for _, name := range c.Clear {
		switch {
		case name == secretPassword:
			out.Password = ""
		case name == secretBearerToken:
			out.BearerToken = ""
		case name == secretAPIKey:
			out.APIKey = ""
		case strings.HasPrefix(name, secretHeaderPrefix):
			delete(out.Headers, strings.TrimPrefix(name, secretHeaderPrefix))
		default:
			return Secrets{}, invalid("clear_secrets", fmt.Sprintf("%q is not a credential name: use password, bearer_token, api_key or header:<name>", name))
		}
	}
	out.Password = cmp.Or(c.Set.Password, out.Password)
	out.BearerToken = cmp.Or(c.Set.BearerToken, out.BearerToken)
	out.APIKey = cmp.Or(c.Set.APIKey, out.APIKey)
	for name, value := range c.Set.Headers {
		if out.Headers == nil {
			out.Headers = map[string]string{}
		}
		out.Headers[name] = value
	}
	if len(out.Headers) == 0 {
		out.Headers = nil
	}
	return out, checkSecrets(out)
}

func checkSecrets(s Secrets) error {
	if len(s.Headers) > maxHeaders {
		return invalid("secrets.headers", fmt.Sprintf("at most %d headers", maxHeaders))
	}
	for field, value := range map[string]string{secretPassword: s.Password, secretBearerToken: s.BearerToken, secretAPIKey: s.APIKey} {
		if len(value) > maxSecretBytes || strings.ContainsFunc(value, unicode.IsControl) {
			return invalid("secrets."+field, fmt.Sprintf("must be at most %d bytes without control characters", maxSecretBytes))
		}
	}
	for name, value := range s.Headers {
		if name == "" || len(value) > maxSecretBytes || strings.ContainsFunc(value, unicode.IsControl) {
			return invalid("secrets.headers", fmt.Sprintf("header names must not be empty and values must be at most %d bytes without control characters", maxSecretBytes))
		}
	}
	if encoded, err := json.Marshal(s); err != nil || len(encoded) > maxSecretsBytes { //nolint:gosec // only its length is read
		return invalid("secrets.headers", fmt.Sprintf("all credentials together must be at most %d bytes", maxSecretsBytes))
	}
	return nil
}

func recordOf(row dbsqlc.Datasource) (Record, error) {
	var def Definition
	if err := json.Unmarshal(row.Config, &def); err != nil {
		return Record{}, fmt.Errorf("stored datasource %q: %w", row.Uid, err)
	}
	def.UID, def.Type, def.Name, def.Audience = row.Uid, row.Type, row.Name, datasource.Audience(row.Audience)
	return Record{Definition: def, SecretsSet: slices.Clone(row.SecretNames), CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}, nil
}

func (s *Service) row(ctx context.Context, uid string) (dbsqlc.Datasource, error) {
	if in, err := s.registry.For(ctx, uid); err == nil {
		switch in.Source {
		case datasource.SourceFile:
			return dbsqlc.Datasource{}, ErrReadOnly
		case datasource.SourceBuiltin:
			return dbsqlc.Datasource{}, ErrBuiltIn
		}
	}
	row, err := s.db.Queries().GetDatasource(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.Datasource{}, ErrNotFound
	}
	return row, err
}

func (s *Service) Get(ctx context.Context, uid string) (Record, error) {
	row, err := s.row(ctx, uid)
	if err != nil {
		return Record{}, err
	}
	return recordOf(row)
}

func (s *Service) currentSecrets(row dbsqlc.Datasource) (Secrets, error) {
	secrets, err := s.secretsOf(row)
	switch {
	case err == nil:
		return secrets, nil
	case errors.Is(err, datasource.ErrUnreadable):
		return Secrets{}, nil
	case errors.Is(err, datasource.ErrNoSecretKey):
		return Secrets{}, ErrNoSecretKey
	}
	return Secrets{}, err
}

func normalizeDefinition(def Definition) Definition {
	def.UID = strings.TrimSpace(def.UID)
	def.Name = strings.TrimSpace(def.Name)
	def.Address = strings.TrimSpace(def.Address)
	if def.Name == "" {
		def.Name = def.UID
	}
	if def.Audience == "" {
		def.Audience = datasource.AudienceEveryone
	}
	return def
}

func checkAudience(audience datasource.Audience) error {
	if audience != datasource.AudienceEveryone && audience != datasource.AudienceAdmins {
		return invalid("audience", "must be everyone or admins")
	}
	return nil
}

func checkName(name string) error {
	if utf8.RuneCountInString(name) > 100 || strings.ContainsFunc(name, unicode.IsControl) {
		return invalid("name", "must be at most 100 characters without control characters")
	}
	return nil
}

type candidate struct {
	def     Definition
	cfg     config.Datasource
	secrets Secrets
	host    endpoint
}

func (s *Service) prepare(ctx context.Context, def Definition, secrets Secrets) (candidate, error) {
	def = normalizeDefinition(def)
	if err := checkName(def.Name); err != nil {
		return candidate{}, err
	}
	if err := checkAudience(def.Audience); err != nil {
		return candidate{}, err
	}
	if err := checkSecrets(secrets); err != nil {
		return candidate{}, err
	}
	cfg, problems := def.Config(secrets)
	problems = append(problems, s.aliasConflicts(ctx, def)...)
	host, addressProblem := endpointOf(def)
	if addressProblem != nil && !slices.ContainsFunc(problems, func(p config.FieldError) bool { return p.Field == "address" }) {
		problems = append(problems, *addressProblem)
	}
	if len(problems) > 0 {
		return candidate{}, &InvalidError{Fields: problems}
	}
	ds, err := provision.Open(cfg)
	if err != nil {
		var configErr *datasource.ConfigError
		if errors.As(err, &configErr) {
			return candidate{}, invalid("definition", configErr.Message)
		}
		return candidate{}, invalid("definition", "the definition could not be used")
	}
	_ = ds.Close()
	return candidate{def: def, cfg: cfg, secrets: secrets, host: host}, nil
}

func (s *Service) aliasConflicts(ctx context.Context, def Definition) []config.FieldError {
	var problems []config.FieldError
	for _, other := range s.registry.Visible(ctx) {
		if other.UID == def.UID {
			continue
		}
		for i, alias := range def.GrafanaUIDs {
			if alias == other.UID || slices.Contains(other.GrafanaUIDs, alias) {
				problems = append(problems, config.FieldError{Field: fmt.Sprintf("grafana_uids[%d]", i), Message: fmt.Sprintf("%q is already used by datasource %q", alias, other.UID)})
			}
		}
	}
	return problems
}

func (s *Service) refusal(list domains.List, host endpoint, mayAdd bool) error {
	if _, isDomain := host.domain(); !isDomain {
		if s.outboundLocked {
			return ErrAddressRefused
		}
		return nil
	}
	if mayAdd || list.Allows(host.host) {
		return nil
	}
	return &egress.HostError{Host: host.host, Err: egress.ErrNotAllowed}
}

func (s *Service) saveOnAllowedHost(ctx context.Context, by auth.Principal, host endpoint, save func(*dbsqlc.Queries) error) error {
	if _, isDomain := host.domain(); !isDomain {
		if s.outboundLocked {
			return ErrAddressRefused
		}
		return s.db.InTx(ctx, save)
	}
	added, err := s.domains.SaveOnAllowedHost(ctx, by, host.host, save)
	switch {
	case err == nil:
		if added != "" {
			s.logger.InfoContext(ctx, "allowed an outbound domain", slog.String("actor_id", by.ID), slog.String("domain", added))
		}
		return nil
	case errors.Is(err, allowlist.ErrLocked):
		return &egress.HostError{Host: host.host, Err: egress.ErrNotAllowed}
	case errors.Is(err, allowlist.ErrTooMany):
		return ErrAllowlistFull
	}
	return err
}

func (s *Service) releaseHost(ctx context.Context, by auth.Principal, host endpoint) {
	if _, isDomain := host.domain(); !isDomain {
		return
	}
	if err := s.domains.Release(context.WithoutCancel(ctx), by, host.host); err != nil {
		s.logger.WarnContext(ctx, "could not release an outbound domain", logattrs.Failure("outbound_release_failed", err, slog.String("host", host.host))...)
	}
}

func (s *Service) mayConnect(ctx context.Context, host endpoint) error {
	list, err := s.domains.Allowed(ctx)
	if err != nil {
		return err
	}
	return s.refusal(list, host, !s.outboundLocked)
}

func newID() pgtype.UUID {
	var id pgtype.UUID
	_, _ = rand.Read(id.Bytes[:])
	id.Bytes[6] = id.Bytes[6]&0x0f | 0x40
	id.Bytes[8] = id.Bytes[8]&0x3f | 0x80
	id.Valid = true
	return id
}

func (s *Service) seal(id pgtype.UUID, secrets Secrets) ([]byte, []string, error) {
	if secrets.Empty() {
		return nil, []string{}, nil
	}
	if !s.key.Set() {
		return nil, nil, ErrNoSecretKey
	}
	ciphertext, err := sealSecrets(s.key, id, secrets)
	if err != nil {
		return nil, nil, err
	}
	return ciphertext, secrets.Names(), nil
}

func (s *Service) Create(ctx context.Context, by auth.Principal, def Definition, secrets Secrets) (Record, error) {
	cand, err := s.prepare(ctx, def, secrets)
	if err != nil {
		return Record{}, err
	}
	if _, err := s.registry.For(ctx, cand.def.UID); err == nil {
		return Record{}, ErrUIDTaken
	}
	id := newID()
	ciphertext, names, err := s.seal(id, secrets)
	if err != nil {
		return Record{}, err
	}
	config, err := json.Marshal(cand.def)
	if err != nil {
		return Record{}, err
	}
	params := dbsqlc.InsertDatasourceParams{ID: id, Uid: cand.def.UID, Type: cand.def.Type, Name: cand.def.Name, Audience: string(cand.def.Audience), Config: config, Secrets: ciphertext, SecretNames: names}
	if creator, err := db.ParseUUID(by.ID); err == nil && by.Kind == auth.KindUser {
		params.CreatedBy = creator
	}
	var row dbsqlc.Datasource
	err = s.saveOnAllowedHost(ctx, by, cand.host, func(q *dbsqlc.Queries) error {
		if err := q.LockDatasourceUID(ctx, cand.def.UID); err != nil {
			return err
		}
		if err := q.LockDatasourceCount(ctx); err != nil {
			return err
		}
		count, err := q.CountDatasources(ctx)
		if err != nil {
			return err
		}
		if count >= MaxAppDatasources {
			return ErrTooMany
		}
		if row, err = q.InsertDatasource(ctx, params); err != nil {
			return err
		}
		if err := refuseSharedHolders(ctx, q, "", cand.def); err != nil {
			return err
		}
		return recordCreated(ctx, q, by, row.ID, cand.def)
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return Record{}, ErrUIDTaken
	}
	if err != nil {
		return Record{}, err
	}
	s.applyRow(ctx, row)
	s.logger.InfoContext(ctx, "datasource added", slog.String("actor_id", by.ID), slog.String("uid", row.Uid), slog.String("type", row.Type))
	return recordOf(row)
}

func (s *Service) applyRow(ctx context.Context, row dbsqlc.Datasource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	allowed, err := s.domains.Allowed(ctx)
	if err != nil {
		s.logger.WarnContext(ctx, "stored datasource waits for the next sync", logattrs.Failure("datasource_apply_failed", err, slog.String("uid", row.Uid))...)
		return
	}
	delete(s.applied, row.Uid)
	s.applyLocked(row, allowed)
}

func (s *Service) compose(row dbsqlc.Datasource, def Definition, change SecretsChange) (Secrets, error) {
	if def.UID != row.Uid || def.Type != row.Type {
		return Secrets{}, ErrIdentityChanged
	}
	current, err := s.currentSecrets(row)
	if err != nil {
		return Secrets{}, err
	}
	secrets := current
	if !change.empty() {
		if secrets, err = change.apply(current); err != nil {
			return Secrets{}, err
		}
	}
	if movesCredentials(row, def) && carriesStored(secrets, change.Set) {
		return Secrets{}, ErrSecretRequired
	}
	return secrets, nil
}

func movesCredentials(row dbsqlc.Datasource, def Definition) bool {
	var stored Definition
	if err := json.Unmarshal(row.Config, &stored); err != nil {
		return true
	}
	return normalizeDefinition(def).Address != stored.Address || def.Username != stored.Username
}

func carriesStored(secrets, sent Secrets) bool {
	if secrets.Password != "" && sent.Password == "" || secrets.BearerToken != "" && sent.BearerToken == "" || secrets.APIKey != "" && sent.APIKey == "" {
		return true
	}
	for name := range secrets.Headers {
		if _, ok := sent.Headers[name]; !ok {
			return true
		}
	}
	return false
}

func (s *Service) Update(ctx context.Context, by auth.Principal, uid string, def Definition, change SecretsChange) (Record, error) {
	row, err := s.row(ctx, uid)
	if err != nil {
		return Record{}, err
	}
	secrets, err := s.compose(row, def, change)
	if err != nil {
		return Record{}, err
	}
	cand, err := s.prepare(ctx, def, secrets)
	if err != nil {
		return Record{}, err
	}
	current, err := recordOf(row)
	if err != nil {
		return Record{}, err
	}
	params := dbsqlc.UpdateDatasourceParams{Uid: uid, Name: cand.def.Name, Audience: string(cand.def.Audience), Secrets: row.Secrets, SecretNames: row.SecretNames}
	if !change.empty() {
		if params.Secrets, params.SecretNames, err = s.seal(row.ID, secrets); err != nil {
			return Record{}, err
		}
	}
	if params.Config, err = json.Marshal(cand.def); err != nil {
		return Record{}, err
	}
	var updated dbsqlc.Datasource
	save := func(q *dbsqlc.Queries) error {
		locked, err := q.LockDatasource(ctx, uid)
		if err != nil {
			return err
		}
		if !locked.UpdatedAt.Time.Equal(row.UpdatedAt.Time) {
			return ErrChanged
		}
		if updated, err = q.UpdateDatasource(ctx, params); err != nil {
			return err
		}
		if err := refuseSharedHolders(ctx, q, current.Definition.Audience, cand.def); err != nil {
			return err
		}
		return recordUpdated(ctx, q, by, updated.ID, current.Definition, cand.def, !change.empty())
	}
	addressChanged := cand.def.Address != current.Definition.Address
	if addressChanged {
		err = s.saveOnAllowedHost(ctx, by, cand.host, save)
	} else {
		err = s.db.InTx(ctx, save)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, err
	}
	if addressChanged {
		if previous, problem := endpointOf(current.Definition); problem == nil {
			s.releaseHost(ctx, by, previous)
		}
	}
	s.applyRow(ctx, updated)
	s.logger.InfoContext(ctx, "datasource updated", slog.String("actor_id", by.ID), slog.String("uid", uid), slog.Bool("credentials_changed", !change.empty()))
	return recordOf(updated)
}

func (s *Service) Delete(ctx context.Context, by auth.Principal, uid string) error {
	row, err := s.row(ctx, uid)
	if err != nil {
		return err
	}
	users, err := s.users(ctx, by, uid)
	if err != nil {
		return err
	}
	if len(users.Users)+users.Hidden > 0 {
		return &users
	}
	current, err := recordOf(row)
	if err != nil {
		return err
	}
	var n int64
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		var err error
		if n, err = q.DeleteDatasource(ctx, uid); err != nil || n == 0 {
			return err
		}
		return recordDeleted(ctx, q, by, row.ID, current.Definition)
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	s.mu.Lock()
	s.forgetLocked(uid)
	s.mu.Unlock()
	if host, problem := endpointOf(current.Definition); problem == nil {
		s.releaseHost(ctx, by, host)
	}
	s.logger.InfoContext(ctx, "datasource removed", slog.String("actor_id", by.ID), slog.String("uid", uid))
	return nil
}

func (s *Service) users(ctx context.Context, by auth.Principal, uid string) (InUseError, error) {
	levels, err := s.access.Levels(ctx, by)
	if err != nil {
		return InUseError{}, err
	}
	visible := levels.Visible()
	q := s.db.Queries()
	dashboards, err := q.ListDatasourceDashboards(ctx, dbsqlc.ListDatasourceDashboardsParams{Uid: uid, Visible: visible})
	if err != nil {
		return InUseError{}, err
	}
	searches, err := q.ListDatasourceSavedSearches(ctx, dbsqlc.ListDatasourceSavedSearchesParams{Uid: uid, Visible: visible})
	if err != nil {
		return InUseError{}, err
	}
	out := InUseError{UID: uid}
	for _, d := range dashboards {
		if !d.Visible {
			out.Hidden++
			continue
		}
		out.Users = append(out.Users, User{Kind: "dashboard", ID: d.ID.String(), Title: d.Title})
	}
	for _, ss := range searches {
		if !ss.Visible {
			out.Hidden++
			continue
		}
		out.Users = append(out.Users, User{Kind: "saved_search", ID: ss.ID.String(), Title: ss.Title})
	}
	return out, nil
}

type TestResult struct {
	Healthy bool
	Message string
	Detail  string
}

func (s *Service) Test(ctx context.Context, baseUID string, def Definition, change SecretsChange) (TestResult, error) {
	var secrets Secrets
	var err error
	if baseUID == "" {
		secrets, err = change.apply(Secrets{})
	} else {
		var row dbsqlc.Datasource
		if row, err = s.row(ctx, baseUID); err == nil {
			secrets, err = s.compose(row, def, change)
		}
	}
	if err != nil {
		return TestResult{}, err
	}
	cand, err := s.prepare(ctx, def, secrets)
	if err != nil {
		return TestResult{}, err
	}
	if err := s.mayConnect(ctx, cand.host); err != nil {
		return TestResult{}, err
	}
	ds, err := provision.Open(cand.cfg)
	if err != nil {
		return TestResult{}, invalid("definition", "the definition could not be used")
	}
	defer func() { _ = ds.Close() }()
	checkCtx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	if err := ds.CheckHealth(checkCtx); err != nil {
		message, detail := datasource.Reason(err)
		return TestResult{Message: message, Detail: detail}, nil
	}
	return TestResult{Healthy: true}, nil
}
