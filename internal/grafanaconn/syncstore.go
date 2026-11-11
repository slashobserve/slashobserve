package grafanaconn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

const (
	uniqueViolationCode = "23505"
	runningSyncIndex    = "grafana_syncs_running_idx"
)

type requestJSON struct {
	All     bool              `json:"all"`
	Folders []string          `json:"folders"`
	Mapping map[string]string `json:"mapping"`
}

func encodeRequest(req SyncRequest) ([]byte, error) {
	out := requestJSON(req)
	if out.Folders == nil {
		out.Folders = []string{}
	}
	if out.Mapping == nil {
		out.Mapping = map[string]string{}
	}
	return json.Marshal(out)
}

func decodeRequest(raw []byte) (SyncRequest, error) {
	var in requestJSON
	if err := json.Unmarshal(raw, &in); err != nil {
		return SyncRequest{}, fmt.Errorf("decode grafana sync request: %w", err)
	}
	return SyncRequest(in), nil
}

func summaryOf(row dbsqlc.GrafanaSync) SyncSummary {
	out := SyncSummary{
		ID:        row.ID.String(),
		State:     SyncState(row.State),
		Total:     int(row.Total),
		Done:      int(row.Done),
		Failed:    int(row.Failed),
		Skipped:   int(row.Skipped),
		StartedAt: row.StartedAt.Time.UTC(),
	}
	if row.Error.Valid {
		out.Error = row.Error.String
	}
	if row.FinishedAt.Valid {
		finished := row.FinishedAt.Time.UTC()
		out.FinishedAt = &finished
	}
	return out
}

func syncOf(row dbsqlc.GrafanaSync) Sync {
	return Sync{SyncSummary: summaryOf(row), ConnectionID: row.ConnectionID.String(), Results: []SyncResult{}}
}

func resultOf(row dbsqlc.GrafanaSyncItem) SyncResult {
	out := SyncResult{
		UID:               row.Uid,
		Title:             row.Title,
		Outcome:           row.Outcome,
		UnverifiedPanels:  int(row.UnverifiedPanels),
		PlaceholderPanels: int(row.PlaceholderPanels),
	}
	if row.DashboardID.Valid {
		out.DashboardID = row.DashboardID.String()
	}
	if row.Error.Valid {
		out.Error = row.Error.String
	}
	return out
}

func itemParams(syncID pgtype.UUID, res SyncResult) (dbsqlc.InsertGrafanaSyncItemParams, error) {
	params := dbsqlc.InsertGrafanaSyncItemParams{
		SyncID:            syncID,
		Uid:               res.UID,
		Title:             res.Title,
		Outcome:           res.Outcome,
		UnverifiedPanels:  httpapi.Int32(res.UnverifiedPanels),
		PlaceholderPanels: httpapi.Int32(res.PlaceholderPanels),
		Error:             textOf(res.Error),
	}
	if res.DashboardID != "" {
		id, err := db.ParseUUID(res.DashboardID)
		if err != nil {
			return dbsqlc.InsertGrafanaSyncItemParams{}, fmt.Errorf("sync result dashboard id: %w", err)
		}
		params.DashboardID = id
	}
	return params, nil
}

