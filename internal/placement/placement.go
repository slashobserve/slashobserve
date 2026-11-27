package placement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type Guard struct {
	reg *datasource.Registry
}

func New(reg *datasource.Registry) *Guard {
	return &Guard{reg: reg}
}

func (g *Guard) fileAdminsOnly(uid string) bool {
	return g != nil && g.reg != nil && g.reg.AdminsOnly(uid)
}

func (g *Guard) Restricted(ctx context.Context, q *dbsqlc.Queries, refs []string) (string, bool, error) {
	if len(refs) == 0 {
		return "", false, nil
	}
	if err := q.ShareDatasourceUIDs(ctx, refs); err != nil {
		return "", false, fmt.Errorf("lock datasource uids: %w", err)
	}
	rows, err := q.ShareDatasourceAudiences(ctx, refs)
	if err != nil {
		return "", false, fmt.Errorf("lock datasource audiences: %w", err)
	}
	stored := make(map[string]bool, len(rows))
	for _, r := range rows {
		stored[r.Uid] = r.Audience == string(datasource.AudienceAdmins)
	}
	ds, found := queryproxy.FirstRestricted(refs, func(uid string) bool {
		if adminsOnly, ok := stored[uid]; ok {
			return adminsOnly
		}
		return g.fileAdminsOnly(uid)
	})
	return ds, found, nil
}

func (g *Guard) Save(ctx context.Context, q *dbsqlc.Queries, kind folders.ContentKind, folderID pgtype.UUID, refs []string) error {
	return g.place(ctx, q, folderID, refs, func(ds string) error { return folders.RestrictedSave(kind, ds) })
}

func (g *Guard) Move(ctx context.Context, q *dbsqlc.Queries, kind folders.ContentKind, folderID pgtype.UUID, refs []string) error {
	return g.place(ctx, q, folderID, refs, func(ds string) error { return folders.RestrictedMove(kind, ds) })
}

func (g *Guard) place(ctx context.Context, q *dbsqlc.Queries, folderID pgtype.UUID, refs []string, refuse func(ds string) error) error {
	ds, found, err := g.Restricted(ctx, q, refs)
	if err != nil || !found {
		return err
	}
	folder, err := q.ShareFolderPlacement(ctx, folderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return refuse(ds)
	}
	if err != nil {
		return fmt.Errorf("lock folder: %w", err)
	}
	if folder.AdminsOnly {
		return nil
	}
	return refuse(ds)
}

func SpecRefs(raw []byte) ([]string, error) {
	var d spec.Dashboard
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("decode dashboard: %w", err)
	}
	return queryproxy.DatasourceRefs(&d), nil
}

func PanelRefs(panel json.RawMessage, variables []spec.Variable) ([]string, error) {
	var p spec.Panel
	if err := json.Unmarshal(panel, &p); err != nil {
		return nil, fmt.Errorf("decode panel: %w", err)
	}
	return queryproxy.DatasourceRefs(&spec.Dashboard{Variables: variables, Panels: []spec.Panel{p}}), nil
}

func SnapshotRefs(panel, query json.RawMessage, stored []string) ([]string, error) {
	var p spec.Panel
	if err := json.Unmarshal(panel, &p); err != nil {
		return nil, fmt.Errorf("decode snapshot panel: %w", err)
	}
	if err := json.Unmarshal(query, &p.Queries); err != nil {
		return nil, fmt.Errorf("decode snapshot queries: %w", err)
	}
	return union(queryproxy.DatasourceRefs(&spec.Dashboard{Panels: []spec.Panel{p}}), stored), nil
}

func union(into []string, refs []string) []string {
	for _, uid := range refs {
		if !slices.Contains(into, uid) {
			into = append(into, uid)
		}
	}
	return into
}

func (g *Guard) SaveProposal(ctx context.Context, q *dbsqlc.Queries, dashboardID pgtype.UUID, refs []string) error {
	folderID, err := q.ShareDashboardPlacement(ctx, dashboardID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock dashboard: %w", err)
	}
	return g.Save(ctx, q, folders.ContentProposal, folderID, refs)
}

