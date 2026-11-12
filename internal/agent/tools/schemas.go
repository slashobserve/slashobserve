package tools

const (
	nameCreateDashboard = "create_dashboard"
	nameCreateComment   = "create_comment"
	nameEditComment     = "edit_comment"
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
	nameLogsForTrace    = "logs_for_trace"
	nameQueryInstant    = "query_instant"
	nameQueryRange      = "query_range"
	nameReopenComment   = "reopen_comment"
	nameReplyToComment  = "reply_to_comment"
	nameResolveComment  = "resolve_comment"
	nameSampleRows      = "sample_rows"
	nameUpdateDashboard = "update_dashboard"
	nameUpdatePanels    = "update_panels"
	nameLogFacets       = "log_facets"
	nameGetTrace        = "get_trace"
	nameTraceStats      = "trace_stats"
)

const pagingNote = "When truncated is true, pass next_offset as offset to page on; to find one name, narrow with search instead."

const (
	descCreateDashboard = "Save a new dashboard from a complete ObserveSpec document and get its dashboard_id, version and URL.{guide_first} The save runs every panel query and rejects the document when a query fails or returns no data, listing each issue with its path; fix those panels and save again. Never include a placeholder panel.{build_note}"
	descGetDashboard    = "Read a saved dashboard's whole ObserveSpec at its latest version, or at version, with its URL, folder and any unverified panels. Use it before rewriting a dashboard with update_dashboard; to read a few panels, use get_panels. A document over 128 KiB returns too_large; read its panels with get_panels instead."
	descGetPanels       = "Read the full JSON of up to 20 panels by id, at the latest version or at version, with the dashboard's variables. Use it to read a panel before changing it with update_panels, and when a large dashboard lists a panel without its JSON; get_dashboard returns the whole document instead. Ids it did not return are in missing; when truncated is true, ask again for those. A panel too large to return is in too_large; leave it unchanged."
	descGetSchema       = "List the tables of a ClickHouse datasource, or the columns and types of one table; on Tempo, list every service with its span names and the attributes TraceQL filters on; on Elastic and OpenSearch, list the concrete indices, or with table set to an index name the fields with their kinds. Use it before writing SQL or TraceQL, and sample_rows to see real values. When truncated is true, pass next_offset as offset to page on."
	descGetSpecGuide    = "{spec_guide_desc}"
	descImportGrafana   = "Import a Grafana dashboard as a new SlashObserve dashboard. Pass the dashboard JSON text in json (classic model, export with __inputs, or an API response). Grafana datasources are matched to ours automatically when unambiguous; otherwise the result lists them and you map each key to one of our datasource uids in mapping. Panels whose queries fail are saved and reported as unverified; fix them afterwards with update_dashboard. The dashboard goes to the folder named in folder, else the folder named in the Grafana export; a missing folder is created when your role allows, otherwise the dashboard goes to your default folder and folder_note says so. The result's folder is the title of the folder the dashboard went to."
	descLintQuery       = "Check a PromQL, LogQL, TraceQL, ClickHouse SQL, KQL, Lucene or Query DSL query for syntax and policy problems without running it; ok is false when any diagnostic is an error. Use it on each new or changed query before you run or save it. It cannot tell whether the query returns data; query_range and query_instant do. An error such as datasource_unavailable means the check could not run, not that the query is wrong."
	descListComments    = "List the comments on a dashboard, observation, annotation or proposal, oldest first: open threads by default. Give exactly one of target, written kind:uuid such as observation:7f0c1d2e-5a4b-4c3d-8e9f-0a1b2c3d4e5f, or dashboard_id, which means the same as dashboard:<id>. A reply has parent_id; on a dashboard, panel_id null means the comment is about the whole dashboard. Use it to find the feedback to work on."
	descListDashboards  = "List the saved dashboards you can see, most recently updated first, with id, title, tags, folder, latest version and URL, 20 per page by default. Pass next_cursor as cursor for the next page. Use it to find the dashboard_id of a dashboard to read or revise."
	descListDatasources = "List the configured datasources with their uid, type (prometheus, clickhouse, loki or tempo) and health. Every discovery and query tool, and every panel, names its datasource by this uid; call it first unless you already know the uids."
	descListFolders     = "List the folders you can see, with their id and whether you can create dashboards in each. Pass a folder id as folder_id to create_dashboard to save a dashboard there."
	descListLabelValues = "List the values of one label or attribute key, optionally only those seen on metric, a stream selector on Loki or a TraceQL filter such as { span.http.method = \"POST\" } on Tempo. On ClickHouse pass table or metric. On Elastic and OpenSearch name a field such as service.name to get its top values over the range. Use it to learn real services, routes or pods before filtering on them. Pass from and to to see only names seen in that range. " + pagingNote
	descListLabels      = "List the label names of a datasource: stream labels on Loki, TraceQL attributes on Tempo, OpenTelemetry attribute keys on ClickHouse, fields you can group or filter by on Elastic and OpenSearch, optionally only those seen on metric, a stream selector on Loki or a TraceQL filter on Tempo. On ClickHouse pass table, an otel_* table from get_schema, or metric. Use it to find the dimensions to group or filter by. Pass from and to to see only names seen in that range. " + pagingNote
	descListMetrics     = "List the metric names of a Prometheus datasource, or the OpenTelemetry metric names of a ClickHouse datasource, optionally filtered by search; with scope, list every metric family in that scope instead, or every stream label key on Loki; Elastic and OpenSearch hold no metrics, so scope there lists nothing. Use it to find real metric names before writing a query. " + pagingNote
	descLogsForTrace    = "Read the log lines that carry a trace id, or one span id, from a Loki, ClickHouse, Elastic or OpenSearch logs datasource over a time window. Use it after get_trace or a TraceQL query to see what the services logged while the trace ran; pass the trace's start and end as from and to."
	descQueryInstant    = "Run a PromQL, LogQL, TraceQL, ClickHouse SQL, KQL, Lucene or Query DSL query as an instant query, PromQL evaluated once at to, and get a compact summary of the rows or series it returns. Use it to check a query with instant: true, or a table panel's query, before saving; for other panels use query_range. It takes no format or max_data_points. Pass variables for each $variable the query uses, and dashboard_title when it uses $__dashboard."
	descQueryRange      = "Run a PromQL, LogQL, TraceQL, ClickHouse SQL, KQL, Lucene or Query DSL query over a time range, as a range query, and get a compact summary: up to 20 series with points, min, max and last, or the first rows of a table. This is how most panels run, a stat or gauge included; use it to check a panel query returns data before saving. Use query_instant instead for a query with instant: true or a table panel's query. Pass format as the panel's query has it, variables for each $variable the query uses, and dashboard_title when it uses $__dashboard."
	descReplyToComment  = "Reply to a comment thread on a dashboard, observation, annotation or proposal, for example to say what you changed or why you did not. A reply to a reply joins the same thread; a reply never reopens a resolved thread."
	descResolveComment  = "Resolve a comment thread once it is addressed. On a dashboard, pass version, the dashboard version that fixed it, so people see Resolved by version N. Resolving a resolved thread changes nothing."
	descCreateComment   = "Start a comment thread on a dashboard, observation, annotation or proposal, naming it in target as kind:uuid. On a dashboard the comment is about the whole dashboard, or about one panel with panel_id, on version (default the latest). Write @observe to address the built-in agent, or @[Name](user:<uuid>) for a person; a mention is recorded and shown, and starts no run. The result's id is the thread's id, for reply_to_comment, edit_comment, resolve_comment and reopen_comment."
	descEditComment     = "Change the text of a comment you wrote, by its id from list_comments or create_comment. Only its author can edit it, and only while they may still comment on its target. Mentions are read again from the new text. Sending the same text changes nothing."
	descReopenComment   = "Reopen a resolved comment thread, for example when the fix did not hold. Reopening an open thread changes nothing."
	descSampleRows      = "Read up to 20 recent rows of a ClickHouse table, up to 20 Loki log lines of the last 15 minutes with their labels, or up to 20 recent documents of an Elastic or OpenSearch index pattern with their fields, 5 by default, to see real values such as log messages, span names or attribute keys before writing a query."
	descUpdateDashboard = "Save a new version of an existing dashboard from a complete ObserveSpec document that replaces it whole.{guide_first} Use it only to rewrite most of a dashboard; for some panels, the variables, the time range, title, tags or links, use update_panels, which sends only what changed. The save runs every panel query and rejects the document when a query fails or returns no data, listing each issue with its path. Placeholder panels may be moved or removed, never added or edited.{build_note}"
	descUpdatePanels    = `Change an existing dashboard by sending only what changed; prefer it to update_dashboard for any edit. upsert is an array of complete panels matched by id, such as [{"id":"errors","type":"stat",...}]: an existing id replaces that panel, keeping its position when you leave out gridPos, and a new id adds a panel. A new or empty row may carry its children in panels; afterwards upsert each child by its own id. delete lists ids to remove, a row only with all of its children. layout moves existing panels by id to a gridPos or into a row (row "" is the top level). variables replaces the whole variable list. dashboard sets the default time, title, tags or links; each key you send replaces that value. The save runs the queries of the panels and variables it changed, or every query when the default time range changes, and rejects panels that fail or return no data, listing them in failing_panels: fix and resend only those. Placeholder panels may be moved or removed, never added or edited.{build_note}`
)

