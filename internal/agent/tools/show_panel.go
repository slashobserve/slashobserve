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

	propShowTypeHead = `"type":{"type":"string","enum":`
	propShowTypeTail = `,"description":"How to draw a datasource and query. Default timeseries."}`
	propShowTitle    = `"title":{"type":"string","maxLength":200,"description":"Title of the chart for a datasource and query. Default the start of the query."}`
	propShowPanel    = `"panel":{"type":"string","maxLength":64,"description":"Panel id in the dashboard, from get_dashboard."}`

	schemaShowPanelHead = `{"type":"object","properties":{`
	schemaShowPanelTail = `,` + propShowTitle + `,` + propVersionDashboardID + `,` + propShowPanel + `,` + propFrom + `,` + propTo + `,"max_data_points":{"type":"integer","minimum":10,"maximum":1000,"description":"Target points per series. Default 200."},` + propVariables + `},"additionalProperties":false}`

	maxShowTitleChars = 200
	untitledChart     = "Query result"
	showTitleFromText = 80
)

var chartTypes = []spec.PanelType{
	spec.Timeseries, spec.BarChart, spec.Stat, spec.Gauge, spec.BarGauge, spec.PieChart, spec.Table, spec.Trend,
	spec.Histogram, spec.Heatmap, spec.StateTimeline, spec.StatusHistory, spec.Logs,
}

var schemaShowPanel = showPanelSchema()

func showPanelSchema() string {
	names, err := json.Marshal(chartTypeNames())
	if err != nil {
		panic(err)
	}
	return schemaShowPanelHead + propDatasource + `,` + propQuery + `,` + propShowTypeHead + string(names) + propShowTypeTail + schemaShowPanelTail
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
	explore     bool
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
	case in.DashboardID != "" && (in.Datasource != "" || in.Query != "" || in.Type != "" || in.Title != ""):
		return invalid("give either datasource and query, or dashboard_id and panel, not both; type and title apply only to a datasource and query"), nil
	case in.DashboardID == "" && in.Panel != "":
		return invalid("panel needs dashboard_id; for a datasource and query leave panel out"), nil
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
		return shownPanel{}, invalid("type must be one of %s", strings.Join(chartTypeNames(), ", ")), false, nil
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
		explore:     true,
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
	return r.panelOfDashboard(ctx, dash.ID, ver.Spec, in, points)
}

func (r *Registry) panelOfDashboard(ctx context.Context, dashboardID string, d *spec.Dashboard, in showPanelInput, points int) (shownPanel, Result, bool, error) {
	listed, exists := d.FindPanel(in.Panel)
	if !exists {
		return shownPanel{}, toolError(string(queryproxy.CodePanelNotFound), "this dashboard has no panel "+clip(in.Panel, maxStepNameBytes)), false, nil
	}
	link := links.Dashboard(r.deps.PublicURL, dashboardID, d.Title)
	entry, found := d.FindDataPanel(in.Panel)
	if !found || !slices.Contains(chartTypes, listed.Panel.Type) {
		return shownPanel{}, invalid("panel %s is a %s panel, which the chart view does not draw; open %s", listed.Panel.ID, listed.Panel.Type, link), false, nil
	}
	displayed := *entry.Panel
	source := entry
	if reused, reuses := d.SourceOf(displayed.ID); reuses {
		if displayed.Queries[0].Hide {
			return shownPanel{}, invalid("panel %s has its query hidden, so it has no data to show", displayed.ID), false, nil
		}
		source = reused
	}
	if res, ok := checkTimeExprs(in.From, in.To); !ok {
		return shownPanel{}, res, false, nil
	}
	window, err := proxyhttp.ResolvePanelWindow(d, source.Panel, in.From, in.To, points, Now(ctx))
	if errors.Is(err, queryproxy.ErrInvalidRequest) {
		return shownPanel{}, invalid("%s", strings.TrimPrefix(err.Error(), queryproxy.ErrInvalidRequest.Error()+": ")), false, nil
	}
	if err != nil {
		return shownPanel{}, Result{}, false, err
	}
	principal, _ := auth.FromContext(ctx)
	selections, err := r.deps.Runner.Defaults(ctx, d, window.Range, nil, queryproxy.UserFor(principal))
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
		rng:         window.Range,
		selections:  selections,
		points:      window.MaxDataPoints,
		datasources: resolvedDatasources(d, selections, slices.Concat(panelDatasources(*source.Panel), panelDatasources(displayed))),
		timezone:    timezone,
		openURL:     savedPanelURL(link, displayed.ID, in.From, in.To),
	}, Result{}, true, nil
}

func savedPanelURL(dashboardLink, panelID, from, to string) string {
	address := url.Values{"viewPanel": {panelID}}
	for name, value := range map[string]string{"from": from, "to": to} {
		if value != "" {
			address.Set(name, value)
		}
	}
	return dashboardLink + "?" + address.Encode()
}

func resolvedDatasources(d *spec.Dashboard, selections map[string]queryproxy.Selection, refs []string) []string {
	uids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if uid, err := queryproxy.ResolveDatasourceUID(d, ref, selections); err == nil {
			uids = append(uids, uid)
		}
	}
	return slices.Compact(slices.Sorted(slices.Values(uids)))
}

func checkTimeExprs(from, to string) (Result, bool) {
	for field, value := range map[string]string{"from": from, "to": to} {
		if len(value) > maxTimeExprBytes {
			return invalid("%s must be at most %d bytes", field, maxTimeExprBytes), false
		}
	}
	return Result{}, true
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
		Explore:       shown.explore,
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
		From:        drawn.Result.Range.From.UTC().Format(time.RFC3339),
		To:          drawn.Result.Range.To.UTC().Format(time.RFC3339),
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

func chartTypeNames() []string {
	names := make([]string, len(chartTypes))
	for i, t := range chartTypes {
		names[i] = string(t)
	}
	return names
}