func (g *Guard) MoveDashboard(ctx context.Context, q *dbsqlc.Queries, id, to pgtype.UUID) error {
	if _, err := q.LockDashboardPlacement(ctx, id); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("lock dashboard: %w", err)
	}
	if err := refuseLinkedMove(ctx, q, id, to); err != nil {
		return err
	}
	if err := refuseRestrictedAnswerMove(ctx, q, folders.ContentDashboard, to, func() (keptWithAdmins, error) {
		row, err := q.DashboardKeptWithAdmins(ctx, id)
		return keptWithAdmins(row), err
	}, func() (folders.AnswerRef, error) {
		row, err := q.DashboardRestrictedRun(ctx, id)
		return answerRef(ctx, answerRun{row.ID, row.Prompt, row.RequestedByKind, row.RequestedByID}, err)
	}); err != nil {
		return err
	}
	refs, err := dashboardRefs(ctx, q, id)
	if err != nil {
		return err
	}
	return g.Move(ctx, q, folders.ContentDashboard, to, refs)
}

func dashboardRefs(ctx context.Context, q *dbsqlc.Queries, id pgtype.UUID) ([]string, error) {
	specs, err := q.PlacementDashboardSpecs(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("read dashboard specs: %w", err)
	}
	var refs []string
	for _, raw := range specs {
		found, err := SpecRefs(raw)
		if err != nil {
			return nil, err
		}
		refs = union(refs, found)
	}
	return refs, nil
}

func (g *Guard) DashboardDatasource(ctx context.Context, q *dbsqlc.Queries, id pgtype.UUID) (string, error) {
	refs, err := dashboardRefs(ctx, q, id)
	if err != nil {
		return "", err
	}
	ds, _, err := g.Restricted(ctx, q, refs)
	return ds, err
}

func (g *Guard) ObservationDatasource(ctx context.Context, q *dbsqlc.Queries, id pgtype.UUID) (string, error) {
	refs, err := observationRefs(ctx, q, id)
	if err != nil {
		return "", err
	}
	ds, _, err := g.Restricted(ctx, q, refs)
	return ds, err
}

func (g *Guard) MoveObservation(ctx context.Context, q *dbsqlc.Queries, id, to pgtype.UUID) error {
	if err := refuseLinkedObservationMove(ctx, q, id, to); err != nil {
		return err
	}
	if err := refuseRestrictedAnswerMove(ctx, q, folders.ContentObservation, to, observationKept(ctx, q, id), func() (folders.AnswerRef, error) {
		row, err := q.ObservationRestrictedRun(ctx, id)
		return answerRef(ctx, answerRun{row.ID, row.Prompt, row.RequestedByKind, row.RequestedByID}, err)
	}); err != nil {
		return err
	}
	refs, err := observationRefs(ctx, q, id)
	if err != nil {
		return err
	}
	return g.Move(ctx, q, folders.ContentObservation, to, refs)
}

func observationRefs(ctx context.Context, q *dbsqlc.Queries, id pgtype.UUID) ([]string, error) {
	bodies, err := q.PlacementObservationPanels(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("read observation panels: %w", err)
	}
	var refs []string
	for _, raw := range bodies {
		var body panelBody
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, fmt.Errorf("decode panel block: %w", err)
		}
		found, err := PanelRefs(body.Panel, body.Variables)
		if err != nil {
			return nil, err
		}
		refs = union(refs, found)
	}
	snaps, err := q.PlacementObservationSnapshots(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("read observation snapshots: %w", err)
	}
	for _, s := range snaps {
		found, err := SnapshotRefs(s.Panel, s.Query, s.DatasourceUids)
		if err != nil {
			return nil, err
		}
		refs = union(refs, found)
	}
	return refs, nil
}

func refuseLinkedMove(ctx context.Context, q *dbsqlc.Queries, id, to pgtype.UUID) error {
	return refuseLinked(ctx, q, to, folders.ErrLinkedMove, func() (bool, error) { return q.DashboardHasLivePublicLinks(ctx, id) })
}

func refuseLinkedObservationMove(ctx context.Context, q *dbsqlc.Queries, id, to pgtype.UUID) error {
	return refuseLinked(ctx, q, to, folders.ErrLinkedObservationMove, func() (bool, error) { return q.ObservationHasLivePublicLinks(ctx, id) })
}

func refuseLinked(ctx context.Context, q *dbsqlc.Queries, to pgtype.UUID, refusal error, linked func() (bool, error)) error {
	folder, err := q.ShareFolderPlacement(ctx, to)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !folder.AdminsOnly {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock folder: %w", err)
	}
	has, err := linked()
	if err != nil {
		return fmt.Errorf("read public links: %w", err)
	}
	if has {
		return refusal
	}
	return nil
}
