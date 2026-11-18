package dashboards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	MaxUpsertPanels = 50
	MaxDeletePanels = 50
	MaxLayoutMoves  = 200
)

type PatchInput struct {
	DashboardID     pgtype.UUID
	Parent          int
	Patch           spec.PanelPatch
	Summary         string
	RequestID       pgtype.UUID
	ProposeIfNeeded bool
	Check           func(next, parent *spec.Dashboard) []spec.Issue
}

func PatchSizeError(p spec.PanelPatch) *InvalidInputError {
	for _, limit := range []struct {
		field    string
		n, limit int
	}{
		{"upsert", len(p.Upsert), MaxUpsertPanels},
		{"delete", len(p.Delete), MaxDeletePanels},
		{"layout", len(p.Layout), MaxLayoutMoves},
	} {
		if limit.n > limit.limit {
			return &InvalidInputError{Message: fmt.Sprintf("%s holds %d items; send at most %d per call", limit.field, limit.n, limit.limit)}
		}
	}
	return nil
}

func (s *Service) SavePatch(ctx context.Context, by auth.Principal, in PatchInput) (SaveResult, error) {
	if err := PatchSizeError(in.Patch); err != nil {
		return SaveResult{}, err
	}
	_, parent, err := s.GetVersion(ctx, in.DashboardID, in.Parent)
	if err != nil {
		return SaveResult{}, err
	}
	if parent.Spec == nil {
		return SaveResult{}, errors.New("parent version has no spec")
	}
	merged, err := spec.ApplyPanelPatch(parent.Spec, in.Patch)
	if err != nil {
		return SaveResult{}, err
	}
	if in.Check != nil {
		if issues := in.Check(merged, parent.Spec); len(issues) > 0 {
			return SaveResult{}, &spec.PatchError{Issues: issues, PanelIDs: PanelIDs(merged)}
		}
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return SaveResult{}, err
	}
	res, err := s.Save(ctx, by, SaveInput{
		DashboardID:     in.DashboardID,
		Parent:          in.Parent,
		Spec:            raw,
		Summary:         in.Summary,
		RequestID:       in.RequestID,
		Partial:         true,
		Merge:           true,
		ProposeIfNeeded: in.ProposeIfNeeded,
	})
	var invalid *InvalidSpecError
	if errors.As(err, &invalid) {
		return SaveResult{}, &spec.PatchError{Issues: invalid.Issues, PanelIDs: PanelIDs(merged)}
	}
	return res, err
}

func PanelIDs(d *spec.Dashboard) map[string]string {
	entries := d.AllPanels()
	ids := make(map[string]string, len(entries))
	for _, e := range entries {
		ids[e.Pointer] = e.Panel.ID
	}
	return ids
}
