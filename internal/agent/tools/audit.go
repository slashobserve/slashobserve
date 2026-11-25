package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/slashobserve/slashobserve/internal/audit/filter"
	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/auth"
)

const (
	nameSearchAuditEvents    = "search_audit_events"
	nameGetAuditEvent        = "get_audit_event"
	nameListAuditFacets      = "list_audit_facets"
	nameGetAuditHistogram    = "get_audit_histogram"
	nameAggregateAuditEvents = "aggregate_audit_events"
	nameListAuditFields      = "list_audit_fields"

	defaultAuditEvents = 20
	maxAuditEvents     = 100

	auditAdminOnly = " Admin only; the call is itself recorded in the audit trail."

	descSearchAuditEvents = "Search the audit trail, the record of every sign-in, read, change, agent tool call and refusal, newest first, with who did it, through what (web, cli, mcp, agent), to what and how it ended. query takes KQL by default (actor_name:alice and outcome:denied, action:dashboard.*, not origin:web), Lucene after a lucene: prefix, or a Query DSL JSON body starting with {; list_audit_fields names the fields. The window is since to until, 24 hours by default and at most 366 days. Page with next_cursor, which keeps the window and refuses another query. An answer is at most 16 KiB: when that cap, not limit, ended a page early the answer says truncated: true with a note, and next_cursor continues it. warnings names a misspelled operator (andd, orr, nott) or a Lucene operator in lower case, which is read as text." + auditAdminOnly
	descGetAuditEvent     = "Read one audit event by id with the other events of its request (same_request, within an hour) and of its agent run (same_run, within a day), newest first, so one tool call can be followed to the request that made it." + auditAdminOnly
	descListAuditFacets   = "Count the most frequent values of one field among the audit events that match query in the window, such as who made the most refused calls (field actor_name, query outcome:denied) or which actions an agent used most (field action, query initiator:agent)." + auditAdminOnly
	descGetAuditHistogram = "Count the audit events that match query over time: the window split into at most buckets buckets of one step from 1s to 1d, empty buckets included, oldest first, to see when something started or stopped." + auditAdminOnly
	descAggregateAudit    = "Run the aggs of a Query DSL body over the audit events that match it in the window: date_histogram and terms buckets, up to three levels, with cardinality, value_count, min, max, avg, sum and percentiles metrics. query must be the JSON body, for example {\"query\":{\"term\":{\"outcome\":\"denied\"}},\"aggs\":{\"by\":{\"terms\":{\"field\":\"actor_name\"},\"aggs\":{\"routes\":{\"cardinality\":{\"field\":\"route\"}}}}}}. Each row has one key per level, doc_count and one value per metric." + auditAdminOnly
	descListAuditFields   = "List the fields an audit query can use, with their type, every alias a query accepts (actor.email, actor.name, target.kind), what each holds and the values of enum fields; read it before writing the query of another audit tool." + auditAdminOnly

	propAuditQuery  = `"query":{"type":"string","maxLength":65536,"description":"The filter: KQL by default, Lucene after lucene:, or a Query DSL JSON body starting with {. Empty matches every event."}`
	propAuditSince  = `"since":{"type":"string","maxLength":128,"description":"Start of the window, inclusive: RFC 3339, epoch milliseconds or relative such as now-7d. Default 24 hours before until."}`
	propAuditUntil  = `"until":{"type":"string","maxLength":128,"description":"End of the window, exclusive, in the same forms. Default now."}`
	propAuditWindow = propAuditQuery + `,` + propAuditSince + `,` + propAuditUntil

	schemaSearchAuditEvents = `{"type":"object","properties":{` + propAuditWindow + `,` +
		`"limit":{"type":"integer","minimum":1,"maximum":100,"description":"Events per page, at most 100 and at most 16 KiB of answer; follow next_cursor when truncated. Default 20."},` +
		`"cursor":{"type":"string","maxLength":1024,"description":"next_cursor from the previous page, with the same query."}},"additionalProperties":false}`
	schemaGetAuditEvent   = `{"type":"object","properties":{"id":{"type":"string","format":"uuid","description":"The event id, from search_audit_events."}},"required":["id"],"additionalProperties":false}`
	schemaListAuditFacets = `{"type":"object","properties":{` + propAuditWindow + `,` +
		`"field":{"type":"string","minLength":1,"maxLength":64,"description":"The field to count, from list_audit_fields."},` +
		`"limit":{"type":"integer","minimum":1,"maximum":100,"description":"Values to return. Default 10."}},"required":["field"],"additionalProperties":false}`
	schemaGetAuditHistogram = `{"type":"object","properties":{` + propAuditWindow + `,` +
		`"buckets":{"type":"integer","minimum":1,"maximum":200,"description":"The most buckets wanted. Default 60."}},"additionalProperties":false}`
	schemaAggregateAudit = `{"type":"object","properties":{` + propAuditWindow + `},"required":["query"],"additionalProperties":false}`
	schemaListAuditField = `{"type":"object","properties":{},"additionalProperties":false}`
)

