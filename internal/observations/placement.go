package observations

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/placement"
)

func blockRefs(kind Kind, body json.RawMessage, snapshot *frozenSnapshot) ([]string, error) {
	switch {
	case snapshot != nil:
		return placement.SnapshotRefs(snapshot.panel, snapshot.query, snapshot.datasources)
	case kind == KindPanel:
		var panel PanelBody
		if err := json.Unmarshal(body, &panel); err != nil {
			return nil, fmt.Errorf("decode panel block: %w", err)
		}
		return placement.PanelRefs(panel.Panel, panel.Variables)
	}
	return nil, nil
}

func (s *Service) placeBlock(ctx context.Context, q *dbsqlc.Queries, parent dbsqlc.Observation, kind Kind, body json.RawMessage, snapshot *frozenSnapshot) error {
	refs, err := blockRefs(kind, body, snapshot)
	if err != nil {
		return err
	}
	return s.guard.Save(ctx, q, folders.ContentObservation, parent.FolderID, refs)
}
