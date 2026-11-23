---
title: Import Grafana dashboards
description: Import a Grafana dashboard from the web page, the CLI or MCP, with datasource and folder matching, the import report, and what each Grafana option becomes.
---

Import a Grafana dashboard to get a SlashObserve dashboard with the same panels, variables and layout.
Use it to move a dashboard over from Grafana, or to try SlashObserve on a dashboard you already know.
Every panel is kept: what cannot render yet becomes a placeholder, and the import report lists what did not convert.

You can import from the web page, from the CLI with `observe import`, over MCP with the `import_grafana_dashboard` tool, or over HTTP.
The JSON can be the classic dashboard model, an export that shares a dashboard externally, or the response of Grafana's dashboard API.
To bring over every dashboard of a running Grafana in one run, with its folders, see [Switch from Grafana](switch-from-grafana.md).

## Import a dashboard in the web app

Every person can create in their own My Dashboards, so an import always has a place to go.
A service-account key needs a folder grant that lets it create.

1. On Home, open the **Import a dashboard** tab.
   Paste the dashboard JSON into **Grafana dashboard** and choose **Preview**, or choose or drop a file there.
   Home opens the **Import from Grafana** page at `/import` with your dashboard loaded; see [Home](home.md).
   On that page you can also drop a file, or paste JSON anywhere outside a text field.
