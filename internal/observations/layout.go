package observations

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/actor"
	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

const (
	MinBlockHeight = 120
	MaxBlockHeight = 960

	automaticHeight = 0

	auditArranged audit.Action = "observation.arrange"
)

type HeightChange struct {
	Set    bool
	Pixels *int
}

func (h *HeightChange) UnmarshalJSON(raw []byte) error {
	h.Set = true
	return json.Unmarshal(raw, &h.Pixels)
}

type BlockLayout struct {
	ID     pgtype.UUID
	Width  *Width
	Height HeightChange
}

func validateHeight(pixels *int) error {
	if pixels != nil && (*pixels < MinBlockHeight || *pixels > MaxBlockHeight) {
		return invalid(CodeInvalidBlock, fmt.Sprintf("height must be between %d and %d pixels, or null for the automatic height", MinBlockHeight, MaxBlockHeight))
	}
	return nil
}

func refuseHeight(kind Kind, pixels *int) *InvalidInputError {
	if pixels != nil && kind != KindPanel && kind != KindSnapshot {
		return invalid(CodeInvalidBlock, fmt.Sprintf("height applies only to panel and snapshot blocks; a %s block keeps its natural height", kind))
	}
	return nil
}

func heightsFit(rows []dbsqlc.ObservationBlock, items []BlockLayout) *InvalidInputError {
	kinds := make(map[pgtype.UUID]Kind, len(rows))
	for _, row := range rows {
		kinds[row.ID] = Kind(row.Kind)
	}
	for _, item := range items {
		if refused := refuseHeight(kinds[item.ID], item.Height.Pixels); refused != nil {
			return invalid(CodeInvalidBlock, fmt.Sprintf("block %s: %s", item.ID, refused.Message))
		}
	}
	return nil
}

func validateLayout(items []BlockLayout) error {
	if len(items) > MaxBlocks {
		return invalid(CodeInvalidBlock, fmt.Sprintf("a layout names at most %d blocks", MaxBlocks))
	}
	seen := make(map[pgtype.UUID]bool, len(items))
	for _, item := range items {
		if seen[item.ID] {
			return invalid(CodeInvalidBlock, fmt.Sprintf("a layout names block %s twice", item.ID))
		}
		seen[item.ID] = true
		if item.Width != nil {
			if err := validateWidth(*item.Width); err != nil {
				return err
			}
		}
		if err := validateHeight(item.Height.Pixels); err != nil {
			return err
		}
	}
	return nil
}

func pgInt4(pixels *int) pgtype.Int4 {
	if pixels == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: httpapi.Int32(*pixels), Valid: true}
}

func heightOf(h pgtype.Int4) *int {
	if !h.Valid {
		return nil
	}
	pixels := int(h.Int32)
	return &pixels
}

func arrangedOrder[T comparable](stored, requested []T) []T {
	known := make(map[T]bool, len(stored))
	for _, id := range stored {
		known[id] = true
	}
	placed := make(map[T]bool, len(requested))
	order := make([]T, 0, len(stored))
	for _, id := range requested {
		if known[id] && !placed[id] {
			placed[id] = true
			order = append(order, id)
		}
	}
	for i, id := range stored {
		if placed[id] {
			continue
		}
		at := 0
		if i > 0 {
			at = slices.Index(order, stored[i-1]) + 1
		}
		order = slices.Insert(order, at, id)
	}
	return order
}

type placedBlock struct {
	id     pgtype.UUID
	width  Width
	height *int
}

func arrangedBlocks(rows []dbsqlc.ObservationBlock, items []BlockLayout) []placedBlock {
	current := make(map[pgtype.UUID]dbsqlc.ObservationBlock, len(rows))
	stored := make([]pgtype.UUID, len(rows))
	for i, row := range rows {
		current[row.ID] = row
		stored[i] = row.ID
	}
	asked := make(map[pgtype.UUID]BlockLayout, len(items))
	requested := make([]pgtype.UUID, len(items))
	for i, item := range items {
		asked[item.ID] = item
		requested[i] = item.ID
	}
	order := arrangedOrder(stored, requested)
	placed := make([]placedBlock, len(order))
	for i, id := range order {
		placed[i] = placedAs(current[id], asked[id])
	}
	return placed
}

func unknownLayoutBlock(rows []dbsqlc.ObservationBlock, items []BlockLayout, abilities folders.Abilities, viewer actor.Actor) *InvalidInputError {
	onPage := make(map[pgtype.UUID]bool, len(rows))
	for _, row := range rows {
		onPage[row.ID] = visibleTo(row, abilities, viewer)
	}
	for _, item := range items {
		if !onPage[item.ID] {
			return invalid(CodeInvalidBlock, fmt.Sprintf("block %s is not on this observation", item.ID))
		}
	}
	return nil
}