const (
	propDatasource    = `"datasource":{"type":"string","description":"Datasource uid from list_datasources."}`
	propSearch        = `"search":{"type":"string","maxLength":200,"description":"Case-insensitive substring filter."}`
	propTable         = `"table":{"type":"string","description":"ClickHouse table such as otel_logs; on Loki, a stream selector such as {service_name=\"payments\"}; not used by Prometheus, Tempo, Elasticsearch or OpenSearch."}`
	propDiscLimit     = `"limit":{"type":"integer","minimum":1,"maximum":500,"description":"Maximum items to return. Default 100."}`
	propScope         = `"scope":{"type":"string","maxLength":256,"description":"A label selector such as {job=\"checkout\"}, a stream selector on Loki, or a table name on ClickHouse; returns every metric family in that scope in one result with type, unit, label keys and series count, and on Loki each stream label key with its stream count. Use it once per create run before planning."}`
	propOffset        = `"offset":{"type":"integer","minimum":0,"maximum":499,"description":"Skip this many items; pass next_offset from an earlier result to page on."}`
	propNamesFrom     = `"from":{"type":"string","description":"Only names seen from this time, relative such as now-6h or absolute; give from and to together or neither."}`
	propNamesTo       = `"to":{"type":"string","description":"Only names seen up to this time, relative such as now or absolute; give from and to together or neither."}`
	propQuery         = `"query":{"type":"string","minLength":1,"maxLength":65536,"description":"PromQL, LogQL, TraceQL, ClickHouse SQL, KQL, Lucene or Query DSL JSON. It may use $variables and the macros in the authoring guide, such as $__rate_interval or $__timeFilter(Timestamp)."}`
	propFrom          = `"from":{"type":"string","description":"Start time such as now-1h or an RFC 3339 time. Default now-1h."}`
	propTo            = `"to":{"type":"string","description":"End time such as now or an RFC 3339 time. Default now."}`
	propVariables     = `"variables":{"type":"object","maxProperties":20,"additionalProperties":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"string"}},"description":"Values for the $variables the query references, by variable name."}`
	propDashTitle     = `"dashboard_title":{"type":"string","maxLength":200,"description":"Title that $__dashboard expands to in the query. Default Untitled dashboard."}`
	propFormat        = `"format":{"type":"string","enum":["timeseries","table","heatmap","logs"],"description":"Result shape. On ClickHouse, table and logs keep the rows and columns as selected; otherwise a leading time column is split into one series per combination of string columns. Use the format the panel will have. Default timeseries."}`
	propSpec          = `"spec":{"type":"object","description":"The complete ObserveSpec document, written by the authoring guide."}`
	propParentVersion = `"parent_version":{"type":"integer","minimum":1,"description":"The version you read. If a newer one was saved since, the save merges when it changed other panels; when it changed the same ones it returns version_conflict with latest_version and conflicts: read that version, apply your change to it and save again."}`
	propSummary       = `"summary":{"type":"string","minLength":1,"maxLength":500,"description":"One line describing the change."}`
	propDashID        = `"dashboard_id":{"type":"string","description":"Dashboard id from list_dashboards or a save result."}`
	propFolderID      = `"folder_id":{"type":"string","format":"uuid","description":"Folder id from list_folders. Default General."}`
	propCommentID     = `"comment_id":{"type":"string","description":"Comment id from list_comments or create_comment."}`
	propBody          = `"body":{"type":"string","minLength":1,"maxLength":4000,"description":"The comment, plain text."}`
	propTarget        = `"target":{"type":"string","description":"What the comment is on, written kind:uuid with kind dashboard, observation, annotation or proposal, for example observation:7f0c1d2e-5a4b-4c3d-8e9f-0a1b2c3d4e5f. A finding:uuid target is deprecated and means the discussion of that finding's observation, which the comment then names as observation:uuid."}`
)

