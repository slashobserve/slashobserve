package proxyhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/observations"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

func (h *Handler) Freeze(ctx context.Context, by auth.Principal, d *spec.Dashboard, panelID string, from, to time.Time) (observations.Frozen, error) {
	release, ok := h.slots.acquire(by.ID)
	if !ok {
		return observations.Frozen{}, queryproxy.ErrUpstreamBusy
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	found, ok := d.FindDataPanel(panelID)
	if !ok {
		return observations.Frozen{}, fmt.Errorf("%w: %q", queryproxy.ErrPanelNotFound, panelID)
	}
	now := time.Now()
	opts, err := spec.TimeOptionsFor(d, "")
	if err != nil {
		return observations.Frozen{}, fmt.Errorf("%w: the dashboard time zone is not known", queryproxy.ErrInvalidRequest)
	}
	rng, err := panelTimeRange(found.Panel, datasource.TimeRange{From: from, To: to}, now, opts)
	if err != nil {
		return observations.Frozen{}, err
	}
	kind := queryproxy.PanelKind(found.Panel)
	al := align(rng, kind, defaultMaxDataPoints)
	user := queryproxy.UserFor(by)
	selections, err := h.runner.Defaults(ctx, d, al.rng, nil, user)
	if err != nil {
		return observations.Frozen{}, err
	}
	req := dataRequest(d, panelID, al, selections, user)
	req.Limits = queryproxy.DraftLimits
	entry, err := h.render(ctx, req)
	if errors.Is(err, errTooLarge) {
		return observations.Frozen{}, observations.ErrSnapshotTooLarge
	}
	if err != nil {
		return observations.Frozen{}, err
	}
	body, err := json.Marshal(dataBody(entry, false, entry.executedAt, ""))
	if err != nil {
		return observations.Frozen{}, fmt.Errorf("encode frozen data: %w", err)
	}
	if len(body) > observations.MaxSnapshotBytes {
		return observations.Frozen{}, observations.ErrSnapshotTooLarge
	}
	return observations.Frozen{Data: body, UserScoped: queryproxy.DashboardUsesUser(d)}, nil
}
