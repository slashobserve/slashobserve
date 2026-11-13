package revisions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/health/repairprompt"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type repairOutcome struct {
	DashboardTitle string
	PanelsRepaired int
}

func (o repairOutcome) data() map[string]any {
	return map[string]any{"dashboard_title": o.DashboardTitle, "panels_repaired": o.PanelsRepaired}
}

func isHealthRepair(row dbsqlc.AgentRequest) bool {
	return row.Source == string(SourcePrompt) && repairprompt.Is(row.Prompt) && row.DashboardID.Valid && row.ResultVersion.Valid
}

func readRepairOutcome(ctx context.Context, q *dbsqlc.Queries, row dbsqlc.AgentRequest) (repairOutcome, bool, error) {
	version := row.ResultVersion.Int32
	saved, err := readSpec(ctx, q, row.DashboardID, version)
	if errors.Is(err, pgx.ErrNoRows) {
		return repairOutcome{}, false, nil
	}
	if err != nil {
		return repairOutcome{}, false, err
	}
	var parent *spec.Dashboard
	if version > 1 {
		parent, err = readSpec(ctx, q, row.DashboardID, version-1)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return repairOutcome{}, false, err
		}
	}
	unverified, err := readUnverifiedPanels(ctx, q, row.DashboardID, version)
	if err != nil {
		return repairOutcome{}, false, err
	}
	return repairOutcome{
		DashboardTitle: saved.Title,
		PanelsRepaired: repairedPanels(repairprompt.PanelIDs(row.Prompt), parent, saved, unverified),
	}, true, nil
}

func readSpec(ctx context.Context, q *dbsqlc.Queries, dashboard pgtype.UUID, version int32) (*spec.Dashboard, error) {
	raw, err := q.GetDashboardVersionSpec(ctx, dbsqlc.GetDashboardVersionSpecParams{ID: dashboard, Version: version})
	if err != nil {
		return nil, err
	}
	var d spec.Dashboard
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("decode version %d of the repaired dashboard: %w", version, err)
	}
	return &d, nil
}

func readUnverifiedPanels(ctx context.Context, q *dbsqlc.Queries, dashboard pgtype.UUID, version int32) ([]string, error) {
	stored, err := q.GetDashboardVersionUnverified(ctx, dbsqlc.GetDashboardVersionUnverifiedParams{ID: dashboard, Version: version})
	if err != nil {
		return nil, err
	}
	if len(stored) == 0 {
		return nil, nil
	}
	var unverified struct {
		Panels []struct {
			PanelID string `json:"panel_id"`
		} `json:"panels"`
	}
	if err := json.Unmarshal(stored, &unverified); err != nil {
		return nil, fmt.Errorf("decode unverified panels of version %d: %w", version, err)
	}
	ids := make([]string, len(unverified.Panels))
	for i, p := range unverified.Panels {
		ids[i] = p.PanelID
	}
	return ids, nil
}

type checkedPanel struct {
	Type          spec.PanelType
	Datasource    string
	Queries       []spec.Query
	TimeFrom      string
	TimeShift     string
	Interval      string
	MaxDataPoints int
}

func checkedPanelOf(p *spec.Panel) checkedPanel {
	return checkedPanel{Type: p.Type, Datasource: p.Datasource, Queries: p.Queries, TimeFrom: p.TimeFrom, TimeShift: p.TimeShift, Interval: p.Interval, MaxDataPoints: p.MaxDataPoints}
}

func repairedPanels(named []string, parent, saved *spec.Dashboard, unverified []string) int {
	repaired := 0
	for _, id := range named {
		now, ok := saved.FindPanel(id)
		if !ok || slices.Contains(unverified, id) {
			continue
		}
		if was, existed := findIn(parent, id); existed && reflect.DeepEqual(checkedPanelOf(was), checkedPanelOf(now.Panel)) {
			continue
		}
		repaired++
	}
	return repaired
}

func findIn(d *spec.Dashboard, id string) (*spec.Panel, bool) {
	if d == nil {
		return nil, false
	}
	e, ok := d.FindPanel(id)
	return e.Panel, ok
}
