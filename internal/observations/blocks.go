package observations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/findings"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type preparedBlock struct {
	kind     Kind
	body     json.RawMessage
	width    Width
	position *int
	snapshot *frozenSnapshot
}

type frozenSnapshot struct {
	panel, query, frames json.RawMessage
	userScoped           bool
	from, to             time.Time
}

func validateWidth(w Width) error {
	if w != WidthFull && w != WidthHalf {
		return invalid(CodeInvalidBlock, "width must be full or half")
	}
	return nil
}

func normalBody(kind Kind, raw json.RawMessage) (json.RawMessage, error) {
	switch kind {
	case KindText:
		body, err := textBody(raw)
		if err != nil {
			return nil, err
		}
		return json.Marshal(body)
	case KindPanel:
		body, err := panelBody(raw)
		if err != nil {
			return nil, err
		}
		return json.Marshal(body)
	}
	return nil, invalid(CodeBodyNotEditable, "only the body of a text or panel block can be changed")
}

func (s *Service) prepare(ctx context.Context, by auth.Principal, observationID pgtype.UUID, in BlockInput) (preparedBlock, error) {
	width := in.Width
	if width == "" {
		width = WidthFull
	}
	if err := validateWidth(width); err != nil {
		return preparedBlock{}, err
	}
	prepared := preparedBlock{kind: in.Kind, width: width, position: in.Position}
	switch in.Kind {
	case KindText, KindPanel:
		body, err := normalBody(in.Kind, in.Body)
		if err != nil {
			return preparedBlock{}, err
		}
		prepared.body = body
	case KindSnapshot:
		if len(in.Body) > 0 {
			return preparedBlock{}, invalid(CodeInvalidBlock, "a snapshot block takes no body; name the panel block with from_block_id")
		}
		frozen, err := s.freeze(ctx, by, observationID, in.FromBlockID)
		if err != nil {
			return preparedBlock{}, err
		}
		prepared.snapshot = &frozen
	case KindRun:
		return preparedBlock{}, invalid(CodeKindNotAddable, "run blocks are added by agent runs, not through the API")
	default:
		return preparedBlock{}, invalid(CodeInvalidBlock, "kind must be text, panel or snapshot")
	}
	return prepared, nil
}

func (s *Service) freeze(ctx context.Context, by auth.Principal, observationID, sourceID pgtype.UUID) (frozenSnapshot, error) {
	notPanel := invalid(CodeInvalidBlock, "from_block_id must name a panel block of this observation")
	if !sourceID.Valid || !observationID.Valid {
		return frozenSnapshot{}, notPanel
	}
	source, err := s.store.block(ctx, sourceID)
	switch {
	case errors.Is(err, ErrNotFound):
		return frozenSnapshot{}, notPanel
	case err != nil:
		return frozenSnapshot{}, err
	case source.ObservationID != observationID || Kind(source.Kind) != KindPanel:
		return frozenSnapshot{}, notPanel
	}
	parent, err := s.store.observation(ctx, observationID)
	if err != nil {
		return frozenSnapshot{}, err
	}
	d, from, to, err := wrapBlock(parent, source)
	if err != nil {
		return frozenSnapshot{}, err
	}
	frozen, err := s.freezer.Freeze(ctx, by, d, BlockPanelID, from, to)
	if err != nil {
		return frozenSnapshot{}, err
	}
	if len(frozen.Data) > MaxSnapshotBytes {
		return frozenSnapshot{}, ErrSnapshotTooLarge
	}
	var body PanelBody
	if err := json.Unmarshal(source.Body, &body); err != nil {
		return frozenSnapshot{}, fmt.Errorf("decode panel block: %w", err)
	}
	panel, query, err := splitQueries(body.Panel)
	if err != nil {
		return frozenSnapshot{}, err
	}
	return frozenSnapshot{panel: panel, query: query, frames: frozen.Data, userScoped: frozen.UserScoped, from: from, to: to}, nil
}

