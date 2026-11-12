package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/observations"
)

const (
	namePublishRCA = "publish_rca"

	codePersonalFolder = "personal_folder"

	descPublishRCA = "Publish an observation as the next RCA revision: its live text blocks and a frozen snapshot of each panel block are kept as an immutable copy, without query text. Needs edit access to the observation's folder, which is not My Dashboards. Never creates a public link; a person creates links in the web app. Returns the revision and counts."

	schemaPublishRCA = `{"type":"object","properties":{` + propObservationID + `},"required":["observation_id"],"additionalProperties":false}`
)

func init() { registerEntries((*Registry).rcaEntries) }

func (r *Registry) rcaEntries() []entry {
	return []entry{
		{Tool{Name: namePublishRCA, Description: descPublishRCA, InputSchema: json.RawMessage(schemaPublishRCA), MCPOnly: true, MinRole: auth.RoleViewer}, r.publishRCA},
	}
}

type rcaPublishedOut struct {
	Code           string `json:"code"`
	ObservationID  string `json:"observation_id"`
	Revision       int    `json:"revision"`
	Blocks         int    `json:"blocks"`
	Snapshots      int    `json:"snapshots"`
	ObservationURL string `json:"observation_url"`
}

func (r *Registry) rcaFailure(err error) (Result, error) {
	var failed *observations.PanelFailedError
	switch {
	case errors.Is(err, observations.ErrRCALimit):
		return toolError(observations.CodeRCALimit, observations.ErrRCALimit.Error()), nil
	case errors.Is(err, observations.ErrObservationChanged):
		return toolError(observations.CodeObservationChanged, observations.ErrObservationChanged.Error()), nil
	case errors.Is(err, observations.ErrPublishPersonal):
		return toolError(codePersonalFolder, observations.ErrPublishPersonal.Error()), nil
	case errors.As(err, &failed):
		issue := failed.Issue()
		return toolError(observations.CodePanelFailed, fmt.Sprintf("the panel block %s at position %d could not be frozen (%s: %s); fix it with %s and publish again", failed.BlockID, failed.Position, issue.Code, issue.Message, nameUpdateBlock)), nil
	}
	return r.observationFailure("Published an observation", err)
}

func (r *Registry) publishRCA(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Observations == nil {
		return Result{}, errObservationsNotConfigured
	}
	var in struct {
		ObservationID string `json:"observation_id"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseObservationID(in.ObservationID)
	if !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	rca, err := r.deps.Observations.Publish(ctx, principal, id)
	if err != nil {
		return r.rcaFailure(err)
	}
	snapshots := 0
	for _, b := range rca.Blocks {
		if b.Kind == observations.KindSnapshot {
			snapshots++
		}
	}
	out := rcaPublishedOut{Code: "ok", ObservationID: rca.ObservationID, Revision: rca.Revision, Blocks: len(rca.Blocks), Snapshots: snapshots, ObservationURL: r.deps.Observations.URL(rca.ObservationID)}
	return success(fmt.Sprintf("Published revision %d of observation %s", rca.Revision, rca.ObservationID), out), nil
}
