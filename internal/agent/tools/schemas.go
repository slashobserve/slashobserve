package tools

const (
	nameCreateDashboard = "create_dashboard"
	nameGetDashboard    = "get_dashboard"
	nameGetPanels       = "get_panels"
	nameGetSchema       = "get_schema"
	nameGetSpecGuide    = "get_spec_guide"
	nameImportGrafana   = "import_grafana_dashboard"
	nameLintQuery       = "lint_query"
	nameListComments    = "list_comments"
	nameListDashboards  = "list_dashboards"
	nameListDatasources = "list_datasources"
	nameListFolders     = "list_folders"
	nameListLabelValues = "list_label_values"
	nameListLabels      = "list_labels"
	nameListMetrics     = "list_metrics"
	nameQueryInstant    = "query_instant"
	nameQueryRange      = "query_range"
	nameReplyToComment  = "reply_to_comment"
	nameResolveComment  = "resolve_comment"
	nameSampleRows      = "sample_rows"
	nameUpdateDashboard = "update_dashboard"
	nameUpdatePanels    = "update_panels"
)

const (
	descCreateDashboard = "Save a new dashboard from an ObserveSpec document and get its id and URL. Every panel query runs during the save, so {read_guide}, and fix any rejected panels it names. {folder_choice}Never include a placeholder panel.{build_guidance}"
	descGetDashboard    = "Read a saved dashboard's ObserveSpec at its latest version, or at a given version. Use it before revising a dashboard."
	descGetPanels       = "Read the full JSON of up to 20 panels of a saved dashboard by id, at its latest version or a given one, together with the dashboard's variables. Use it to read a panel before changing it with update_panels, in particular when a large dashboard lists a panel without its JSON. Ids it did not return are listed in missing; when truncated is true, ask again for those. A panel too large to return is listed in too_large; leave it unchanged."
	descGetSchema       = "List the tables of a ClickHouse datasource, or the columns of one table. Use it before writing SQL. When truncated is true, next_offset pages on; to find one name, narrow with search instead."
	descGetSpecGuide    = "{spec_guide_desc}"
	descImportGrafana   = "Import a Grafana dashboard as a new SlashObserve dashboard. Pass the dashboard JSON text in json (classic model, export with __inputs, or an API response). Grafana datasources are matched to ours automatically when unambiguous; otherwise the result lists them and you map each key to one of our datasource uids in mapping. Panels whose queries fail are saved and reported as unverified; fix them afterwards with update_dashboard. The dashboard goes to the folder named in folder, else the folder named in the Grafana export; a missing folder is created when your role allows, otherwise the dashboard goes to your default folder and folder_note says so. The result's folder is the title of the folder the dashboard went to."
	descLintQuery       = "Check a PromQL or ClickHouse SQL query for syntax and policy problems without running it. Use it on a draft query before running or saving it. When the datasource cannot be reached it returns an error such as datasource_unavailable instead of diagnostics, which says nothing about the query."
	descListComments    = "List a dashboard's comments, oldest first: open threads by default. A reply has parent_id; panel_id null means the comment is about the whole dashboard. Use it to find the feedback to work on."
	descListDashboards  = "List saved dashboards, most recently updated first, with their latest version and URL. Use it to find a dashboard to open or revise."
	descListDatasources = "List the configured datasources with their uid, type and health. Call it first to learn which datasource uid to pass to the other tools."
	descListFolders     = "List the folders you can see, with their id and whether you can create dashboards in each. Pass a folder id as folder_id to create_dashboard to save a dashboard there."
	descListLabelValues = "List the values of one label (or OpenTelemetry attribute key) on a datasource, optionally for one metric. On ClickHouse pass table or metric. Use it to learn real services, routes or pods before filtering on them. When truncated is true, next_offset pages on; to find one name, narrow with search instead."
	descListLabels      = "List label names (or OpenTelemetry attribute keys on ClickHouse) on a datasource, optionally for one metric. On ClickHouse pass table (an otel_* table from get_schema) or metric. Use it to find the dimensions a metric can be grouped by. When truncated is true, next_offset pages on; to find one name, narrow with search instead."
	descListMetrics     = "List the metrics of a Prometheus datasource, or the OpenTelemetry metric names of a ClickHouse datasource, optionally filtered by a search string. Use it to find real metric names before writing queries. When truncated is true, next_offset pages on; to find one name, narrow with search instead."
	descQueryInstant    = "Run a PromQL or ClickHouse SQL query at one point in time and get a compact summary (rows or series with min, max, last). Use it to check a stat, table or bar gauge query returns data before saving. Pass dashboard_title when the query uses $__dashboard."
	descQueryRange      = "Run a PromQL or ClickHouse SQL query over a time range and get a compact summary (series, min, max, last). Use it to check a panel query returns data before saving. Pass dashboard_title when the query uses $__dashboard."
	descReplyToComment  = "Reply to a comment thread, for example to say what you changed or why you did not. A reply to a reply joins the same thread."
	descResolveComment  = "Resolve a comment thread once it is addressed. Pass version, the dashboard version that fixed it, so people see Resolved by version N. Resolving a resolved thread changes nothing."
	descSampleRows      = "Read a few recent rows from a ClickHouse table. Use it to see real column values, such as log messages or span names, before writing a query."
	descUpdateDashboard = "Save a new version of an existing dashboard from a full ObserveSpec document. Use it to rewrite a dashboard as a whole; to change some panels, or the time range, title, tags or links, use update_panels, which sends only what changed. Pass the version you read as parent_version; a newer saved version is reported as a conflict. Placeholder panels may be moved or removed, never added or edited.{build_guidance}"
	descUpdatePanels    = "Save a new version of an existing dashboard from a panel patch, sending only what changed. upsert holds complete panels: one with an existing id replaces that panel in place (an omitted gridPos keeps its position, and an upserted row may carry its children in panels while it is new or empty, afterwards upsert each child by its own id), and one with a new id is added. delete lists the ids of panels to remove, a row only together with all of its children. layout moves panels by id to a gridPos or into a row (row \"\" is the top level). variables replaces the whole variable list. dashboard changes the default time range, title, tags or links. Pass the version you read as parent_version; a newer saved version is reported as a conflict. The save runs the queries of the panels and variables it changed, or every query when the default time range changes, and rejects panels that fail or return no data; the rejection lists failing_panels, so resend only those. Placeholder panels may be moved or removed, never added or edited.{build_guidance}"
)

