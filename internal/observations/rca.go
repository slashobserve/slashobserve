package observations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	MaxRCARevisions = 50

	CodePanelFailed        = "panel_failed"
	CodeRCALimit           = "rca_limit"
	CodeRCAShared          = "rca_shared"
	CodeRCAUnchanged       = "rca_unchanged"
	CodeObservationChanged = "observation_changed"
	codeQueryFailed        = "query_failed"
	panelFailedMessage     = "a panel could not be frozen"

	publishTimeout     = 2 * time.Minute
	publishConcurrency = 4
)

var (
	ErrRevisionNotFound   = errors.New("RCA revision not found")
	ErrRCALimit           = errors.New("an observation keeps at most 50 RCA revisions")
	ErrRCAShared          = errors.New("this revision still has public links; revoke them before withdrawing it")
	ErrObservationChanged = errors.New("the observation changed while it was being published; publish again")
	ErrPublishPersonal    = errors.New("an observation in My Dashboards is private; move it to a shared folder before publishing it")
)

type RCAUnchangedError struct {
	Revision int
}

func (e *RCAUnchangedError) Error() string {
	return fmt.Sprintf("revision %d already holds this page; change the observation before publishing again", e.Revision)
}

type PanelFailedError struct {
	BlockID  string
	Position int
	Err      error
}

func (e *PanelFailedError) Error() string {
	return fmt.Sprintf("%s: block %d", panelFailedMessage, e.Position)
}

func (e *PanelFailedError) Unwrap() error {
	return e.Err
}

func (e *PanelFailedError) Issue() spec.Issue {
	issue := spec.Issue{Path: fmt.Sprintf("/blocks/%d", e.Position), Code: codeQueryFailed, Message: panelFailedMessage}
	if status, code, message := queryproxy.Classify(e.Err); status != http.StatusInternalServerError {
		issue.Code, issue.Message = spec.IssueCode(code), message
	}
	return issue
}

type RCABlock struct {
	Kind       Kind            `json:"kind"`
	Markdown   string          `json:"markdown"`
	SnapshotID string          `json:"snapshot_id"`
	Title      string          `json:"title"`
	Panel      json.RawMessage `json:"panel"`
	Width      Width           `json:"width"`
	Height     *int            `json:"height,omitempty"`
}

func (b RCABlock) MarshalJSON() ([]byte, error) {
	if b.Kind == KindText {
		return json.Marshal(struct {
			Kind     Kind   `json:"kind"`
			Markdown string `json:"markdown"`
			Width    Width  `json:"width"`
		}{b.Kind, b.Markdown, b.Width})
	}
	return json.Marshal(struct {
		Kind       Kind            `json:"kind"`
		SnapshotID string          `json:"snapshot_id"`
		Title      string          `json:"title"`
		Panel      json.RawMessage `json:"panel"`
		Width      Width           `json:"width"`
		Height     *int            `json:"height,omitempty"`
	}{b.Kind, b.SnapshotID, b.Title, b.Panel, b.Width, b.Height})
}

type rcaContent struct {
	Blocks []RCABlock `json:"blocks"`
}

type RCA struct {
	ID             string
	ObservationID  string
	Revision       int
	Title          string
	Summary        string
	State          State
	RevisionState  RCAState
	AllowedActions []RCAAction
	From, To       *time.Time
	Blocks         []RCABlock
	Actor          actor.Actor
	CreatedAt      time.Time
}

type RCASummary struct {
	ID             string
	Revision       int
	Title          string
	Summary        string
	State          State
	RevisionState  RCAState
	AllowedActions []RCAAction
	Actor          actor.Actor
	CreatedAt      time.Time
}

type publishedEventData struct {
	Title    string `json:"title"`
	Revision int    `json:"revision"`
}

type snapshotRef struct {
	id    pgtype.UUID
	panel json.RawMessage
}

func withoutQueries(panel json.RawMessage) (json.RawMessage, error) {
	rest, _, err := splitQueries(panel)
	return rest, err
}

func publishable(rows []dbsqlc.ObservationBlock) []dbsqlc.ObservationBlock {
	out := make([]dbsqlc.ObservationBlock, 0, len(rows))
	for _, b := range rows {
		if BlockState(b.State) != BlockLive {
			continue
		}
		if k := Kind(b.Kind); k == KindText || k == KindPanel || k == KindSnapshot {
			out = append(out, b)
		}
	}
	return out
}

func sameFrozenInputs(a, b dbsqlc.Observation) bool {
	return a.Title == b.Title && a.TimeFrom.Valid == b.TimeFrom.Valid && a.TimeFrom.Time.Equal(b.TimeFrom.Time) &&
		a.TimeTo.Valid == b.TimeTo.Valid && a.TimeTo.Time.Equal(b.TimeTo.Time)
}

func sameBlocks(a, b []dbsqlc.ObservationBlock) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Version != b[i].Version {
			return false
		}
	}
	return true
}

func panelTitle(panel json.RawMessage) string {
	var fields struct {
		Title string `json:"title"`
	}
	_ = json.Unmarshal(panel, &fields)
	return fields.Title
}

