package tools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/observations"
)

const (
	namePinObservation = "pin_observation"
	nameMakeDashboard  = "make_dashboard"

	codeObservationHasNoDashboard = "observation_has_no_dashboard"
	codeObservationHasNoPanels    = "observation_has_no_panels"

	descPinObservation = "Pin an observation to a dashboard as an annotation (not pin_dashboard, which stars a dashboard). Needs edit access to the dashboard; dashboard_id defaults to the one the observation's run was about. Pinning again returns the same annotation."
	descMakeDashboard  = "Make a dashboard in My Dashboards from the live panel blocks of an observation, or from one block with block_id; the variables each block carries come along. Making it again returns the first one unless new is true. When the observation, or the block, came from the caller's own chat and that chat is about nothing else, the chat becomes about the new dashboard."

	schemaPinObservation = `{"type":"object","properties":{` + propObservationID + `,` +
		`"dashboard_id":{"type":"string","format":"uuid","description":"The dashboard to pin to. Default: the dashboard the observation's run was about."}},"required":["observation_id"],"additionalProperties":false}`
	schemaMakeDashboard = `{"type":"object","properties":{` + propObservationID + `,` +
		`"block_id":{"type":"string","format":"uuid","description":"Only this panel block, from get_observation."},` +
		`"new":{"type":"boolean","description":"Make another dashboard even when you already made one from this."}},"required":["observation_id"],"additionalProperties":false}`
)

func init() { registerEntries((*Registry).observationOutputEntries) }

func (r *Registry) observationOutputEntries() []entry {
	return []entry{
		{Tool{Name: namePinObservation, Description: descPinObservation, InputSchema: json.RawMessage(schemaPinObservation), MinRole: auth.RoleViewer}, r.pinObservation},
		{Tool{Name: nameMakeDashboard, Description: descMakeDashboard, InputSchema: json.RawMessage(schemaMakeDashboard), MinRole: auth.RoleViewer}, r.makeDashboard},
	}
}

type pinObservationOut struct {
	ObservationID string `json:"observation_id"`
	DashboardID   string `json:"dashboard_id"`
	AnnotationID  string `json:"annotation_id"`
	Created       bool   `json:"created"`
}

func (r *Registry) pinObservation(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Observations == nil {
		return Result{}, errObservationsNotConfigured
	}
	var in struct {
		ObservationID string `json:"observation_id"`
		DashboardID   string `json:"dashboard_id"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseObservationID(in.ObservationID)
	if !ok {
		return res, nil
	}
	var dashboardID pgtype.UUID
	if in.DashboardID != "" {
		if dashboardID, res, ok = parseDashboardID(in.DashboardID); !ok {
			return res, nil
		}
	}
	principal, _ := auth.FromContext(ctx)
	pin, created, err := r.deps.Observations.Pin(ctx, principal, id, dashboardID)
	if err != nil {
		return r.observationOutputFailure(ctx, "Pinned an observation", err)
	}
	return success("Pinned observation "+in.ObservationID+" to dashboard "+pin.DashboardID, pinObservationOut{
		ObservationID: in.ObservationID, DashboardID: pin.DashboardID, AnnotationID: pin.AnnotationID, Created: created,
	}), nil
}

func (r *Registry) makeDashboard(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Observations == nil {
		return Result{}, errObservationsNotConfigured
	}
	var in struct {
		ObservationID string `json:"observation_id"`
		BlockID       string `json:"block_id"`
		New           bool   `json:"new"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseObservationID(in.ObservationID)
	if !ok {
		return res, nil
	}
	var blockID pgtype.UUID
	if in.BlockID != "" {
		if blockID, res, ok = parseBlockID(in.BlockID); !ok {
			return res, nil
		}
	}
	principal, _ := auth.FromContext(ctx)
	dash, ver, created, err := r.deps.Observations.MakeDashboard(ctx, principal, id, blockID, in.New)
	if err != nil {
		return r.observationOutputFailure(ctx, "Made a dashboard from an observation", err)
	}
	existing := !created
	return success("Made dashboard "+dash.ID, r.savedOut(dash, ver, &existing)), nil
}

func (r *Registry) observationOutputFailure(ctx context.Context, step string, err error) (Result, error) {
	switch {
	case errors.Is(err, observations.ErrNoDashboard):
		return toolError(codeObservationHasNoDashboard, err.Error()), nil
	case errors.Is(err, observations.ErrDashboardNotFound):
		return toolError(codeNotFound, err.Error()), nil
	case errors.Is(err, observations.ErrNoPanels):
		return toolError(codeObservationHasNoPanels, err.Error()), nil
	}
	if res, ok := observationRefusal(ctx, err); ok {
		return res, nil
	}
	return saveFailure(ctx, err)
}
