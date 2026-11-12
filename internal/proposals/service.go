package proposals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/db"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/grantmode"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/lifecycle"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	defaultLimit = 50
	maxLimit     = 100

	dashboardForeignKey = "proposals_dashboard_id_fkey"

	StateFilterAll = "all"
)

type Service struct {
	db         *db.DB
	access     *folders.Access
	dashboards *dashboards.Service
	logger     *slog.Logger
}

func New(database *db.DB, access *folders.Access, dashboardService *dashboards.Service, logger *slog.Logger) *Service {
	return &Service{db: database, access: access, dashboards: dashboardService, logger: logger}
}

type Proposal struct {
	Item
	Spec      *spec.Dashboard
	Changes   []spec.UnitChange
	Conflicts []spec.Conflict
}

type ListQuery struct {
	DashboardID pgtype.UUID
	State       string
	Limit       int
	Cursor      string
}

type Page struct {
	Items      []Item
	NextCursor string
}

func (s *Service) dashboardAccess(ctx context.Context, by auth.Principal, id pgtype.UUID) (folders.DashboardAccess, error) {
	access, err := s.access.Dashboard(ctx, by, id)
	if errors.Is(err, folders.ErrNotFound) || errors.Is(err, folders.ErrNoAccess) {
		return folders.DashboardAccess{}, dashboards.ErrNotFound
	}
	return access, err
}

func (s *Service) load(ctx context.Context, by auth.Principal, id pgtype.UUID) (head, folders.DashboardAccess, error) {
	h, err := s.head(ctx, id)
	if err != nil {
		return head{}, folders.DashboardAccess{}, err
	}
	access, err := s.dashboardAccess(ctx, by, h.row.DashboardID)
	if errors.Is(err, dashboards.ErrNotFound) {
		return head{}, folders.DashboardAccess{}, ErrNotFound
	}
	return h, access, err
}

func (s *Service) Access(ctx context.Context, by auth.Principal, id pgtype.UUID) (pgtype.UUID, folders.Abilities, error) {
	h, access, err := s.load(ctx, by, id)
	if err != nil {
		return pgtype.UUID{}, folders.Abilities{}, err
	}
	return h.row.DashboardID, access.Abilities, nil
}

func (s *Service) Propose(ctx context.Context, by auth.Principal, in dashboards.ProposeInput) (dashboards.ProposalRef, error) {
	access, err := s.dashboardAccess(ctx, by, in.DashboardID)
	switch {
	case err != nil:
		return dashboards.ProposalRef{}, err
	case !access.Abilities.Comment:
		return dashboards.ProposalRef{}, ErrForbidden
	}
	saveBase, err := s.dashboards.VersionSpec(ctx, in.DashboardID, in.BaseVersion)
	if errors.Is(err, dashboards.ErrNotFound) {
		return dashboards.ProposalRef{}, &dashboards.InvalidInputError{Message: "parent_version is not a version of this dashboard"}
	}
	if err != nil {
		return dashboards.ProposalRef{}, err
	}
	if err := s.dashboards.CheckProposal(ctx, by, in.DashboardID, in.BaseVersion, in.Spec, in.Partial); err != nil {
		return dashboards.ProposalRef{}, err
	}
	who := actor.Of(by, origin.From(ctx), in.RequestID.String())
	cols, err := who.Columns()
	if err != nil {
		return dashboards.ProposalRef{}, err
	}
	for range maxProposeTrials {
		ref, err := s.proposeOnce(ctx, in, saveBase, who, cols)
		if !errors.Is(err, errRetry) {
			return ref, err
		}
	}
	return dashboards.ProposalRef{}, fmt.Errorf("proposal on dashboard %s kept changing: %w", in.DashboardID, errRetry)
}

type revisionContent struct {
	spec    []byte
	base    int
	summary string
}

func (s *Service) proposeOnce(ctx context.Context, in dashboards.ProposeInput, saveBase *spec.Dashboard, who actor.Actor, cols actor.Columns) (dashboards.ProposalRef, error) {
	next, observed, err := s.contentFor(ctx, in, saveBase, cols)
	if err != nil {
		return dashboards.ProposalRef{}, err
	}
	var ref dashboards.ProposalRef
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		locked, found, err := lockOpen(ctx, q, in.DashboardID, cols)
		switch {
		case err != nil:
			return err
		case !found && observed == nil:
			ref, err = insertProposal(ctx, q, in, next, who, cols)
			return err
		case !found || observed == nil || locked.revision != observed.row.Revision:
			return errRetry
		}
		ref, err = addRevision(ctx, q, locked.id, observed.row.Title, in.DashboardID, next, who)
		return err
	})
	switch {
	case isUniqueViolation(err):
		return dashboards.ProposalRef{}, errRetry
	case isForeignKey(err, dashboardForeignKey):
		return dashboards.ProposalRef{}, dashboards.ErrNotFound
	case err != nil:
		return dashboards.ProposalRef{}, err
	}
	s.logger.InfoContext(ctx, "proposal saved",
		slog.String("actor_id", who.ID), slog.String("proposal_id", ref.ID), slog.String("dashboard_id", in.DashboardID.String()),
		slog.Int("revision", ref.Revision), slog.Bool("opened", ref.Opened))
	return ref, nil
}

