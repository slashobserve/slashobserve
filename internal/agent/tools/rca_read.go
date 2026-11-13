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
	nameListRCAs = "list_rcas"
	nameGetRCA   = "get_rca"

	descListRCAs = "List the published RCA revisions of an observation, newest first: revision, title, summary, state and who published it."
	descGetRCA   = "Read one published RCA revision: its frozen text blocks and snapshot ids, without query text."

	schemaListRCAs = `{"type":"object","properties":{` + propObservationID + `},"required":["observation_id"],"additionalProperties":false}`
	schemaGetRCA   = `{"type":"object","properties":{` + propObservationID + `,` +
		`"revision":{"type":"integer","minimum":1,"description":"The revision, from list_rcas."}},"required":["observation_id","revision"],"additionalProperties":false}`
)

func init() { registerEntries((*Registry).rcaReadEntries) }

func (r *Registry) rcaReadEntries() []entry {
	return []entry{
		{Tool{Name: nameListRCAs, Description: descListRCAs, InputSchema: json.RawMessage(schemaListRCAs), ReadOnly: true, MCPOnly: true, MinRole: auth.RoleViewer}, r.listRCAs},
		{Tool{Name: nameGetRCA, Description: descGetRCA, InputSchema: json.RawMessage(schemaGetRCA), ReadOnly: true, MCPOnly: true, MinRole: auth.RoleViewer}, r.getRCA},
	}
}

type rcaListOut struct {
	RCAs      []observations.RCASummaryJSON `json:"rcas"`
	Truncated bool                          `json:"truncated,omitempty"`
}

type rcaReadOut struct {
	observations.RCAJSON
	Truncated bool `json:"truncated,omitempty"`
}

func (r *Registry) rcaReadFailure(step string, err error) (Result, error) {
	if errors.Is(err, observations.ErrRevisionNotFound) {
		return toolError(codeNotFound, "RCA revision not found"), nil
	}
	return r.observationFailure(step, err)
}

func (r *Registry) listRCAs(ctx context.Context, input json.RawMessage) (Result, error) {
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
	list, err := r.deps.Observations.ListRCAs(ctx, principal, id)
	if err != nil {
		return r.rcaReadFailure("Listed RCAs", err)
	}
	items := make([]observations.RCASummaryJSON, len(list))
	for i, s := range list {
		items[i] = observations.RCASummaryOf(s)
	}
	kept, cut := fit(items, listBudget-jsonSize(rcaListOut{RCAs: []observations.RCASummaryJSON{}, Truncated: true}))
	return success("Listed "+plural(len(kept), "RCA revision", "RCA revisions"), rcaListOut{RCAs: kept, Truncated: cut}), nil
}

func (r *Registry) getRCA(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Observations == nil {
		return Result{}, errObservationsNotConfigured
	}
	var in struct {
		ObservationID string `json:"observation_id"`
		Revision      *int   `json:"revision"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	id, res, ok := parseObservationID(in.ObservationID)
	if !ok {
		return res, nil
	}
	if in.Revision == nil || *in.Revision < 1 {
		return invalid("revision is required and must be at least 1; list them with %s", nameListRCAs), nil
	}
	principal, _ := auth.FromContext(ctx)
	rca, err := r.deps.Observations.GetRCA(ctx, principal, id, *in.Revision)
	if err != nil {
		return r.rcaReadFailure("Read an RCA", err)
	}
	out := rcaReadOut{RCAJSON: observations.RCAOf(rca)}
	if jsonSize(out) > listBudget {
		envelope := out
		envelope.Blocks, envelope.Truncated = []observations.RCABlock{}, true
		out.Blocks, out.Truncated = fit(out.Blocks, listBudget-jsonSize(envelope))
	}
	return success(fmt.Sprintf("Read revision %d of observation %s", rca.Revision, rca.ObservationID), out), nil
}
