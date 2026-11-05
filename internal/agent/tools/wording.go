package tools

import (
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"
)

type wording struct{ replacer *strings.Replacer }

type variant struct {
	name  string
	tools []string
	mcp   map[string]string
	agent map[string]string
}

var variants = []variant{
	{name: "guide", tools: []string{nameCreateDashboard, nameUpdateDashboard, nameGetSpecGuide},
		mcp:   map[string]string{"{guide_first}": mcpGuideFirst, "{spec_guide_desc}": mcpSpecGuideDesc},
		agent: map[string]string{"{spec_guide_desc}": agentSpecGuideDesc}},
	{name: "folder", tools: []string{nameCreateDashboard},
		mcp: map[string]string{"{folder_prop}": "," + propFolderID}},
	{name: "run target", tools: []string{nameUpdateDashboard, nameUpdatePanels, nameGetPanels},
		mcp: map[string]string{
			"{target_props}":     propDashID + "," + propParentVersion + ",",
			"{target_required}":  `"dashboard_id","parent_version",`,
			"{panels_required}":  `"dashboard_id",`,
			"{panels_dashboard}": mcpPanelsDashboard,
		},
		agent: map[string]string{"{panels_dashboard}": agentPanelsDashboard}},
	{name: "build", tools: []string{nameCreateDashboard, nameUpdateDashboard, nameUpdatePanels},
		mcp: map[string]string{"{upsert_max}": strconv.Itoa(maxUpsertPanels), "{layout_gridpos}": mcpLayoutGridPos},
		agent: map[string]string{
			"{build_note}":     agentBuildNote,
			"{build_props}":    agentBuildProps,
			"{row_prop}":       agentRowProp,
			"{upsert_max}":     "1",
			"{layout_gridpos}": agentLayoutGridPos,
		}},
}

func (v variant) placeholders() []string {
	keys := slices.Collect(maps.Keys(v.mcp))
	for key := range v.agent {
		if !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

func surfaceWording(fills func(variant) map[string]string) wording {
	var pairs []string
	for _, v := range variants {
		for _, placeholder := range v.placeholders() {
			pairs = append(pairs, placeholder, fills(v)[placeholder])
		}
	}
	return wording{strings.NewReplacer(pairs...)}
}

var (
	mcpWording   = surfaceWording(func(v variant) map[string]string { return v.mcp })
	agentWording = surfaceWording(func(v variant) map[string]string { return v.agent })
)

const (
	mcpGuideFirst = " Read get_spec_guide before your first save."

	mcpSpecGuideDesc = "Get the ObserveSpec authoring guide, JSON Schema and an example, in one result of up to 64 KiB. Read it once before your first create_dashboard or update_dashboard; every save is checked against them."

	agentSpecGuideDesc = "Get the ObserveSpec JSON Schema. The authoring guide and an example are already in your instructions; call this when a save is rejected with a schema issue you cannot fix from the guide, or before you set an option the guide does not show."

	mcpPanelsDashboard   = "Dashboard id from list_dashboards or a save result."
	agentPanelsDashboard = "Dashboard id; omit it to read the dashboard you are working on, your build included."

	agentBuildNote = " In a build, each save adds or changes exactly one panel, and the server sets every position."

	agentBuildProps = `,"finish":{"type":"boolean","description":"Set on your last save. The build then becomes the dashboard's version."},"planned_panels":{"type":"integer","minimum":1,"maximum":200,"description":"How many panels you expect the finished dashboard to have, rows not counted. Send it with the first save of a new dashboard; resend it to change it."}`

	agentRowProp = `,"row":{"type":"object","additionalProperties":false,"required":["id","title"],"properties":{"id":{"type":"string","description":"Id of the row; a new id creates the row at the end of the dashboard."},"title":{"type":"string","minLength":1,"maxLength":200,"description":"Title for a new row; an existing row keeps its title."}},"description":"The row of the panel in upsert, or of the only panel in spec when you create the dashboard: a new panel is placed at the first free spot of that row, and an existing panel in another row moves there. Omit it for a panel above the first row."}`

	mcpLayoutGridPos   = "New position, with x, y, w and h."
	agentLayoutGridPos = "Size w and h, and the x and y you would like inside the panel's row; the server uses that spot only when it is free."
)

const ServerInstructions = "SlashObserve stores each dashboard as an ObserveSpec JSON document and checks every save by running its queries. " +
	"Call get_spec_guide once before your first create_dashboard or update_dashboard; it returns the authoring guide, the JSON Schema and an example. " +
	"Work one panel at a time: take a datasource uid from list_datasources, find real names with list_metrics, list_labels, list_label_values, get_schema and sample_rows, " +
	"check the query with lint_query, and run it with query_range, or with query_instant for an instant query or a table. " +
	"Save a new dashboard with create_dashboard; list_folders gives a folder_id. " +
	"To change a dashboard, read it with get_dashboard, or only the panels you need with get_panels, and send just the change with update_panels; update_dashboard rewrites a whole dashboard. " +
	"Saves take the version you read as parent_version and answer version_conflict when someone saved since: read the latest version and apply your change again. " +
	"A save rejects panels whose queries fail or return no data and names them; fix only those and save again. " +
	"List tools page with next_offset or next_cursor. " +
	"For feedback, read list_comments, fix the dashboard, then reply_to_comment and resolve_comment with the version you saved."

const missingSpec = "spec is required: the complete ObserveSpec document, written by the authoring guide"

func (t Tool) ForAgent() Tool {
	if t.AgentDescription != "" {
		t.Description = t.AgentDescription
	}
	if len(t.AgentInputSchema) > 0 {
		t.InputSchema = t.AgentInputSchema
	}
	return t
}

func (w wording) render(t Tool) (description string, schema json.RawMessage) {
	return w.replacer.Replace(t.Description), json.RawMessage(w.replacer.Replace(string(t.InputSchema)))
}
