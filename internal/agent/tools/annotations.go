package tools

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/slashobserve/slashobserve/internal/annotations"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

const (
	nameListAnnotations  = "list_annotations"
	nameCreateAnnotation = "create_annotation"

	defaultListAnnotations = 50
	maxListAnnotations     = 100

	descListAnnotations  = "List annotations, such as deploy markers, newest first: those on every dashboard, plus with dashboard_id that dashboard's own, optionally only those within from and to or carrying any of tags. An investigation can read them to see a deploy or change beside a spike. Page with next_cursor."
	descCreateAnnotation = "Add an annotation, such as a deploy marker: text and tags at a time or a range, on one dashboard (or one of its panels) or, without dashboard_id, on every dashboard. Returns the annotation with its id."

	propAnnotationTags = `"tags":{"type":"array","maxItems":10,"items":{"type":"string","pattern":"^[a-z0-9][a-z0-9_.-]{0,39}$"},"description":"Lowercase tags such as deploy or checkout."}`

	schemaListAnnotations = `{"type":"object","properties":{` +
		`"dashboard_id":{"type":"string","format":"uuid","description":"Also list this dashboard's own annotations, from list_dashboards."},` +
		`"from":{"type":"string","format":"date-time","description":"Only annotations that end at or after this RFC 3339 time."},` +
		`"to":{"type":"string","format":"date-time","description":"Only annotations that start at or before this RFC 3339 time."},` +
		`"tags":{"type":"array","maxItems":10,"items":{"type":"string"},"description":"Only annotations with any of these tags."},` +
		`"limit":{"type":"integer","minimum":1,"maximum":100,"description":"Annotations per page. Default 50."},` +
		`"cursor":{"type":"string","description":"next_cursor from the previous page."}},"additionalProperties":false}`

	schemaCreateAnnotation = `{"type":"object","properties":{` +
		`"text":{"type":"string","minLength":1,"maxLength":2000,"description":"What happened, such as the version deployed."},` +
		propAnnotationTags + `,` +
		`"dashboard_id":{"type":"string","format":"uuid","description":"The dashboard to annotate, from list_dashboards. Omit it to annotate every dashboard."},` +
		`"panel_id":{"type":"string","minLength":1,"maxLength":64,"description":"One panel of dashboard_id to annotate; needs dashboard_id."},` +
		`"time_from":{"type":"string","format":"date-time","description":"When it happened, as an RFC 3339 time. Default now."},` +
		`"time_to":{"type":"string","format":"date-time","description":"End of a range, as an RFC 3339 time not before time_from. Omit it for a single moment."}},` +
		`"required":["text"],"additionalProperties":false}`
)

var errAnnotationsNotConfigured = errors.New("annotations are not configured")

func init() { registerEntries((*Registry).annotationEntries) }

func (r *Registry) annotationEntries() []entry {
	return []entry{
		{Tool{Name: nameListAnnotations, Description: descListAnnotations, InputSchema: json.RawMessage(schemaListAnnotations), ReadOnly: true, InvestigateOnly: true, MinRole: auth.RoleViewer}, r.listAnnotations},
		{Tool{Name: nameCreateAnnotation, Description: descCreateAnnotation, InputSchema: json.RawMessage(schemaCreateAnnotation), MCPOnly: true, MinRole: auth.RoleEditor}, r.createAnnotation},
	}
}

type listAnnotationsInput struct {
	DashboardID string   `json:"dashboard_id"`
	From        string   `json:"from"`
	To          string   `json:"to"`
	Tags        []string `json:"tags"`
	Limit       *int     `json:"limit"`
	Cursor      string   `json:"cursor"`
}

type annotationOut struct {
	ID             string   `json:"id"`
	DashboardID    string   `json:"dashboard_id,omitempty"`
	PanelID        string   `json:"panel_id,omitempty"`
	TimeFrom       string   `json:"time_from"`
	TimeTo         string   `json:"time_to,omitempty"`
	Text           string   `json:"text"`
	Tags           []string `json:"tags"`
	CreatedByName  string   `json:"created_by_name,omitempty"`
	CreatedByLabel string   `json:"created_by_label,omitempty"`
}

type annotationListOut struct {
	Annotations []annotationOut `json:"annotations"`
	NextCursor  *string         `json:"next_cursor"`
}

func (r *Registry) listAnnotations(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.Annotations == nil {
		return Result{}, errAnnotationsNotConfigured
	}
	var in listAnnotationsInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	q, res, ok := annotationQuery(in)
	if !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	for {
		page, err := r.deps.Annotations.List(ctx, principal, q)
		switch {
		case errors.Is(err, annotations.ErrNotFound):
			return toolError(codeNotFound, "dashboard not found"), nil
		case err != nil:
			return Result{}, err
		}
		out := annotationPage(page)
		kept, cut := fit(out.Annotations, listBudget-jsonSize(annotationListOut{Annotations: []annotationOut{}, NextCursor: out.NextCursor}))
		if !cut || len(out.Annotations) == 1 {
			return success("Listed "+plural(len(out.Annotations), "annotation", "annotations"), out), nil
		}
		q.Limit = max(len(kept), 1)
	}
}