const (
	propDatasource = `"datasource":{"type":"string","description":"Datasource uid from list_datasources."}`
	propSearch     = `"search":{"type":"string","maxLength":200,"description":"Case-insensitive substring filter."}`
	propTable      = `"table":{"type":"string","description":"ClickHouse table such as otel_logs; not used by Prometheus."}`
	propDiscLimit  = `"limit":{"type":"integer","minimum":1,"maximum":500,"description":"Maximum items to return. Default 100."}`
	propOffset     = `"offset":{"type":"integer","minimum":0,"maximum":499,"description":"Skip this many items; pass next_offset from an earlier result to page on."}`
	propQuery      = `"query":{"type":"string","minLength":1,"maxLength":65536,"description":"PromQL or ClickHouse SQL, using the macros in {guide_ref}."}`
	propFrom       = `"from":{"type":"string","description":"Start time such as now-1h or an RFC 3339 time. Default now-1h."}`
	propTo         = `"to":{"type":"string","description":"End time such as now or an RFC 3339 time. Default now."}`
	propVariables  = `"variables":{"type":"object","maxProperties":20,"additionalProperties":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"string"}},"description":"Values for the $variables the query references, by variable name."}`
	propDashTitle  = `"dashboard_title":{"type":"string","maxLength":200,"description":"Title that $__dashboard expands to in the query. Default Untitled dashboard."}`
	propFormat     = `"format":{"type":"string","enum":["timeseries","table","heatmap","logs"],"description":"Result shape. On ClickHouse, table and logs keep the rows and columns as selected; otherwise a leading time column is split into one series per combination of string columns. Use the format the panel will have. Default timeseries."}`
	propSpec       = `"spec":{"type":"object","description":"The complete ObserveSpec document; see {guide_ref}."}`
	propSummary    = `"summary":{"type":"string","minLength":1,"maxLength":500,"description":"One line describing the change."}`
	propDashID     = `"dashboard_id":{"type":"string","description":"Dashboard id."}`
	propFolderID   = `"folder_id":{"type":"string","format":"uuid","description":"Folder id from list_folders. Default your last used folder, else General."}`
	propCommentID  = `"comment_id":{"type":"string","description":"Comment id from list_comments."}`
)

