package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const (
	nameLogContext = "log_context"
	descLogContext = "Read the log lines just before and after one line, by its time_ns anchor from a query_instant or log facets result, on Loki, ClickHouse, Elastic or OpenSearch. Use it to see what happened around an error line."

	schemaLogContext = `{"type":"object","properties":{` + propDatasource + `,` + propQuery + `,"anchor":{"type":"string","minLength":1,"maxLength":20,"description":"The line's time_ns: its time as a nanosecond epoch decimal string."},"before":{"type":"integer","minimum":0,"maximum":100,"description":"Lines to read before the line. Default 20."},"after":{"type":"integer","minimum":0,"maximum":100,"description":"Lines to read after the line. Default 20."}},"required":["datasource","query","anchor"],"additionalProperties":false}`

	defaultContextLines = 20
	maxContextLines     = 100
	contextWindow       = time.Hour
	notLineSource       = "log_context reads Loki, ClickHouse, Elasticsearch and OpenSearch datasources"
)

type logContextInput struct {
	Datasource string `json:"datasource"`
	Query      string `json:"query"`
	Anchor     string `json:"anchor"`
	Before     *int   `json:"before"`
	After      *int   `json:"after"`
}

type contextLine struct {
	Time   string            `json:"time"`
	TimeNs string            `json:"time_ns"`
	Body   string            `json:"body"`
	Labels map[string]string `json:"labels,omitempty"`
}

type logContextOut struct {
	Anchor          string        `json:"anchor"`
	Before          []contextLine `json:"before"`
	After           []contextLine `json:"after"`
	BeforeTruncated bool          `json:"before_truncated"`
	AfterTruncated  bool          `json:"after_truncated"`
}

func init() { registerEntries((*Registry).contextEntries) }

func (r *Registry) contextEntries() []entry {
	return []entry{
		{Tool{Name: nameLogContext, Description: descLogContext, InputSchema: json.RawMessage(schemaLogContext), ReadOnly: true, MinRole: auth.RoleViewer}, r.logContext},
	}
}

func (r *Registry) logContext(ctx context.Context, input json.RawMessage) (Result, error) {
	var in logContextInput
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
	anchor, err := strconv.ParseInt(in.Anchor, 10, 64)
	if err != nil {
		return invalid("anchor must be a nanosecond epoch time, the time_ns of a line"), nil
	}
	before, res, ok := boundedInt(in.Before, defaultContextLines, 0, maxContextLines, "before")
	if !ok {
		return res, nil
	}
	after, res, ok := boundedInt(in.After, defaultContextLines, 0, maxContextLines, "after")
	if !ok {
		return res, nil
	}
	source, ok := inst.Datasource.(datasource.LineSource)
	if !ok {
		return invalid(notLineSource), nil
	}
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	query := datasource.Query{Text: in.Query, Limits: queryproxy.DraftLimits, User: queryproxy.UserFor(principal)}
	at := time.Unix(0, anchor)
	sides := [2]datasource.LinesRequest{
		{Query: query, Direction: datasource.Backward, Limit: before, Range: datasource.TimeRange{From: at.Add(-contextWindow), To: at}},
		{Query: query, Direction: datasource.Forward, Limit: after, Range: datasource.TimeRange{From: at.Add(time.Nanosecond), To: at.Add(contextWindow)}},
	}
	var frames [2][]datasource.Frame
	g, gctx := errgroup.WithContext(ctx)
	for i := range sides {
		g.Go(func() error {
			var err error
			frames[i], err = r.readLines(gctx, inst.UID, source, sides[i])
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return datasourceFailure("Read lines around "+in.Anchor+" on "+inst.UID, err)
	}
	out := logContextOut{Anchor: in.Anchor}
	out.Before, out.BeforeTruncated = contextLines(frames[0], datasource.Backward)
	out.After, out.AfterTruncated = contextLines(frames[1], datasource.Forward)
	n := len(out.Before) + len(out.After)
	return success(fmt.Sprintf("Read %s around %s on %s", plural(n, "line", "lines"), in.Anchor, inst.UID), out), nil
}

func (r *Registry) readLines(ctx context.Context, datasourceUID string, source datasource.LineSource, req datasource.LinesRequest) ([]datasource.Frame, error) {
	if req.Limit == 0 {
		return nil, nil
	}
	if err := req.Range.Validate(); err != nil {
		return nil, err
	}
	release, err := r.acquireUpstream(ctx, datasourceUID)
	if err != nil {
		return nil, err
	}
	defer release()
	return source.Lines(ctx, req)
}

type timedLine struct {
	ns   int64
	line contextLine
}

func contextLines(frames []datasource.Frame, direction datasource.LineDirection) ([]contextLine, bool) {
	var timed []timedLine
	for _, f := range frames {
		timed = append(timed, frameLines(f)...)
	}
	slices.SortStableFunc(timed, func(a, b timedLine) int {
		if direction == datasource.Backward {
			return cmp.Compare(b.ns, a.ns)
		}
		return cmp.Compare(a.ns, b.ns)
	})
	lines := make([]contextLine, len(timed))
	for i, t := range timed {
		lines[i] = t.line
	}
	lines, cut := fit(lines, tableRowsBudget)
	return lines, cut || datasource.MergeMeta(frames).Truncated
}

func frameLines(f datasource.Frame) []timedLine {
	timeField, bodyField := -1, -1
	for i, field := range f.Fields {
		switch {
		case timeField < 0 && field.Type == datasource.FieldTime:
			timeField = i
		case bodyField < 0 && field.Name == "Body" && field.Type == datasource.FieldString:
			bodyField = i
		}
	}
	if timeField < 0 {
		return nil
	}
	labels := clippedLabels(f.Labels)
	out := make([]timedLine, 0, f.Rows())
	for row := range f.Rows() {
		at, ok := lineTime(f, timeField, row)
		if !ok {
			continue
		}
		line := contextLine{Time: at.UTC().Format(time.RFC3339Nano), TimeNs: strconv.FormatInt(at.UnixNano(), 10), Labels: lineLabels(f, labels, timeField, bodyField, row)}
		if bodyField >= 0 {
			line.Body, _ = cellValue(f.Fields[bodyField].Values[row], maxSummaryCell).(string)
		}
		out = append(out, timedLine{ns: at.UnixNano(), line: line})
	}
	return out
}

func lineTime(f datasource.Frame, timeField, row int) (time.Time, bool) {
	if ns, ok := datasource.TimeNsOf(f, row); ok {
		return time.Unix(0, ns), true
	}
	values := f.Fields[timeField].Values
	if row >= len(values) {
		return time.Time{}, false
	}
	at, ok := values[row].(time.Time)
	return at, ok
}

func lineLabels(f datasource.Frame, labels map[string]string, timeField, bodyField, row int) map[string]string {
	var out map[string]string
	for i, field := range f.Fields {
		if i == timeField || i == bodyField || field.Name == datasource.TimeNsFieldName || row >= len(field.Values) || field.Values[row] == nil {
			continue
		}
		if out == nil {
			out = make(map[string]string, len(labels)+len(f.Fields))
			for k, v := range labels {
				out[k] = v
			}
		}
		out[clip(field.Name, maxStepNameBytes)] = clip(fmt.Sprint(cellValue(field.Values[row], maxSummaryCell)), maxSummaryCell)
	}
	if out == nil {
		return labels
	}
	return out
}
