package grafana

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	defaultTitle = "Imported dashboard"
	maxLinks     = 20
	maxTags      = 20
)

var (
	timePattern    = regexp.MustCompile(`^(now([+-][1-9][0-9]*[smhdwMy])?(/[smhdwMy])?|[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?(Z|[+-][0-9]{2}:[0-9]{2}))$`)
	refreshPattern = regexp.MustCompile(`^[1-9][0-9]{0,4}[smh]$`)
	zonePattern    = regexp.MustCompile(`^(browser|utc|[A-Za-z_]+(/[A-Za-z0-9_+-]+){1,2})$`)
	tagPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,39}$`)
	tagInvalid     = regexp.MustCompile(`[^a-z0-9_.-]`)
)

var silentDashboardKeys = map[string]bool{
	"id": true, "uid": true, "version": true, "editable": true, "gnetId": true,
	"graphTooltip": true, "fiscalYearStartMonth": true, "liveNow": true,
	"weekStart": true, "style": true, "iteration": true, "schemaVersion": true,
	"__inputs": true, "__requires": true, "__elements": true, "timepicker": true,
	"hideControls": true, "preload": true, "revision": true,
	"title": true, "description": true, "tags": true, "time": true, "refresh": true,
	"timezone": true, "templating": true, "links": true, "annotations": true,
	"panels": true, "rows": true,
}

type converter struct {
	src         *source
	reg         *registry
	head        spec.Dashboard
	notes       []Note
	vars        []*builtVar
	droppedVars map[string]bool
	built       []*builtPanel
	emptyRows   []string
	noRepeat    map[string]bool
	repeatNoted map[string]bool
	staleRows   []string
}

func Convert(raw []byte, opts Options) (*Result, error) {
	src, err := load(raw)
	if err != nil {
		return nil, err
	}
	reg := newRegistry(src)
	if err := reg.resolve(opts); err != nil {
		return nil, err
	}
	c := &converter{src: src, reg: reg, droppedVars: map[string]bool{}, noRepeat: map[string]bool{}, repeatNoted: map[string]bool{}}
	c.convertHead()
	c.convertVariables()
	c.convertPanels()
	c.noteRepeats()
	parsed, err := c.settle()
	if err != nil {
		return nil, err
	}
	result := &Result{
		Summary:     src.summary(),
		Datasources: reg.sources(),
		ready:       reg.ready(),
		Report:      c.report(),
	}
	for _, v := range c.vars {
		result.variablePath = append(result.variablePath, v.path)
	}
	if result.ready {
		result.Spec = parsed
	}
	return result, nil
}

func (c *converter) convertHead() {
	dash := c.src.dash
	d := &c.head
	d.SpecVersion = spec.Version

	title := strings.TrimSpace(asString(dash["title"]))
	if title == "" {
		title = defaultTitle
	}
	var cut bool
	if d.Title, cut = cutRunes(title, 200); cut {
		c.note("title", "The title was cut to 200 characters.", "title")
	}
	if description := asString(dash["description"]); description != "" {
		if d.Description, cut = cutRunes(description, 2000); cut {
			c.note("description", "The description was cut to 2000 characters.", "description")
		}
	}
	c.convertTags(dash)
	c.convertTime(dash)
	c.convertRefresh(dash)
	c.convertTimezone(dash)
	c.convertLinks(dash)
	c.convertAnnotations(dash)
	if picker, ok := asObject(dash["timepicker"]); ok && truthy(picker["hidden"]) {
		c.note("time picker", "The time picker is always shown.", "timepicker.hidden")
	}
	var unknown []string
	for _, key := range sortedKeys(dash) {
		if !silentDashboardKeys[key] && present(dash[key]) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		c.note("dashboard options", "Some Grafana dashboard options are not supported yet.", unknown...)
	}
}

func (c *converter) convertTags(dash map[string]any) {
	items, _ := asArray(dash["tags"])
	var dropped []string
	for i, item := range items {
		tag, ok := item.(string)
		path := fmt.Sprintf("tags[%d]", i)
		if !ok {
			dropped = append(dropped, path)
			continue
		}
		tag = tagInvalid.ReplaceAllString(strings.ToLower(tag), "-")
		tag, _ = cutRunes(tag, 40)
		switch {
		case !tagPattern.MatchString(tag), slices.Contains(c.head.Tags, tag), len(c.head.Tags) >= maxTags:
			dropped = append(dropped, path)
		default:
			c.head.Tags = append(c.head.Tags, tag)
		}
	}
	if len(dropped) > 0 {
		c.note("tags", "Some tags are empty, repeated, start with a symbol or are past the first 20, and are not kept.", dropped...)
	}
}

func (c *converter) convertTime(dash map[string]any) {
	c.head.Time = spec.TimeRange{From: "now-6h", To: "now"}
	t, ok := asObject(dash["time"])
	if !ok {
		return
	}
	from, to := asString(t["from"]), asString(t["to"])
	if !timePattern.MatchString(from) || !timePattern.MatchString(to) {
		c.note("time", "The default time range is not supported, so the last 6 hours is used.", "time")
		return
	}
	c.head.Time = spec.TimeRange{From: from, To: to}
}

func (c *converter) convertRefresh(dash map[string]any) {
	switch v := dash["refresh"].(type) {
	case string:
		switch {
		case v == "":
		case refreshPattern.MatchString(v):
			c.head.Refresh = v
		default:
			c.note("refresh", "The auto-refresh interval is not supported and is not kept.", "refresh")
		}
	case bool, nil:
	default:
		c.note("refresh", "The auto-refresh interval is not supported and is not kept.", "refresh")
	}
}

func (c *converter) convertTimezone(dash map[string]any) {
	zone := asString(dash["timezone"])
	if strings.EqualFold(zone, "utc") {
		zone = "utc"
	}
	switch {
	case zone == "", zone == "browser", zone == "default":
	case zonePattern.MatchString(zone):
		c.head.Timezone = zone
	default:
		c.note("timezone", "The time zone is not supported and is not kept.", "timezone")
	}
}

func (c *converter) convertLinks(dash map[string]any) {
	items, _ := asArray(dash["links"])
	var unsupported, invalid []string
	for i, item := range items {
		link, ok := asObject(item)
		path := fmt.Sprintf("links[%d]", i)
		if !ok || asString(link["type"]) != "link" {
			unsupported = append(unsupported, path)
			continue
		}
		url := strings.TrimSpace(asString(link["url"]))
		title := strings.TrimSpace(asString(link["title"]))
		if title == "" {
			title = url
		}
		_, longURL := cutRunes(url, 2000)
		if url == "" || longURL || len(c.head.Links) >= maxLinks {
			invalid = append(invalid, path)
			continue
		}
		title, _ = cutRunes(title, 200)
		c.head.Links = append(c.head.Links, spec.Link{Title: title, URL: url, NewTab: truthy(link["targetBlank"])})
	}
	if len(unsupported) > 0 {
		c.note("dashboard links", "Only plain links are supported; links to other dashboards and other kinds are not kept.", unsupported...)
	}
	if len(invalid) > 0 {
		c.note("dashboard links", "Some links have no address, a very long address or are past the first 20, and are not kept.", invalid...)
	}
}

func (c *converter) convertAnnotations(dash map[string]any) {
	annotations, _ := asObject(dash["annotations"])
	items, _ := asArray(annotations["list"])
	var paths []string
	for i, item := range items {
		if a, ok := asObject(item); ok && truthy(a["builtIn"]) {
			continue
		}
		paths = append(paths, fmt.Sprintf("annotations.list[%d]", i))
	}
	if len(paths) > 0 {
		c.note("annotations", "Annotation queries are not supported yet.", paths...)
	}
}

func (c *converter) convertPanels() {
	for _, n := range c.src.nodes {
		if !n.row {
			c.built = append(c.built, c.convertPanel(n))
			continue
		}
		title := strings.TrimSpace(n.title)
		if title == "" {
			title = "Row"
			c.emptyRows = append(c.emptyRows, n.path)
		}
		title, cut := cutRunes(title, 200)
		if cut {
			c.note("rows", "A row title was cut to 200 characters.", n.path+".title")
		}
		if n.staleKids {
			c.staleRows = append(c.staleRows, n.path+".panels")
		}
		row := &builtPanel{
			node:   n,
			panel:  spec.Panel{ID: n.id, Type: spec.Row, Title: title, GridPos: spec.GridPos{X: n.pos.x, Y: n.pos.y, W: n.pos.w, H: n.pos.h}, Collapsed: n.collapsed},
			repeat: asString(n.raw["repeat"]),
		}
		for _, child := range n.children {
			child.rowTitle = ptr(title)
			row.children = append(row.children, c.convertPanel(child))
		}
		c.built = append(c.built, row)
	}
	if len(c.emptyRows) > 0 {
		c.note("rows", "Rows without a title are named Row.", c.emptyRows...)
	}
	if len(c.staleRows) > 0 {
		c.note("rows", "These rows are not collapsed but still list panels inside them; Grafana ignores those, and so does the import.", c.staleRows...)
	}
}

func (c *converter) noteRepeats() {
	if len(c.src.clones) > 0 {
		c.note("repeated panels", "Grafana saved copies of repeated panels and rows; SlashObserve repeats them from their variable, so only the original is imported.", c.src.clones...)
	}
}

func (c *converter) assemble() (spec.Dashboard, map[string]*builtPanel) {
	d := c.head
	d.Links = slices.Clone(c.head.Links)
	d.Variables = nil
	declared := map[string]bool{}
	for _, v := range c.vars {
		d.Variables = append(d.Variables, v.v)
		declared[v.v.Name] = true
	}
	index := map[string]*builtPanel{}
	d.Panels = make([]spec.Panel, 0, len(c.built))
	for i, bp := range c.built {
		key := "/panels/" + strconv.Itoa(i)
		index[key] = bp
		if !bp.isRow() {
			c.placeholderDroppedReferences(bp)
			d.Panels = append(d.Panels, bp.panel)
			continue
		}
		row := bp.panel
		row.Repeat = ""
		if bp.repeat != "" {
			switch {
			case declared[bp.repeat] && !c.noRepeat[bp.node.id]:
				row.Repeat = bp.repeat
			case !c.repeatNoted[bp.node.id]:
				c.repeatNoted[bp.node.id] = true
				c.note("row "+row.Title, fmt.Sprintf("Its repeat variable $%s could not be imported, so the row is not repeated.", bp.repeat), bp.node.path+".repeat")
			}
		}
		row.Panels = nil
		for j, child := range bp.children {
			index[key+"/panels/"+strconv.Itoa(j)] = child
			c.placeholderDroppedReferences(child)
			row.Panels = append(row.Panels, child.panel)
		}
		d.Panels = append(d.Panels, row)
	}
	dropDefaults(&d)
	return d, index
}

func (c *converter) placeholderDroppedReferences(bp *builtPanel) {
	if bp.isPlaceholder() || len(c.droppedVars) == 0 {
		return
	}
	var names []string
	if bp.panel.Repeat != "" {
		names = append(names, bp.panel.Repeat)
	}
	for _, q := range bp.panel.Queries {
		refs, err := spec.ScanQuery(q.Query)
		if err != nil {
			continue
		}
		for _, ref := range spec.FlattenReferences(refs) {
			if ref.Kind == spec.VariableRef {
				names = append(names, ref.Name)
			}
		}
	}
	for _, name := range names {
		if c.droppedVars[name] {
			*bp = *placeholderPanel(bp.node, fmt.Sprintf("It uses variable %s, which could not be imported.", name))
			return
		}
	}
}

func (c *converter) panelCount() int {
	n := 0
	c.src.walk(func(*panelNode) { n++ })
	return n
}

func (c *converter) settle() (*spec.Dashboard, error) {
	limit := c.panelCount() + len(c.vars) + len(c.head.Links) + 4
	var last error
	for range limit {
		d, index := c.assemble()
		data, err := json.Marshal(d)
		if err != nil {
			return nil, inputErrorf(codeNotADashboard, "the dashboard could not be converted: %v", err)
		}
		parsed, err := spec.Parse(data)
		if err == nil {
			return parsed, nil
		}
		last = err
		var ve *spec.ValidationError
		if !errors.As(err, &ve) || !c.repair(ve.Issues, index) {
			break
		}
	}
	return nil, inputErrorf(codeNotADashboard, "the dashboard could not be converted: %v", last)
}

func splitPointer(p string) []string {
	if p == "" {
		return nil
	}
	tokens := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for i, t := range tokens {
		tokens[i] = strings.NewReplacer("~1", "/", "~0", "~").Replace(t)
	}
	return tokens
}

func sentence(message string) string {
	return strings.TrimRight(message, ".") + "."
}

type repairs struct {
	changed       bool
	dropVars      map[int]string
	clearDefaults map[int]string
	dropLinks     map[int]bool
}

func (r *repairs) variable(i int, t []string, message string) {
	if len(t) == 3 && t[2] == "default" {
		r.clearDefaults[i] = message
		return
	}
	r.dropVars[i] = message
}

func (c *converter) repair(issues []spec.Issue, index map[string]*builtPanel) bool {
	r := &repairs{dropVars: map[int]string{}, clearDefaults: map[int]string{}, dropLinks: map[int]bool{}}
	for _, issue := range issues {
		c.collectRepair(r, issue, splitPointer(issue.Path), index)
	}
	c.applyRepairs(r)
	return r.changed
}

func (c *converter) collectRepair(r *repairs, issue spec.Issue, t []string, index map[string]*builtPanel) {
	if len(t) == 0 {
		return
	}
	if t[0] == "time" {
		r.changed = c.resetTime() || r.changed
		return
	}
	if len(t) < 2 {
		return
	}
	i, err := strconv.Atoi(t[1])
	switch t[0] {
	case "variables":
		if err == nil && i >= 0 && i < len(c.vars) {
			r.variable(i, t, issue.Message)
		}
	case "links":
		if err == nil && i >= 0 && i < len(c.head.Links) {
			r.dropLinks[i] = true
		}
	case "panels":
		r.changed = c.repairPanel(issue, t, index) || r.changed
	}
}

func (c *converter) repairPanel(issue spec.Issue, t []string, index map[string]*builtPanel) bool {
	key := "/panels/" + t[1]
	if len(t) >= 4 && t[2] == "panels" {
		key += "/panels/" + t[3]
	}
	bp := index[key]
	switch {
	case bp == nil || bp.isPlaceholder():
		return false
	case bp.isRow():
		if t[len(t)-1] != "repeat" || c.noRepeat[bp.node.id] {
			return false
		}
		c.noRepeat[bp.node.id] = true
		return true
	}
	*bp = *placeholderPanel(bp.node, "SlashObserve could not validate this panel: "+sentence(issue.Message))
	return true
}

func (c *converter) applyRepairs(r *repairs) {
	for i, message := range r.clearDefaults {
		if _, dropped := r.dropVars[i]; dropped {
			continue
		}
		v := c.vars[i]
		v.v.Default = nil
		c.note("variable "+v.v.Name, "SlashObserve could not validate the default selection: "+sentence(message), v.path+".current")
		r.changed = true
	}
	order := slices.Sorted(maps.Keys(r.dropVars))
	slices.Reverse(order)
	for _, i := range order {
		v := c.vars[i]
		c.droppedVars[v.v.Name] = true
		c.note("variable "+v.v.Name, "SlashObserve could not validate this variable: "+sentence(r.dropVars[i]), v.path)
		c.vars = slices.Delete(c.vars, i, i+1)
		r.changed = true
	}
	linkOrder := slices.Sorted(maps.Keys(r.dropLinks))
	slices.Reverse(linkOrder)
	for _, i := range linkOrder {
		c.head.Links = slices.Delete(c.head.Links, i, i+1)
		c.note("dashboard links", "SlashObserve could not validate a link, so it is not kept.", "links")
		r.changed = true
	}
}

func (c *converter) resetTime() bool {
	def := spec.TimeRange{From: "now-6h", To: "now"}
	if c.head.Time == def {
		return false
	}
	c.head.Time = def
	c.note("time", "The default time range is not supported, so the last 6 hours is used.", "time")
	return true
}

func (c *converter) report() Report {
	report := Report{ImporterVersion: ImporterVersion, Panels: []PanelLine{}, Notes: []Note{}}
	for _, bp := range c.built {
		if !bp.isRow() {
			report.Panels = append(report.Panels, bp.line)
			continue
		}
		for _, child := range bp.children {
			report.Panels = append(report.Panels, child.line)
		}
	}
	report.Notes = append(report.Notes, c.notes...)
	report.recount()
	return report
}
