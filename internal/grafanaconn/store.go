package grafanaconn

import (
	"context"
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
	At          time.Time
	OK          bool
	Message     string
	FailureKind FailureKind
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
		c.LastTest = &TestResult{At: row.LastTestAt.Time.UTC(), OK: row.LastTestOk, Message: row.LastTestMessage, FailureKind: FailureKind(row.LastTestFailureKind.String)}
	}
	return stored{Connection: c, id: row.ID, sealed: row.TokenCiphertext}, nil
}

func (s store) insert(ctx context.Context, q *dbsqlc.Queries, params dbsqlc.InsertGrafanaConnectionParams) (dbsqlc.GrafanaConnection, error) {
	row, err := q.InsertGrafanaConnection(ctx, params)
	if err != nil {
		return dbsqlc.GrafanaConnection{}, fmt.Errorf("insert grafana connection: %w", err)
	}
	return row, nil
}

func (s store) withLastSyncs(ctx context.Context, list []stored) error {
	ids := make([]pgtype.UUID, len(list))
	for i, c := range list {
		ids[i] = c.id
	}
	rows, err := s.db.Queries().LastGrafanaSyncs(ctx, ids)
	if err != nil {
		return fmt.Errorf("list last grafana syncs: %w", err)
	}
	last := make(map[pgtype.UUID]SyncSummary, len(rows))
	for _, row := range rows {
		last[row.ConnectionID] = summaryOf(row)
	}
	for i := range list {
		if summary, ok := last[list[i].id]; ok {
			list[i].LastSync = &summary
		}
	}
	return nil
}

func (s store) one(ctx context.Context, row dbsqlc.GrafanaConnection) (stored, error) {
	c, err := storedOf(row)
	if err != nil {
		return stored{}, err
	}
	list := []stored{c}
	if err := s.withLastSyncs(ctx, list); err != nil {
		return stored{}, err
	}
	return list[0], nil
}

func (s store) get(ctx context.Context, id pgtype.UUID) (stored, error) {
	row, err := s.db.Queries().GetGrafanaConnection(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return stored{}, ErrNotFound
	}
	if err != nil {
		return stored{}, fmt.Errorf("get grafana connection: %w", err)
	}
	return s.one(ctx, row)
}

func (s store) list(ctx context.Context) ([]Connection, error) {
	rows, err := s.db.Queries().ListGrafanaConnections(ctx)
	if err != nil {
		return nil, fmt.Errorf("list grafana connections: %w", err)
	}
	all := make([]stored, 0, len(rows))
	for _, row := range rows {
		c, err := storedOf(row)
		if err != nil {
			return nil, err
		}
		all = append(all, c)
	}
	if err := s.withLastSyncs(ctx, all); err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(all))
	for _, c := range all {
		out = append(out, c.Connection)
	}
	return out, nil
}

func (s store) update(ctx context.Context, q *dbsqlc.Queries, params dbsqlc.UpdateGrafanaConnectionParams, record updateRecorder) (dbsqlc.GrafanaConnection, error) {
	before, err := q.GetGrafanaConnectionForUpdate(ctx, params.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.GrafanaConnection{}, ErrNotFound
	}
	if err != nil {
		return dbsqlc.GrafanaConnection{}, fmt.Errorf("update grafana connection: %w", err)
	}
	row, err := q.UpdateGrafanaConnection(ctx, params)
	if err != nil {
		return dbsqlc.GrafanaConnection{}, fmt.Errorf("update grafana connection: %w", err)
	}
	return row, record(ctx, q, before)
}

func (s store) delete(ctx context.Context, id pgtype.UUID, record recorder) error {
	err := s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		n, err := q.DeleteGrafanaConnection(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return record(ctx, q)
	})
	if errors.Is(err, ErrNotFound) {
		return err
	}
	if err != nil {
		return fmt.Errorf("delete grafana connection: %w", err)
	}
	return nil
}

func (s store) setTest(ctx context.Context, id pgtype.UUID, outcome testOutcome) (stored, error) {
	row, err := s.db.Queries().SetGrafanaConnectionTest(ctx, dbsqlc.SetGrafanaConnectionTestParams{
		ID: id, Ok: outcome.failure == nil, Message: outcome.message(), FailureKind: outcome.kind(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return stored{}, ErrNotFound
	}
	if err != nil {
		return stored{}, fmt.Errorf("record grafana connection test: %w", err)
	}
	return s.one(ctx, row)
}