func (s *Service) contentFor(ctx context.Context, in dashboards.ProposeInput, saveBase *spec.Dashboard, cols actor.Columns) (revisionContent, *head, error) {
	raw, err := json.Marshal(in.Spec)
	if err != nil {
		return revisionContent{}, nil, fmt.Errorf("marshal proposed spec: %w", err)
	}
	next := revisionContent{spec: raw, base: in.BaseVersion, summary: in.Summary}
	found, ok, err := s.findOpen(ctx, in.DashboardID, cols)
	if err != nil || !ok {
		return next, nil, err
	}
	h, err := s.head(ctx, found.id)
	if err != nil {
		return revisionContent{}, nil, err
	}
	if State(h.row.State) != StateOpen {
		return revisionContent{}, nil, errRetry
	}
	folded, base, err := s.foldOnto(ctx, in, saveBase, h)
	if err != nil {
		return revisionContent{}, nil, err
	}
	if next.spec, err = json.Marshal(folded); err != nil {
		return revisionContent{}, nil, fmt.Errorf("marshal folded spec: %w", err)
	}
	next.base = base
	return next, &h, nil
}

func (s *Service) foldOnto(ctx context.Context, in dashboards.ProposeInput, saveBase *spec.Dashboard, h head) (*spec.Dashboard, int, error) {
	var headSpec spec.Dashboard
	if err := json.Unmarshal(h.spec, &headSpec); err != nil {
		return nil, 0, fmt.Errorf("decode proposal spec: %w", err)
	}
	headBase := saveBase
	if h.baseVersion != in.BaseVersion {
		var err error
		if headBase, err = s.dashboards.VersionSpec(ctx, in.DashboardID, h.baseVersion); err != nil {
			return nil, 0, err
		}
	}
	return fold(headBase, &headSpec, saveBase, in.Spec, h.baseVersion, in.BaseVersion)
}

func insertProposal(ctx context.Context, q *dbsqlc.Queries, in dashboards.ProposeInput, next revisionContent, who actor.Actor, cols actor.Columns) (dashboards.ProposalRef, error) {
	row, err := q.InsertProposal(ctx, dbsqlc.InsertProposalParams{
		DashboardID:  in.DashboardID,
		Title:        truncate(next.summary, maxTitleRunes),
		AuthorKind:   cols.Kind,
		AuthorID:     cols.ID,
		AuthorName:   cols.Name,
		Origin:       cols.Origin,
		OriginClient: cols.Client,
		RunID:        cols.RunID,
	})
	if err != nil {
		return dashboards.ProposalRef{}, fmt.Errorf("insert proposal: %w", err)
	}
	if err := insertRevision(ctx, q, row.ID, int(row.Revision), next); err != nil {
		return dashboards.ProposalRef{}, err
	}
	base := next.base
	if err := appendEvent(ctx, q, activity.TypeProposalOpened, who, row.ID, in.DashboardID, eventData{Title: row.Title, Revision: int(row.Revision), BaseVersion: &base}); err != nil {
		return dashboards.ProposalRef{}, err
	}
	return dashboards.ProposalRef{ID: row.ID.String(), Revision: int(row.Revision), Opened: true}, nil
}

func addRevision(ctx context.Context, q *dbsqlc.Queries, id pgtype.UUID, title string, dashboardID pgtype.UUID, next revisionContent, who actor.Actor) (dashboards.ProposalRef, error) {
	revision, err := q.AdvanceProposalRevision(ctx, id)
	if err != nil {
		return dashboards.ProposalRef{}, fmt.Errorf("advance proposal revision: %w", err)
	}
	if err := insertRevision(ctx, q, id, int(revision), next); err != nil {
		return dashboards.ProposalRef{}, err
	}
	base := next.base
	if err := appendEvent(ctx, q, activity.TypeProposalUpdated, who, id, dashboardID, eventData{Title: title, Revision: int(revision), BaseVersion: &base}); err != nil {
		return dashboards.ProposalRef{}, err
	}
	return dashboards.ProposalRef{ID: id.String(), Revision: int(revision)}, nil
}