func rcaBlocks(blocks []dbsqlc.ObservationBlock, refs map[pgtype.UUID]snapshotRef) ([]RCABlock, error) {
	out := make([]RCABlock, 0, len(blocks))
	for _, b := range blocks {
		item := RCABlock{Kind: Kind(b.Kind), Width: Width(b.Width)}
		if item.Kind == KindText {
			var body TextBody
			if err := json.Unmarshal(b.Body, &body); err != nil {
				return nil, fmt.Errorf("decode text block: %w", err)
			}
			item.Markdown = body.Markdown
			out = append(out, item)
			continue
		}
		ref, ok := refs[b.ID]
		if !ok {
			return nil, fmt.Errorf("block %s has no snapshot", b.ID)
		}
		item.Kind = KindSnapshot
		item.SnapshotID, item.Panel, item.Title = ref.id.String(), ref.panel, panelTitle(ref.panel)
		item.Height = heightOf(b.Height)
		out = append(out, item)
	}
	return out, nil
}

func (s *Service) Publish(ctx context.Context, by auth.Principal, observationID pgtype.UUID) (RCA, error) {
	row, folder, abilities, err := s.open(ctx, by, observationID)
	if err != nil {
		return RCA{}, err
	}
	rca, err := s.publish(ctx, by, row, folder, abilities)
	if err != nil {
		viewer := actor.Of(by, origin.Origin{}, "")
		return RCA{}, refusedPublish(err, State(row.State), observationOf(row, folder, abilities, viewer, pgtype.UUID{}, 0).AllowedActions)
	}
	rca.RevisionState = RCAPublished
	rca.AllowedActions = allowedRCAActions(RCAPublished, folder, abilities)
	return rca, nil
}

func (s *Service) publish(ctx context.Context, by auth.Principal, row dbsqlc.Observation, folder folders.Folder, abilities folders.Abilities) (RCA, error) {
	observationID := row.ID
	if err := permit(Machine, State(row.State), ActionPublish, can(abilities, false)); err != nil {
		return RCA{}, err
	}
	if folder.Personal {
		return RCA{}, ErrPublishPersonal
	}
	revisions, err := s.store.db.Queries().NextRCARevision(ctx, observationID)
	if err != nil {
		return RCA{}, fmt.Errorf("count RCA revisions: %w", err)
	}
	if revisions.Total >= MaxRCARevisions {
		return RCA{}, ErrRCALimit
	}
	rows, err := s.store.blocks(ctx, observationID)
	if err != nil {
		return RCA{}, err
	}
	live := publishable(rows)
	frozen, err := s.freezePanels(ctx, by, folder, observationID, live)
	if err != nil {
		return RCA{}, err
	}
	var published dbsqlc.ObservationRca
	err = s.write(ctx, by, observationID, pgtype.UUID{}, func(q *dbsqlc.Queries, locked dbsqlc.Observation, abilities folders.Abilities, who actor.Actor) error {
		if err := permit(Machine, State(locked.State), ActionPublish, can(abilities, false)); err != nil {
			return err
		}
		if !sameFrozenInputs(row, locked) {
			return ErrObservationChanged
		}
		var err error
		published, err = s.insertRCA(ctx, q, locked, live, frozen, who)
		return err
	})
	if err != nil {
		return RCA{}, err
	}
	s.logger.InfoContext(ctx, "observation published",
		slog.String("actor_id", by.ID), slog.String("observation_id", observationID.String()), slog.Int("revision", int(published.Revision)))
	return rcaOf(published)
}

