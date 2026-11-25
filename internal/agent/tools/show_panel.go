package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/dashboards"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/links"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
	"github.com/slashobserve/slashobserve/internal/queryproxy/proxyhttp"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	nameShowPanel = "show_panel"

	NameShowPanel = nameShowPanel

	descShowPanel = "Draw a chart in the conversation, in a host that supports MCP Apps: a query, or a panel of a saved dashboard. Give datasource and query (type picks the drawing, timeseries by default) or dashboard_id and panel. It runs like query_range, under the same limits, and returns the same summary plus a link that opens it in SlashObserve. A host without apps shows only the summary; use query_range to read numbers."

	propShowType  = `"type":{"type":"string","enum":["timeseries","barchart","stat","gauge","bargauge","piechart","table","trend","histogram","heatmap","state-timeline","status-history","logs"],"description":"How to draw a datasource and query. Default timeseries."}`
	propShowTitle = `"title":{"type":"string","maxLength":200,"description":"Title of the chart for a datasource and query. Default the start of the query."}`
	propShowPanel = `"panel":{"type":"string","maxLength":64,"description":"Panel id in the dashboard, from get_dashboard."}`

	schemaShowPanel = `{"type":"object","properties":{` + propDatasource + `,` + propQuery + `,` + propShowType + `,` + propShowTitle + `,` + propVersionDashboardID + `,` + propShowPanel + `,` + propFrom + `,` + propTo + `,"max_data_points":{"type":"integer","minimum":10,"maximum":1000,"description":"Target points per series. Default 200."},` + propVariables + `},"additionalProperties":false}`

	maxShowTitleChars = 200
	untitledChart     = "Query result"
	showTitleFromText = 80
)

var chartTypes = []spec.PanelType{
	spec.Timeseries, spec.BarChart, spec.Stat, spec.Gauge, spec.BarGauge, spec.PieChart, spec.Table, spec.Trend,
	spec.Histogram, spec.Heatmap, spec.StateTimeline, spec.StatusHistory, spec.Logs,
}

var chartFormats = map[spec.PanelType]string{
	spec.Table:   "table",
	spec.Heatmap: "heatmap",
	spec.Logs:    "logs",
}

func init() { registerEntries((*Registry).showPanelEntries) }

func (r *Registry) showPanelEntries() []entry {
	return []entry{
		{Tool{Name: nameShowPanel, Description: descShowPanel, InputSchema: json.RawMessage(schemaShowPanel), ReadOnly: true, MinRole: auth.RoleViewer}, r.showPanel},
	}
}

type showPanelInput struct {
	Datasource    string              `json:"datasource"`
	Query         string              `json:"query"`
	Type          string              `json:"type"`
	Title         string              `json:"title"`
	DashboardID   string              `json:"dashboard_id"`
	Panel         string              `json:"panel"`
	From          string              `json:"from"`
	To            string              `json:"to"`
	MaxDataPoints *int                `json:"max_data_points"`
	Variables     map[string][]string `json:"variables"`
}

type panelAppDatasource struct {
	UID  string `json:"uid"`
	Type string `json:"type"`
}

type panelAppOut struct {
	Title       string               `json:"title"`
	Panel       spec.Panel           `json:"panel"`
	Data        json.RawMessage      `json:"data"`
	Datasources []panelAppDatasource `json:"datasources"`
	Variables   map[string][]string  `json:"variables"`
	Timezone    string               `json:"timezone,omitempty"`
	From        string               `json:"from"`
	To          string               `json:"to"`
	OpenURL     string               `json:"open_url"`
}

type shownPanelOut struct {
	querySummary
	OpenURL string `json:"open_url"`
}

type shownPanel struct {
	title       string
	panel       spec.Panel
	dashboard   *spec.Dashboard
	panelID     string
	rng         datasource.TimeRange
	selections  map[string]queryproxy.Selection
	points      int
	datasources []string
	timezone    string
	from, to    string
	openURL     string
}

func (r *Registry) showPanel(ctx context.Context, input json.RawMessage) (Result, error) {
	var in showPanelInput
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	points, res, ok := boundedInt(in.MaxDataPoints, queryproxy.AgentMaxDataPoints, minDataPoints, maxDataPoints, "max_data_points")
	if !ok {
		return res, nil
	}
	var (
		shown shownPanel
		err   error
	)
	switch {
	case in.DashboardID != "" && (in.Datasource != "" || in.Query != ""):
		return invalid("give either datasource and query, or dashboard_id and panel, not both"), nil
	case in.DashboardID != "":
		shown, res, ok, err = r.savedPanel(ctx, in, points)
	case in.Datasource != "" || in.Query != "":
		shown, res, ok, err = r.adhocPanel(ctx, in, points)
	default:
		return invalid("give a datasource and a query, or a dashboard_id and a panel"), nil
	}
	if err != nil || !ok {
		return res, err
	}
	return r.drawPanel(ctx, shown)
}