const (
	schemaEmpty = `{"type":"object","properties":{},"additionalProperties":false}`

	schemaListMetrics = `{"type":"object","properties":{` + propDatasource + `,` + propSearch + `,` + propDiscLimit + `,` + propOffset + `},"required":["datasource"],"additionalProperties":false}`

	schemaListLabels = `{"type":"object","properties":{` + propDatasource + `,"metric":{"type":"string","description":"Only labels seen on this metric."},` + propTable + `,` + propSearch + `,` + propDiscLimit + `,` + propOffset + `},"required":["datasource"],"additionalProperties":false}`

	schemaListLabelValues = `{"type":"object","properties":{` + propDatasource + `,"label":{"type":"string","description":"Label name or attribute key."},"metric":{"type":"string","description":"Only values seen on this metric."},` + propTable + `,` + propSearch + `,` + propDiscLimit + `,` + propOffset + `},"required":["datasource","label"],"additionalProperties":false}`

	schemaGetSchema = `{"type":"object","properties":{` + propDatasource + `,"table":{"type":"string","description":"Table to describe. Omit to list tables."},` + propOffset + `},"required":["datasource"],"additionalProperties":false}`

	schemaSampleRows = `{"type":"object","properties":{` + propDatasource + `,"table":{"type":"string","description":"Table to sample."},"limit":{"type":"integer","minimum":1,"maximum":20,"description":"Rows to return. Default 5."}},"required":["datasource","table"],"additionalProperties":false}`

	schemaLintQuery = `{"type":"object","properties":{` + propDatasource + `,` + propQuery + `},"required":["datasource","query"],"additionalProperties":false}`

	schemaQueryRange = `{"type":"object","properties":{` + propDatasource + `,` + propQuery + `,` + propFrom + `,` + propTo + `,"max_data_points":{"type":"integer","minimum":10,"maximum":1000,"description":"Target points per series. Default 200."},` + propFormat + `,` + propVariables + `,` + propDashTitle + `},"required":["datasource","query"],"additionalProperties":false}`

	schemaQueryInstant = `{"type":"object","properties":{` + propDatasource + `,` + propQuery + `,` + propFrom + `,` + propTo + `,` + propVariables + `,` + propDashTitle + `},"required":["datasource","query"],"additionalProperties":false}`

	schemaImportGrafana = `{"type":"object","additionalProperties":false,"required":["json"],"properties":{"json":{"type":"string","minLength":1,"description":"The Grafana dashboard JSON text."},"mapping":{"type":"object","maxProperties":50,"additionalProperties":{"type":"string","minLength":1,"maxLength":200},"description":"Grafana datasource key (uid, name or ${DS_X}) to our datasource uid."},"folder":{"type":"string","minLength":1,"maxLength":100,"description":"Folder title for the new dashboard. Default the folder named in the Grafana export, else your default folder."}}}`

	schemaListDashboards = `{"type":"object","properties":{"limit":{"type":"integer","minimum":1,"maximum":50,"description":"Dashboards per page. Default 20."},"cursor":{"type":"string","description":"next_cursor from the previous page."}},"additionalProperties":false}`

	schemaGetDashboard = `{"type":"object","properties":{` + propDashID + `,"version":{"type":"integer","minimum":1,"description":"Version number. Default latest."}},"required":["dashboard_id"],"additionalProperties":false}`

	schemaCreateDashboard = `{"type":"object","properties":{` + propSpec + `,` + propSummary + `{folder_prop}{build_props}{row_prop}},"required":["spec"],"additionalProperties":false}`

	schemaUpdateDashboard = `{"type":"object","properties":{` + propDashID + `,"parent_version":{"type":"integer","minimum":1,"description":"The version this change is based on."},` + propSpec + `,` + propSummary + `{build_props}},"required":[{update_target}"spec"],"additionalProperties":false}`

	schemaGetPanels = `{"type":"object","properties":{` + propDashID + `,"version":{"type":"integer","minimum":1,"description":"Version number. Default latest."},"ids":{"type":"array","minItems":1,"maxItems":20,"items":{"type":"string"},"description":"Ids of the panels to read."}},"required":[{read_target}"ids"],"additionalProperties":false}`

	schemaUpdatePanels = `{"type":"object","properties":{` + propDashID + `,"parent_version":{"type":"integer","minimum":1,"description":"The version this change is based on."},"upsert":{"type":"array","maxItems":{upsert_max},"items":{"type":"object"},"description":"Complete panels to add or replace, matched by id."},"delete":{"type":"array","maxItems":50,"items":{"type":"string"},"description":"Ids of panels to remove."},"layout":{"type":"array","maxItems":200,"items":{"type":"object","additionalProperties":false,"required":["id"],"properties":{"id":{"type":"string","description":"Id of the panel to place."},"gridPos":{"type":"object","description":"New position, with x, y, w and h."},"row":{"type":"string","description":"Id of the row to move the panel into; an empty string moves it to the top level. Omit to keep its row."}}},"description":"Moves of existing panels."},"variables":{"type":"array","description":"The complete variable list; replaces the existing one."},"dashboard":{"type":"object","additionalProperties":false,"properties":{"time":{"type":"object","description":"Default range, with both from and to. A different range runs every query in the dashboard."},"title":{"type":"string"},"tags":{"type":"array","items":{"type":"string"},"description":"Replaces all tags; [] clears them."},"links":{"type":"array","items":{"type":"object"},"description":"Replaces all dashboard links; [] clears them."}},"description":"Dashboard-level values to change. Omit a key to keep it."},` + propSummary + `{build_props}{row_prop}},"required":[{update_target}"summary"],"additionalProperties":false}`
)

const (
	schemaListComments   = `{"type":"object","properties":{` + propDashID + `,"status":{"type":"string","enum":["open","resolved","all"],"description":"Which threads to list. Default open."},"panel_id":{"type":"string","minLength":1,"maxLength":64,"description":"Only comments on this panel."},"limit":{"type":"integer","minimum":1,"maximum":100,"description":"Comments per page. Default 50."},"cursor":{"type":"string","description":"next_cursor from the previous page."}},"required":["dashboard_id"],"additionalProperties":false}`
	schemaReplyToComment = `{"type":"object","properties":{` + propCommentID + `,"body":{"type":"string","minLength":1,"maxLength":4000,"description":"The reply, plain text."}},"required":["comment_id","body"],"additionalProperties":false}`
	schemaResolveComment = `{"type":"object","properties":{` + propCommentID + `,"version":{"type":"integer","minimum":1,"description":"The dashboard version that fixed the comment."}},"required":["comment_id"],"additionalProperties":false}`
)
