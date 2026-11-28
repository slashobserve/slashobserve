package datasourceadmin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/provision"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/egress/domains"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/sealed"
)

const (
	secretsKeyLabel = "slashobserve datasource secrets v1"
	pollEvery       = 10 * time.Second
)

type Slots interface {
	SetTotal(datasourceUID string, total int)
}

type Domains interface {
	Allowed(ctx context.Context) (domains.List, error)
	SaveOnAllowedHost(ctx context.Context, by auth.Principal, host string, save func(*dbsqlc.Queries) error) (string, error)
	Release(ctx context.Context, by auth.Principal, host string) error
}

type Deps struct {
	DB             *db.DB
	Registry       *datasource.Registry
	Slots          Slots
	Key            sealed.Key
	Domains        Domains
	OutboundLocked bool
	Access         *folders.Access
	Logger         *slog.Logger
}

type version struct {
	id        pgtype.UUID
	updatedAt time.Time
	blocked   bool
	failed    bool
}

func (v version) sameRow(o version) bool {
	return v.id == o.id && v.updatedAt.Equal(o.updatedAt) && v.blocked == o.blocked
}

const (
	definitionNotJSON   = "the stored definition is not valid JSON"
	definitionInvalid   = "the stored definition is invalid"
	definitionUnopened  = "the stored definition cannot be opened"
	codeNotLoaded       = "datasource_not_loaded"
	codeNotLoadable     = "datasource_not_loadable"
	codeServerOwned     = "datasource_server_owned"
	codeNoSealingKey    = "datasource_no_secret_key"
	codeUnreadable      = "datasource_credentials_unreadable"
	codeDefinitionJSON  = "datasource_definition_not_json"
	codeDefinitionBad   = "datasource_definition_invalid"
	codeDefinitionOpens = "datasource_definition_not_opened"
)

func failureCode(err error) string {
	switch {
	case errors.Is(err, datasource.ErrServerOwned):
		return codeServerOwned
	case errors.Is(err, datasource.ErrNoSecretKey):
		return codeNoSealingKey
	case errors.Is(err, datasource.ErrUnreadable):
		return codeUnreadable
	case errors.Is(err, datasource.ErrNotLoadable):
		var upstream *datasource.UpstreamError
		if !errors.As(err, &upstream) {
			return codeNotLoadable
		}
		switch upstream.Message {
		case definitionNotJSON:
			return codeDefinitionJSON
		case definitionInvalid:
			return codeDefinitionBad
		case definitionUnopened:
			return codeDefinitionOpens
		}
		return codeNotLoadable
	}
	return codeNotLoaded
}

type Service struct {
	db       *db.DB
	registry *datasource.Registry
	slots    Slots
	key      sealed.Key
	domains  Domains
	access   *folders.Access
	logger   *slog.Logger

	outboundLocked bool

	mu      sync.Mutex
	applied map[string]version
	list    func(context.Context) ([]dbsqlc.Datasource, error)
}

func New(d Deps) *Service {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	s := &Service{db: d.DB, registry: d.Registry, slots: d.Slots, key: d.Key, domains: d.Domains, access: d.Access, outboundLocked: d.OutboundLocked, logger: logger, applied: map[string]version{}}
	s.list = func(ctx context.Context) ([]dbsqlc.Datasource, error) { return s.db.Queries().ListDatasources(ctx) }
	return s
}

func (s *Service) Load(ctx context.Context) error { return s.Reconcile(ctx) }

func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Reconcile(ctx); err != nil && ctx.Err() == nil {
				s.logger.Warn("datasource sync failed", logattrs.Failure("datasource_sync_failed", err)...)
			}
		}
	}
}

func (s *Service) Reconcile(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.list(ctx)
	if err != nil {
		return err
	}
	allowed, err := s.domains.Allowed(ctx)
	if err != nil {
		return err
	}
	present := make(map[string]bool, len(rows))
	for _, row := range rows {
		present[row.Uid] = true
		s.applyLocked(row, allowed)
	}
	for uid := range s.applied {
		if !present[uid] {
			s.forgetLocked(uid)
		}
	}
	return nil
}