func placedAs(row dbsqlc.ObservationBlock, item BlockLayout) placedBlock {
	block := placedBlock{id: row.ID, width: Width(row.Width), height: heightOf(row.Height)}
	if item.Width != nil {
		block.width = *item.Width
	}
	if item.Height.Set {
		block.height = item.Height.Pixels
	}
	return block
}

func changedLayout(rows []dbsqlc.ObservationBlock, placed []placedBlock) []string {
	before := make(map[pgtype.UUID]dbsqlc.ObservationBlock, len(rows))
	for _, row := range rows {
		before[row.ID] = row
	}
	var order, width, height bool
	for i, block := range placed {
		row := before[block.id]
		order = order || rows[i].ID != block.id
		width = width || Width(row.Width) != block.width
		height = height || !equalHeights(heightOf(row.Height), block.height)
	}
	var changed []string
	for _, kind := range []struct {
		name string
		did  bool
	}{{"height", height}, {"order", order}, {"width", width}} {
		if kind.did {
			changed = append(changed, kind.name)
		}
	}
	return changed
}

func unchangedLayout(rows []dbsqlc.ObservationBlock, placed []placedBlock) bool {
	return slices.EqualFunc(rows, placed, func(row dbsqlc.ObservationBlock, block placedBlock) bool {
		return row.ID == block.id && Width(row.Width) == block.width && equalHeights(heightOf(row.Height), block.height)
	})
}

func equalHeights(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func layoutArrays(placed []placedBlock) dbsqlc.ArrangeBlocksParams {
	var params dbsqlc.ArrangeBlocksParams
	for i, block := range placed {
		height := automaticHeight
		if block.height != nil {
			height = *block.height
		}
		params.Ids = append(params.Ids, block.id)
		params.Positions = append(params.Positions, httpapi.Int32(i))
		params.Widths = append(params.Widths, string(block.width))
		params.Heights = append(params.Heights, httpapi.Int32(height))
	}
	return params
}

func shownBlocks(rows []dbsqlc.ObservationBlock, abilities folders.Abilities, viewer actor.Actor) []Block {
	shown := make([]Block, 0, len(rows))
	for _, row := range rows {
		if !visibleTo(row, abilities, viewer) {
			continue
		}
		block := blockOf(row, abilities, viewer)
		block.Position = len(shown)
		shown = append(shown, block)
	}
	return shown
}

func (s *Service) Arrange(ctx context.Context, by auth.Principal, observationID pgtype.UUID, items []BlockLayout, runID pgtype.UUID) ([]Block, error) {
	if err := validateLayout(items); err != nil {
		return nil, err
	}
	var shown []Block
	var unchanged *audit.Result
	err := s.write(ctx, by, observationID, runID, func(q *dbsqlc.Queries, parent dbsqlc.Observation, abilities folders.Abilities, who actor.Actor) error {
		if err := permit(BlockMachine, BlockLive, BlockUpdate, can(abilities, false)); err != nil {
			return err
		}
		rows, err := q.BlocksOfObservation(ctx, parent.ID)
		if err != nil {
			return fmt.Errorf("list observation blocks: %w", err)
		}
		if refused := unknownLayoutBlock(rows, items, abilities, who); refused != nil {
			return refused
		}
		if refused := heightsFit(rows, items); refused != nil {
			return refused
		}
		placed := arrangedBlocks(rows, items)
		if unchangedLayout(rows, placed) {
			unchanged = &audit.Result{
				Action:  auditArranged,
				Outcome: audit.OutcomeSuccess,
				Reason:  audit.ReasonNoChange,
				Target:  audit.Target{Kind: "observation", ID: parent.ID.String(), Name: parent.Title},
				Data:    map[string]any{"title": parent.Title},
			}
		} else {
			params := layoutArrays(placed)
			params.ObservationID = parent.ID
			if err := q.ArrangeBlocks(ctx, params); err != nil {
				return fmt.Errorf("arrange observation blocks: %w", err)
			}
			if err := q.TouchObservation(ctx, parent.ID); err != nil {
				return fmt.Errorf("touch observation: %w", err)
			}
			_, err := activity.Append(ctx, q, activity.Entry{
				Type:        activity.TypeObservationUpdated,
				TargetID:    parent.ID,
				Owner:       activity.OnObservation(parent.ID),
				Actor:       who,
				Data:        eventData{Title: parent.Title},
				AuditAction: auditArranged,
				AuditOnly:   map[string]any{"changed": changedLayout(rows, placed)},
			}, nil)
			if err != nil {
				return err
			}
			if rows, err = q.BlocksOfObservation(ctx, parent.ID); err != nil {
				return fmt.Errorf("list observation blocks: %w", err)
			}
		}
		shown = shownBlocks(rows, abilities, who)
		return nil
	})
	if err == nil && unchanged != nil {
		audit.Conclude(ctx, *unchanged)
	}
	return shown, err
}