const (
	schemaEmpty = `{"type":"object","properties":{},"additionalProperties":false}`

	schemaListMetrics = `{"type":"object","properties":{` + propDatasource + `,` + propSearch + `,` + propScope + `,` + propDiscLimit + `,` + propOffset + `},"required":["datasource"],"additionalProperties":false}`

	schemaListLabels = `{"type":"object","properties":{` + propDatasource + `,"metric":{"type":"string","description":"Only labels seen on this metric."},` + propTable + `,` + propSearch + `,` + propNamesFrom + `,` + propNamesTo + `,` + propDiscLimit + `,` + propOffset + `},"required":["datasource"],"additionalProperties":false}`

	schemaListLabelValues = `{"type":"object","properties":{` + propDatasource + `,"label":{"type":"string","description":"Label name or attribute key."},"metric":{"type":"string","description":"Only values seen on this metric."},` + propTable + `,` + propSearch + `,` + propNamesFrom + `,` + propNamesTo + `,` + propDiscLimit + `,` + propOffset + `},"required":["datasource","label"],"additionalProperties":false}`

	schemaGetSchema = `{"type":"object","properties":{` + propDatasource + `,"table":{"type":"string","description":"Table to describe. Omit to list tables."},` + propOffset + `},"required":["datasource"],"additionalProperties":false}`

	schemaSampleRows = `{"type":"object","properties":{` + propDatasource + `,"table":{"type":"string","description":"required on ClickHouse; ClickHouse table such as otel_logs; on Loki, a stream selector such as {service_name=\"payments\"}; leave it out on Loki, Elasticsearch and OpenSearch to read every stream or the index pattern; not used by Prometheus or Tempo."},"limit":{"type":"integer","minimum":1,"maximum":20,"description":"Rows to return. Default 5."}},"required":["datasource"],"additionalProperties":false}`

	schemaLintQuery = `{"type":"object","properties":{` + propDatasource + `,` + propQuery + `},"required":["datasource","query"],"additionalProperties":false}`

	schemaQueryRange = `{"type":"object","properties":{` + propDatasource + `,` + propQuery + `,` + propFrom + `,` + propTo + `,"max_data_points":{"type":"integer","minimum":10,"maximum":1000,"description":"Target points per series. Default 200."},` + propFormat + `,` + propVariables + `,` + propDashTitle + `},"required":["datasource","query"],"additionalProperties":false}`

	schemaLogsForTrace = `{"type":"object","properties":{` + propDatasource + `,"trace_id":{"type":"string","description":"The trace id, 16 or 32 hex characters."},"span_id":{"type":"string","description":"Only the lines of this span, 16 hex characters."},"service":{"type":"string","description":"The span's service name, to narrow the Loki stream selector or the Elastic and OpenSearch service field; ClickHouse ignores it."},` + propFrom + `,` + propTo + `,"limit":{"type":"integer","minimum":1,"maximum":200,"description":"Lines to return. Default 50."}},"required":["datasource","trace_id"],"additionalProperties":false}`

	schemaImportGrafana = `{"type":"object","additionalProperties":false,"required":["json"],"properties":{"json":{"type":"string","minLength":1,"description":"The Grafana dashboard JSON text."},"mapping":{"type":"object","maxProperties":50,"additionalProperties":{"type":"string","minLength":1,"maxLength":200},"description":"Grafana datasource key (uid, name or ${DS_X}) to our datasource uid."},"folder":{"type":"string","minLength":1,"maxLength":100,"description":"Folder title for the new dashboard. Default the folder named in the Grafana export, else your default folder."}}}`

	schemaListDashboards = `{"type":"object","properties":{"limit":{"type":"integer","minimum":1,"maximum":50,"description":"Dashboards per page. Default 20."},"cursor":{"type":"string","description":"next_cursor from the previous page."}},"additionalProperties":false}`

	schemaGetDashboard = `{"type":"object","properties":{` + propDashID + `,"version":{"type":"integer","minimum":1,"description":"Version number. Default latest."}},"required":["dashboard_id"],"additionalProperties":false}`

	schemaCreateDashboard = `{"type":"object","properties":{` + propSpec + `,` + propSummary + `{folder_prop}{build_props}{row_prop}},"required":["spec"],"additionalProperties":false}`

	schemaUpdateDashboard = `{"type":"object","properties":{{target_props}` + propSpec + `,` + propSummary + `{build_props}},"required":[{target_required}"spec"],"additionalProperties":false}`

	schemaGetPanels = `{"type":"object","properties":{"dashboard_id":{"type":"string","description":"{panels_dashboard}"},"version":{"type":"integer","minimum":1,"description":"Version number. Default latest."},"ids":{"type":"array","minItems":1,"maxItems":20,"items":{"type":"string"},"description":"Ids of the panels to read."}},"required":[{panels_required}"ids"],"additionalProperties":false}`

	schemaUpdatePanels = `{"type":"object","properties":{{target_props}"upsert":{"type":"array","maxItems":{upsert_max},"items":{"type":"object"},"description":"An array of complete panel objects to add or replace, matched by id."},"delete":{"type":"array","maxItems":50,"items":{"type":"string"},"description":"Ids of panels to remove."},"layout":{"type":"array","maxItems":200,"items":{"type":"object","additionalProperties":false,"required":["id"],"properties":{"id":{"type":"string","description":"Id of the panel to place."},"gridPos":{"type":"object","description":"{layout_gridpos}"},"row":{"type":"string","description":"Id of the row to move the panel into; an empty string moves it to the top level. Omit to keep its row."}}},"description":"Moves of existing panels."},"variables":{"type":"array","description":"The complete variable list; replaces the existing one."},"dashboard":{"type":"object","additionalProperties":false,"properties":{"time":{"type":"object","description":"Default range, with both from and to. A different range runs every query in the dashboard."},"title":{"type":"string"},"tags":{"type":"array","items":{"type":"string"},"description":"Replaces all tags; [] clears them."},"links":{"type":"array","items":{"type":"object"},"description":"Replaces all dashboard links; [] clears them."}},"description":"Dashboard-level values to change. Omit a key to keep it."},` + propSummary + `{build_props}{row_prop}},"required":[{target_required}"summary"],"additionalProperties":false}`
)

