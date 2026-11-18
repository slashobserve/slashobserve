package datasourceadmin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/provision"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
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

type Deps struct {
	DB       *db.DB
	Registry *datasource.Registry
	Slots    Slots
	Key      sealed.Key
	Logger   *slog.Logger
}

type version struct {
	id        pgtype.UUID
	updatedAt time.Time
}

type Service struct {
	db       *db.DB
	registry *datasource.Registry
	slots    Slots
	key      sealed.Key
	logger   *slog.Logger

	mu      sync.Mutex
	applied map[string]version
}

func New(d Deps) *Service {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{db: d.DB, registry: d.Registry, slots: d.Slots, key: d.Key, logger: logger, applied: map[string]version{}}
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
	rows, err := s.db.Queries().ListDatasources(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	present := make(map[string]bool, len(rows))
	for _, row := range rows {
		present[row.Uid] = true
		s.applyLocked(row)
	}
	for uid := range s.applied {
		if !present[uid] {
			s.forgetLocked(uid)
		}
	}
	return nil
}

func (s *Service) applyLocked(row dbsqlc.Datasource) {
	v := version{id: row.ID, updatedAt: row.UpdatedAt.Time}
	if known, ok := s.applied[row.Uid]; ok && known == v {
		return
	}
	s.applied[row.Uid] = v
	instance, total := s.instanceOf(row)
	if err := s.registry.Put(instance); err != nil {
		_ = instance.Datasource.Close()
		s.logger.Warn("stored datasource not loaded", slog.String("uid", row.Uid), slog.String("reason", err.Error()))
		return
	}
	s.slots.SetTotal(row.Uid, total)
}

func (s *Service) forgetLocked(uid string) {
	delete(s.applied, uid)
	s.registry.Remove(uid)
	s.slots.SetTotal(uid, 0)
}

func (s *Service) instanceOf(row dbsqlc.Datasource) (datasource.Instance, int) {
	instance := datasource.Instance{UID: row.Uid, Name: row.Name, Source: datasource.SourceApp}
	var def Definition
	ds, total, err := s.open(row, &def)
	if err != nil {
		s.logger.Warn("stored datasource cannot be used", slog.String("uid", row.Uid), slog.String("type", row.Type), slog.String("reason", err.Error()))
		instance.Datasource = datasource.NewUnavailable(row.Type, err)
		return instance, 0
	}
	instance.GrafanaUIDs = def.GrafanaUIDs
	instance.Datasource = ds
	return instance, total
}

func (s *Service) open(row dbsqlc.Datasource, def *Definition) (datasource.Datasource, int, error) {
	if err := json.Unmarshal(row.Config, def); err != nil {
		return nil, 0, datasource.Errorf(datasource.ErrNotLoadable, "the stored definition is not valid JSON")
	}
	secrets, err := s.secretsOf(row)
	if err != nil {
		return nil, 0, err
	}
	cfg, problems := def.Config(secrets)
	if len(problems) > 0 {
		return nil, 0, datasource.Errorf(datasource.ErrNotLoadable, "the stored definition is invalid")
	}
	ds, err := provision.Open(cfg)
	if err != nil {
		return nil, 0, datasource.Errorf(datasource.ErrNotLoadable, "the stored definition cannot be opened")
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
