package placement

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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
}

type Census struct {
	Listed   []Holder
	Shared   int
	Personal int
}

func (c Census) Held() bool { return c.Shared+c.Personal > 0 }

func (c Census) Unlisted() int { return c.Shared - len(c.Listed) }

type scope struct {
	uids       []string
	folderID   pgtype.UUID
	sharedOnly bool
	hidePersonal bool
	keep         int
	stopAt       int
}

type found struct {
	scope
	Census
}

func (f *found) done() bool { return f.stopAt > 0 && f.Shared+f.Personal >= f.stopAt }

type place struct {
	folderID    pgtype.UUID
	folderTitle string
	personal    bool
}

func (f *found) hold(kind folders.ContentKind, id pgtype.UUID, title string, at place, refs []string) bool {
	ds, restricted := queryproxy.FirstRestricted(refs, func(uid string) bool { return slices.Contains(f.uids, uid) })
	if !restricted {
		return false
	}
	if at.personal && f.hidePersonal {
		f.Personal++
		return true
	}
	f.Shared++
	if len(f.Listed) < f.keep {
		f.Listed = append(f.Listed, Holder{
			ContentItem: folders.ContentItem{Kind: kind, ID: id.String(), Title: title, FolderID: at.folderID.String(), FolderTitle: at.folderTitle},
			Datasource:  ds,
		})
	}
	return true
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
	c, err := scan(ctx, q, scope{uids: uids, folderID: folderID, keep: 1, stopAt: 1})
	if err != nil || len(c.Listed) == 0 {
		return Holder{}, false, err
	}
	return c.Listed[0], true, nil
}

func SharedCensus(ctx context.Context, q *dbsqlc.Queries, uid string) (Census, error) {
	return scan(ctx, q, scope{uids: []string{uid}, sharedOnly: true, hidePersonal: true, keep: MaxListed})
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

func scan(ctx context.Context, q *dbsqlc.Queries, s scope) (Census, error) {
	f := &found{scope: s}
	for _, step := range []func(context.Context, *dbsqlc.Queries, *found) error{scanSavedSearches, scanDashboards, scanProposals, scanObservations} {
		if f.done() {
			break
		}
		if err := step(ctx, q, f); err != nil {
			return Census{}, err
		}
	}
	return f.Census, nil
}

func scanSavedSearches(ctx context.Context, q *dbsqlc.Queries, f *found) error {
	after := pgtype.UUID{Valid: true}
	for !f.done() {
		rows, err := q.PlacementSavedSearches(ctx, dbsqlc.PlacementSavedSearchesParams{Uids: f.uids, FolderID: f.folderID, SharedOnly: f.sharedOnly, AfterID: after, PageSize: pageSize})
		if err != nil {
			return fmt.Errorf("scan saved searches: %w", err)
		}
		for _, r := range rows {
			f.hold(folders.ContentSavedSearch, r.ID, r.Title, place{r.FolderID, r.FolderTitle, r.FolderPersonal}, []string{r.DatasourceUid})
			after = r.ID
			if f.done() {
				return nil
			}
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

type versionRow struct {
	id      pgtype.UUID
	seq     int32
	title   string
	at      place
	payload []byte
}

type versionPage func(after pgtype.UUID, seq int32) ([]versionRow, error)

func scanVersions(f *found, kind folders.ContentKind, page versionPage) error {
	after, seq := pgtype.UUID{Valid: true}, int32(0)
	for !f.done() {
		rows, err := page(after, seq)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if r.id == after && seq == math.MaxInt32 {
				continue
			}
			refs, err := SpecRefs(r.payload)
			if err != nil {
				return err
			}
			after, seq = r.id, r.seq
			if f.hold(kind, r.id, r.title, r.at, refs) {
				seq = math.MaxInt32
			}
			if f.done() {
				return nil
			}
		}
		if len(rows) < pageSize {
			return nil
		}
	}
	return nil
}

func scanDashboards(ctx context.Context, q *dbsqlc.Queries, f *found) error {
	return scanVersions(f, folders.ContentDashboard, func(after pgtype.UUID, seq int32) ([]versionRow, error) {
		rows, err := q.PlacementDashboardVersions(ctx, dbsqlc.PlacementDashboardVersionsParams{
			FolderID: f.folderID, SharedOnly: f.sharedOnly, AfterID: after, AfterSeq: seq, Uids: f.uids, PageSize: pageSize,
		})
		if err != nil {
			return nil, fmt.Errorf("scan dashboards: %w", err)
		}
		out := make([]versionRow, len(rows))
		for i, r := range rows {
			out[i] = versionRow{r.ID, r.Seq, r.Title, place{r.FolderID, r.FolderTitle, r.FolderPersonal}, r.Payload}
		}
		return out, nil
	})
}

func scanProposals(ctx context.Context, q *dbsqlc.Queries, f *found) error {
	return scanVersions(f, folders.ContentProposal, func(after pgtype.UUID, seq int32) ([]versionRow, error) {
		rows, err := q.PlacementProposalRevisions(ctx, dbsqlc.PlacementProposalRevisionsParams{
			FolderID: f.folderID, SharedOnly: f.sharedOnly, AfterID: after, AfterSeq: seq, Uids: f.uids, PageSize: pageSize,
		})
		if err != nil {
			return nil, fmt.Errorf("scan proposals: %w", err)
		}
		out := make([]versionRow, len(rows))
		for i, r := range rows {
			out[i] = versionRow{r.ID, r.Seq, r.Title, place{r.FolderID, r.FolderTitle, r.FolderPersonal}, r.Payload}
		}
		return out, nil
	})
}

func scanObservations(ctx context.Context, q *dbsqlc.Queries, f *found) error {
	after := pgtype.UUID{Valid: true}
	for !f.done() {
		rows, err := q.PlacementObservations(ctx, dbsqlc.PlacementObservationsParams{
			FolderID: f.folderID, SharedOnly: f.sharedOnly, AfterID: after, Uids: f.uids, PageSize: pageSize,
		})
		if err != nil {
			return fmt.Errorf("scan observations: %w", err)
		}
		for _, r := range rows {
			refs, err := observationRefs(ctx, q, r.ID)
			if err != nil {
				return err
			}
			f.hold(folders.ContentObservation, r.ID, r.Title, place{r.FolderID, r.FolderTitle, r.FolderPersonal}, refs)
			after = r.ID
			if f.done() {
				return nil
			}
		}
		if len(rows) < pageSize {
			return nil
		}
	}
	return nil
}