var errAuditNotConfigured = errors.New("the audit trail is not configured")

func init() { registerEntries((*Registry).auditEntries) }

func (r *Registry) auditEntries() []entry {
	tool := func(name, desc, schema string) Tool {
		return Tool{Name: name, Description: desc, InputSchema: json.RawMessage(schema), ReadOnly: true, MCPOnly: true, MinRole: auth.RoleAdmin}
	}
	return []entry{
		{tool(nameSearchAuditEvents, descSearchAuditEvents, schemaSearchAuditEvents), r.searchAuditEvents},
		{tool(nameGetAuditEvent, descGetAuditEvent, schemaGetAuditEvent), r.getAuditEvent},
		{tool(nameListAuditFacets, descListAuditFacets, schemaListAuditFacets), r.listAuditFacets},
		{tool(nameGetAuditHistogram, descGetAuditHistogram, schemaGetAuditHistogram), r.getAuditHistogram},
		{tool(nameAggregateAuditEvents, descAggregateAudit, schemaAggregateAudit), r.aggregateAuditEvents},
		{tool(nameListAuditFields, descListAuditFields, schemaListAuditField), r.listAuditFields},
	}
}

type auditWindowInput struct {
	Query string `json:"query"`
	Since string `json:"since"`
	Until string `json:"until"`
}

func (r *Registry) prepareAudit(ctx context.Context, in auditWindowInput, cursor string) (trail.Prepared, Result, bool) {
	p, err := r.deps.Audit.Prepare(ctx, trail.ActionSearch, trail.Request{Text: in.Query, Since: in.Since, Until: in.Until, Cursor: cursor})
	if err != nil {
		res, rerr := auditFailure(err)
		if rerr != nil {
			return trail.Prepared{}, invalid("%s", err.Error()), false
		}
		return trail.Prepared{}, res, false
	}
	return p, Result{}, true
}

func auditFailure(err error) (Result, error) {
	problem, ok := trail.ProblemOf(err)
	if !ok {
		return Result{}, err
	}
	message := problem.Message
	if problem.Start != nil && problem.End != nil {
		message = fmt.Sprintf("%s (bytes %d to %d of query)", message, *problem.Start, *problem.End)
	}
	return toolError(problem.Code, message), nil
}

