package grafanaconn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

var ErrNotFound = errors.New("grafana connection not found")

type Connection struct {
	ID        string
	Name      string
	URL       string
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
	LastTest  *TestResult
	LastSync  *SyncSummary
}

type TestResult struct {
	At      time.Time
	OK      bool
	Message string
}

type stored struct {
	Connection
	id     pgtype.UUID
	sealed []byte
}

type store struct {
	db *db.DB
}

func storedOf(row dbsqlc.GrafanaConnection) (stored, error) {
	c := Connection{
		ID:        row.ID.String(),
		Name:      row.Name,
		URL:       row.Url,
		CreatedAt: row.CreatedAt.Time.UTC(),
		UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	if row.CreatedBy.Valid {
		c.CreatedBy = row.CreatedBy.String()
	}
	if row.LastTestAt.Valid {
		c.LastTest = &TestResult{At: row.LastTestAt.Time.UTC(), OK: row.LastTestOk, Message: row.LastTestMessage}
	}
	if row.LastSync != nil {
		var summary SyncSummary
		if err := json.Unmarshal(row.LastSync, &summary); err != nil {
			return stored{}, fmt.Errorf("decode last sync: %w", err)
		}
		c.LastSync = &summary
	}
	return stored{Connection: c, id: row.ID, sealed: row.TokenCiphertext}, nil
}

func (s store) insert(ctx context.Context, params dbsqlc.InsertGrafanaConnectionParams) (stored, error) {
	row, err := s.db.Queries().InsertGrafanaConnection(ctx, params)
	if err != nil {
		return stored{}, fmt.Errorf("insert grafana connection: %w", err)
	}
	return storedOf(row)
}

func (s store) get(ctx context.Context, id pgtype.UUID) (stored, error) {
	row, err := s.db.Queries().GetGrafanaConnection(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return stored{}, ErrNotFound
	}
	if err != nil {
		return stored{}, fmt.Errorf("get grafana connection: %w", err)
	}
	return storedOf(row)
}

func (s store) list(ctx context.Context) ([]Connection, error) {
	rows, err := s.db.Queries().ListGrafanaConnections(ctx)
	if err != nil {
		return nil, fmt.Errorf("list grafana connections: %w", err)
	}
	out := make([]Connection, 0, len(rows))
	for _, row := range rows {
		c, err := storedOf(row)
		if err != nil {
			return nil, err
		}
		out = append(out, c.Connection)
	}
	return out, nil
}

func (s store) update(ctx context.Context, params dbsqlc.UpdateGrafanaConnectionParams) (stored, error) {
	row, err := s.db.Queries().UpdateGrafanaConnection(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return stored{}, ErrNotFound
	}
	if err != nil {
		return stored{}, fmt.Errorf("update grafana connection: %w", err)
	}
	return storedOf(row)
}

func (s store) delete(ctx context.Context, id pgtype.UUID) error {
	n, err := s.db.Queries().DeleteGrafanaConnection(ctx, id)
	if err != nil {
		return fmt.Errorf("delete grafana connection: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s store) setTest(ctx context.Context, id pgtype.UUID, ok bool, message string) (stored, error) {
	row, err := s.db.Queries().SetGrafanaConnectionTest(ctx, dbsqlc.SetGrafanaConnectionTestParams{ID: id, Ok: ok, Message: message})
	if errors.Is(err, pgx.ErrNoRows) {
		return stored{}, ErrNotFound
	}
	if err != nil {
		return stored{}, fmt.Errorf("record grafana connection test: %w", err)
	}
	return storedOf(row)
}

func (s store) setSync(ctx context.Context, id pgtype.UUID, summary SyncSummary) error {
	raw, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("encode last sync: %w", err)
	}
	if _, err := s.db.Queries().SetGrafanaConnectionSync(ctx, dbsqlc.SetGrafanaConnectionSyncParams{ID: id, LastSync: raw}); err != nil {
		return fmt.Errorf("record grafana sync: %w", err)
	}
	return nil
}
