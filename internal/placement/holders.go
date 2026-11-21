package placement

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	MaxListed = 20
	pageSize  = 200
)

type Holder struct {
	folders.ContentItem
	Datasource string
	Personal   bool
}

type scope struct {
	uids       []string
	folderID   pgtype.UUID
	sharedOnly bool
	limit      int32
}

type found struct {
	scope
	items []Holder
	seen  map[string]bool
}

func (f *found) full() bool { return len(f.items) >= int(f.limit) }

type place struct {
	folderID    pgtype.UUID
	folderTitle string
	personal    bool
}

func (f *found) add(kind folders.ContentKind, id pgtype.UUID, title string, at place, refs []string) {
	key := string(kind) + ":" + id.String()
	if f.seen[key] || f.full() {
		return
	}
	ds, restricted := queryproxy.FirstRestricted(refs, func(uid string) bool { return slices.Contains(f.uids, uid) })
	if !restricted {
		return
	}
	f.seen[key] = true
	f.items = append(f.items, Holder{
		ContentItem: folders.ContentItem{Kind: kind, ID: id.String(), Title: title, FolderID: at.folderID.String(), FolderTitle: at.folderTitle},
		Datasource:  ds,
		Personal:    at.personal,
	})
}

func (g *Guard) adminsOnlyUIDs(ctx context.Context, q *dbsqlc.Queries) ([]string, error) {
	stored, err := q.AdminsOnlyDatasourceUIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list Admins-only datasources: %w", err)
	}
	uids := stored
	if g != nil && g.reg != nil {
		for _, uid := range g.reg.AdminsOnlyUIDs() {
			if !slices.Contains(uids, uid) {
				uids = append(uids, uid)
			}
		}
	}
	return uids, nil
}

func (g *Guard) FolderHolder(ctx context.Context, q *dbsqlc.Queries, folderID pgtype.UUID) (Holder, bool, error) {
	uids, err := g.adminsOnlyUIDs(ctx, q)
	if err != nil || len(uids) == 0 {
		return Holder{}, false, err
	}
	items, err := scan(ctx, q, scope{uids: uids, folderID: folderID, limit: 1})
	if err != nil || len(items) == 0 {
		return Holder{}, false, err
	}
	return items[0], true, nil
}

func SharedHolders(ctx context.Context, q *dbsqlc.Queries, uid string) ([]Holder, error) {
	return scan(ctx, q, scope{uids: []string{uid}, sharedOnly: true, limit: MaxListed})
}

func (g *Guard) OpeningGuard() folders.OpeningGuard {
	return func(ctx context.Context, q *dbsqlc.Queries, folder dbsqlc.Folder) error {
		h, held, err := g.FolderHolder(ctx, q, folder.ID)
		if err != nil || !held {
			return err
		}
		return folders.RestrictedOpening(h.ContentItem, h.Datasource)
	}
}

func scan(ctx context.Context, q *dbsqlc.Queries, s scope) ([]Holder, error) {
	f := &found{scope: s, seen: map[string]bool{}}
	for _, step := range []func(context.Context, *dbsqlc.Queries, *found) error{scanSavedSearches, scanDashboards, scanProposals, scanPanelBlocks, scanSnapshots} {
		if f.full() {
			break
		}
		if err := step(ctx, q, f); err != nil {
			return nil, err
		}
	}
	return f.items, nil
}

func scanSavedSearches(ctx context.Context, q *dbsqlc.Queries, f *found) error {
	rows, err := q.PlacementSavedSearches(ctx, dbsqlc.PlacementSavedSearchesParams{Uids: f.uids, FolderID: f.folderID, SharedOnly: f.sharedOnly, PageSize: f.limit})
	if err != nil {
		return fmt.Errorf("scan saved searches: %w", err)
	}
	for _, r := range rows {
		f.add(folders.ContentSavedSearch, r.ID, r.Title, place{r.FolderID, r.FolderTitle, r.FolderPersonal}, []string{r.DatasourceUid})
	}
	return nil
}

