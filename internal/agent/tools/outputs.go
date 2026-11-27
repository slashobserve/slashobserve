package tools

import (
	"encoding/json"
	"strings"

	"github.com/slashobserve/slashobserve/internal/folders"
)

const (
	propErrorCode    = `"code":{"type":"string","description":"Present only when the call failed."}`
	propErrorMessage = `"message":{"type":"string","description":"Present only when the call failed."}`
	propErrorNote    = `"note":{"type":"string"}`
)

func outputSchema(successKey, properties string) json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{` + properties + `,` + propErrorCode + `,` + propErrorMessage + `,` + propErrorNote + `},"anyOf":[{"required":["` + successKey + `"]},{"required":["code"]}]}`)
}

const (
	typeString       = `{"type":"string"}`
	typeInteger      = `{"type":"integer"}`
	typeBoolean      = `{"type":"boolean"}`
	typeNumber       = `{"type":"number"}`
	typeNullString   = `{"type":["string","null"]}`
	specObjectSchema = `{"type":"object","description":"The dashboard Spec; get_spec_guide has its JSON Schema."}`
)

var managedBySchema = `{"type":"string","enum":["` + strings.Join(folders.ManagedByStates.Names(), `","`) + `"],"description":"What writes the dashboard: a server release (starter), a file in the provisioning directory (file), observe dashboards apply (apply), or only people in the app (workspace)."}`

var outputSchemas = map[string]json.RawMessage{
	nameListDatasources: outputSchema("datasources", `"datasources":{"type":"array","items":{"type":"object","properties":{"uid":`+typeString+`,"type":`+typeString+`,"name":`+typeString+`,"healthy":`+typeBoolean+`,"message":`+typeString+`,"detail":`+typeString+`,"audience":{"type":"string","enum":["everyone","admins"]}},"required":["uid","type","name","healthy"]}},"truncated":`+typeBoolean),
	nameListDashboards:  outputSchema("dashboards", `"dashboards":{"type":"array","items":{"type":"object","properties":{"id":`+typeString+`,"title":`+typeString+`,"tags":{"type":"array","items":`+typeString+`},"latest_version":`+typeInteger+`,"updated_at":`+typeString+`,"view_count":`+typeInteger+`,"url":`+typeString+`,"folder":`+typeString+`,"managed_by":`+managedBySchema+`},"required":["id","title","tags","latest_version","updated_at","view_count","url","folder","managed_by"]}},"next_cursor":`+typeNullString),
	nameGetDashboard:    outputSchema("dashboard_id", `"dashboard_id":`+typeString+`,"version":`+typeInteger+`,"title":`+typeString+`,"url":`+typeString+`,"folder":`+typeString+`,"view_count":`+typeInteger+`,"window_days":`+typeInteger+`,"saved_by":`+typeString+`,"origin":`+typeString+`,"managed_by":`+managedBySchema+`,"saved_at":`+typeString+`,"summary":`+typeString+`,"spec":`+specObjectSchema+`,"unverified":{"type":"object"},"import_attention":{"type":"array","items":{"type":"object"}},"import_attention_truncated":`+typeBoolean),
	nameQueryRange:      querySummarySchema,
	nameQueryInstant:    querySummarySchema,
}

var querySummarySchema = outputSchema("kind", `"kind":{"type":"string","enum":["range","instant"]},"from":`+typeString+`,"to":`+typeString+`,"step_ms":`+typeInteger+`,"empty":`+typeBoolean+`,"frames":`+typeInteger+`,"truncated":`+typeBoolean+`,"more_rows":`+typeBoolean+`,"partial":`+typeBoolean+`,"warning":`+typeString+`,"hint":`+typeString+`,"series":{"type":"array","items":{"type":"object","properties":{"name":`+typeString+`,"labels":{"type":"object","additionalProperties":`+typeString+`},"points":`+typeInteger+`,"non_null":`+typeInteger+`,"min":`+typeNumber+`,"max":`+typeNumber+`,"last":`+typeNumber+`},"required":["name","points","non_null"]}},"table":{"type":"object","properties":{"columns":{"type":"array","items":{"type":"object","properties":{"name":`+typeString+`,"type":`+typeString+`},"required":["name","type"]}},"rows":{"type":"array","items":{"type":"array"}},"total_rows":`+typeInteger+`},"required":["columns","rows","total_rows"]},"page":{"type":"object","properties":{"page":`+typeInteger+`,"page_size":`+typeInteger+`,"total":`+typeInteger+`,"total_relation":`+typeString+`,"has_next":`+typeBoolean+`,"next":`+typeString+`},"required":["page","page_size","total_relation","has_next"]}`)