const (
	schemaListComments   = `{"type":"object","properties":{` + propTarget + `,"dashboard_id":{"type":"string","description":"Dashboard id from list_dashboards; the same as target dashboard:<id>. Give exactly one of target and dashboard_id."},"status":{"type":"string","enum":["open","resolved","all"],"description":"Which threads to list. Default open."},"panel_id":{"type":"string","minLength":1,"maxLength":64,"description":"Only comments on this panel of a dashboard."},"limit":{"type":"integer","minimum":1,"maximum":100,"description":"Comments per page. Default 50."},"cursor":{"type":"string","description":"next_cursor from the previous page."}},"additionalProperties":false}`
	schemaCreateComment  = `{"type":"object","properties":{` + propTarget + `,` + propBody + `,"panel_id":{"type":"string","minLength":1,"maxLength":64,"description":"The panel the comment is about; dashboards only. Omit it to comment on the whole dashboard."},"version":{"type":"integer","minimum":1,"description":"The dashboard version the comment is about; dashboards only. Default the latest."}},"required":["target","body"],"additionalProperties":false}`
	schemaEditComment    = `{"type":"object","properties":{` + propCommentID + `,"body":{"type":"string","minLength":1,"maxLength":4000,"description":"The new text, plain text."}},"required":["comment_id","body"],"additionalProperties":false}`
	schemaReopenComment  = `{"type":"object","properties":{` + propCommentID + `},"required":["comment_id"],"additionalProperties":false}`
	schemaReplyToComment = `{"type":"object","properties":{` + propCommentID + `,"body":{"type":"string","minLength":1,"maxLength":4000,"description":"The reply, plain text."}},"required":["comment_id","body"],"additionalProperties":false}`
	schemaResolveComment = `{"type":"object","properties":{` + propCommentID + `,"version":{"type":"integer","minimum":1,"description":"The dashboard version that fixed the comment; dashboard threads only."}},"required":["comment_id"],"additionalProperties":false}`
)
