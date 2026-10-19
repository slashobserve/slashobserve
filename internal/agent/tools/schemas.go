package tools

const (
	nameCreateDashboard = "create_dashboard"
	nameGetDashboard    = "get_dashboard"
	nameGetSchema       = "get_schema"
	nameGetSpecGuide    = "get_spec_guide"
	nameLintQuery       = "lint_query"
	nameListComments    = "list_comments"
	nameListDashboards  = "list_dashboards"
	nameListDatasources = "list_datasources"
	nameListLabelValues = "list_label_values"
	nameListLabels      = "list_labels"
	nameListMetrics     = "list_metrics"
	nameQueryInstant    = "query_instant"
	nameQueryRange      = "query_range"
	nameReplyToComment  = "reply_to_comment"
	nameResolveComment  = "resolve_comment"
	nameSampleRows      = "sample_rows"
	nameUpdateDashboard = "update_dashboard"
)

const (
	descCreateDashboard = "Save a new dashboard from an ObserveSpec document and get its id and URL. Every panel query runs during the save, so call get_spec_guide first if you have not read it, and fix any rejected panels it names. Never include a placeholder panel."
	descGetDashboard    = "Read a saved dashboard's ObserveSpec at its latest version, or at a given version. Use it before revising a dashboard."
	descGetSchema       = "List the tables of a ClickHouse datasource, or the columns of one table. Use it before writing SQL."
	descGetSpecGuide    = "Get the ObserveSpec authoring guide, JSON Schema and an example. Read it before calling create_dashboard or update_dashboard."
	descLintQuery       = "Check a PromQL or ClickHouse SQL query for syntax and policy problems without running it. Use it on a draft query before running or saving it. When the datasource cannot be reached it returns an error such as datasource_unavailable instead of diagnostics, which says nothing about the query."
	descListComments    = "List a dashboard's comments, oldest first: open threads by default. A reply has parent_id; panel_id null means the comment is about the whole dashboard. Use it to find the feedback to work on."
	descListDashboards  = "List saved dashboards, most recently updated first, with their latest version and URL. Use it to find a dashboard to open or revise."
	descListDatasources = "List the configured datasources with their uid, type and health. Call it first to learn which datasource uid to pass to the other tools."
	descListLabelValues = "List the values of one label (or OpenTelemetry attribute key) on a datasource, optionally for one metric. On ClickHouse pass table or metric. Use it to learn real services, routes or pods before filtering on them."
	descListLabels      = "List label names (or OpenTelemetry attribute keys on ClickHouse) on a datasource, optionally for one metric. On ClickHouse pass table (an otel_* table from get_schema) or metric. Use it to find the dimensions a metric can be grouped by."
	descListMetrics     = "List the metrics of a Prometheus datasource, or the OpenTelemetry metric names of a ClickHouse datasource, optionally filtered by a search string. Use it to find real metric names before writing queries."
	descQueryInstant    = "Run a PromQL or ClickHouse SQL query at one point in time and get a compact summary (rows or series with min, max, last). Use it to check a stat, table or bar gauge query returns data before saving. Pass dashboard_title when the query uses $__dashboard."
	descQueryRange      = "Run a PromQL or ClickHouse SQL query over a time range and get a compact summary (series, min, max, last). Use it to check a panel query returns data before saving. Pass dashboard_title when the query uses $__dashboard."
	descReplyToComment  = "Reply to a comment thread, for example to say what you changed or why you did not. A reply to a reply joins the same thread."
	descResolveComment  = "Resolve a comment thread once it is addressed. Pass version, the dashboard version that fixed it, so people see Resolved by version N. Resolving a resolved thread changes nothing."
	descSampleRows      = "Read a few recent rows from a ClickHouse table. Use it to see real column values, such as log messages or span names, before writing a query."
	descUpdateDashboard = "Save a new version of an existing dashboard from a full ObserveSpec document. Pass the version you read as parent_version; a newer saved version is reported as a conflict. Placeholder panels may be moved or removed, never added or edited."
)

const (
	propDatasource = `"datasource":{"type":"string","description":"Datasource uid from list_datasources."}`
	propSearch     = `"search":{"type":"string","maxLength":200,"description":"Case-insensitive substring filter."}`
	propTable      = `"table":{"type":"string","description":"ClickHouse table such as otel_logs; not used by Prometheus."}`
	propDiscLimit  = `"limit":{"type":"integer","minimum":1,"maximum":500,"description":"Maximum items to return. Default 100."}`
	propQuery      = `"query":{"type":"string","minLength":1,"maxLength":65536,"description":"PromQL or ClickHouse SQL, using the macros in get_spec_guide."}`
	propFrom       = `"from":{"type":"string","description":"Start time such as now-1h or an RFC 3339 time. Default now-1h."}`
	propTo         = `"to":{"type":"string","description":"End time such as now or an RFC 3339 time. Default now."}`
	propVariables  = `"variables":{"type":"object","maxProperties":20,"additionalProperties":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"string"}},"description":"Values for the $variables the query references, by variable name."}`
	propDashTitle  = `"dashboard_title":{"type":"string","maxLength":200,"description":"Title that $__dashboard expands to in the query. Default Untitled dashboard."}`
	propFormat     = `"format":{"type":"string","enum":["timeseries","table","heatmap","logs"],"description":"Result shape. On ClickHouse, table and logs keep the rows and columns as selected; otherwise a leading time column is split into one series per combination of string columns. Use the format the panel will have. Default timeseries."}`
	propSpec       = `"spec":{"type":"object","description":"The complete ObserveSpec document; see get_spec_guide."}`
	propSummary    = `"summary":{"type":"string","minLength":1,"maxLength":500,"description":"One line describing the change."}`
	propDashID     = `"dashboard_id":{"type":"string","description":"Dashboard id."}`
	propCommentID  = `"comment_id":{"type":"string","description":"Comment id from list_comments."}`
)