func (r *Registry) adhocPanel(ctx context.Context, in showPanelInput, points int) (shownPanel, Result, bool, error) {
	panelType := spec.PanelType(cmp.Or(in.Type, string(spec.Timeseries)))
	if !slices.Contains(chartTypes, panelType) {
		return shownPanel{}, invalid("type must be one of %s", joinTypes(chartTypes)), false, nil
	}
	if utf8.RuneCountInString(in.Title) > maxShowTitleChars {
		return shownPanel{}, invalid("title must be at most %d characters", maxShowTitleChars), false, nil
	}
	inst, res, ok := r.instance(ctx, in.Datasource)
	if !ok {
		return shownPanel{}, res, false, nil
	}
	if res, ok := checkQueryText(in.Query); !ok {
		return shownPanel{}, res, false, nil
	}
	rng, res, ok := resolveRunRange(ctx, in.From, in.To)
	if !ok {
		return shownPanel{}, res, false, nil
	}
	dash, selections, res, ok := adhocDashboard(inst, panelType, instantInput{Datasource: in.Datasource, Query: in.Query, Variables: in.Variables}, chartFormats[panelType])
	if !ok {
		return shownPanel{}, res, false, nil
	}
	title := cmp.Or(in.Title, clip(strings.TrimSpace(in.Query), showTitleFromText), untitledChart)
	dash.Panels[0].Title = title
	from, to := cmp.Or(in.From, defaultFrom), cmp.Or(in.To, defaultTo)
	return shownPanel{
		title:       title,
		panel:       dash.Panels[0],
		dashboard:   dash,
		panelID:     adhocPanelID,
		rng:         rng,
		selections:  selections,
		points:      points,
		datasources: []string{inst.UID},
		from:        from,
		to:          to,
		openURL:     r.exploreURL(inst, in.Query, from, to),
	}, Result{}, true, nil
}

func (r *Registry) exploreURL(inst datasource.Instance, query, from, to string) string {
	address := url.Values{"from": {from}, "to": {to}, "q": {query}}
	switch t := typeOf(inst); {
	case t == spec.Prometheus:
		return links.Metrics(r.deps.PublicURL, inst.UID, address)
	case isSpanStore(t):
		return links.Traces(r.deps.PublicURL, inst.UID, address)
	}
	return links.Logs(r.deps.PublicURL, inst.UID, address)
}

func (r *Registry) savedPanel(ctx context.Context, in showPanelInput, points int) (shownPanel, Result, bool, error) {
	if in.Panel == "" {
		return shownPanel{}, invalid("panel is required with dashboard_id"), false, nil
	}
	if len(in.Variables) > 0 {
		return shownPanel{}, invalid("variables apply to a datasource and query; a saved panel uses the variables of its dashboard"), false, nil
	}
	id, res, ok := parseDashboardID(in.DashboardID)
	if !ok {
		return shownPanel{}, res, false, nil
	}
	res, ok, err := r.viewableDashboard(ctx, id, "dashboard not found")
	if err != nil || !ok {
		return shownPanel{}, res, false, err
	}
	dash, ver, err := r.deps.Dashboards.Get(ctx, id)
	if errors.Is(err, dashboards.ErrNotFound) {
		return shownPanel{}, toolError(codeNotFound, "dashboard not found"), false, nil
	}
	if err != nil {
		return shownPanel{}, Result{}, false, err
	}
	d := ver.Spec
	entry, found := d.FindDataPanel(in.Panel)
	if !found {
		return shownPanel{}, toolError(string(queryproxy.CodePanelNotFound), "this dashboard has no data panel "+clip(in.Panel, maxStepNameBytes)), false, nil
	}
	displayed := *entry.Panel
	link := links.Dashboard(r.deps.PublicURL, dash.ID, d.Title)
	address := url.Values{"viewPanel": {displayed.ID}}
	for name, value := range map[string]string{"from": in.From, "to": in.To} {
		if value != "" {
			address.Set(name, value)
		}
	}
	openURL := link + "?" + address.Encode()
	if !slices.Contains(chartTypes, displayed.Type) {
		return shownPanel{}, invalid("panel %s is a %s panel, which the chart view does not draw; open %s", displayed.ID, displayed.Type, link), false, nil
	}
	source := entry
	if reused, reuses := d.SourceOf(displayed.ID); reuses {
		if displayed.Queries[0].Hide {
			return shownPanel{}, invalid("panel %s has its query hidden, so it has no data to show", displayed.ID), false, nil
		}
		source = reused
	}
	now := Now(ctx)
	opts, err := spec.TimeOptionsFor(d, "")
	if err != nil {
		return shownPanel{}, invalid("the dashboard time zone is not known"), false, nil
	}
	from, to := cmp.Or(in.From, d.Time.From), cmp.Or(in.To, d.Time.To)
	rng, res, ok := resolveSavedRange(from, to, now, opts)
	if !ok {
		return shownPanel{}, res, false, nil
	}
	rng = queryproxy.PanelRange(source.Panel, rng, now, opts)
	if rng.To.Sub(rng.From) > queryproxy.MaxRange {
		return shownPanel{}, invalid("the time range must be at most 400 days"), false, nil
	}
	if source.Panel.MaxDataPoints > 0 {
		points = min(points, source.Panel.MaxDataPoints)
	}
	principal, _ := auth.FromContext(ctx)
	selections, err := r.deps.Runner.Defaults(ctx, d, rng, nil, queryproxy.UserFor(principal))
	if err != nil {
		res, err := datasourceFailure("Showed a panel of "+d.Title, err)
		return shownPanel{}, res, false, err
	}
	if err := r.deps.Runner.CheckPanelAccess(ctx, d, source.Panel.ID, selections); err != nil {
		res, err := datasourceFailure("Showed a panel of "+d.Title, err)
		return shownPanel{}, res, false, err
	}
	timezone := ""
	if d.Timezone != spec.TimezoneBrowser {
		timezone = d.Timezone
	}
	return shownPanel{
		title:       cmp.Or(displayed.Title, displayed.ID),
		panel:       displayed,
		dashboard:   d,
		panelID:     source.Panel.ID,
		rng:         rng,
		selections:  selections,
		points:      points,
		datasources: slices.Compact(slices.Sorted(slices.Values(slices.Concat(panelDatasources(*source.Panel), panelDatasources(displayed))))),
		timezone:    timezone,
		from:        from,
		to:          to,
		openURL:     openURL,
	}, Result{}, true, nil
}

