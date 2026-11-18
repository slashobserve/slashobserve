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
)

const (
	MaxAppDatasources = 100
	testTimeout       = 10 * time.Second
	uniqueViolation   = "23505"
	maxHeaders        = 20
	maxSecretBytes    = 4096
)

var (
	ErrNotFound        = errors.New("datasource not found")
	ErrReadOnly        = errors.New("this datasource is defined in the datasources file on the server and cannot be changed here")
	ErrUIDTaken        = errors.New("a datasource with this uid already exists")
	ErrNoSecretKey     = errors.New("SLASHOBSERVE_SECRET_KEY must be set to store credentials")
	ErrTooMany         = errors.New("too many datasources were added in the app")
	ErrAddressRefused  = errors.New("the outbound allowlist is set by configuration and cannot list an address like this; use a domain name that is on the list, or define the datasource in the datasources file")
	ErrAllowlistFull   = errors.New("the outbound allowlist is full")
	ErrIdentityChanged = errors.New("uid and type cannot be changed; add a new datasource instead")
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
	return fmt.Sprintf("datasource %q is used by %d dashboards and saved searches", e.UID, len(e.Users)+e.Hidden)
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
	return nil
}

func recordOf(row dbsqlc.Datasource) (Record, error) {
	var def Definition
	if err := json.Unmarshal(row.Config, &def); err != nil {
		return Record{}, fmt.Errorf("stored datasource %q: %w", row.Uid, err)
	}
	def.UID, def.Type, def.Name = row.Uid, row.Type, row.Name
	return Record{Definition: def, SecretsSet: slices.Clone(row.SecretNames), CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}, nil
}

func (s *Service) row(ctx context.Context, uid string) (dbsqlc.Datasource, error) {
	row, err := s.db.Queries().GetDatasource(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		if in, ok := s.registry.Get(uid); ok && in.Source == datasource.SourceFile {
			return dbsqlc.Datasource{}, ErrReadOnly
		}
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
	return def
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

func (s *Service) prepare(def Definition, secrets Secrets) (candidate, error) {
	def = normalizeDefinition(def)
	if err := checkName(def.Name); err != nil {
		return candidate{}, err
	}
	if err := checkSecrets(secrets); err != nil {
		return candidate{}, err
	}
	cfg, problems := def.Config(secrets)
	problems = append(problems, s.aliasConflicts(def)...)
	host, addressProblem := endpointOf(def)
	if addressProblem != nil && !slices.ContainsFunc(problems, func(p config.FieldError) bool { return p.Field == "url" || p.Field == "addr" }) {
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

func (s *Service) aliasConflicts(def Definition) []config.FieldError {
	var problems []config.FieldError
	for _, other := range s.registry.List() {
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

func (s *Service) allow(ctx context.Context, by auth.Principal, host endpoint) error {
	pattern, isDomain := host.domain()
	if !isDomain {
		if s.outboundLocked {
			return ErrAddressRefused
		}
		return nil
	}
	list, err := s.domains.Allowed(ctx)
	if err != nil {
		return err
	}
	if list.Allows(host.host) {
		return nil
	}
	_, err = s.domains.Add(ctx, by, pattern)
	switch {
	case err == nil:
		s.logger.InfoContext(ctx, "allowed an outbound domain", slog.String("actor_id", by.ID), slog.String("domain", pattern.String()))
		return nil
	case errors.Is(err, allowlist.ErrExists):
		return nil
	case errors.Is(err, allowlist.ErrLocked):
		return &egress.HostError{Host: host.host, Err: egress.ErrNotAllowed}
	case errors.Is(err, allowlist.ErrTooMany):
		return ErrAllowlistFull
	}
	return err
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
	cand, err := s.prepare(def, secrets)
	if err != nil {
		return Record{}, err
	}
	if _, taken := s.registry.Get(cand.def.UID); taken {
		return Record{}, ErrUIDTaken
	}
	count, err := s.db.Queries().CountDatasources(ctx)
	if err != nil {
		return Record{}, err
	}
	if count >= MaxAppDatasources {
		return Record{}, ErrTooMany
	}
	id := newID()
	ciphertext, names, err := s.seal(id, secrets)
	if err != nil {
		return Record{}, err
	}
	if err := s.allow(ctx, by, cand.host); err != nil {
		return Record{}, err
	}
	config, err := json.Marshal(cand.def)
	if err != nil {
		return Record{}, err
	}
	params := dbsqlc.InsertDatasourceParams{ID: id, Uid: cand.def.UID, Type: cand.def.Type, Name: cand.def.Name, Config: config, Secrets: ciphertext, SecretNames: names}
	if creator, err := db.ParseUUID(by.ID); err == nil && by.Kind == auth.KindUser {
		params.CreatedBy = creator
	}
	row, err := s.db.Queries().InsertDatasource(ctx, params)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return Record{}, ErrUIDTaken
	}
	if err != nil {
		return Record{}, err
	}
	s.applyRow(row)
	s.logger.InfoContext(ctx, "datasource added", slog.String("actor_id", by.ID), slog.String("uid", row.Uid), slog.String("type", row.Type))
	return recordOf(row)
}

func (s *Service) applyRow(row dbsqlc.Datasource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.applied, row.Uid)
	s.applyLocked(row)
}

func (s *Service) compose(row dbsqlc.Datasource, def Definition, change SecretsChange) (Secrets, error) {
	if def.UID != row.Uid || def.Type != row.Type {
		return Secrets{}, ErrIdentityChanged
	}
	current, err := s.currentSecrets(row)
	if err != nil {
		return Secrets{}, err
	}
	if change.empty() {
		return current, nil
	}
	return change.apply(current)
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
	cand, err := s.prepare(def, secrets)
	if err != nil {
		return Record{}, err
	}
	current, err := recordOf(row)
	if err != nil {
		return Record{}, err
	}
	if cand.def.Address != current.Definition.Address {
		if err := s.allow(ctx, by, cand.host); err != nil {
			return Record{}, err
		}
	}
	params := dbsqlc.UpdateDatasourceParams{Uid: uid, Name: cand.def.Name, Secrets: row.Secrets, SecretNames: row.SecretNames}
	if !change.empty() {
		if params.Secrets, params.SecretNames, err = s.seal(row.ID, secrets); err != nil {
			return Record{}, err
		}
	}
	if params.Config, err = json.Marshal(cand.def); err != nil {
		return Record{}, err
	}
	updated, err := s.db.Queries().UpdateDatasource(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, err
	}
	s.applyRow(updated)
	s.logger.InfoContext(ctx, "datasource updated", slog.String("actor_id", by.ID), slog.String("uid", uid), slog.Bool("credentials_changed", !change.empty()))
	return recordOf(updated)
}

func (s *Service) Delete(ctx context.Context, by auth.Principal, uid string) error {
	if _, err := s.row(ctx, uid); err != nil {
		return err
	}
	users, err := s.users(ctx, by, uid)
	if err != nil {
		return err
	}
	if len(users.Users)+users.Hidden > 0 {
		return &users
	}
	n, err := s.db.Queries().DeleteDatasource(ctx, uid)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	s.mu.Lock()
	s.forgetLocked(uid)
	s.mu.Unlock()
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

func (s *Service) Test(ctx context.Context, by auth.Principal, baseUID string, def Definition, change SecretsChange) (TestResult, error) {
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
	cand, err := s.prepare(def, secrets)
	if err != nil {
		return TestResult{}, err
	}
	if err := s.allow(ctx, by, cand.host); err != nil {
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
