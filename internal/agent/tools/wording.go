package tools

import (
	"encoding/json"
	"strings"
)

type wording struct{ replacer *strings.Replacer }

func newWording(readGuide, guideRef, folderChoice, folderProp string) wording {
	return wording{strings.NewReplacer(
		"{read_guide}", readGuide,
		"{guide_ref}", guideRef,
		"{folder_choice}", folderChoice,
		"{folder_prop}", folderProp,
	)}
}

var (
	mcpWording = newWording(
		"call get_spec_guide first if you have not read it",
		"get_spec_guide",
		"Pass folder_id (see list_folders) to choose the folder; the default is the folder you last created a dashboard in, else General. ",
		","+propFolderID,
	)
	agentWording = newWording(
		"follow the authoring guide in your instructions",
		"the authoring guide in your instructions",
		"",
		"",
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