func splitQueries(panel json.RawMessage) (json.RawMessage, json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(panel, &fields); err != nil {
		return nil, nil, fmt.Errorf("decode panel: %w", err)
	}
	query := fields["queries"]
	if len(query) == 0 || string(query) == "null" {
		query = json.RawMessage("[]")
	}
	delete(fields, "queries")
	rest, err := json.Marshal(fields)
	if err != nil {
		return nil, nil, fmt.Errorf("encode panel: %w", err)
	}
	return rest, query, nil
}

func wrapBlock(parent dbsqlc.Observation, block dbsqlc.ObservationBlock) (*spec.Dashboard, time.Time, time.Time, error) {
	var body PanelBody
	if err := json.Unmarshal(block.Body, &body); err != nil {
		return nil, time.Time{}, time.Time{}, fmt.Errorf("decode panel block: %w", err)
	}
	from, to := blockRange(body, timePtr(parent.TimeFrom), timePtr(parent.TimeTo), time.Now().UTC())
	from, to = from.Truncate(storedPrecision), to.Truncate(storedPrecision)
	d, err := findings.EvidenceSpec(parent.Title, from, to, []json.RawMessage{body.Panel})
	if err != nil {
		return nil, time.Time{}, time.Time{}, fmt.Errorf("wrap panel block: %w", err)
	}
	d.Panels[0].ID = BlockPanelID
	return d, from, to, nil
}

func blockRange(body PanelBody, observationFrom, observationTo *time.Time, now time.Time) (time.Time, time.Time) {
	switch {
	case body.From != nil && body.To != nil:
		return *body.From, *body.To
	case observationFrom != nil && observationTo != nil:
		return *observationFrom, *observationTo
	}
	return now.Add(-defaultPanelRange), now
}

func (s *Service) PanelDashboard(ctx context.Context, by auth.Principal, blockID pgtype.UUID) (*spec.Dashboard, time.Time, time.Time, error) {
	block, err := s.store.block(ctx, blockID)
	if err != nil {
		return nil, time.Time{}, time.Time{}, err
	}
	parent, _, _, err := s.open(ctx, by, block.ObservationID)
	if err != nil {
		return nil, time.Time{}, time.Time{}, err
	}
	if Kind(block.Kind) != KindPanel {
		return nil, time.Time{}, time.Time{}, ErrNotFound
	}
	return wrapBlock(parent, block)
}

func (s *Service) AddBlock(ctx context.Context, by auth.Principal, observationID pgtype.UUID, in BlockInput) (Block, error) {
	_, _, abilities, err := s.open(ctx, by, observationID)
	if err != nil {
		return Block{}, err
	}
	if _, ok := addedState(abilities); !ok {
		return Block{}, folders.ErrForbidden
	}
	prepared, err := s.prepare(ctx, by, observationID, in)
	if err != nil {
		return Block{}, err
	}
	var added dbsqlc.ObservationBlock
	var held folders.Abilities
	var who actor.Actor
	err = s.write(ctx, by, observationID, in.RunID, func(q *dbsqlc.Queries, row dbsqlc.Observation, abilities folders.Abilities, by actor.Actor) error {
		state, ok := addedState(abilities)
		if !ok {
			return folders.ErrForbidden
		}
		var err error
		added, err = insertBlock(ctx, q, row, prepared, state, by)
		held, who = abilities, by
		return err
	})
	if err != nil {
		return Block{}, err
	}
	return blockOf(added, held, who), nil
}