func insertRevision(ctx context.Context, q *dbsqlc.Queries, id pgtype.UUID, revision int, next revisionContent) error {
	err := q.InsertProposalRevision(ctx, dbsqlc.InsertProposalRevisionParams{
		ProposalID:  id,
		Revision:    httpapi.Int32(revision),
		BaseVersion: httpapi.Int32(next.base),
		Spec:        next.spec,
		Summary:     next.summary,
	})
	if err != nil {
		return fmt.Errorf("insert proposal revision: %w", err)
	}
	return nil
}

func (s *Service) itemFor(ctx context.Context, by auth.Principal, h head, abilities folders.Abilities, latest int) Item {
	item := itemOf(h.row, h.baseVersion, h.summary, h.merged)
	item.LatestVersion = latest
	item.Behind = latest > item.BaseVersion
	item.AllowedActions = allowedActions(factsOf(ctx, by, h.row, abilities))
	return item
}

func factsOf(ctx context.Context, by auth.Principal, row dbsqlc.Proposal, abilities folders.Abilities) facts {
	return facts{
		state:   State(row.State),
		author:  string(by.Kind) == row.AuthorKind && by.ID == row.AuthorID.String(),
		edit:    abilities.Edit,
		propose: grantmode.From(ctx) == grantmode.Propose,
	}
}

func (s *Service) Get(ctx context.Context, by auth.Principal, id pgtype.UUID) (Proposal, error) {
	h, access, err := s.load(ctx, by, id)
	if err != nil {
		return Proposal{}, err
	}
	_, latest, err := s.dashboards.Get(ctx, h.row.DashboardID)
	if errors.Is(err, dashboards.ErrNotFound) {
		return Proposal{}, ErrNotFound
	}
	if err != nil {
		return Proposal{}, err
	}
	var headSpec spec.Dashboard
	if err := json.Unmarshal(h.spec, &headSpec); err != nil {
		return Proposal{}, fmt.Errorf("decode proposal spec: %w", err)
	}
	base, err := s.dashboards.VersionSpec(ctx, h.row.DashboardID, h.baseVersion)
	if err != nil {
		return Proposal{}, err
	}
	p := Proposal{
		Item:      s.itemFor(ctx, by, h, access.Abilities, latest.Number),
		Spec:      &headSpec,
		Changes:   spec.Changes(base, &headSpec),
		Conflicts: []spec.Conflict{},
	}
	if p.State == StateOpen && p.Behind {
		merge, err := spec.Merge3(base, latest.Spec, &headSpec)
		if err != nil {
			return Proposal{}, fmt.Errorf("merge proposal onto the latest version: %w", err)
		}
		p.Conflicts = append(p.Conflicts, merge.Conflicts...)
	}
	return p, nil
}

func (s *Service) List(ctx context.Context, by auth.Principal, q ListQuery) (Page, error) {
	access, err := s.dashboardAccess(ctx, by, q.DashboardID)
	if err != nil {
		return Page{}, err
	}
	params := dbsqlc.ListDashboardProposalsParams{DashboardID: q.DashboardID}
	switch q.State {
	case "", string(StateOpen):
		params.State = pgtype.Text{String: string(StateOpen), Valid: true}
	case string(StateMerged), string(StateDiscarded):
		params.State = pgtype.Text{String: q.State, Valid: true}
	case StateFilterAll:
	default:
		return Page{}, &dashboards.InvalidInputError{Message: "state must be open, merged, discarded or all"}
	}
	limit := q.Limit
	if limit < 1 || limit > maxLimit {
		limit = defaultLimit
	}
	params.RowLimit = httpapi.Int32(limit + 1)
	after, ok := httpapi.DecodeTimeCursor(q.Cursor)
	if !ok {
		return Page{}, dashboards.ErrInvalidCursor
	}
	if after != nil {
		params.BeforeAt = pgtype.Timestamptz{Time: after.Time, Valid: true}
		params.BeforeID = after.ID
	}
	rows, err := s.db.Queries().ListDashboardProposals(ctx, params)
	if err != nil {
		return Page{}, fmt.Errorf("list proposals: %w", err)
	}
	latest, err := s.dashboards.LatestVersion(ctx, q.DashboardID)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: make([]Item, 0, min(len(rows), limit))}
	for i, r := range rows {
		if i == limit {
			last := page.Items[limit-1]
			page.NextCursor = httpapi.EncodeTimeCursor(last.CreatedAt, last.ID)
			break
		}
		h := head{row: r.Proposal, baseVersion: int(r.BaseVersion), summary: r.Summary, merged: r.MergedVersion}
		page.Items = append(page.Items, s.itemFor(ctx, by, h, access.Abilities, latest))
	}
	return page, nil
}

