package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	maxFacetFieldsOut   = 20
	maxFacetValuesOut   = 10
	maxFacetFieldsIn    = 50
	maxFacetFieldBytes  = 200
	maxFacetLimit       = 50
	defaultFacetSample  = 1000
	facetsWorkOn        = "log_facets works on Loki, ClickHouse, Elasticsearch and OpenSearch datasources; %s is a %s datasource"
	descLogFacets       = "Count the top values of each label, field or column a logs query matches over a time range, on Loki, ClickHouse, Elastic or OpenSearch: exact counts where the datasource can, counts from the newest sampled lines where it cannot, each field saying which. Use it to find which service, level, route or pod carries an error before narrowing the query."
	propFacetFields     = `"fields":{"type":"array","minItems":1,"maxItems":50,"items":{"type":"string","minLength":1,"maxLength":200},"description":"Count only these fields. Default: every index label and detected field on Loki, every result column plus the top LogAttributes keys on ClickHouse, the level and service fields then the first keyword fields on Elasticsearch and OpenSearch."}`
	propFacetLimit      = `"limit":{"type":"integer","minimum":1,"maximum":50,"description":"Values per field. Default 10."}`
	schemaLogFacetsBody = `{"type":"object","properties":{` + propDatasource + `,` + propQuery + `,` + propFrom + `,` + propTo + `,` + propFacetFields + `,` + propFacetLimit + `},"required":["datasource","query"],"additionalProperties":false}`
)

func init() { registerEntries((*Registry).facetEntries) }

func (r *Registry) facetEntries() []entry {
	return []entry{
		{Tool{Name: nameLogFacets, Description: descLogFacets, InputSchema: json.RawMessage(schemaLogFacetsBody), ReadOnly: true, MinRole: auth.RoleViewer}, r.logFacets},
	}
}

func (r *Registry) logFacets(ctx context.Context, input json.RawMessage) (Result, error) {
	var in struct {
		Datasource string   `json:"datasource"`
		Query      string   `json:"query"`
		From       string   `json:"from"`
		To         string   `json:"to"`
		Fields     []string `json:"fields"`
		Limit      *int     `json:"limit"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	inst, res, ok := r.instance(in.Datasource)
	if !ok {
		return res, nil
	}
	if res, ok := checkQueryText(in.Query); !ok {
		return res, nil
	}
	rng, res, ok := resolveRange(in.From, in.To, time.Now())
	if !ok {
		return res, nil
	}
	limit, res, ok := boundedInt(in.Limit, maxFacetValuesOut, 1, maxFacetLimit, "limit")
	if !ok {
		return res, nil
	}
	if res, ok := checkFacetFields(in.Fields); !ok {
		return res, nil
	}
	source, ok := inst.Datasource.(datasource.LogFacetSource)
	if !ok {
		return invalid(facetsWorkOn, inst.UID, inst.Datasource.Type()), nil
	}
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	release, err := r.deps.Runner.AcquireUpstream(ctx, inst.UID)
	if err != nil {
		return datasourceFailure("Counting facets on "+inst.UID, err)
	}
	defer release()
	principal, _ := auth.FromContext(ctx)
	facets, err := source.LogFacets(ctx, datasource.FacetRequest{
		Query:  datasource.Query{Text: in.Query, Limits: queryproxy.DraftLimits, Dashboard: defaultDashTitle, User: queryproxy.UserFor(principal)},
		Range:  rng,
		Fields: in.Fields,
		Limit:  limit,
		Sample: defaultFacetSample,
	})
	if err != nil {
		return datasourceFailure("Counting facets on "+inst.UID, err)
	}
	datasource.SortFacets(&facets)
	datasource.TrimFacets(&facets, maxFacetFieldsOut, maxFacetValuesOut)
	if facets.Fields == nil {
		facets.Fields = []datasource.FacetField{}
	}
	step := fmt.Sprintf("Counted %s on %s", plural(len(facets.Fields), "field", "fields"), inst.UID)
	if facets.Sampled {
		step += fmt.Sprintf(" (sampled from %s)", plural(facets.LinesSampled, "line", "lines"))
	}
	return success(step, facets), nil
}

func checkFacetFields(fields []string) (Result, bool) {
	if fields != nil && (len(fields) < 1 || len(fields) > maxFacetFieldsIn) {
		return invalid("fields must name from 1 to %d fields", maxFacetFieldsIn), false
	}
	for _, name := range fields {
		if name == "" || len(name) > maxFacetFieldBytes {
			return invalid("each field must be from 1 to %d bytes", maxFacetFieldBytes), false
		}
	}
	return Result{}, true
}