func insertBlock(ctx context.Context, q *dbsqlc.Queries, parent dbsqlc.Observation, p preparedBlock, state BlockState, by actor.Actor) (dbsqlc.ObservationBlock, error) {
	counts, err := q.CountBlocks(ctx, parent.ID)
	if err != nil {
		return dbsqlc.ObservationBlock{}, fmt.Errorf("count observation blocks: %w", err)
	}
	if counts.Total >= MaxBlocks {
		return dbsqlc.ObservationBlock{}, ErrTooManyBlocks
	}
	if (p.kind == KindPanel || p.kind == KindSnapshot) && counts.Panels >= MaxPanelBlocks {
		return dbsqlc.ObservationBlock{}, ErrTooManyPanels
	}
	cols, err := by.Columns()
	if err != nil {
		return dbsqlc.ObservationBlock{}, err
	}
	position := clamp(p.position, int(counts.Total), int(counts.Total))
	if position < int(counts.Total) {
		err := q.ShiftBlocks(ctx, dbsqlc.ShiftBlocksParams{ObservationID: parent.ID, Delta: 1, FromPosition: httpapi.Int32(position), ToPosition: math.MaxInt32})
		if err != nil {
			return dbsqlc.ObservationBlock{}, fmt.Errorf("make room for a block: %w", err)
		}
	}
	body, snapshotID := p.body, pgtype.UUID{}
	if p.snapshot != nil {
		snap, err := q.InsertSnapshot(ctx, dbsqlc.InsertSnapshotParams{
			ObservationID: parent.ID,
			Panel:         p.snapshot.panel,
			Query:         p.snapshot.query,
			Frames:        p.snapshot.frames,
			UserScoped:    p.snapshot.userScoped,
			TimeFrom:      pgtype.Timestamptz{Time: p.snapshot.from, Valid: true},
			TimeTo:        pgtype.Timestamptz{Time: p.snapshot.to, Valid: true},
			CreatedByKind: cols.Kind,
			CreatedByID:   cols.ID,
			CreatedByName: cols.Name,
			Origin:        cols.Origin,
			OriginClient:  cols.Client,
			RunID:         cols.RunID,
		})
		if err != nil {
			return dbsqlc.ObservationBlock{}, fmt.Errorf("insert snapshot: %w", err)
		}
		snapshotID = snap.ID
		if body, err = json.Marshal(map[string]string{"snapshot_id": snap.ID.String()}); err != nil {
			return dbsqlc.ObservationBlock{}, fmt.Errorf("encode snapshot block body: %w", err)
		}
	}
	row, err := q.InsertBlock(ctx, dbsqlc.InsertBlockParams{
		ObservationID: parent.ID,
		Kind:          string(p.kind),
		State:         string(state),
		Position:      httpapi.Int32(position),
		Width:         string(p.width),
		Body:          body,
		SnapshotID:    snapshotID,
		CreatedByKind: cols.Kind,
		CreatedByID:   cols.ID,
		CreatedByName: cols.Name,
		Origin:        cols.Origin,
		OriginClient:  cols.Client,
		RunID:         cols.RunID,
	})
	if err != nil {
		return dbsqlc.ObservationBlock{}, fmt.Errorf("insert observation block: %w", err)
	}
	if err := q.TouchObservation(ctx, parent.ID); err != nil {
		return dbsqlc.ObservationBlock{}, fmt.Errorf("touch observation: %w", err)
	}
	err = appendEvent(ctx, q, activity.TypeObservationBlockAdded, by, parent.ID, blockEventData{Title: parent.Title, BlockID: row.ID.String(), BlockKind: p.kind})
	return row, err
}

func clamp(requested *int, fallback, ceiling int) int {
	if requested == nil {
		return fallback
	}
	return max(0, min(*requested, ceiling))
}

type blockFunc func(q *dbsqlc.Queries, parent dbsqlc.Observation, current dbsqlc.ObservationBlock, abilities folders.Abilities, by actor.Actor) error

func (s *Service) changeBlock(ctx context.Context, by auth.Principal, blockID, runID pgtype.UUID, fn blockFunc) error {
	located, err := s.store.block(ctx, blockID)
	if err != nil {
		return err
	}
	return s.write(ctx, by, located.ObservationID, runID, func(q *dbsqlc.Queries, parent dbsqlc.Observation, abilities folders.Abilities, who actor.Actor) error {
		current, err := q.BlockByID(ctx, blockID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return ErrNotFound
		case err != nil:
			return fmt.Errorf("lock observation block: %w", err)
		}
		return fn(q, parent, current, abilities, who)
	})
}