func scanDashboards(ctx context.Context, q *dbsqlc.Queries, f *found) error {
	after, seq := pgtype.UUID{Valid: true}, int32(0)
	for !f.full() {
		rows, err := q.PlacementDashboardVersions(ctx, dbsqlc.PlacementDashboardVersionsParams{
			FolderID: f.folderID, SharedOnly: f.sharedOnly, AfterID: after, AfterSeq: seq, Uids: f.uids, PageSize: pageSize,
		})
		if err != nil {
			return fmt.Errorf("scan dashboards: %w", err)
		}
		for _, r := range rows {
			refs, err := SpecRefs(r.Payload)
			if err != nil {
				return err
			}
			f.add(folders.ContentDashboard, r.ID, r.Title, place{r.FolderID, r.FolderTitle, r.FolderPersonal}, refs)
			after, seq = r.ID, r.Seq
		}
		if len(rows) < pageSize {
			return nil
		}
	}
	return nil
}

func scanProposals(ctx context.Context, q *dbsqlc.Queries, f *found) error {
	after, seq := pgtype.UUID{Valid: true}, int32(0)
	for !f.full() {
		rows, err := q.PlacementProposalRevisions(ctx, dbsqlc.PlacementProposalRevisionsParams{
			FolderID: f.folderID, SharedOnly: f.sharedOnly, AfterID: after, AfterSeq: seq, Uids: f.uids, PageSize: pageSize,
		})
		if err != nil {
			return fmt.Errorf("scan proposals: %w", err)
		}
		for _, r := range rows {
			refs, err := SpecRefs(r.Payload)
			if err != nil {
				return err
			}
			f.add(folders.ContentProposal, r.ID, r.Title, place{r.FolderID, r.FolderTitle, r.FolderPersonal}, refs)
			after, seq = r.ID, r.Seq
		}
		if len(rows) < pageSize {
			return nil
		}
	}
	return nil
}

type panelBody struct {
	Panel     json.RawMessage `json:"panel"`
	Variables []spec.Variable `json:"variables"`
}

func scanPanelBlocks(ctx context.Context, q *dbsqlc.Queries, f *found) error {
	after := pgtype.UUID{Valid: true}
	for !f.full() {
		rows, err := q.PlacementPanelBlocks(ctx, dbsqlc.PlacementPanelBlocksParams{
			FolderID: f.folderID, SharedOnly: f.sharedOnly, AfterID: after, Uids: f.uids, PageSize: pageSize,
		})
		if err != nil {
			return fmt.Errorf("scan panel blocks: %w", err)
		}
		for _, r := range rows {
			var body panelBody
			if err := json.Unmarshal(r.Payload, &body); err != nil {
				return fmt.Errorf("decode panel block: %w", err)
			}
			refs, err := PanelRefs(body.Panel, body.Variables)
			if err != nil {
				return err
			}
			f.add(folders.ContentObservation, r.ID, r.Title, place{r.FolderID, r.FolderTitle, r.FolderPersonal}, refs)
			after = r.BlockID
		}
		if len(rows) < pageSize {
			return nil
		}
	}
	return nil
}

func scanSnapshots(ctx context.Context, q *dbsqlc.Queries, f *found) error {
	after := pgtype.UUID{Valid: true}
	for !f.full() {
		rows, err := q.PlacementSnapshots(ctx, dbsqlc.PlacementSnapshotsParams{
			FolderID: f.folderID, SharedOnly: f.sharedOnly, AfterID: after, Uids: f.uids, PageSize: pageSize,
		})
		if err != nil {
			return fmt.Errorf("scan snapshots: %w", err)
		}
		for _, r := range rows {
			refs, err := SnapshotRefs(r.Panel, r.Query, r.DatasourceUids)
			if err != nil {
				return err
			}
			f.add(folders.ContentObservation, r.ID, r.Title, place{r.FolderID, r.FolderTitle, r.FolderPersonal}, refs)
			after = r.SnapshotID
		}
		if len(rows) < pageSize {
			return nil
		}
	}
	return nil
}
