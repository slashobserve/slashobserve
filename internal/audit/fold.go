package audit

import (
	"encoding/json"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type foldKey struct {
	action     string
	reason     string
	targetKind string
	targetID   string
	targetName string
	actor      pgtype.UUID
	provider   string
	overflow   bool
}

type fold struct {
	latest dbsqlc.InsertAuditEventParams
	later  int64
	opened time.Time
}

type folds struct {
	mu     sync.Mutex
	window time.Duration
	limit  int
	open   map[foldKey]*fold
}

func newFolds(window time.Duration, limit int) *folds {
	return &folds{window: window, limit: limit, open: map[foldKey]*fold{}}
}

func isFolded(action string) bool {
	return slices.Contains(foldedActions, Action(action))
}

func providerErrorOf(row dbsqlc.InsertAuditEventParams) string {
	var data struct {
		ProviderError string `json:"provider_error"`
	}
	_ = json.Unmarshal(row.Data, &data)
	return data.ProviderError
}

func foldKeyOf(row dbsqlc.InsertAuditEventParams) foldKey {
	return foldKey{
		provider:   providerErrorOf(row),
		action:     row.Action,
		reason:     row.Reason.String,
		targetKind: row.TargetKind.String,
		targetID:   row.TargetID.String,
		targetName: row.TargetName.String,
		actor:      row.ActorID,
	}
}

func overflowRow(row dbsqlc.InsertAuditEventParams) dbsqlc.InsertAuditEventParams {
	return dbsqlc.InsertAuditEventParams{
		ID:            row.ID,
		OccurredAt:    row.OccurredAt,
		Category:      row.Category,
		Action:        row.Action,
		Outcome:       row.Outcome,
		Reason:        row.Reason,
		Status:        row.Status,
		ActorKind:     string(ActorAnonymous),
		Initiator:     row.Initiator,
		Origin:        row.Origin,
		HttpMethod:    row.HttpMethod,
		Route:         row.Route,
		Data:          []byte(`{"overflow":true}`),
		ServerID:      row.ServerID,
		ServerVersion: row.ServerVersion,
	}
}

func (f *folds) absorb(row dbsqlc.InsertAuditEventParams, now time.Time) (write dbsqlc.InsertAuditEventParams, enqueue bool) {
	if !isFolded(row.Action) {
		return row, true
	}
	key := foldKeyOf(row)
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, found := f.open[key]; !found && len(f.open) >= f.limit {
		key = foldKey{action: row.Action, reason: row.Reason.String, overflow: true}
		row = overflowRow(row)
	}
	if open, found := f.open[key]; found {
		open.latest = row
		open.later++
		return write, false
	}
	f.open[key] = &fold{latest: row, opened: now}
	return withCount(row, 1), true
}

func (f *folds) due(now time.Time) []dbsqlc.InsertAuditEventParams {
	return f.take(func(open *fold) bool { return now.Sub(open.opened) >= f.window })
}

func (f *folds) drain() []dbsqlc.InsertAuditEventParams {
	return f.take(func(*fold) bool { return true })
}

func (f *folds) take(ready func(*fold) bool) []dbsqlc.InsertAuditEventParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	var rows []dbsqlc.InsertAuditEventParams
	for key, open := range f.open {
		if !ready(open) {
			continue
		}
		delete(f.open, key)
		if open.later > 0 {
			rows = append(rows, withCount(open.latest, open.later))
		}
	}
	slices.SortFunc(rows, func(a, b dbsqlc.InsertAuditEventParams) int { return a.OccurredAt.Time.Compare(b.OccurredAt.Time) })
	return rows
}

func withCount(row dbsqlc.InsertAuditEventParams, count int64) dbsqlc.InsertAuditEventParams {
	data := map[string]any{}
	if err := json.Unmarshal(row.Data, &data); err != nil {
		data = map[string]any{}
	}
	data["count"] = count
	row.Data, _ = json.Marshal(data)
	return row
}

func weigh(batch []dbsqlc.InsertAuditEventParams) int {
	total := 0
	for _, row := range batch {
		total += weightOf(row)
	}
	return total
}

func weightOf(row dbsqlc.InsertAuditEventParams) int {
	if !isFolded(row.Action) {
		return 1
	}
	var data struct {
		Count int `json:"count"`
	}
	if json.Unmarshal(row.Data, &data) != nil || data.Count < 1 {
		return 1
	}
	return data.Count
}