func (r *Registry) searchAuditEvents(ctx context.Context, input json.RawMessage) (Result, error) {
	if res, refused := guardAuditRead(ctx, nameSearchAuditEvents); refused {
		return res, nil
	}
	if r.deps.Audit == nil {
		return Result{}, errAuditNotConfigured
	}
	var in struct {
		auditWindowInput
		Limit  *int   `json:"limit"`
		Cursor string `json:"cursor"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	limit, res, ok := boundedInt(in.Limit, defaultAuditEvents, 1, maxAuditEvents, "limit")
	if !ok {
		return res, nil
	}
	p, res, ok := r.prepareAudit(ctx, in.auditWindowInput, in.Cursor)
	if !ok {
		return res, nil
	}
	page, err := r.deps.Audit.Search(ctx, p, limit)
	if err != nil {
		return auditFailure(err)
	}
	empty := auditSearchAnswer{SearchJSON: trail.SearchOf(p, trail.Page{NextCursor: page.NextCursor}), Truncated: true, Note: sizeCapNote(maxAuditEvents)}
	empty.Events = []trail.Event{}
	answer := auditSearchAnswer{}
	if kept, cut := fit(page.Events, listBudget-jsonSize(empty)-trail.MaxCursorBytes); cut {
		page = p.Cut(page, max(len(kept), 1))
		answer.Truncated, answer.Note = true, sizeCapNote(len(page.Events))
	}
	answer.SearchJSON = trail.SearchOf(p, page)
	return success("Searched the audit trail: "+plural(len(page.Events), "event", "events"), answer), nil
}

type auditSearchAnswer struct {
	trail.SearchJSON
	Truncated bool   `json:"truncated,omitempty"`
	Note      string `json:"note,omitempty"`
}

func sizeCapNote(events int) string {
	return fmt.Sprintf("the answer size cap cut this page to %d events; continue with next_cursor", events)
}

func (r *Registry) getAuditEvent(ctx context.Context, input json.RawMessage) (Result, error) {
	if res, refused := guardAuditRead(ctx, nameGetAuditEvent); refused {
		return res, nil
	}
	if r.deps.Audit == nil {
		return Result{}, errAuditNotConfigured
	}
	var in struct {
		ID string `json:"id"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if in.ID == "" {
		return invalid("id is required; search_audit_events gives each event's id"), nil
	}
	d, err := r.deps.Audit.Event(ctx, in.ID)
	if err != nil {
		return auditFailure(err)
	}
	out := trail.DetailOf(d)
	budget := (listBudget - jsonSize(trail.DetailJSON{Event: out.Event, SameRequest: []trail.Event{}, SameRun: []trail.Event{}})) / 2
	out.SameRequest, _ = fit(out.SameRequest, budget)
	out.SameRun, _ = fit(out.SameRun, budget)
	return success("Read audit event "+d.Event.Action, out), nil
}

func (r *Registry) listAuditFacets(ctx context.Context, input json.RawMessage) (Result, error) {
	if res, refused := guardAuditRead(ctx, nameListAuditFacets); refused {
		return res, nil
	}
	if r.deps.Audit == nil {
		return Result{}, errAuditNotConfigured
	}
	var in struct {
		auditWindowInput
		Field string `json:"field"`
		Limit *int   `json:"limit"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if in.Field == "" {
		return invalid("field is required; list_audit_fields names the fields"), nil
	}
	limit, res, ok := boundedInt(in.Limit, filter.DefaultFacets, 1, filter.MaxFacets, "limit")
	if !ok {
		return res, nil
	}
	p, res, ok := r.prepareAudit(ctx, in.auditWindowInput, "")
	if !ok {
		return res, nil
	}
	values, err := r.deps.Audit.Facets(ctx, p, in.Field, limit)
	if err != nil {
		return auditFailure(err)
	}
	values, _ = fit(values, listBudget)
	return success("Counted "+in.Field+" in the audit trail", trail.FacetsOf(p, in.Field, values)), nil
}

func (r *Registry) getAuditHistogram(ctx context.Context, input json.RawMessage) (Result, error) {
	if res, refused := guardAuditRead(ctx, nameGetAuditHistogram); refused {
		return res, nil
	}
	if r.deps.Audit == nil {
		return Result{}, errAuditNotConfigured
	}
	var in struct {
		auditWindowInput
		Buckets *int `json:"buckets"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	buckets, res, ok := boundedInt(in.Buckets, trail.DefaultBuckets, 1, trail.MaxBuckets, "buckets")
	if !ok {
		return res, nil
	}
	p, res, ok := r.prepareAudit(ctx, in.auditWindowInput, "")
	if !ok {
		return res, nil
	}
	h, err := r.deps.Audit.Histogram(ctx, p, buckets)
	if err != nil {
		return auditFailure(err)
	}
	return success("Counted "+strconv.FormatInt(h.Total, 10)+" audit events over time", trail.HistogramOf(p, h)), nil
}

func (r *Registry) aggregateAuditEvents(ctx context.Context, input json.RawMessage) (Result, error) {
	if res, refused := guardAuditRead(ctx, nameAggregateAuditEvents); refused {
		return res, nil
	}
	if r.deps.Audit == nil {
		return Result{}, errAuditNotConfigured
	}
	var in auditWindowInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	if in.Query == "" {
		return invalid("query is required: a Query DSL body with aggs, such as {\"aggs\":{\"by\":{\"terms\":{\"field\":\"origin\"}}}}"), nil
	}
	p, res, ok := r.prepareAudit(ctx, in, "")
	if !ok {
		return res, nil
	}
	agg, err := r.deps.Audit.Aggregate(ctx, p)
	if err != nil {
		return auditFailure(err)
	}
	out := trail.AggregationOf(p, agg)
	rows := out.Rows
	out.Rows = []trail.AggRow{}
	kept, cut := fit(rows, listBudget-jsonSize(out))
	out.Rows, out.Truncated = kept, out.Truncated || cut
	return success("Aggregated the audit trail: "+plural(len(out.Rows), "row", "rows"), out), nil
}

func (r *Registry) listAuditFields(_ context.Context, input json.RawMessage) (Result, error) {
	var in struct{}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	return success("Listed the audit fields", trail.FieldListJSON{Fields: trail.FieldsOf()}), nil
}