2. Under **Datasources**, match each Grafana datasource to one of yours.
   SlashObserve matches the ones it can; [Match datasources](#match-datasources) says how.
   A row that is not matched has a **Choose a datasource** menu, and the import stays off until you pick one.
3. Read the panel list.
   The filters **Needs a look**, **Partial**, **Placeholders** and **All panels** show which panels did not convert whole.
   Nothing runs against your datasources yet, so these counts cover only what the importer already knows.
4. Under **Folder**, check where the dashboard goes.
   The card starts on the folder you chose on Home, else the shared folder you imported into last, else **Automatic**.
   [Choose the folder](#choose-the-folder) says where each choice goes.
5. Choose **Import as new dashboard**.
   You see "Imported as `<title>`, version 1" and "In folder `<title>`", and **Open dashboard** takes you there.

The server saves the dashboard as version 1 and runs every query once.
A panel whose queries fail is kept and marked unverified instead of failing the import.
The preview before the import runs no query, so more panels can need a look afterwards; see [Panels marked unverified](#panels-marked-unverified).

## Import from the CLI, MCP or HTTP

With the CLI, preview first, then import:

```sh
observe import checkout.json --preview
observe import checkout.json --map prometheus=prom --folder Platform
```

`--preview` saves nothing.
It prints the dashboard's title, format and sizes, and whether it is ready.
It prints an `Already imported as` line when you imported the file before.
Then it prints a datasource table (`GRAFANA`, `TYPE`, `PANELS`, `MATCH`, `BY`), the counts and the panels that need a look.
The import prints the new dashboard's address on standard output.
The counts line, the `Folder:` line, a folder note and the panels that need a look go to standard error:

```text
https://observe.example.com/d/9f2c.../checkout
Imported "Checkout": 12 panels, 10 imported, 1 partial, 1 placeholders, 0 unverified
Folder: Platform
```

`-` reads standard input.
A file can be up to 8 MiB, and the request the CLI builds from it, with the JSON written as an escaped string, must also fit in 8 MiB.
A larger request answers `413 too_large`.
If a datasource has no match, the command fails with `datasources_unresolved`, shows the table and tells you to pass `--map '<grafana>=<uid>'`, which you can repeat.
`--update <dashboard id>` and `--as-new` are for a file you imported before; see [Update a dashboard you already imported](#update-a-dashboard-you-already-imported).
[CLI](cli.md) lists the other flags.

Over HTTP, send the JSON as a string in `json`:

```sh
curl -fsS -X POST "$OBSERVE_SERVER/api/v1/imports/preview" \
  -H "Authorization: Bearer $OBSERVE_TOKEN" \
  -H "Content-Type: application/json" \
  -d "$(jq -n --rawfile json checkout.json '{json: $json}')"
```

`POST /api/v1/imports/preview` saves nothing, and `POST /api/v1/imports` saves the dashboard.
The web page and `observe import` call these two routes.
Both take `json`, `mapping` (Grafana datasource key to your datasource uid), `folder`, `update_dashboard` and `refuse_existing`.
A `mapping` with more than 50 entries, or a key or value that is not 1 to 200 bytes, answers `400 invalid_request`.
Both answer `403 forbidden_role` when the caller cannot create dashboards in any folder.
A caller can run 1 import and 2 previews at a time, and the instance 4 of each.

The `import_grafana_dashboard` MCP tool runs the same import with `json`, `mapping`, `folder`, `update_dashboard` and `import_as_new`.
`import_as_new` cannot be combined with `update_dashboard`.
Inside the built-in agent's runs the JSON has to be in the run's prompt, `update_dashboard` is refused, and `refuse_existing` is not set.
The tool, and the built-in agent that uses it, refuse JSON that converts to an HTML text panel, a placeholder panel or a panel action.
They fail with `import_content_not_allowed`, because the agent and MCP clients never write those.
The web page and `observe import` keep them.

## Match datasources

Each datasource the dashboard uses is matched to one of yours, in this order:

1. the mapping you give;
2. a Grafana uid equal to one of your datasource uids;
3. `grafanaUids` in the [datasources file](../reference/datasources-file.md), shown as `alias` in the `BY` column;
4. being the only datasource of its type.

An import cannot be saved while a datasource of a supported, configured type has no match.
SlashObserve never guesses between two datasources of the same type.
Give the match with the mapping that comes with the request, or add the Grafana uid to `grafanaUids`.

SlashObserve imports Prometheus, ClickHouse, Loki, Tempo, Elasticsearch (the `elasticsearch` plugin) and OpenSearch (the `grafana-opensearch-datasource` plugin) datasources.
A Grafana datasource whose type has no configured datasource here does not block the import.
Its panels import as placeholders that say so, such as "No Loki datasource is configured.".
A datasource of another type, such as InfluxDB, imports its panels as placeholders that say "Influxdb is not supported yet.".
Over MCP, and in the built-in agent, such a dashboard is refused with `import_content_not_allowed`, because placeholders are import-only.

## Import from a Grafana link

An Admin can paste a dashboard link instead of the JSON, such as `https://grafana.example.com/d/abc123/checkout?orgId=1`.
The Grafana needs a [connection](switch-from-grafana.md#connect) in Settings.

1. On Home, in the **Import a dashboard** tab, paste the link into the **Grafana dashboard** field.
2. Choose **Preview**.
   Home fetches the dashboard and opens the import page with it.
   The **Folder** field there chooses the folder the import page starts on.
3. Continue as in [Import a dashboard in the web app](#import-a-dashboard-in-the-web-app).

Only Admins can do this, because the connection's token is an Admin's credential and could read every dashboard it is allowed to.
Everyone else pastes the JSON.

Over HTTP, `POST /api/v1/grafana/link` takes the link as `url`, fetches the dashboard with the connection's token and returns it as `json`.
`POST /api/v1/imports/preview` and `POST /api/v1/imports` take that value as it is.
The link must be a `/d/<uid>` or `/d-solo/<uid>` link under the connection's address, and the server fetches only from the hosts of stored connections.
The fetch never adds the connection's domain to the [outbound allowlist](../operate/outbound-access.md).

| Answer | Meaning |
| --- | --- |
| `403 outbound_domain_not_allowed` | The connection's domain is not on the outbound allowlist, because an Admin removed it or `SLASHOBSERVE_OUTBOUND_DOMAINS` does not name it. The link works again once it is allowed. |
| `404 no_connection` | No stored connection matches the link. |
| `404 dashboard_not_found` | Grafana has no such dashboard, or the token cannot read it. |
| `409 secret_key_missing` | `SLASHOBSERVE_SECRET_KEY` is not set, so the stored token cannot be used. |
| `409 token_unreadable` | The server cannot read the stored token. |
| `422 not_a_dashboard_link` | The text is not a web address, or the link is on a connection's address but is not a `/d/<uid>` or `/d-solo/<uid>` dashboard link. |
| `502 grafana_unavailable` | Grafana failed or refused the token. |

## Import a Spec you exported

A dashboard exported with **Export spec JSON** is an ObserveSpec, not a Grafana dashboard.
Drop or paste it on the import page.
The page recognises it when the text starts with `{` and has a string `specVersion` and a `panels` array, skips the Grafana conversion and the datasource matching, and shows **Create dashboard** instead of **Import as new dashboard**.
It creates the dashboard from the Spec as it is, in the folder you pick, as version 1.

The server validates the Spec and runs every query as it does for [`observe dashboards apply`](cli.md), and it reports a Spec it refuses with the first problem.
No import report is made, because nothing was converted.

## Choose the folder

The dashboard goes into a folder, chosen in this order:

1. The request's `folder` title: `--folder` for `observe import`, `folder` for the API and the MCP tool.
   The web page sends the folder you pick in its **Folder** card, and sends none when the choice is **Automatic**.
2. The `meta.folderTitle` of an export taken from Grafana's dashboard API, which names the folder the dashboard had in Grafana.
3. Your default folder: your My Dashboards, or General for a key.

A title matches an existing folder without regard to case.
A title of `General` or `My Dashboards` means your default folder, and so does an export title that is empty or not a valid folder title.
An invalid `folder` in the request is refused with `400 invalid_request`.

When no folder has the title, an Editor or Admin creates it, and a Viewer's dashboard goes to the default folder instead.
When a folder has the title but you cannot create dashboards in it, the dashboard also goes to your default folder.
Either way the response carries a `folder_note` saying so, and `observe import` prints it on standard error, even with `-q`.

On the web page, the **Folder** card lists the folders where you may create dashboards, except General.
General is left out because a title of `General` means your default folder, which **Automatic** already gives you.
The page starts on the folder you chose on Home, else on the shared folder you last imported into in this browser, as long as you can still create in it.
Creating a dashboard from a Spec also counts.
General and My Dashboards are never remembered.
Pick **Automatic** to use the export's folder or your My Dashboards instead.
After an import the page shows the `folder_note` when the dashboard did not go where the request or the export asked.

The server chooses the folder before it saves the dashboard, so an import that then fails can leave a new, empty folder behind.
Delete it from its settings page.

## Update a dashboard you already imported

Importing the same Grafana dashboard twice would make two copies, so the import checks first.
The import record keeps the dashboard's Grafana uid in `grafana_uid`, and `GET /api/v1/grafana/resolve/{uid}` finds the dashboard from an old Grafana link.
Only the first import of a uid holds it, and only a uid of 1 to 40 letters, digits, `-` or `_`.
Importing the same uid again makes a separate dashboard whose record has an empty `grafana_uid`, until the dashboard holding it is deleted.

When you can see the dashboard that holds the uid, the preview's `existing` field names it.
The import page then says "Already imported as `<title>` (version `<n>`)".
When you can edit it, the page also offers **Update `<title>`**.
Updating saves the file as the dashboard's next version and refreshes its import record, so edits made since stay in the version history.
**Import as new dashboard** still makes a second copy.

On the CLI, `--update <dashboard id>` does the update, and `--as-new` makes another dashboard on purpose.
Without either flag, `observe import` and the MCP tool set `refuse_existing`, so the call fails with `409 already_imported` and an `existing` object (`dashboard_id`, `title`, `version`, `can_update`).
Over MCP, pass `update_dashboard` or `import_as_new`.
Over HTTP, `update_dashboard` on `POST /api/v1/imports` takes the dashboard id and answers 200 instead of 201, and `refuse_existing` asks for the 409.

An update only takes the file the dashboard was imported from.
A file with another Grafana uid, or none, answers `409 grafana_uid_mismatch`, names the dashboard's uid and the file's, and changes nothing.
Import it as a new dashboard instead.
A dashboard imported from a file with no uid takes any file.
An update keeps the dashboard in its folder and ignores `folder`.

## Panels marked unverified

A first import is the one save that tolerates failing queries.
It saves the dashboard as version 1, runs every query once, and records the panels and variables whose queries fail as unverified, in the import report instead of an error.
For Editors and Admins, the **Imported, not verified** problem filter on the [dashboards list](dashboard-health.md#in-the-dashboards-list) lists every dashboard that still has unverified panels or variables or placeholder panels.
**Repair** sends them to the agent; see [Repair](switch-from-grafana.md#repair).

Later saves keep a failing panel or variable unverified for as long as these stay unchanged:

- its queries and the variables they use;
- its panel type;
- its time override (`timeFrom` and `timeShift`), `maxDataPoints` and `interval`.

A new or changed query must return data, and a panel whose queries pass again is no longer unverified.

A save with the `update_dashboard` tool runs every query.
A save with the `update_panels` tool runs only the queries it changed.
It runs every query when it changes the dashboard's default `time`, or its title while a query or variable uses the `__dashboard` macro.
Either way a panel the save did not change stays unverified while it still fails, and an unverified panel that now returns data leaves the list.
A panel or row that repeats on a variable is checked as one copy, with the first value the variable offers or the first value selected.
That is how the dashboard draws each copy, with a single value, so its query is not run with All and a healthy repeated panel is not marked unverified.

## Read the import report

The report sits below the panel list.
**Show every option, block by block** opens it, grouped by block: the dashboard settings, each variable, each row, and each panel with its queries, transformations, field config and overrides.
Each block is collapsed until you open it.
It lists every Grafana option the block carried as one of:

- mapped, with the ObserveSpec fields it became;
- excluded by decision, with the reason SlashObserve leaves it out;
- not imported, with the reason.

An option mapped without a field name is one no catalog describes yet, such as the options of legacy panels.
Options that need no setting (Grafana defaults, options with no visible effect and editor bookkeeping) show as a count that you can open.
A very large dashboard lists at most 200 options per block and 10,000 in all, keeping excluded, then not imported, then mapped options before those that need no setting.

A value SlashObserve does not support, such as a delay in days or a time zone name the server does not know, is left out and listed.
Query-based annotations are not imported, and the report says so under the dashboard's annotations.
Add SlashObserve annotations with the CLI, the HTTP API or an MCP client; see [Annotations](annotations.md).

When any panel carried Grafana alert rules, one dashboard note named "alert rules" lists those panels.
That covers an alert list of Grafana-managed alerts and a legacy graph alert.
The note says that alert rules, contact points and silences are not imported, so alerting stays on Prometheus rules and Alertmanager.

The server stores the original JSON with the import, so a newer importer can convert it again.
The API never returns it, and the agent never sees it.

## Reference

### Limits and errors

| Limit or error | Value |
| --- | --- |
| Request body | 8 MiB, the JSON as a string plus the mapping; larger answers `413 too_large` |
| JSON that does not parse | `422 invalid_json` with the line and column, or a note that the file ends early and may be cut off |
| JSON that is not a dashboard, a format it does not know, or too many panels | `422 not_a_dashboard`, `422 unsupported_format` or `422 too_many_panels` |
| A mapping that names a datasource the dashboard does not use, one that is not supported, or one that is not a configured datasource of that type | `422 invalid_mapping` |
| Too many imports or previews at once | `429 too_many_imports` |
| An update target that is not an import | `404 not_imported` |
| A datasource with no match | `422 datasources_unresolved`, listing each datasource with its candidates |
| Already imported | `409 already_imported` with `existing` |
| Update with another Grafana uid | `409 grafana_uid_mismatch` |
| Invalid `folder`, a mapping with more than 50 entries, or a mapping key or value that is not 1 to 200 bytes | `400 invalid_request` |
| HTML text, placeholder or panel action over MCP | `import_content_not_allowed` |
| Options in the report | 200 per block, 10,000 in all |
| Transformations | 20 per panel |
| Queries on a panel | 26 |

### Dashboard settings

A dashboard's time settings, links and editability import as follows.

| Grafana setting | Imports as | Notes |
| --- | --- | --- |
| Time zone, week start and fiscal year start month | The [dashboard fields](../concepts/observespec.md#the-dashboard) `timezone`, `weekStart` and `fiscalYearStartMonth` | |
| The time picker's now delay | The `nowDelay` dashboard field | |
| Time range | The same range | It may use `now/fy` and `now/fQ`. A range written in epoch milliseconds becomes the same moment as an absolute time. |
| The time picker's hidden flag, refresh intervals (in seconds, minutes, hours or days) and quick ranges, and the dashboard's live now switch | Carried over | A refresh interval of `500ms` is left out and reported. |
| Links to other dashboards by tag | Links that list the matching dashboards | |
| Links to an address | Links that keep their keep-time and include-variables options | |
| A link's tooltip | Imported as written | It is cut to 200 characters and reported when cut. |
| Dashboard, panel and row titles | Trimmed, with control characters turned into a space and invisible format characters removed | Dashboard, panel and row titles are also cut to 200 characters and reported when cut. |
| Link icons other than Grafana's default, and placement in the controls menu | Reported | |
| A dashboard that is not editable in Grafana (`editable: false`) | Reported; read-only dashboards are not part of SlashObserve | |

Details of titles:
line breaks and other control characters turn into a space, and an empty result becomes `Imported dashboard`, `Row` or no panel title.
Format characters are removed (such as zero-width and bidirectional override characters), except U+200C, U+200D, U+200E, U+200F and U+061C, which are kept.
A few filler characters are removed too: U+034F, U+115F, U+1160, U+2800, U+3164 and U+FFA0.
The [text rules](../reference/text-rules.md) then accept the title.

Details of the other settings:

- A dashboard keeps at most 20 links and 20 tags.
  A link by tag with an invalid tag, or with more than 20 tags, is dropped whole.
- Tags are lowercased, other characters become `-`, and a tag is cut to 40 characters.
  An empty or repeated tag, or one past the first 20, is dropped and reported.
- A link of any type other than an address or a list of dashboards by tag is dropped and reported.
  So is an address that is empty, over 2000 characters, or does not start with `http`, `https` or `mailto`.
- At most 20 refresh intervals import, in whole seconds, minutes, hours or days.
- At most 30 quick ranges import, with the name cut to 100 characters.
- The dashboard's refresh, description, tags and shared tooltip setting (`graphTooltip`, as `cursorSync`) import too.
- A time range the Spec cannot hold falls back to the last 6 hours.

### Variables

#### Custom, text box and interval variables

A query's min step written as a variable (`$name`, `${name}` or `[[name]]`) imports as `${name}` when that variable imports as an `interval` variable, so the query uses the step selected in that variable.
Otherwise the min step is not kept and the report says so.
A `custom` variable becomes an `interval` variable, without a note, when all of these hold:

- its options are all durations, and it has at most 50 of them;
- it is single-select and has no All;
- some query uses it as a min step.

A `custom` variable's `text : value` pairs import as `{ text, value }` options, so the picker shows the text and the selection uses the value.
A text box imports its saved value as its default, else its default text.

#### Query variables

- A `query` variable's sort imports as `alpha`, `num`, `alphaCaseInsensitive` or `natural`, each in its two directions.
- `allowCustomValue` imports as written, with Grafana's own default of true when it is missing.
- Static options import as `staticOptions`, with Grafana's placement as `staticOrder`.
- A query variable that Grafana never refreshes imports as `refresh: "never"` with its saved options as the static options, so only those are offered.
  One that never refreshes and has no saved options keeps loading, and the report says so.
- `regexApplyTo` imports without a note at its default `value`.
  At `text` the regular expression is not applied, because options here carry a value only, and the report says so when the variable has a regular expression.

A `query` variable whose query uses a variable the dashboard does not declare is dropped and reported, and the panels that use it become placeholders.
The exception is a `query` variable (tested with Prometheus) whose undeclared `$name`, `${name}` or `${name:format}` is the label that exactly one other `query` variable lists with `label_values(selector, name)`.
VictoriaMetrics' per-tenant dashboard has this after its rename of `accountID` to `account`.
The reference imports as that variable, the report says so on the variable, and the panels that use it import whole.
A variable that lists the label without a selector, a variable that depends on the one being repaired, or two variables that list the label keep the drop.

#### URL sync, hidden variables and datasource variables

A variable's `skipUrlSync` imports as written, so it stays out of the URL and starts from its default on every load.
A variable that Grafana hides in the controls menu (`hide` 3) imports shown and is reported, except a `constant` variable, which is not reported.
So is a datasource variable's `multi`, `includeAll` or `allowCustomValue: false`, because a datasource variable selects one datasource from the ones it lists.

#### Ad hoc variables

- A saved ad hoc filter list imports as the variable's `default`, and its base filters as `baseFilters`.
  Each filter keeps its key label and first value label.
  A filter's `values` list is not stored.
  It is accepted without a note when it is the regular expression's alternatives joined with `|`, and reported otherwise.
- On ClickHouse the operators `<` and `>` and the condition OR import too.
  On Prometheus those three are reported, because label matchers can only be ANDed and have no comparison.
- A filter with another operator, an invalid key or regular expression, or another condition is reported, in the base filters as in the saved ones.
- `defaultKeys` import as `keys` (up to 100), the keys the filter picker offers instead of asking the datasource.
  Grafana's key texts are shown as the keys themselves, and the report says so.
- On Prometheus, `allowCustomValue: false` imports as `listedValuesOnly`, so a filter's key and value must be picked from the lists the picker shows.
  On another datasource the picker lists no values, so it is reported.
- A ClickHouse ad hoc variable imports with `applyToAll`, so its filters apply to every query on its datasource; see [Ad hoc filters](../concepts/datasources-and-variables.md#ad-hoc-filters).
  For the Altinity plugin the report notes that Grafana filters only the outermost query.

#### Group-by variables

A Grafana group-by variable on Prometheus imports as a `groupby` variable.
Its current labels (else its default value) become the `default`, up to 20 labels that are valid label names.
Like Grafana, it adds the selected labels to the grouping of every aggregation in the queries on its datasource, so the queries need no change.
Its saved label options import as `keys` (up to 100 valid label names), the labels the picker offers instead of asking the datasource.
A group-by variable on any other datasource is dropped and reported.

A Grafana 13.1 ad hoc variable with `enableGroupBy` on Prometheus imports as the ad hoc variable plus a `groupby` variable.
The new variable uses the same datasource and visibility, has no saved labels and is named `<name>_groupby` (or the first free name that follows that pattern).
It keeps the ad hoc variable's `skipUrlSync` and its `defaultKeys` as `keys`.
On another datasource, or when the dashboard already holds 20 variables, `enableGroupBy` is reported.

#### Loki and Tempo variables

A Loki query variable imports as written, and one saved in the label values form becomes `label_values(<stream>, <label>)`.
A Loki label names variable and any Tempo query variable keep their saved values as a fixed list.
An ad hoc variable on Loki or Tempo is dropped and reported.

While a Loki variable selects All, `${var}` in a stream selector becomes `.+`, because Loki refuses a selector whose matchers all match an empty value.
In a label or line filter after the selector it stays `.*`.
A custom All value that matches an empty value, such as `.*`, is kept as written and reported when a Loki query uses the variable, so change it to `.+`.

### Queries

#### Prometheus

- A min step of `$__interval` or `$__rate_interval` imports as no minimum, and a resolution factor from 2 to 10 is kept as `intervalFactor`.
- A query that Grafana runs as both an instant and a range query (`instant` and `range` both on) imports as two queries.
  The range query keeps its ref.
  An `instant` copy without the min step or resolution factor takes the next free ref, which the report lists as `refId A (instant) -> B`.
  Both queries are hidden together, and a style override or a `filterByRefId` transformation that names the original ref covers the copy too.
- A query with `instant` on and `range` unset or off stays one instant query.
- When an instant copy would take the panel past 26 queries, only the range query is kept and the report lists the instant one.
- A target's `exemplar` flag is kept on a time series and a heatmap.
  It is dropped without a note on an instant query and on every other panel type, which draw no exemplars.

#### Loki

A Loki query imports its LogQL as written, on the matched Loki datasource.
A Loki metric query keeps its legend and keeps a resolution from 2 to 10 as `intervalFactor`.
It runs as an instant query when its `queryType` is `instant` (or, with no `queryType`, when `instant` is on).

A Loki `step` of `$__interval` or `$__auto` imports as no minimum.
Another step becomes the smallest step and is reported, because Grafana uses that step as is.
`$__auto` in a Loki query becomes `$__range` in an instant query and `$__interval` otherwise, and the report lists the rewrite.

A Loki log query, one that starts with a stream selector, always reads the whole time range.
Its legend, resolution and step have no effect, and an instant log query is reported.
A Loki query whose `queryType` is `stream` imports as a range query and is reported.

#### Tempo

A Tempo query imports its TraceQL or trace id as written, on the matched Tempo datasource.
A trace id held in a variable, such as `${trace_id}`, is looked up after the variable is expanded.
The Tempo search form state saved beside a TraceQL query is editor state and imports without a note.
That state is the filters, group-by, service names, durations and the metrics options.

These Tempo queries are dropped, and a panel left with no query becomes a placeholder:

- the search builder and the legacy search;
- the service graph;
- an uploaded trace file;
- a TraceQL metrics pipeline such as `| rate()`.

#### Elasticsearch and OpenSearch

An Elasticsearch or OpenSearch query is compiled from its Lucene query, metrics and bucket aggregations on the matched datasource.
Two kinds import as `lucene:<query>`:

- a query whose only metric is a count over an automatic date histogram on the time field;
- a logs, raw data or raw document query (by `queryType`, by its first metric, or by the OpenSearch `luceneQueryType`), which runs as an instant query that returns lines.

A raw document query is reported, because it shows `_source` and SlashObserve shows lines.

Every other query imports as a Query DSL body.
It holds a `query_string` of the Lucene query, with the bucket aggregations nested in their order around the metrics, keyed by their Grafana ids.
The bucket aggregations import as follows:

- date histograms with `$__interval` or the fixed interval, and `$__from` and `$__to` as extended bounds;
- terms with their size, order and missing value, where a size of 0 means 500;
- filters, labelled as saved;
- histograms.

Metrics import as follows.

| Metric | Imports as |
| --- | --- |
| `avg`, `sum`, `min`, `max`, `cardinality`, `percentiles`, `extended_stats` and `rate` | The same metric, with its fields and settings |
| `moving_avg` | A `moving_fn` with an unweighted average over its window; the model is not kept and nothing is reported |
| `moving_fn`, `derivative`, `cumulative_sum`, `serial_diff` and `bucket_script` | The same metric, with its window, bucket paths and scripts |
| `count` | The implicit metric |
| A hidden metric (`hide: true`) | Drawn anyway, and reported |
| A metric of a type SlashObserve does not know | Reported and left out of the query; the query is dropped when no metric is left |
| A bucket aggregation other than those listed above | The query is not imported and the report gives the reason |

An empty Lucene query matches everything, as in Grafana.
A query's `alias` imports as the legend, with `{{term <field>}}` rewritten to `{{<field>}}` and `{{field}}` written out as the metric's field name.
A `timeField` on a query is editor state: Grafana copies the datasource's time field onto every query, and a date histogram without a field uses it.

A query variable saved as `{"find": "terms", "field": <field>, "query": <lucene>}` becomes `label_values(lucene:<query>, <field>)`, or `label_values(<field>)` without a query, and its `size` is reported.
A `{"find": "fields"}` variable is dropped and reported.
A variable query in any other form keeps its saved values as a fixed list, and is dropped when it has none.
An ad hoc variable on Elasticsearch or OpenSearch imports as on ClickHouse, with `applyToAll`, the operators `<` and `>`, the condition OR, and dotted field names as keys.

#### ClickHouse queries from the Altinity plugin

A query that starts with a whole-query macro is written out as plain ClickHouse SQL that uses SlashObserve's time macros, so the saved query no longer needs the Altinity macros.
The whole-query macros are `$rate`, `$perSecond`, `$delta` and `$increase` with their `Columns` and `ColumnsAggregated` forms, `$columns` and `$columnsMs`, and `$lttb` and `$lttbMs`.
A whole-query macro that is not at the start of the query, or whose arguments cannot be read, means the query is not imported.
The query is dropped and the report gives the reason.

Inside a query, these macros import as follows.

| Macro | Imports as | Notes |
| --- | --- | --- |
| `$naturalTimeSeries` | Buckets by year, month, week or day for long ranges | |
| `$timeFilter64ByColumn` | The millisecond time filter | |
| `$timeFilterByColumn(column)` | `$__timeFilter(column)` | Only for a plain `DATETIME` column without `round`. See the time column rules below. An unreadable call means the query is not imported. |
| `$adhoc` | The dashboard's ad hoc variable, when exactly one ad hoc variable uses the query's datasource | See the details below. |
| `$conditionalTest(condition, $variable)` and its three-argument form `$conditionalTest(condition, otherwise, $variable)` | `$__conditionalAll`, which writes the otherwise condition when the variable selects All, as the plugin does | See the keyword rule below. |

Details of `$adhoc`:
it becomes `1` without a note when the dashboard declares no ad hoc variable.
It becomes `1` with the note "Ad hoc filters inside this query are not applied." when ad hoc variables exist but not exactly one uses the query's datasource.

A condition that starts with `AND` or `OR` keeps that keyword outside the macro.
For example `$conditionalTest(AND service IN ($service), $service)` becomes `AND $__conditionalAll(service IN ($service), $service)`, because the macro writes the condition inside parentheses.
With an otherwise condition, both conditions must start with the same keyword.
A call whose two conditions start with different keywords, or only one of them with a keyword, means the query is not imported, and the report gives the reason.

#### Time columns and counter macros

The query's time column type decides how the time macros read the column.
`DATETIME64` keeps its sub-second precision.
`TIMESTAMP` and `FLOAT` hold seconds since the epoch.
`TIMESTAMP64_3`, `_6` and `_9` hold milliseconds, microseconds and nanoseconds since the epoch.
These numeric types are compared as numbers and bucketed as times.
A type that is not in this list is read as `DATETIME`, and the report says so.

`$timeFilterByColumn(column)` becomes `$__timeFilter(column)` only for a plain `DATETIME` column without `round`.
A `DATETIME64` column gives `$__timeFilter_ms(column)`, a numeric time type gives explicit comparisons, and a `round` gives `toDateTime(...)` bounds.

The `round` option rounds the start of the range down and the end up in the time filter, `$from` and `$to`.
The first bucket of a counter macro has no value when `extrapolate` is off, because by default it is compared with a virtual earlier bucket.

The `Columns` forms return one row per time and key (time, key, value), which SlashObserve draws as one series per key, named by the key.
Counter macros always use window functions (`lagInFrame`), whatever the plugin's `useWindowFuncForMacros` says, because `runningDifference` is deprecated in current ClickHouse versions.
The first point of a counter series has nothing to subtract from:

- a rate treats the previous point as one step earlier;
- a delta or increase is 0;
- a per-second value is empty.

#### Options the plugin carries

The plugin's `extrapolate` option is converted, and the report does not mention it.
When it is off, the first bucket of a counter macro is nan.
The `round` option is applied when it is `$step` or a whole number of seconds from `1s` up.
`0s` or an empty value means no rounding.
Any other value is reported and the range is not rounded.
A ClickHouse query that keeps its generated SQL drops the query builder's settings without a note.

#### Server-side expressions

A server-side expression (math, reduce, resample or threshold) imports as an expression query that reads the queries it names; see [Expressions](../concepts/observespec.md#expressions).
A threshold keeps its main condition without its recovery threshold, because Grafana applies the recovery threshold only when it evaluates alert rules.

These are dropped, with the reason in the report:

- a classic condition, because it belongs to alert rules, which SlashObserve does not import;
- a SQL expression, because SlashObserve does not support it;
- a math function SlashObserve does not support, and math that reads a dashboard variable or a `$__` global;
- an expression whose input query did not import;
- expressions that depend on each other in a circle.

A panel keeps at least one data query or becomes a placeholder.

### Field options and overrides

#### Units, names and descriptions

A unit imports as itself.
Unit text Grafana does not know imports as the suffix Grafana shows, up to 20 characters.

A display name keeps `${__field.name}`, `${__field.displayName}`, `${__series.name}`, `${__data.refId}`, `${__field.labels.<label>}` and dashboard variables in all four syntaxes.
Other `__` tokens, such as `${__data.fields[0]}`, are reported.

A field description (`fieldConfig.defaults.description`, also as an override property) imports on tables, where it is the tooltip of the column header, cut to 2000 characters.
On every other panel type it is reported, because they draw no field description.
A field's `fieldMinMax` imports on gauges and bar gauges, where fields otherwise share one range.
It is left out silently elsewhere, where every field already has its own range.

#### Colors and value mappings

A field's color mode imports as one of these:

- the classic palette (the default);
- a palette by series name;
- a fixed color;
- shades;
- a named color scheme;
- `thresholds`, where the panel type draws it.

A series-colored mode imports on time series and trend panels.
`thresholds` imports on time series and trend panels, and on the panel types that color by threshold anyway it imports without a note.
A scheme also imports on stat, gauge, bar gauge, table (when its cells are colored), bar chart, XY chart, state timeline and status history.
There, a state is colored by its place between the field's min and max, else the data's, and a value mapping color wins.

A histogram colored by `thresholds` whose thresholds are only the base step imports as a fixed color, the base step's, because Grafana draws a histogram series in the color of the step its last value meets.
With more steps it is reported.
A color mode a panel type does not draw is reported as "The color mode X is not drawn on Y panels."

A value mapping keyed by the empty string imports as the empty-value mapping.
A value mapping's icon is reported, and its text and color are kept.

#### Overrides

An override imports its matcher and its properties like the panel's own.
The matchers are:

- by name, by a regular expression, by type, by a list of types, by a regular expression or names, and by a list of names;
- numeric fields, time fields, first field, first time field and every field;
- query ref, and value.

A list of types becomes one override per type.
A value matcher may reduce with `allIsNull` or `allIsZero`, which is how many dashboards hide series that are empty.

On time series and trend panels, and on the extra series of a candlestick, the series properties import too:

- negative-Y and constant transforms;
- hiding from the legend or tooltip;
- stack group and fill below to.

A legacy graph's series overrides import the same way.
In a series override's `custom.stacking.group`, a stack group written as `false`, `true` or a number becomes that text, so its series stack together and apart from the default group `A`.
In a legacy graph override, `stack` is a boolean (stacking on or off) or a group name, and a number is reported.
A group on a series whose stacking is `none` has no effect.

#### Panel links and data links

A data link's `oneClick` imports on stat, gauge, bar gauge and table panels.
On every other type it is reported, and the link stays a panel link.

A panel link to another dashboard, named by its title, imports as a link that opens that dashboard when it is clicked.
Imported dashboards get new ids, so the title is looked up at click time.
When several dashboards you can see share the title, or none does, the link opens the dashboards list filtered by the title instead.
The link carries the time range when Grafana's keep-time option was on and the variable selections when include-variables was on.
A panel link to an address keeps both options the same way, as the link tokens `${__url_time_range}` and `${__all_variables}`.
A panel link is reported and not kept in these cases:

- its type is neither an address nor a dashboard;
- it has no address, or its address is over 2000 characters or does not start with `http`, `https` or `mailto`;
- it is a dashboard link with no title, or with a title over 200 characters.

#### Field actions

Field actions are HTTP fetch actions in a field's defaults and overrides, with their inputs, button color, confirmation and one-click.
They import on stat, gauge, bar gauge and table panels, the types that draw them, and are reported on other types.
An action that is not an HTTP fetch is reported.
A one-click action stays one-click only when it is the field's only data link or action.

### Panel types

#### Titles, time overrides and time zones

A panel with no title imports with an internal name (`Panel <id>`) that menus and screen readers use but that is not drawn, as in Grafana.
A transparent panel is drawn without a border or background.
A panel that hides its time override (`hideTimeOverride`) imports as `hideTimeOverride`, which leaves the override note out of the panel header while the override still applies to the queries.

A time series, state timeline or status history panel's single time zone imports as the panel's `timezone`, so its times are drawn in that zone.
The zone is `options.timezone`: `utc`, `browser` or an IANA zone.
`default` and an empty entry mean the dashboard's.
A list of several zones is reported, because Grafana draws one axis per zone and a panel draws in one.

#### Layout and panels that reuse another panel

Panels in a collapsed row keep their layout relative to the topmost one, whatever order they were saved in.
An entry of a panel list that is not an object, and a row inside a collapsed row, are not kept, and the report lists each one by its path under "panels".

Grafana's `-- Dashboard --` query, which shows another panel's results, imports as a query that reuses that panel; see [Reusing another panel's results](../concepts/observespec.md#reusing-another-panels-results).
Panels that reuse another are converted after all the others.
Its `withTransforms` setting is kept, and `adHocFiltersEnabled` imports as `adHocFilters`, which filters the reused rows by the dashboard's ad hoc filters.

The query is dropped with "The panel this one reuses could not be imported." when the source panel:

- is missing;
- is not a data panel;
- repeats;
- itself reuses another panel.

A panel left with no query becomes a placeholder.
A reusing panel takes its time range and step from its source, so its own `interval`, `maxDataPoints`, `timeFrom` and `timeShift` are reported.
A reusing query that sits beside other queries is dropped and reported.

#### Time series, trend and candlestick

A time series, trend, candlestick or legacy graph panel imports:

- its legend: placement, list or table mode, values, sort, width and, on a legacy graph, hiding series that are all zero or empty;
- its tooltip: sort, hide zeros, maximum width and height, and a legacy graph's cumulative values;
- the axis options;
- the line style: interpolation, width, dots, dashes with a custom dash and gap length from 1 to 100 pixels, square dots, and points;
- the fixed line, fill and point colors, and the point symbol (circle or square);
- value labels;
- how gaps draw: spanning nulls, or inserted nulls given in milliseconds or at the query interval;
- the bar alignment and width, the threshold style, and a candlestick's extra number fields.

A named Grafana color becomes its hex color.
Grafana's `text` color (the theme's text color) becomes the Spec color `text`.
Any other CSS color name, such as `gray`, becomes its hex color.
These are reported:

- a point symbol other than a circle or square;
- a dash pattern that is not one dash length and one gap length, each a whole number from 1 to 100 pixels;
- an axis placement of top or bottom.

A point size on a time series with automatic points sizes the isolated points that automatic points draw.
Grafana's own size 5 is left out without a note, because it draws them at the usual size.
This applies to automatic points only.
A size of 4 is reported, because it cannot be told from no size.
An axis color mode other than `series` is Grafana's default, `text`.

A candlestick imports its up, down and flat candle colors.
A flat candle, whose open equals its close, otherwise takes the up color.

The annotation lane options of a time series panel (`options.annotations.multiLane` and `clustering`) import without a note.
Grafana 12.3 does not define them, and SlashObserve draws annotations as markers, not lanes.

#### State timeline and status history

Both import:

- the row height (and a status history's column width);
- the legend and tooltip, including hiding a series from the tooltip;
- line width, fill, null spanning and inserted nulls;
- the rows per page (`perPage`, from 1 to 100), which pages the rows with previous and next buttons.

A state timeline also imports its value alignment.

These import without a note, because these panels have no value axis and their legend lists states:

- a legend sort or calculation;
- hiding a series from the legend;
- the value axis options.

A legacy discrete panel imports as a state timeline with its value, range and color maps and its unit.
The keys that Grafana's own discrete panel migration does not read import without a note: row height, text sizes, colors, time axis, time precision and the legend value choices.

#### Value options of stat, gauge, bar gauge and pie chart

These panels import their value options: the first reducer, "Show" as every value (`allValues`, with `limit` when it is not 5000) and the `fields` name or pattern.
Grafana uses only the first reducer of these panels, so extra reducers are dropped without a note.

#### Stat

A stat panel imports its text alignment (`justifyMode`), `wideLayout`, percent change (`showPercentChange` and `percentChangeColorMode`), and its title and value sizes.
The sizes are `text.titleSize` and `text.valueSize`, from 1 to 200 pixels, and a size outside that range is reported.
The percent size (`text.percentSize`) imports like the other text sizes.
The line graph mode (`graphMode: line`) draws the sparkline as a line without the area under it.

#### Gauge and bar gauge

A gauge imports its threshold labels and its neutral value (`custom.neutral`, the value the bar grows from).
A bar gauge imports its `lcd` display mode, value mode, name placement, unfilled area and orientation (horizontal, vertical or auto).
Both import their text sizes (`text.titleSize` and `text.valueSize`, from 1 to 200 pixels).
The `basic` and `gradient` bar gauge display modes import as the basic bar, because bar gauges are drawn flat.

When the sizing is Manual, a gauge or bar gauge imports its minimum width and height, and a bar gauge its maximum height.
It uses Grafana's own defaults when they are not set.
With Auto sizing Grafana ignores these limits, so they are left out without a note.

#### Pie chart

A pie chart imports its slice labels (name, value and percent), their text sizes (`text.titleSize` and `text.valueSize`, from 1 to 200 pixels), its legend and its tooltip.
The legend imports its placement, list or table mode, the values it shows (value and percent), the sort by name or value, the sort order and the width.
The tooltip imports its mode, sort, "Hide zeros" and maximum width and height.

A pie chart's slice order (`options.sort`) imports: largest first by default, smallest first, or the order of the data.
A series hidden from the legend or the tooltip in the field options imports as it does on a time series, and stays drawn.

A legend sorted by a calculation such as Mean, the pie chart's orientation and its legend calculations import without a note, because a pie has no orientation and its legend shows no calculations.

#### Bar chart and histogram

A bar chart imports:

- its label rotation, spacing and maximum length (`xTickLabelRotation`, `xTickLabelSpacing` and `xTickLabelMaxLength`);
- `barRadius` (up to 0.5), `fullHighlight` and `colorByField`;
- the value text size (`text.valueSize`, 6 to 100 pixels);
- its legend and tooltip like a time series does, including the table mode, legend calculations, sort, width and the tooltip sort, "Hide zeros" and maximum sizes;
- the threshold style (lines, areas or both over the value axis).

A histogram imports `bucketOffset`, `combine`, its stacking mode (normal, percent or none), the stack group of each series, and the same legend and tooltip options.
Grafana's own `none` stacking and `single` tooltip are written explicitly, because a histogram without them stacks series and lists all of them.

On both:

- the axis options (label, soft limits, log and symlog scales, centered zero, width, grid, border and color mode) apply to the value axis;
- `lineWidth` is the bar border and `fillOpacity` the bar fill;
- a series hidden from the legend or the tooltip in the field options imports and stays drawn.

A fully transparent fill is reported, because bars are drawn opaque at 0.
A histogram's `thresholdsStyle` and the bottom or top axis placement are still reported.

#### Heatmap

A heatmap imports:

- the color mode, scheme, steps, scale and exponent, fill, range (`color.min` and `color.max`) and reverse;
- the cell gap and radius, the cell value unit and decimals, the legend toggle, `showValue`, and the low and high value filters (`filterValues.le` and `ge`);
- the calculated buckets (`calculation.xBuckets` and `yBuckets`: mode, value and the y scale with its log base);
- the y axis: placement left, right or hidden, unit, decimals, range, width, label and reverse;
- the rows layout (`rowsFrame.layout`);
- the tooltip: `single`, `multi` as all rows, `none` as hidden, the y histogram, the color scale and the size limits;
- the exemplar color.

It writes Grafana's own defaults when the panel does not set them: scheme mode with the Oranges scheme, 64 steps, a gap of 1 and values shown where they fit.

Grafana ignores the scale, exponent and fill in scheme mode and the scheme in opacity mode, so they are left out without a note.
A scheme with no color ramp (such as Cool) or a fill color with no matching theme color is reported, and the heatmap then uses the default scheme or the first series color.
A time bucket size is read in seconds when it has no unit, and a lone bucket value without a mode is taken as a size.
The selection modes `xy` and `y` are reported.
Hiding a series from the heatmap legend or tooltip imports without a note, because a heatmap draws one field.

A heatmap that Grafana still runs through its own Angular migration imports whole.
That is a `dataFormat` with a schema version below 37, or with empty `options`.
These import: pre-bucketed or calculated rows, bucket bound as the rows layout, bucket sizes and counts, the log base, y axis unit, decimals, range and visibility, color mode, scheme and scale, card padding and rounding, hidden zero buckets, reverse, tooltip decimals as the cell decimals, tooltip and legend.
The highlight of the hovered card is not drawn, and a hidden time axis, `splitFactor` and a log base other than 2 and 10 are reported.

A heatmap that already has `options` keeps the Angular keys it was saved with (`dataFormat`, `yAxis`, `color`, `cards`, `tooltip` and the rest) without a note, because Grafana never reads them again.
Its `options.tooltip.show` decides the tooltip as Grafana's migration does.
A `fieldConfig` color is not drawn, since a heatmap colors its cells from `options.color`.

#### Table

A table column imports how its cells are drawn (`custom.cellOptions`, or the older `custom.displayMode`) as the field option `cell`:

- colored text;
- a colored background, with `applyToRow` as `wholeRow`;
- a bar gauge (basic or `lcd`, with its value display);
- a sparkline, with its line style, fill, points, null spanning and soft axis limits as the column's field options;
- JSON, an image (with its alt text and title), data links, pills, the `actions` cell type, and markdown (with `dynamicHeight`);
- `wrapText` as `wrap`.

Gradient backgrounds and gauges, and the sparkline gradient mode, are drawn flat.
Map cells are reported by decision.
Thresholds are kept for the columns whose cells they color.

A table column also imports these options, and the older table panel's column alignment.

| Grafana key | Imports as |
| --- | --- |
| `custom.width` and `custom.minWidth` | Width and minimum width |
| `custom.align` | Alignment |
| `custom.filterable` | Filter |
| `custom.inspect` | Inspect |
| `custom.hideHeader` | Hidden header |
| `custom.wrapHeaderText` | Wrapped header |
| `custom.tooltip` | The cell tooltip, with its field and placement |

A table imports:

- its row height (`sm`, `md`, `lg` and `auto`, which fits each row to its content) and the maximum row height (20 to 1000 pixels);
- the frame it shows (`frameIndex`, 0 to 99);
- type icons, pagination and frozen columns;
- the first column of `sortBy` with its direction;
- its footer: the reducers (a footer that is shown without a reducer sums), the columns it covers and the row count.

A table panel's `options.showSearch` and `options.filterable` are not table options in Grafana (the column option is `custom.filterable`), and they import without a note.
A second sort column, more than 10 frozen columns and the footer reducers of a single column's override are reported.

The older table panel's saved sort is dropped silently when it is Grafana's own default (the first column, descending) and reported otherwise, because it names a column by position.
A table with `styles` and no `fieldConfig` is read as Grafana reads it, as the older table panel.
Its styles import as column units, decimals, names, alignment, thresholds and hidden columns.
A style's `dateFormat` on a column that is not a date and its link options when `link` is off are Grafana's own no-ops and import without a note.
A table that already has a `fieldConfig` keeps the Angular keys it was saved with without a note, because Grafana never reads them again.
Those keys are `styles`, `transform`, `columns`, `sort`, `showHeader`, `fontSize`, `pageSize` and leftover graph options such as `yaxes`, `lines` and `legend`.

On a table, `custom.hidden` (written `true` or `"true"`) and `custom.hideFrom.viz` hide the column, in the panel's defaults or in an override.

#### Logs

A logs panel imports:

- its unique and common labels, log details and where they open (`detailsMode`);
- prettified JSON, deduplication, controls bar, font size and syntax highlighting;
- the displayed fields (at most 20, shown as the line instead of the message);
- the log attributes switch (turning it off hides the per-line labels);
- the nanosecond timestamp resolution.

Infinite scrolling, the field selector and the context toggle are reported.

#### Traces and flame graph

A traces panel imports its span filters:

- the search (`spanFilters.query`);
- the service and span name lists, with their `!=` operators;
- the minimum and maximum duration (`from` and `to`, written with a unit from `ns` to `h`, up to 24 hours);
- `matchesOnly` and `criticalPathOnly`.

A filter that is not valid, such as a duration without a unit or a list of more than 50 names, and the tag filters are reported.
A flame graph has no options of its own to import.

#### Text

A text panel imports its HTML mode and its code mode with the language (JSON, YAML, XML, SQL, Go, TypeScript, Markdown or HTML) and the line numbers.
Another language is shown as plain text and reported, and the code view has no minimap.
Only people can write HTML text: an agent or the MCP server cannot, and the sanitizer still removes scripts and styles.

#### Geomap

A geomap imports its basemap, the basemap opacity, theme and labels, the view, the map controls and the tooltip.
The basemap is the CARTO map Grafana draws by default, OpenStreetMap, an Esri server, or an `xyz` tile address that is an https address written out in full with `{z}`, `{x}` and `{y}`.
It also imports the zoom levels an `xyz` tile server serves, and tiles are never requested outside them.
The view covers every named region, the zoom limits, the fit padding, fitting to one layer or only the last points, the shared view, and whether the world is drawn only once.

A tile address over http is changed to https and reported.
An address with a variable in the host, a `maplibre` basemap and its access token are reported and never imported.
The mouse wheel zoom is imported as on, because Grafana's default is on.
The map draws tiles only when the server may fetch the basemap's host, so allow it under [Images and GeoJSON](../operate/outbound-access.md#images-and-geojson).
Until then the map shows country outlines.

A geomap imports its first 10 layers, each with its opacity, tooltip and the query that feeds it.
Later layers are reported.

| Layer | Imported options |
| --- | --- |
| Markers | Size, color, fill opacity, outline width, symbol or a symbol from a field with its alignment, text or a fixed label with its offsets and alignment, fixed and clamped rotation, and a WKT location column |
| Heatmap | Radius, blur, and a fixed weight or the weight range of a weight field |
| GeoJSON | The bundled country and state maps, an https address and its rules |
| Day and night | The night color, the sun, and whether the night is drawn at the start or the end of the time range |
| Photos | Border color and size, crop and shadow |
| Route | Arrow, line width and line color |
| Network | Node color, edge color, edge width and arrow |
| Tile (CARTO, OpenStreetMap, Esri, `xyz`) | Theme, labels, Esri server, tile address and attribution |

A markers layer without a fill opacity imports Grafana's own 0.4, so the markers are as light as they are in Grafana.
A markers layer imports its legend, which Grafana shows unless it is turned off.
The legend is a color swatch under the layer name for a fixed color, one entry per threshold step when the color field has thresholds, else the smallest and largest color values.
The zoom levels of a tile layer are still reported.

Locating points by name uses the bundled country and state gazetteers.
These are reported: an airport lookup, other gazetteers, other GeoJSON files, dynamic GeoJSON, the last-point tracker, photo frames, MapLibre basemaps and the other layer types.

#### Canvas

A canvas imports:

- text, metric value, rectangle, ellipse and icon elements (a bundled icon, an https image or an image taken from a field), buttons, and server, triangle, cloud, parallelogram, wind turbine and drone elements;
- the constraint and pixel placement, rotation, vertical alignment, border width and radius, an icon's outline color and width, and template text;
- background images, and the background, border, pan and zoom and tooltip of the canvas.

Pixel placement inside a group is still converted to percent of the panel and reported.
A template text shows the values of the element's data, and a reference to a field the data lacks shows as nothing.
An outline width that is not a whole number from 0 to 20 is reported.

Element links import with their open-in-new-tab setting, and an element's one-click link or action imports as its one-click behavior.
A connection imports its path, line style, arrow, a width that follows a field with its range, and its bends (at most 20).
Widths or directions that follow a rule are reported.
A canvas keeps at most 50 elements and 100 connections, and an element keeps at most 10 links and 10 actions.
The rest are reported.

#### Canvas actions

A canvas element action and a button's API call import when the address is an https address written out in full.
The action needs Edit to run, nothing is sent without a confirmation, and only people can add or change actions (the agent and MCP cannot).
A canvas action also imports its inputs (`variables`).
Its button color (`style`) is reported because a canvas action draws none.

An action with a credential in a header or query (such as `Authorization`, a token or an API key) is kept without that header, and the report says so.
These are left out and reported:

- an action whose address is not https, or has a variable in the host;
- an action that refers to a variable the dashboard does not declare;
- an action with a body over 10,000 characters;
- an infinity action.

#### XY chart

An XY chart imports its series mapping.
With `Manual` mapping each series keeps its x, y, size and color fields, its frame or query and its name.
Fields are chosen by name, and y also by names or a regular expression.
The older `seriesMapping` and `dims` options import the same way.

Point size and color follow the size and color fields, and `custom.pointSize.min` and `custom.pointSize.max` set the size range.
These import too: the point shape (circle or square), point stroke width, line width, fill opacity, axis options, legend and tooltip options.
The square line style and custom dash patterns import as on a time series, and an ordinal scale is reported.

#### Node graph

A node graph imports its layout (`layered`, the default, `force` or `grid`) and the greedy zoom mode as mouse wheel zoom.
Its stat units and arcs import as overrides on the main stat, secondary stat and arc fields.
Node and edge units that differ are reported.

#### Placeholders and special panel types

These Grafana panel types import as follows.

| Grafana panel | Imports as | Notes |
| --- | --- | --- |
| Annotation list, live panel and Alertmanager panel | A placeholder that gives the reason | Annotations, live streaming and alert rules are not part of SlashObserve. |
| Library panel | A normal panel when the export carries its definition, else a placeholder that says library panels are not supported | A dashboard exported for sharing externally carries the definition. SlashObserve has no library panels, so an imported one is copied into the dashboard. |
| `welcome` | A text panel with a one-line note | |
| `gettingstarted` | A text checklist that links these docs | |
| `datagrid` | A table that shows the frame its `selectedSeries` names | It shows the first frame when `selectedSeries` is empty. |
| `debug` | A table over its own targets, or a text panel without targets | The debug modes are not part of SlashObserve. |
| Alert list | One Prometheus `ALERTS` query | A Grafana-managed or legacy alert list, or one on a datasource that is not Prometheus, becomes a placeholder. |
| Dashboard list | A static panel | It needs no datasource match. A starred list is reported and shows dashboards by last update. |
| News | A news panel with the feed address, changed from http to https and reported | An empty address gets Grafana's blog feed. The feed's domain must be on the [outbound allowlist](../operate/outbound-access.md#news-feeds) before the panel shows items. |

#### Legacy panel types

Grafana's renamed and retired panel types import as their modern type, such as `graph`, `table-old` and `singlestat`.
`grafana-worldmap-panel` imports as a geomap, with its circle sizes, thresholds and colors, view and wheel zoom.

A world map whose `locationData` is `countries`, `countries_3letter` or `states` imports as a markers layer that looks places up by name in the countries or US states gazetteer.
The `valueName` becomes a `reduce` transform of the queries, and `current` reduces to the last value that is not null.
A table-format query makes the transform unnecessary, because its rows already hold one value each.
On a world map with table data the `valueName` has no effect, as in Grafana.
On any other location, a `valueName` other than `current` is reported.
The keys Grafana's own world map migration does not read import without a note: `esMetric`, `hideEmpty`, `hideZero`, `stickyLabels`, the legend, the unit words and the label field.

Graph options that Grafana's own time series migration drops import without a note, such as `paceLength`, `hiddenSeries`, `renderer`, `options.alertThreshold`, `yaxis.align`, `yaxis.alignLevel`, the tooltip's millisecond resolution, thresholds without a value, and `percentage` without stacking.
A legacy graph alert is excluded with alert rules.
A legacy graph's threshold fill and line choose the threshold style the way Grafana's migration does (lines, areas, or both), and `dashLength` and `spaceLength` set the pattern of a dashed line.

A legacy singlestat imports without a note for the keys Grafana's own singlestat-to-stat migration drops, such as:

- `minSpan`, the prefix and postfix colors, the font sizes, the sparkline fill and full width, and the mapping editor choices;
- a prefix or postfix beside a unit (the unit is kept, as in Grafana);
- a null mode, except that a connected null mode turns the first value into the first value that is not null.

The sparkline line color imports without a note too, because the sparkline is drawn in the series color.

### Transformations

Transformations import in order, up to 20 per panel, and a disabled one is skipped.
These import:

- `merge`, `seriesToRows`, `concatenate`, `transpose` and `heatmap`;
- `organize` (and the older `rename`, `order` and `filterFields`), `filterFieldsByName` and `renameByRegex`;
- `reduce`, `joinByField` (and `seriesToColumns`), `calculateField`, `sortBy` and `limit`;
- `convertFieldType`, `formatString`, `formatTime`, `filterByValue`, `filterByRefId` and `extractFields`;
- `labelsToFields`, `joinByLabels`, `partitionByValues`, `prepareTimeSeries`, `timeSeriesTable`, `rowsToFields` and `configFromData`;
- `groupBy`, `groupToNestedTable`, `groupingToMatrix`, `histogram` and `regression`;
- `spatial` and `fieldLookup`.

See [Field options, overrides and transforms](../concepts/observespec.md#field-options-overrides-and-transforms) for what each does.
Any other transformation, one past the 20th, and an option the Spec cannot hold are reported at their path.

#### Grouping steps and calculateField

The grouping steps keep only their `by` and `aggregate` fields, as Grafana's output does.
They take every reducer, including the true or false and list reducers (`allIsNull`, `allIsZero`, `allValues` and `uniqueValues`).
A reducer name Grafana does not have is reported.

`calculateField` imports its binary, row reduce (including its list of fields), unary, index, cumulative and window modes.
The unary `percent`, a row reduce `nullValueMode` and `timeSeries: false` are reported.
A binary calculation over field names with spaces imports.
A name that holds a spaced operator, or a matcher other than `byName`, is reported.
A `convertFieldType` with no conversions imports as nothing, since Grafana changes no field either.

#### Limiting a transformation to some queries

A transformation that Grafana limits to some queries (a `filter` of `byRefId`) imports as `queries`, matched against the refs the panel imported.
A match is one ref, refs joined with `|`, a list or a `/regex/` pattern.
A frame filter (`byFrameRefID`) imports the same way.
A ref that did not import, a pattern that matches none and any other filter are reported at `filter`, and the transformation then applies to every query.

`filterByRefId` imports `include` and `exclude`.
`filterByValue` imports every filter Grafana offers, including `between` and `substring`.
`notSubstring` imports only as a transformation's only filter.
`formatTime` imports the `utc` time zone, and any other time zone is reported.

#### Heatmap, spatial and lookups

The `heatmap` transformation imports its x and y buckets, the log, symlog or ordinal row scale, the log or symlog time scale and `keepOriginalData`.
A heatmap panel draws its cells directly.
An ordinal time scale, a log base other than 2 and 10, and `rowsFrame` are reported.

`spatial` imports its action (prepare, calculate or modify), its source and target locations (automatic, coordinates, geohash or lookup) and the line builder.
A lookup in `spatial` or `fieldLookup` uses the built-in countries or US states gazetteer.
An airports gazetteer or one from a URL drops the transformation and is reported ("Airport lookup is not available." or "Only the built-in countries and US states gazetteers are supported.").

### Options that are not imported

Some Grafana options are reported and not imported by decision, because SlashObserve does not have the feature.

#### Grafana services and links

- Grafana scopes on Prometheus queries, and images served by a Grafana server.
- The Infinity datasource (canvas actions and field actions), Explore links, editing data from a panel, and CSS taken from a field.
- Recently viewed dashboards and Grafana folder filters (`folderUID` and `folderId`) in dashboard lists.
- Alert rules in alert lists: inactive alerts, instances, the normal state filter, alert grouping, dashboard alerts and the alert folder.
- The modes and counters of the debug panel.

#### Altinity queries and ClickHouse

- Grafana Live streaming on Altinity queries, and the log context window of Altinity queries.
- Hidden log labels in Altinity logs settings, and the Altinity ad hoc values query.
- A group-by variable on ClickHouse.

#### Logs panels

- The log context toggle, infinite log scrolling and the log field selector.

#### Axes and scales

- Vertical value axes placed at the top or bottom, and ordinal value scales, on time series, trend, candlestick, histogram, bar chart and XY chart panels.
- The time comparison and horizontal orientation of time series.

#### Gauges, heatmaps and tables

- Bar gauge legends (turning one off imports without a note).
- The horizontal and vertical orientation of a gauge (a bar gauge imports its orientation).
- The heatmap selection modes `xy` and `y`.
- Map cells in tables.
- The `anyMatch`, `allMatch` and `invertMatch` override matchers, and the `byType` matcher for other fields.

#### Canvas

- Server element animation (blink rate and bulb color).
- Canvas connection widths that follow a rule and connection directions that follow a field, canvas zoom to content, the one-click canvas tooltip and the canvas background image mapping mode.

#### Geomap

- MapLibre basemaps, dynamic GeoJSON layers, the last-point tracker and photo frames.

#### Loki

- Grafana Live streaming, and the line limit of Loki queries (a logs panel shows up to the datasource's row limit).
- Reading Loki lines oldest first.

#### Tempo

- The search builder, the service graph, uploaded trace files and TraceQL metrics queries.
- The trace limit, span-set limit and span tables of Tempo queries (a search lists one row per trace).

#### Elasticsearch and OpenSearch

- PPL queries (rewrite them in KQL or Query DSL).
- `geohash_grid` and `nested` bucket aggregations, and trimmed histogram edges.
- The `top_metrics` metric, inline scripts on a metric, and hiding one metric of a query.
- Field-name variables (`{"find": "fields"}`), which have no equivalent.

#### Transformation options

- The `convertFrameType` conversions to annotations, time regions and exemplars.
- Transformations of annotations (`topic: annotations`).
- The percent unary operator and the max window reducer.
- Enum and time zone conversions of field types.
- Ordering by label or in descending name order, and by mode `auto`.
- The semicolon and pipe separators and names of partition by values.
- The ignore and threshold modes of rows to fields with its name and value fields, and the inline statistic of time series table.

The report marks these options as excluded by decision, apart from options that are not supported yet.
A panel whose only report lines are such decisions counts as imported, and its report line still lists them.

### Imported without a note

These imports carry no report line:

- Options that Grafana itself does not draw or read where they are written, such as the legend options of a stat panel, a log base on a query, the unit scale of a field, and a Prometheus exemplar flag on an instant query or on a panel that draws no exemplars.
- Editor state, such as a query's help toggle or a query's own `adhocFilters` list.
- Inputs Grafana rejects.

A query with no text is dropped without a note, because Grafana sends no request for it and the panel shows the same data without it.
A panel left with no query becomes a placeholder.