func authored(row dbsqlc.ObservationBlock, who actor.Actor) bool {
	return row.CreatedByKind == string(who.Kind) && row.CreatedByID.String() == who.ID
}

func blockPermit(current dbsqlc.ObservationBlock, a BlockAction, abilities folders.Abilities, who actor.Actor) error {
	return permit(BlockMachine, BlockState(current.State), a, can(abilities, authored(current, who)))
}

func staleVersion(current dbsqlc.ObservationBlock, version int, abilities folders.Abilities, who actor.Actor) error {
	if int(current.Version) == version {
		return nil
	}
	return &ConflictError{Current: blockOf(current, abilities, who)}
}

func blockEvent(parent dbsqlc.Observation, block dbsqlc.ObservationBlock) blockEventData {
	return blockEventData{Title: parent.Title, BlockID: block.ID.String(), BlockKind: Kind(block.Kind)}
}

func (s *Service) UpdateBlock(ctx context.Context, by auth.Principal, blockID pgtype.UUID, in BlockChange) (Block, error) {
	if len(in.Body) == 0 && in.Width == nil && in.Position == nil {
		return Block{}, ErrNothingToUpdate
	}
	if in.Width != nil {
		if err := validateWidth(*in.Width); err != nil {
			return Block{}, err
		}
	}
	var updated dbsqlc.ObservationBlock
	var held folders.Abilities
	var who actor.Actor
	err := s.changeBlock(ctx, by, blockID, in.RunID, func(q *dbsqlc.Queries, parent dbsqlc.Observation, current dbsqlc.ObservationBlock, abilities folders.Abilities, by actor.Actor) error {
		if err := blockPermit(current, BlockUpdate, abilities, by); err != nil {
			return err
		}
		if err := staleVersion(current, in.Version, abilities, by); err != nil {
			return err
		}
		params, err := changedBlock(ctx, q, current, in)
		if err != nil {
			return err
		}
		row, err := q.UpdateBlock(ctx, params)
		if err != nil {
			return fmt.Errorf("update observation block: %w", err)
		}
		if err := q.TouchObservation(ctx, parent.ID); err != nil {
			return fmt.Errorf("touch observation: %w", err)
		}
		updated, held, who = row, abilities, by
		return appendEvent(ctx, q, activity.TypeObservationBlockUpdated, by, parent.ID, blockEvent(parent, row))
	})
	if err != nil {
		return Block{}, err
	}
	return blockOf(updated, held, who), nil
}

func changedBlock(ctx context.Context, q *dbsqlc.Queries, current dbsqlc.ObservationBlock, in BlockChange) (dbsqlc.UpdateBlockParams, error) {
	params := dbsqlc.UpdateBlockParams{
		ID: current.ID, Version: current.Version, FromStates: BlockMachine.From(BlockUpdate),
		Body: current.Body, Width: current.Width, Position: current.Position,
	}
	if len(in.Body) > 0 {
		body, err := normalBody(Kind(current.Kind), in.Body)
		if err != nil {
			return params, err
		}
		params.Body = body
	}
	if in.Width != nil {
		params.Width = string(*in.Width)
	}
	if in.Position != nil {
		position, err := moveBlock(ctx, q, current, *in.Position)
		if err != nil {
			return params, err
		}
		params.Position = position
	}
	return params, nil
}