func (s *Service) applyLocked(row dbsqlc.Datasource, allowed domains.List) {
	v := version{id: row.ID, updatedAt: row.UpdatedAt.Time, blocked: s.blocked(row, allowed)}
	if known, ok := s.applied[row.Uid]; ok && known.sameRow(v) {
		return
	}
	instance, total := s.instanceOf(row, v.blocked)
	if err := s.registry.Put(instance); err != nil {
		_ = instance.Datasource.Close()
		v.failed = true
		s.applied[row.Uid] = v
		s.logger.Warn("stored datasource not loaded", logattrs.Failure(failureCode(err), err, slog.String("uid", row.Uid), slog.String("type", row.Type))...)
		return
	}
	s.applied[row.Uid] = v
	s.slots.SetTotal(row.Uid, total)
}

func (s *Service) forgetLocked(uid string) {
	delete(s.applied, uid)
	if s.registry.Remove(uid) {
		s.slots.SetTotal(uid, 0)
	}
}

func (s *Service) blocked(row dbsqlc.Datasource, allowed domains.List) bool {
	var def Definition
	if err := json.Unmarshal(row.Config, &def); err != nil {
		return false
	}
	def.Type = row.Type
	host, problem := endpointOf(def)
	return problem == nil && s.refusal(allowed, host, false) != nil
}

func (s *Service) instanceOf(row dbsqlc.Datasource, blocked bool) (datasource.Instance, int) {
	instance := datasource.Instance{UID: row.Uid, Name: row.Name, Audience: datasource.Audience(row.Audience), Source: datasource.SourceApp}
	if blocked {
		instance.Datasource = datasource.NewUnavailable(row.Type, datasource.Errorf(datasource.ErrOutboundBlocked, "the address is not on the outbound allowlist"))
		return instance, 0
	}
	var def Definition
	ds, total, err := s.open(row, &def)
	if err != nil {
		s.logger.Warn("stored datasource cannot be used", logattrs.Failure(failureCode(err), err, slog.String("uid", row.Uid), slog.String("type", row.Type))...)
		instance.Datasource = datasource.NewUnavailable(row.Type, err)
		return instance, 0
	}
	instance.GrafanaUIDs = def.GrafanaUIDs
	instance.Datasource = ds
	return instance, total
}

func (s *Service) open(row dbsqlc.Datasource, def *Definition) (datasource.Datasource, int, error) {
	if err := json.Unmarshal(row.Config, def); err != nil {
		return nil, 0, datasource.Errorf(datasource.ErrNotLoadable, definitionNotJSON)
	}
	secrets, err := s.secretsOf(row)
	if err != nil {
		return nil, 0, err
	}
	cfg, problems := def.Config(secrets)
	if len(problems) > 0 {
		return nil, 0, datasource.Errorf(datasource.ErrNotLoadable, definitionInvalid)
	}
	ds, err := provision.Open(cfg)
	if err != nil {
		return nil, 0, datasource.Errorf(datasource.ErrNotLoadable, definitionUnopened)
	}
	return ds, cfg.MaxConcurrentQueries, nil
}

func (s *Service) secretsOf(row dbsqlc.Datasource) (Secrets, error) {
	var secrets Secrets
	if len(row.Secrets) == 0 {
		return secrets, nil
	}
	plain, err := s.key.Open(secretsKeyLabel, row.ID, row.Secrets)
	switch {
	case errors.Is(err, sealed.ErrNoKey):
		return secrets, datasource.Errorf(datasource.ErrNoSecretKey, "no secret key is set")
	case err != nil:
		return secrets, datasource.Errorf(datasource.ErrUnreadable, "the sealed credentials cannot be read")
	}
	if err := json.Unmarshal(plain, &secrets); err != nil {
		return Secrets{}, datasource.Errorf(datasource.ErrUnreadable, "the sealed credentials cannot be read")
	}
	return secrets, nil
}

func sealSecrets(key sealed.Key, id pgtype.UUID, secrets Secrets) ([]byte, error) {
	if secrets.Empty() {
		return nil, nil
	}
	plain, err := json.Marshal(secrets) //nolint:gosec // sealed before it is stored
	if err != nil {
		return nil, err
	}
	return key.Seal(secretsKeyLabel, id, plain)
}