func annotationQuery(in listAnnotationsInput) (annotations.ListQuery, Result, bool) {
	limit, res, ok := boundedInt(in.Limit, defaultListAnnotations, 1, maxListAnnotations, "limit")
	if !ok {
		return annotations.ListQuery{}, res, false
	}
	q := annotations.ListQuery{Limit: limit}
	if in.DashboardID != "" {
		id, res, ok := parseDashboardID(in.DashboardID)
		if !ok {
			return annotations.ListQuery{}, res, false
		}
		q.DashboardID = id
	}
	if q.From, res, ok = annotationTime("from", in.From); !ok {
		return annotations.ListQuery{}, res, false
	}
	if q.To, res, ok = annotationTime("to", in.To); !ok {
		return annotations.ListQuery{}, res, false
	}
	if q.From != nil && q.To != nil && q.To.Before(*q.From) {
		return annotations.ListQuery{}, invalid("to must not be before from"), false
	}
	if len(in.Tags) > 0 {
		tags, valid := annotations.CleanTags(in.Tags)
		if !valid {
			return annotations.ListQuery{}, invalid("%s", annotations.MessageInvalidTags), false
		}
		q.Tags = tags
	}
	if res, ok := checkLength("cursor", in.Cursor, maxNameBytes); !ok {
		return annotations.ListQuery{}, res, false
	}
	after, valid := httpapi.DecodeTimeCursor(in.Cursor)
	if !valid {
		return annotations.ListQuery{}, invalid("cursor is not valid"), false
	}
	q.After = after
	return q, Result{}, true
}

func annotationTime(field, raw string) (*time.Time, Result, bool) {
	if raw == "" {
		return nil, Result{}, true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, invalid("%s must be an RFC 3339 time", field), false
	}
	return &t, Result{}, true
}

func annotationPage(page annotations.Page) annotationListOut {
	out := annotationListOut{Annotations: make([]annotationOut, 0, len(page.Annotations))}
	for _, a := range page.Annotations {
		out.Annotations = append(out.Annotations, annotationOf(a))
	}
	if page.Next != nil {
		next := httpapi.EncodeTimeCursor(page.Next.Time, page.Next.ID.String())
		out.NextCursor = &next
	}
	return out
}

func annotationOf(a annotations.Annotation) annotationOut {
	out := annotationOut{
		ID:            a.ID,
		DashboardID:   a.DashboardID,
		PanelID:       a.PanelID,
		TimeFrom:      a.From.UTC().Format(time.RFC3339),
		Text:          a.Text,
		Tags:          a.Tags,
		CreatedByName: a.CreatedByName,
	}
	if out.Tags == nil {
		out.Tags = []string{}
	}
	if a.To != nil {
		out.TimeTo = a.To.UTC().Format(time.RFC3339)
	}
	if who, ok := a.Actor(); ok {
		out.CreatedByLabel = who.Label()
	}
	return out
}

func (r *Registry) createAnnotation(ctx context.Context, input json.RawMessage) (Result, error) {
	if _, scoped := RunFrom(ctx); scoped {
		return invalid("%s is for MCP clients; a run saves only dashboards or a finding", nameCreateAnnotation), nil
	}
	if r.deps.Annotations == nil {
		return Result{}, errAnnotationsNotConfigured
	}
	var req annotations.CreateRequest
	if res, ok := decodeInput(input, &req); !ok {
		return res, nil
	}
	in, err := annotations.ParseCreate(req)
	if errors.Is(err, annotations.ErrInvalidDashboardID) {
		return invalid("dashboard_id must be a UUID from list_dashboards"), nil
	}
	if err != nil {
		return annotationFailure(err)
	}
	principal, _ := auth.FromContext(ctx)
	a, err := r.deps.Annotations.Create(ctx, principal, in)
	if err != nil {
		return annotationFailure(err)
	}
	return success("Added annotation "+a.ID, annotationOf(a)), nil
}

func annotationFailure(err error) (Result, error) {
	var bad *annotations.InvalidError
	switch {
	case errors.As(err, &bad):
		return invalid("%s", bad.Message), nil
	case errors.Is(err, annotations.ErrNotFound):
		return toolError(codeNotFound, "dashboard not found"), nil
	case errors.Is(err, annotations.ErrForbidden):
		return toolError(codeForbiddenRole, roleDeniedMessage), nil
	}
	return Result{}, err
}