func textOf(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func (s *Service) logMove(ctx context.Context, a SyncAction, to SyncState, connectionID, syncID pgtype.UUID, extra ...slog.Attr) {
	attrs := append([]slog.Attr{slog.String("connection_id", connectionID.String()), slog.String("sync_id", syncID.String())}, extra...)
	s.logger.LogAttrs(ctx, slog.LevelInfo, SyncMachine.EventFor(a, to), attrs...)
}

func idAttrs(row dbsqlc.GrafanaSync) []any {
	return []any{slog.String("connection_id", row.ConnectionID.String()), slog.String("sync_id", row.ID.String())}
}

func (s *Service) StartSync(ctx context.Context, by auth.Principal, id pgtype.UUID, req SyncRequest) (Sync, error) {
	if err := s.checkMapping(req.Mapping); err != nil {
		return Sync{}, err
	}
	_, client, err := s.client(ctx, id)
	if err != nil {
		return Sync{}, err
	}
	request, err := encodeRequest(req)
	if err != nil {
		return Sync{}, fmt.Errorf("encode grafana sync request: %w", err)
	}
	starter, err := db.ParseUUID(by.ID)
	if err != nil {
		return Sync{}, fmt.Errorf("grafana sync starter id: %w", err)
	}
	var row dbsqlc.GrafanaSync
	err = s.store.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		inserted, err := q.InsertGrafanaSync(ctx, dbsqlc.InsertGrafanaSyncParams{
			ConnectionID: id, ToState: string(SyncRunning), Request: request,
			StartedByKind: string(by.Kind), StartedByID: starter,
			InstanceID: s.instanceID, LeaseSeconds: s.leaseTTL.Seconds(),
		})
		if err != nil {
			return err
		}
		row = inserted
		_, err = q.PruneGrafanaSyncs(ctx, dbsqlc.PruneGrafanaSyncsParams{ConnectionID: id, Kept: keptSyncs})
		return err
	})
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode && pgErr.ConstraintName == runningSyncIndex:
		return Sync{}, ErrSyncRunning
	case db.IsForeignKeyViolation(err):
		return Sync{}, ErrNotFound
	case err != nil:
		return Sync{}, fmt.Errorf("start grafana sync: %w", err)
	}
	s.logMove(ctx, ActionStart, SyncRunning, row.ConnectionID, row.ID, slog.String("actor_id", by.ID))
	s.drive(held{row: row, by: by, req: req}, client)
	return syncOf(row), nil
}

func (s *Service) SyncStatus(ctx context.Context, connectionID pgtype.UUID, syncID string) (Sync, error) {
	id, err := db.ParseUUID(syncID)
	if err != nil {
		return Sync{}, ErrSyncNotFound
	}
	q := s.store.db.Queries()
	row, err := q.GetGrafanaSync(ctx, dbsqlc.GetGrafanaSyncParams{ID: id, ConnectionID: connectionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Sync{}, ErrSyncNotFound
	}
	if err != nil {
		return Sync{}, fmt.Errorf("get grafana sync: %w", err)
	}
	items, err := q.ListGrafanaSyncItems(ctx, id)
	if err != nil {
		return Sync{}, fmt.Errorf("list grafana sync items: %w", err)
	}
	out := syncOf(row)
	for _, item := range items {
		out.Results = append(out.Results, resultOf(item))
	}
	return out, nil
}

func (s *Service) CancelSync(ctx context.Context, by auth.Principal, connectionID pgtype.UUID, syncID string) (Sync, error) {
	id, err := db.ParseUUID(syncID)
	if err != nil {
		return Sync{}, ErrSyncNotFound
	}
	q := s.store.db.Queries()
	canceled, err := q.CancelGrafanaSync(ctx, dbsqlc.CancelGrafanaSyncParams{
		ToState: string(SyncCanceled), ID: id, ConnectionID: connectionID, FromStates: SyncMachine.From(ActionCancel),
	})
	if err == nil {
		s.logMove(ctx, ActionCancel, SyncCanceled, canceled.ConnectionID, canceled.ID, slog.String("actor_id", by.ID))
		return s.SyncStatus(ctx, connectionID, syncID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Sync{}, fmt.Errorf("cancel grafana sync: %w", err)
	}
	row, err := q.GetGrafanaSync(ctx, dbsqlc.GetGrafanaSyncParams{ID: id, ConnectionID: connectionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Sync{}, ErrSyncNotFound
	}
	if err != nil {
		return Sync{}, fmt.Errorf("get grafana sync: %w", err)
	}
	return Sync{}, SyncMachine.Refuse(SyncState(row.State), ActionCancel, canOf(by))
}
