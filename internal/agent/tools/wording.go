package tools

import (
	"encoding/json"
	"strings"
)

type wording struct{ replacer *strings.Replacer }

func newWording(readGuide, guideRef, folderChoice, folderProp, updateTarget, readTarget, buildProps, buildGuidance, rowProp, upsertMax, specGuideDesc string) wording {
	return wording{strings.NewReplacer(
		"{read_guide}", readGuide,
		"{guide_ref}", guideRef,
		"{folder_choice}", folderChoice,
		"{folder_prop}", folderProp,
		"{update_target}", updateTarget,
		"{read_target}", readTarget,
		"{build_props}", buildProps,
		"{build_guidance}", buildGuidance,
		"{row_prop}", rowProp,
		"{upsert_max}", upsertMax,
		"{spec_guide_desc}", specGuideDesc,
	)}
}

const (
	mcpSpecGuideDesc = "Get the ObserveSpec authoring guide, JSON Schema and an example. Read it before calling create_dashboard or update_dashboard."

	agentSpecGuideDesc = "Get the ObserveSpec JSON Schema. The authoring guide and an example are already in your instructions; call this when a save is rejected with a schema issue you cannot fix from the guide, or before you set an option the guide does not show."

	agentBuildProps = `,"finish":{"type":"boolean","description":"Set on your last save. The build then becomes the dashboard's version."},"planned_panels":{"type":"integer","minimum":1,"maximum":200,"description":"How many panels you expect the finished dashboard to have, rows not counted. Send it with the first save of a new dashboard; resend it to change it."}`

	agentRowProp = `,"row":{"type":"object","additionalProperties":false,"required":["id","title"],"properties":{"id":{"type":"string","description":"Id of the row the new panel belongs to; a new id creates the row at the end of the dashboard."},"title":{"type":"string","minLength":1,"maxLength":200,"description":"Title for a new row; an existing row keeps its title."}},"description":"The row of the new panel: the panel in upsert, or the only panel in spec when you create the dashboard. Omit it for a panel above the first row."}`

	agentBuildGuidance = " In this run every save goes to a build that people watch while you work, and the build becomes one version when you finish. Each save adds or changes exactly one panel. Make the first save of a new dashboard `create_dashboard` with the title, time, variables and only the first panel, with `row` naming its row, and send `planned_panels`. Add every other panel with its own `update_panels` call: the panel alone in `upsert`, with `row` naming its row; a new row is created with its first panel, and the server places each new panel at the end of its row. Set `finish: true` on your last save."
)

var (
	mcpWording = newWording(
		"call get_spec_guide first if you have not read it",
		"get_spec_guide",
		"Pass folder_id (see list_folders) to choose the folder; the default is the folder you last created a dashboard in, else General. ",
		","+propFolderID,
		`"dashboard_id","parent_version",`,
		`"dashboard_id",`,
		"",
		"",
		"",
		"50",
		mcpSpecGuideDesc,
	)
	agentWording = newWording(
		"follow the authoring guide in your instructions",
		"the authoring guide in your instructions",
		"",
		"",
		"",
		"",
		agentBuildProps,
		agentBuildGuidance,
		agentRowProp,
		"1",
		agentSpecGuideDesc,
	)
)

const missingSpecMCP = "spec is required; see get_spec_guide"

const missingSpecAgent = "spec is required; see the authoring guide in your instructions"

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