func checkChoices(choices []spec.Choice) error {
	for _, c := range choices {
		switch c.Kind {
		case spec.UnitPanel, spec.UnitVariable, spec.UnitSetting:
		default:
			return &dashboards.InvalidInputError{Message: "a choice's kind must be panel, variable or setting"}
		}
		if c.Take != spec.SideOurs && c.Take != spec.SideTheirs {
			return &dashboards.InvalidInputError{Message: "a choice's take must be ours or theirs"}
		}
	}
	return nil
}

func (s *Service) Merge(ctx context.Context, by auth.Principal, id pgtype.UUID, revision int, choices []spec.Choice) (Proposal, error) {
	h, access, err := s.load(ctx, by, id)
	if err != nil {
		return Proposal{}, err
	}
	f := factsOf(ctx, by, h.row, access.Abilities)
	switch {
	case f.state != StateOpen:
		return Proposal{}, ProposalMachine.Refuse(f.state, ActionMerge, f.can)
	case f.propose:
		return Proposal{}, ErrProposeMode
	case !f.edit:
		return Proposal{}, ErrForbidden
	case revision != int(h.row.Revision):
		return Proposal{}, &ChangedError{Revision: int(h.row.Revision)}
	}
	if err := checkChoices(choices); err != nil {
		return Proposal{}, err
	}
	who := actor.Of(by, origin.From(ctx), "")
	cols, err := who.Columns()
	if err != nil {
		return Proposal{}, err
	}
	merged := func(ctx context.Context, q *dbsqlc.Queries, version *int) error {
		if err := closeProposal(ctx, q, h, StateMerged, cols); err != nil {
			return err
		}
		return appendEvent(ctx, q, activity.TypeProposalMerged, who, h.row.ID, h.row.DashboardID, mergedEventData{Title: h.row.Title, Revision: int(h.row.Revision), Version: version})
	}
	res, err := s.dashboards.Save(ctx, by, dashboards.SaveInput{
		DashboardID:    h.row.DashboardID,
		Parent:         h.baseVersion,
		Spec:           h.spec,
		Summary:        h.summary,
		Partial:        true,
		Merge:          true,
		Choices:        choices,
		MergedProposal: h.row.ID,
		InTx: func(q *dbsqlc.Queries, version int) error {
			return merged(ctx, q, &version)
		},
	})
	if err == nil && res.Replayed {
		err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error { return merged(ctx, q, nil) })
	}
	if errors.Is(err, errLostClose) {
		return Proposal{}, s.lost(ctx, id, f.can, ActionMerge)
	}
	if err != nil {
		return Proposal{}, err
	}
	s.logger.InfoContext(ctx, "proposal merged",
		slog.String("actor_id", by.ID), slog.String("proposal_id", id.String()), slog.String("dashboard_id", h.row.DashboardID.String()))
	return s.Get(ctx, by, id)
}

func (s *Service) lost(ctx context.Context, id pgtype.UUID, can func(lifecycle.Who) bool, a Action) error {
	h, err := s.head(ctx, id)
	if err != nil {
		return err
	}
	if State(h.row.State) != StateOpen {
		return ProposalMachine.Refuse(State(h.row.State), a, can)
	}
	return &ChangedError{Revision: int(h.row.Revision)}
}

func (s *Service) Discard(ctx context.Context, by auth.Principal, id pgtype.UUID) (Proposal, error) {
	h, access, err := s.load(ctx, by, id)
	if err != nil {
		return Proposal{}, err
	}
	f := factsOf(ctx, by, h.row, access.Abilities)
	switch {
	case f.state != StateOpen:
		return Proposal{}, ProposalMachine.Refuse(f.state, ActionDiscard, f.can)
	case !slices.Contains(allowedActions(f), ActionDiscard):
		return Proposal{}, ErrForbidden
	}
	who := actor.Of(by, origin.From(ctx), "")
	cols, err := who.Columns()
	if err != nil {
		return Proposal{}, err
	}
	err = s.db.InTx(ctx, func(q *dbsqlc.Queries) error {
		if err := closeProposal(ctx, q, h, StateDiscarded, cols); err != nil {
			return err
		}
		return appendEvent(ctx, q, activity.TypeProposalDiscarded, who, h.row.ID, h.row.DashboardID, eventData{Title: h.row.Title, Revision: int(h.row.Revision)})
	})
	if errors.Is(err, errLostClose) {
		return Proposal{}, s.lost(ctx, id, f.can, ActionDiscard)
	}
	if err != nil {
		return Proposal{}, err
	}
	s.logger.InfoContext(ctx, "proposal discarded",
		slog.String("actor_id", by.ID), slog.String("proposal_id", id.String()), slog.String("dashboard_id", h.row.DashboardID.String()))
	return s.Get(ctx, by, id)
}