func resolveSavedRange(from, to string, now time.Time, opts spec.TimeOptions) (datasource.TimeRange, Result, bool) {
	for field, value := range map[string]string{"from": from, "to": to} {
		if len(value) > maxTimeExprBytes {
			return datasource.TimeRange{}, invalid("%s must be at most %d bytes", field, maxTimeExprBytes), false
		}
	}
	start, err := spec.ResolveTime(from, now, false, opts)
	if err != nil {
		return datasource.TimeRange{}, invalid("from: %s", err), false
	}
	end, err := spec.ResolveTime(to, now, true, opts)
	if err != nil {
		return datasource.TimeRange{}, invalid("to: %s", err), false
	}
	if !start.Before(end) {
		return datasource.TimeRange{}, invalid("from must be before to"), false
	}
	return datasource.TimeRange{From: start, To: end}, Result{}, true
}

func (r *Registry) drawPanel(ctx context.Context, shown shownPanel) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, queryproxy.DraftLimits.Timeout+queryproxy.DraftGrace)
	defer cancel()
	principal, _ := auth.FromContext(ctx)
	step := "Showed a panel: " + shown.title
	drawn, err := proxyhttp.PanelData(ctx, r.deps.Runner, r.deps.Logger, queryproxy.Request{
		Dashboard:     shown.dashboard,
		PanelID:       shown.panelID,
		Range:         shown.rng,
		Selections:    shown.selections,
		MaxDataPoints: shown.points,
		Limits:        queryproxy.DraftLimits,
		Explore:       shown.panelID == adhocPanelID,
		User:          queryproxy.UserFor(principal),
	})
	if err != nil {
		return datasourceFailure(step, err)
	}
	app, err := json.Marshal(panelAppOut{
		Title:       shown.title,
		Panel:       shown.panel,
		Data:        drawn.Body,
		Datasources: r.panelAppDatasources(ctx, shown.datasources),
		Variables:   selectedValues(shown.selections),
		Timezone:    shown.timezone,
		From:        shown.from,
		To:          shown.to,
		OpenURL:     shown.openURL,
	})
	if err != nil {
		return Result{}, err
	}
	summary := summarize(drawn.Result)
	res := success(step+": "+summary.describe(), shownPanelOut{querySummary: summary, OpenURL: shown.openURL})
	res.App = app
	return res, nil
}

func (r *Registry) panelAppDatasources(ctx context.Context, uids []string) []panelAppDatasource {
	out := make([]panelAppDatasource, 0, len(uids))
	for _, uid := range uids {
		if inst, err := r.deps.Datasources.For(ctx, uid); err == nil {
			out = append(out, panelAppDatasource{UID: uid, Type: inst.Datasource.Type()})
		}
	}
	return out
}

func selectedValues(selections map[string]queryproxy.Selection) map[string][]string {
	out := make(map[string][]string, len(selections))
	for name, s := range selections {
		out[name] = s.Values
	}
	return out
}

func joinTypes(types []spec.PanelType) string {
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = string(t)
	}
	return strings.Join(names, ", ")
}