func (s *Service) freezePanels(ctx context.Context, by auth.Principal, folder folders.Folder, observationID pgtype.UUID, live []dbsqlc.ObservationBlock) (map[pgtype.UUID]frozenSnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(publishConcurrency)
	results := make([]*frozenSnapshot, len(live))
	for i, b := range live {
		if Kind(b.Kind) != KindPanel {
			continue
		}
		if ctx.Err() != nil {
			break
		}
		group.Go(func() error {
			snap, err := s.freeze(ctx, by, folder, observationID, b.ID, anyBlock)
			if err != nil {
				return s.panelFailure(ctx, observationID, b, err)
			}
			results[i] = &snap
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	out := make(map[pgtype.UUID]frozenSnapshot)
	for i, snap := range results {
		if snap != nil {
			out[live[i].ID] = *snap
		}
	}
	return out, nil
}

func (s *Service) panelFailure(ctx context.Context, observationID pgtype.UUID, b dbsqlc.ObservationBlock, err error) error {
	switch {
	case errors.Is(err, ErrSnapshotTooLarge), errors.Is(err, context.Canceled):
		return err
	case errors.Is(err, &InvalidInputError{Code: CodeInvalidBlock}):
		return ErrObservationChanged
	}
	if status, _, _ := queryproxy.Classify(err); status == http.StatusInternalServerError {
		s.logger.ErrorContext(ctx, "freeze a panel for publishing",
			logattrs.Failure("internal", err, slog.String("observation_id", observationID.String()), slog.String("block_id", b.ID.String()))...)
	}
	return &PanelFailedError{BlockID: b.ID.String(), Position: int(b.Position), Err: err}
}

func (s *Service) ListRCAs(ctx context.Context, by auth.Principal, observationID pgtype.UUID) ([]RCASummary, error) {
	_, folder, abilities, err := s.open(ctx, by, observationID)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.db.Queries().ListRCAs(ctx, observationID)
	if err != nil {
		return nil, fmt.Errorf("list RCAs: %w", err)
	}
	out := make([]RCASummary, len(rows))
	for i, r := range rows {
		state := rcaState(int(r.LiveLinks), int(r.Revision), int(rows[0].Revision))
		out[i] = RCASummary{
			ID:             r.ID.String(),
			Revision:       int(r.Revision),
			Title:          r.Title,
			Summary:        r.Summary.String,
			State:          State(r.ObservationState),
			RevisionState:  state,
			AllowedActions: allowedRCAActions(state, folder, abilities),
			Actor:          actorOf(r.CreatedByKind, r.CreatedByID, r.CreatedByName, r.Origin, r.OriginClient, r.RunID),
			CreatedAt:      r.CreatedAt.Time.UTC(),
		}
	}
	return out, nil
}

func (s *Service) WithdrawRCA(ctx context.Context, by auth.Principal, observationID pgtype.UUID, revision int) error {
	if revision < 1 || revision > math.MaxInt32 {
		return ErrRevisionNotFound
	}
	err := s.write(ctx, by, observationID, pgtype.UUID{}, func(q *dbsqlc.Queries, row dbsqlc.Observation, abilities folders.Abilities, who actor.Actor) error {
		if !mayWithdraw(abilities) {
			return folders.ErrForbidden
		}
		rcaID, err := q.LockRCAByRevision(ctx, dbsqlc.LockRCAByRevisionParams{ObservationID: observationID, Revision: httpapi.Int32(revision)})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return ErrRevisionNotFound
		case err != nil:
			return fmt.Errorf("lock RCA revision: %w", err)
		}
		links, err := q.LiveRCALinkCount(ctx, rcaID)
		if err != nil {
			return fmt.Errorf("count RCA public links: %w", err)
		}
		next, err := q.NextRCARevision(ctx, observationID)
		if err != nil {
			return fmt.Errorf("read the newest RCA revision: %w", err)
		}
		state := rcaState(int(links), revision, int(next.Revision)-1)
		if _, open := RCAMachine.Find(state, RCAWithdraw); !open {
			folder, _, err := s.locate(ctx, by, row.FolderID)
			if err != nil {
				return err
			}
			return refusedWithdraw(state, folder, abilities)
		}
		if _, err := q.DeleteRCA(ctx, rcaID); err != nil {
			return fmt.Errorf("delete RCA revision: %w", err)
		}
		return appendEvent(ctx, q, activity.TypeObservationRCAWithdrawn, who, observationID, publishedEventData{Title: row.Title, Revision: revision})
	})
	if err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "observation revision withdrawn",
		slog.String("actor_id", by.ID), slog.String("observation_id", observationID.String()), slog.Int("revision", revision))
	return nil
}

func (s *Service) GetRCA(ctx context.Context, by auth.Principal, observationID pgtype.UUID, revision int) (RCA, error) {
	_, folder, abilities, err := s.open(ctx, by, observationID)
	if err != nil {
		return RCA{}, err
	}
	row, err := s.store.rca(ctx, observationID, revision)
	if err != nil {
		return RCA{}, err
	}
	rca, err := rcaOf(row)
	if err != nil {
		return RCA{}, err
	}
	q := s.store.db.Queries()
	links, err := q.LiveRCALinkCount(ctx, row.ID)
	if err != nil {
		return RCA{}, fmt.Errorf("count RCA public links: %w", err)
	}
	next, err := q.NextRCARevision(ctx, observationID)
	if err != nil {
		return RCA{}, fmt.Errorf("read the newest RCA revision: %w", err)
	}
	rca.RevisionState = rcaState(int(links), revision, int(next.Revision)-1)
	rca.AllowedActions = allowedRCAActions(rca.RevisionState, folder, abilities)
	return rca, nil
}

func (s *Service) RCASnapshotFrames(ctx context.Context, observationID pgtype.UUID, revision int, snapshotID pgtype.UUID) (json.RawMessage, error) {
	row, err := s.store.rca(ctx, observationID, revision)
	if err != nil {
		return nil, err
	}
	var content rcaContent
	if err := json.Unmarshal(row.Content, &content); err != nil {
		return nil, fmt.Errorf("decode RCA content: %w", err)
	}
	if !containsSnapshot(content.Blocks, snapshotID) {
		return nil, ErrNotFound
	}
	snap, err := s.store.db.Queries().RCASnapshot(ctx, dbsqlc.RCASnapshotParams{SnapshotID: snapshotID, ObservationID: observationID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("get RCA snapshot: %w", err)
	}
	return snap.Frames, nil
}

func containsSnapshot(blocks []RCABlock, id pgtype.UUID) bool {
	want := id.String()
	for _, b := range blocks {
		if b.Kind == KindSnapshot && b.SnapshotID == want {
			return true
		}
	}
	return false
}