func moveBlock(ctx context.Context, q *dbsqlc.Queries, current dbsqlc.ObservationBlock, requested int) (int32, error) {
	counts, err := q.CountBlocks(ctx, current.ObservationID)
	if err != nil {
		return 0, fmt.Errorf("count observation blocks: %w", err)
	}
	target := httpapi.Int32(clamp(&requested, 0, int(counts.Total)-1))
	shift := dbsqlc.ShiftBlocksParams{ObservationID: current.ObservationID, ExceptID: current.ID}
	switch {
	case target < current.Position:
		shift.Delta, shift.FromPosition, shift.ToPosition = 1, target, current.Position-1
	case target > current.Position:
		shift.Delta, shift.FromPosition, shift.ToPosition = -1, current.Position+1, target
	default:
		return target, nil
	}
	if err := q.ShiftBlocks(ctx, shift); err != nil {
		return 0, fmt.Errorf("reorder observation blocks: %w", err)
	}
	return target, nil
}

func (s *Service) KeepBlock(ctx context.Context, by auth.Principal, blockID pgtype.UUID, version int, runID pgtype.UUID) (Block, error) {
	var kept dbsqlc.ObservationBlock
	var held folders.Abilities
	var who actor.Actor
	err := s.changeBlock(ctx, by, blockID, runID, func(q *dbsqlc.Queries, parent dbsqlc.Observation, current dbsqlc.ObservationBlock, abilities folders.Abilities, by actor.Actor) error {
		if err := blockPermit(current, BlockKeep, abilities, by); err != nil {
			return err
		}
		if err := staleVersion(current, version, abilities, by); err != nil {
			return err
		}
		row, err := q.KeepBlock(ctx, dbsqlc.KeepBlockParams{ID: blockID, Version: current.Version, FromStates: BlockMachine.From(BlockKeep)})
		if err != nil {
			return fmt.Errorf("keep observation block: %w", err)
		}
		if err := q.TouchObservation(ctx, parent.ID); err != nil {
			return fmt.Errorf("touch observation: %w", err)
		}
		kept, held, who = row, abilities, by
		return appendEvent(ctx, q, activity.TypeObservationBlockKept, by, parent.ID, blockEvent(parent, row))
	})
	if err != nil {
		return Block{}, err
	}
	return blockOf(kept, held, who), nil
}

func (s *Service) RemoveBlock(ctx context.Context, by auth.Principal, blockID pgtype.UUID, version int, runID pgtype.UUID) error {
	return s.deleteBlock(ctx, by, blockID, BlockRemove, &version, runID, activity.TypeObservationBlockRemoved)
}

func (s *Service) DiscardBlock(ctx context.Context, by auth.Principal, blockID pgtype.UUID, runID pgtype.UUID) error {
	return s.deleteBlock(ctx, by, blockID, BlockDiscard, nil, runID, activity.TypeObservationBlockDiscarded)
}

func (s *Service) deleteBlock(ctx context.Context, by auth.Principal, blockID pgtype.UUID, action BlockAction, version *int, runID pgtype.UUID, event string) error {
	return s.changeBlock(ctx, by, blockID, runID, func(q *dbsqlc.Queries, parent dbsqlc.Observation, current dbsqlc.ObservationBlock, abilities folders.Abilities, by actor.Actor) error {
		if err := blockPermit(current, action, abilities, by); err != nil {
			return err
		}
		if version != nil {
			if err := staleVersion(current, *version, abilities, by); err != nil {
				return err
			}
		}
		n, err := q.DeleteBlock(ctx, dbsqlc.DeleteBlockParams{ID: blockID, FromStates: BlockMachine.From(action)})
		if err != nil {
			return fmt.Errorf("delete observation block: %w", err)
		}
		if n != 1 {
			return ErrNotFound
		}
		err = q.ShiftBlocks(ctx, dbsqlc.ShiftBlocksParams{ObservationID: parent.ID, Delta: -1, FromPosition: current.Position + 1, ToPosition: math.MaxInt32})
		if err != nil {
			return fmt.Errorf("close the gap of a deleted block: %w", err)
		}
		if err := q.TouchObservation(ctx, parent.ID); err != nil {
			return fmt.Errorf("touch observation: %w", err)
		}
		return appendEvent(ctx, q, event, by, parent.ID, blockEvent(parent, current))
	})
}
