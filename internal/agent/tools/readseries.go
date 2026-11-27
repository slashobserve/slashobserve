package tools

import (
	"context"
	"encoding/json"
	"maps"
	"math"
	"time"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	nameReadSeries = "read_series"

	defaultSeriesPoints = 120
	maxSeriesPoints     = 150
	maxReadSeries       = 50
	maxReadSeriesBytes  = 120 << 10

	descReadSeries   = "Read every point of a range query: each series' labels and its points as [unix seconds, value], for comparing a window with the one before it across every service. At most 50 series and about max_data_points points per series; a result over that is cut and says truncated. To check a query or read a value, use query_range or measure_window, which summarize."
	schemaReadSeries = `{"type":"object","properties":{` + propDatasource + `,` + propQuery + `,` + propFrom + `,` + propTo + `,"max_data_points":{"type":"integer","minimum":10,"maximum":150,"description":"Target points per series. Default 120."}},"required":["datasource","query"],"additionalProperties":false}`
)

func init() { registerEntries((*Registry).readSeriesEntries) }

func (r *Registry) readSeriesEntries() []entry {
	return []entry{
		{Tool{Name: nameReadSeries, Description: descReadSeries, InputSchema: json.RawMessage(schemaReadSeries), ReadOnly: true, AgentOnly: true, OnlyIn: ModeInvestigate, MinRole: auth.RoleViewer}, r.readSeries},
	}
}

type readSeriesIn struct {
	Datasource    string `json:"datasource"`
	Query         string `json:"query"`
	From          string `json:"from"`
	To            string `json:"to"`
	MaxDataPoints *int   `json:"max_data_points"`
}

type seriesPoints struct {
	Labels map[string]string `json:"labels,omitempty"`
	Points [][2]float64      `json:"points"`
}

type readSeriesOut struct {
	From      string         `json:"from"`
	To        string         `json:"to"`
	Series    []seriesPoints `json:"series"`
	Truncated bool           `json:"truncated,omitempty"`
}

func (r *Registry) readSeries(ctx context.Context, input json.RawMessage) (Result, error) {
	var in readSeriesIn
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	points, res, ok := boundedInt(in.MaxDataPoints, defaultSeriesPoints, minDataPoints, maxSeriesPoints, "max_data_points")
	if !ok {
		return res, nil
	}
	inst, res, ok := r.instance(ctx, in.Datasource)
	if !ok {
		return res, nil
	}
	if res, ok := checkQueryText(in.Query); !ok {
		return res, nil
	}
	rng, res, ok := resolveRunRange(ctx, in.From, in.To)
	if !ok {
		return res, nil
	}
	dash, selections, res, ok := adhocDashboard(inst, spec.Timeseries, instantInput{Datasource: inst.UID, Query: in.Query}, "")
	if !ok {
		return res, nil
	}
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	out, err := r.deps.Runner.RunPanel(ctx, queryproxy.Request{
		Dashboard:     dash,
		PanelID:       adhocPanelID,
		Range:         rng,
		Selections:    selections,
		MaxDataPoints: points,
		Limits:        queryproxy.DraftLimits,
		Explore:       true,
		User:          queryproxy.UserFor(principal),
	})
	if err != nil {
		return datasourceFailure("Read series on "+inst.UID, err)
	}
	read := pointsOf(out.Frames(), points)
	read.From, read.To = rng.From.UTC().Format(time.RFC3339), rng.To.UTC().Format(time.RFC3339)
	return success("Read "+plural(len(read.Series), "series", "series")+" on "+inst.UID, read), nil
}

func pointsOf(frames []datasource.Frame, points int) readSeriesOut {
	out := readSeriesOut{Series: []seriesPoints{}}
	size := 0
	for _, f := range frames {
		if !f.IsSeries() {
			continue
		}
		times := f.Fields[0].Values
		for _, field := range f.Fields[1:] {
			s := seriesPoints{Labels: maps.Clone(f.Labels), Points: [][2]float64{}}
			for i, v := range field.Values {
				at, isTime := times[i].(time.Time)
				x, isNum := v.(float64)
				if !isTime || !isNum || math.IsNaN(x) || math.IsInf(x, 0) {
					continue
				}
				if len(s.Points) == 2*points {
					out.Truncated = true
					break
				}
				s.Points = append(s.Points, [2]float64{float64(at.Unix()), x})
			}
			raw, _ := json.Marshal(s)
			if len(out.Series) == maxReadSeries || size+len(raw) > maxReadSeriesBytes {
				out.Truncated = true
				return out
			}
			size += len(raw) + 1
			out.Series = append(out.Series, s)
		}
	}
	return out
}