const (
	schemaEmpty = `{"type":"object","properties":{},"additionalProperties":false}`

	schemaListMetrics = `{"type":"object","properties":{` + propDatasource + `,` + propSearch + `,` + propDiscLimit + `},"required":["datasource"],"additionalProperties":false}`

	schemaListLabels = `{"type":"object","properties":{` + propDatasource + `,"metric":{"type":"string","description":"Only labels seen on this metric."},` + propTable + `,` + propSearch + `,` + propDiscLimit + `},"required":["datasource"],"additionalProperties":false}`

	schemaListLabelValues = `{"type":"object","properties":{` + propDatasource + `,"label":{"type":"string","description":"Label name or attribute key."},"metric":{"type":"string","description":"Only values seen on this metric."},` + propTable + `,` + propSearch + `,` + propDiscLimit + `},"required":["datasource","label"],"additionalProperties":false}`

	schemaGetSchema = `{"type":"object","properties":{` + propDatasource + `,"table":{"type":"string","description":"Table to describe. Omit to list tables."}},"required":["datasource"],"additionalProperties":false}`

	schemaSampleRows = `{"type":"object","properties":{` + propDatasource + `,"table":{"type":"string","description":"Table to sample."},"limit":{"type":"integer","minimum":1,"maximum":20,"description":"Rows to return. Default 5."}},"required":["datasource","table"],"additionalProperties":false}`

	schemaLintQuery = `{"type":"object","properties":{` + propDatasource + `,` + propQuery + `},"required":["datasource","query"],"additionalProperties":false}`

	schemaQueryRange = `{"type":"object","properties":{` + propDatasource + `,` + propQuery + `,` + propFrom + `,` + propTo + `,"max_data_points":{"type":"integer","minimum":10,"maximum":1000,"description":"Target points per series. Default 200."},` + propFormat + `,` + propVariables + `,` + propDashTitle + `},"required":["datasource","query"],"additionalProperties":false}`

	schemaQueryInstant = `{"type":"object","properties":{` + propDatasource + `,` + propQuery + `,` + propFrom + `,` + propTo + `,` + propVariables + `,` + propDashTitle + `},"required":["datasource","query"],"additionalProperties":false}`

	schemaListDashboards = `{"type":"object","properties":{"limit":{"type":"integer","minimum":1,"maximum":50,"description":"Dashboards per page. Default 20."},"cursor":{"type":"string","description":"next_cursor from the previous page."}},"additionalProperties":false}`

	schemaGetDashboard = `{"type":"object","properties":{` + propDashID + `,"version":{"type":"integer","minimum":1,"description":"Version number. Default latest."}},"required":["dashboard_id"],"additionalProperties":false}`

	schemaCreateDashboard = `{"type":"object","properties":{` + propSpec + `,` + propSummary + `},"required":["spec"],"additionalProperties":false}`

	schemaUpdateDashboard = `{"type":"object","properties":{` + propDashID + `,"parent_version":{"type":"integer","minimum":1,"description":"The version this change is based on."},` + propSpec + `,` + propSummary + `},"required":["spec"],"additionalProperties":false}`
)

const (
	schemaListComments   = `{"type":"object","properties":{` + propDashID + `,"status":{"type":"string","enum":["open","resolved","all"],"description":"Which threads to list. Default open."},"panel_id":{"type":"string","minLength":1,"maxLength":64,"description":"Only comments on this panel."},"limit":{"type":"integer","minimum":1,"maximum":100,"description":"Comments per page. Default 50."},"cursor":{"type":"string","description":"next_cursor from the previous page."}},"required":["dashboard_id"],"additionalProperties":false}`
	schemaReplyToComment = `{"type":"object","properties":{` + propCommentID + `,"body":{"type":"string","minLength":1,"maxLength":4000,"description":"The reply, plain text."}},"required":["comment_id","body"],"additionalProperties":false}`
	schemaResolveComment = `{"type":"object","properties":{` + propCommentID + `,"version":{"type":"integer","minimum":1,"description":"The dashboard version that fixed the comment."}},"required":["comment_id"],"additionalProperties":false}`
)
