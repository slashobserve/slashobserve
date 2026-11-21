package surface

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/api/apitest"
)

type outcome string

const (
	servesData      outcome = "200 with data"
	absentFromList  outcome = "absent from the list"
	forbiddenRole   outcome = "403 forbidden_role"
	notFound        outcome = "404 not found"
	unauthenticated outcome = "401"
	builtInReadOnly outcome = "409 datasource_read_only"
	wrongKind       outcome = "400 invalid_request"
	noModel         outcome = "503 agent_unavailable"
	sessionRequired outcome = "403 user_session_required"
	unknownSource   outcome = "refused unknown_datasource"
)

const (
	missingUID = "no-such-source"
	missingID  = "00000000-0000-4000-8000-000000000000"
	window     = `"from":"now-1h","to":"now"`
)

type observed struct {
	status  int
	code    string
	body    string
	refusal string
}

func observeREST(r response) observed {
	var parsed struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal([]byte(r.body), &parsed)
	return observed{status: r.status, code: parsed.Code, body: r.body}
}

func observeTool(a toolAnswer) observed {
	o := observed{status: a.status, body: a.text}
	if a.isError {
		o.refusal = a.code
		if o.refusal == "" {
			o.refusal = "unclassified"
		}
	}
	return o
}

func (o observed) classify(want string, list bool) outcome {
	switch {
	case o.status == http.StatusUnauthorized:
		return unauthenticated
	case o.status == http.StatusForbidden && o.code == "forbidden_role", o.refusal == "forbidden_role":
		return forbiddenRole
	case o.status == http.StatusForbidden && o.code == "user_session_required":
		return sessionRequired
	case o.status == http.StatusConflict && o.code == "datasource_read_only":
		return builtInReadOnly
	case o.status == http.StatusBadRequest && o.code == "invalid_request":
		return wrongKind
	case o.status == http.StatusServiceUnavailable && o.code == "agent_unavailable":
		return noModel
	case o.status == http.StatusNotFound, o.refusal == "not_found":
		return notFound
	case o.refusal != "":
		return outcome("refused " + o.refusal)
	case o.status == http.StatusOK && strings.Contains(o.body, want):
		return servesData
	case o.status == http.StatusOK && list:
		return absentFromList
	}
	return outcome(fmt.Sprintf("status %d code %q", o.status, o.code))
}

type surfaceRow struct {
	name    string
	send    func(s *stack, c caller) observed
	twin    func(s *stack, c caller) observed
	proof   func(s *stack) string
	control func(s *stack) string
	list    bool
	admin   outcome
	member  outcome
	special func(c caller) (outcome, bool)
}

func (r surfaceRow) expected(c caller) outcome {
	if r.special != nil {
		if o, ok := r.special(c); ok {
			return o
		}
	}
	switch {
	case c.kind == anonymousCaller:
		return unauthenticated
	case c.admin():
		return r.admin
	}
	return r.member
}

type text func(s *stack) string

func fixed(v string) text { return func(*stack) string { return v } }

var (
	auditFolderID = func(s *stack) string { return s.auditFolder }
	starterID     = func(s *stack) string { return s.starters[0] }
	starterTitle  = func(s *stack) string { return s.starterTitle }
	controlTitle  = fixed(controlBoard)
)

func queryOf(pairs ...string) string {
	values := url.Values{}
	for i := 0; i+1 < len(pairs); i += 2 {
		values.Set(pairs[i], pairs[i+1])
	}
	return values.Encode()
}

type restSpec struct {
	method  string
	path    string
	target  text
	body    string
	hidden  text
	proof   text
	control text
	list    bool
	admin   outcome
	member  outcome
	special func(c caller) (outcome, bool)
}

func (r restSpec) row(name string) surfaceRow {
	target := r.target
	if target == nil {
		target = fixed(r.path)
	}
	send := func(resolve text) func(s *stack, c caller) observed {
		return func(s *stack, c caller) observed {
			return observeREST(s.rest(c, r.method, resolve(s), r.body))
		}
	}
	row := surfaceRow{name: name, send: send(target), proof: r.proof, control: r.control, list: r.list, admin: r.admin, member: r.member, special: r.special}
	if r.hidden != nil {
		row.twin = send(func(s *stack) string {
			replacement := missingID
			if r.hidden(s) == auditUID {
				replacement = missingUID
			}
			return strings.Replace(target(s), r.hidden(s), replacement, 1)
		})
	}
	return row
}

type toolSpec struct {
	tool    string
	label   string
	args    func(s *stack, hidden string) map[string]any
	hidden  text
	proof   text
	control text
	list    bool
	admin   outcome
	member  outcome
}

func (r toolSpec) row() surfaceRow {
	send := func(replace func(s *stack) string) func(s *stack, c caller) observed {
		return func(s *stack, c caller) observed {
			args := map[string]any{}
			if r.args != nil {
				args = r.args(s, replace(s))
			}
			return observeTool(s.call(c, r.tool, args))
		}
	}
	row := surfaceRow{name: "MCP " + r.tool + r.label, proof: r.proof, control: r.control, list: r.list, admin: r.admin, member: r.member}
	row.send = send(func(s *stack) string {
		if r.hidden == nil {
			return ""
		}
		return r.hidden(s)
	})
	if r.hidden != nil {
		row.twin = send(func(s *stack) string {
			if r.hidden(s) == auditUID {
				return missingUID
			}
			return missingID
		})
	}
	return row
}

func auditRESTRows() []surfaceRow {
	adminOnly := func(name, path, proof string) surfaceRow {
		return restSpec{method: http.MethodGet, path: path, proof: fixed(proof), admin: servesData, member: forbiddenRole}.row(name)
	}
	return []surfaceRow{
		adminOnly("GET /audit/events", "/audit/events", marker),
		restSpec{method: http.MethodGet, target: func(s *stack) string { return "/audit/events/" + s.eventID }, proof: fixed(marker), admin: servesData, member: forbiddenRole}.row("GET /audit/events/{id}"),
		adminOnly("GET /audit/facets", "/audit/facets?"+queryOf("field", "actor_name"), "Ada"),
		adminOnly("GET /audit/histogram", "/audit/histogram", `"form"`),
		adminOnly("GET /audit/aggregate", "/audit/aggregate?"+queryOf("q", `{"aggs":{"by":{"terms":{"field":"actor_name"}}}}`), `"rows"`),
		adminOnly("GET /audit/fields", "/audit/fields", "actor_name"),
		adminOnly("GET /audit/export", "/audit/export", marker),
		{
			name:  "GET /audit/stream",
			send:  func(s *stack, c caller) observed { return observeREST(s.stream(c, "/audit/stream")) },
			proof: fixed("event: ready"), admin: servesData, member: forbiddenRole,
		},
	}
}

func onAuditDatasource(method, template, suffix, body, proof string) surfaceRow {
	return restSpec{method: method, path: "/datasources/" + auditUID + suffix, body: body, hidden: fixed(auditUID), proof: fixed(proof), admin: servesData, member: notFound}.row(method + " /datasources/{uid}" + template)
}

func adminOnlyDatasource(method, suffix, body string, admin outcome) surfaceRow {
	return restSpec{method: method, path: "/datasources/" + auditUID + suffix, body: body, proof: fixed(auditUID), admin: admin, member: forbiddenRole}.row(method + " /datasources/{uid}" + suffix)
}

func unsupportedOnAudit(row surfaceRow) surfaceRow {
	row.admin = wrongKind
	return row
}

func datasourceRESTRows() []surfaceRow {
	return []surfaceRow{
		restSpec{method: http.MethodGet, path: "/datasources", proof: fixed(auditUID), control: fixed(controlUID), list: true, admin: servesData, member: absentFromList}.row("GET /datasources"),
		adminOnlyDatasource(http.MethodGet, "/config", "", builtInReadOnly),
		onAuditDatasource(http.MethodPost, "/recheck", "/recheck", "", auditUID),
		onAuditDatasource(http.MethodPost, "/query", "/query", `{"query":"actor_name:Ada","kind":"instant",`+window+`}`, "frames"),
		onAuditDatasource(http.MethodPost, "/log-volume", "/log-volume", `{"query":"*",`+window+`}`, "buckets"),
		onAuditDatasource(http.MethodPost, "/log-facets", "/log-facets", `{"query":"*",`+window+`}`, `"fields"`),
		onAuditDatasource(http.MethodPost, "/log-patterns", "/log-patterns", `{"query":"*",`+window+`}`, "patterns"),
		onAuditDatasource(http.MethodPost, "/log-context", "/log-context", `{"query":"*","anchor":"1700000000000000000"}`, "before"),
		onAuditDatasource(http.MethodPost, "/lint", "/lint", `{"query":"outcome:denied"}`, "diagnostics"),
		unsupportedOnAudit(onAuditDatasource(http.MethodPost, "/trace-stats", "/trace-stats", `{"query":"*",`+window+`}`, "meta")),
		unsupportedOnAudit(onAuditDatasource(http.MethodPost, "/trace-compare", "/trace-compare", `{"selected":["0123456789abcdef"],"baseline":["fedcba9876543210"],`+window+`}`, "attributes")),
		onAuditDatasource(http.MethodPost, "/trace-logs", "/trace-logs", `{"trace_id":"0123456789abcdef0123456789abcdef",`+window+`}`, "frames"),
		restSpec{method: http.MethodPost, path: "/datasources/" + auditUID + "/draft-query", body: `{"question":"how many denied reads"}`, proof: fixed("query"), admin: noModel, member: noModel}.row("POST /datasources/{uid}/draft-query"),
		{
			name: "POST /datasources/{uid}/preview",
			send: func(s *stack, c caller) observed {
				return observeREST(s.rest(c, http.MethodPost, "/datasources/"+auditUID+"/preview", s.previewBody()))
			},
			proof: fixed("frames"), admin: servesData, member: notFound,
			special: func(c caller) (outcome, bool) { return sessionRequired, c.kind == serviceAccountKey },
		},
		onAuditDatasource(http.MethodGet, "/schema", "/schema", "", "tables"),
		onAuditDatasource(http.MethodGet, "/sample", "/sample", "", "rows"),
		onAuditDatasource(http.MethodGet, "/metrics", "/metrics", "", "metrics"),
		onAuditDatasource(http.MethodGet, "/labels", "/labels", "", "labels"),
		onAuditDatasource(http.MethodGet, "/labels/{name}/values", "/labels/outcome/values", "", "values"),
		onAuditDatasource(http.MethodGet, "/services", "/services", "", "services"),
		unsupportedOnAudit(onAuditDatasource(http.MethodGet, "/traces/{trace_id}", "/traces/0123456789abcdef0123456789abcdef", "", "frames")),
		{
			name: "GET /datasources/{uid}/tail",
			send: func(s *stack, c caller) observed {
				return observeREST(s.stream(c, "/datasources/"+auditUID+"/tail?"+queryOf("query", "*")))
			},
			twin: func(s *stack, c caller) observed {
				return observeREST(s.stream(c, "/datasources/"+missingUID+"/tail?"+queryOf("query", "*")))
			},
			proof: fixed("event: ready"), admin: servesData, member: notFound,
		},
	}
}

func datasourceMutationRows() []surfaceRow {
	return []surfaceRow{
		adminOnlyDatasource(http.MethodPatch, "", `{"name":"renamed"}`, builtInReadOnly),
		adminOnlyDatasource(http.MethodDelete, "", "", builtInReadOnly),
	}
}

func workspaceRESTRows() []surfaceRow {
	dashboard := func(template, suffix, proof string) surfaceRow {
		return restSpec{
			method: http.MethodGet, target: func(s *stack) string { return "/dashboards/" + s.starters[0] + suffix },
			hidden: starterID, proof: fixed(proof), admin: servesData, member: notFound,
		}.row("GET /dashboards/{id}" + template)
	}
	return []surfaceRow{
		restSpec{method: http.MethodGet, path: "/folders", proof: fixed(`"Audit"`), control: fixed(controlFolderName), list: true, admin: servesData, member: absentFromList}.row("GET /folders"),
		restSpec{method: http.MethodGet, target: func(s *stack) string { return "/folders/" + s.auditFolder }, hidden: auditFolderID, proof: fixed(`"Audit"`), admin: servesData, member: notFound}.row("GET /folders/{id}"),
		restSpec{method: http.MethodGet, target: func(s *stack) string { return "/folders/" + s.auditFolder + "/permissions" }, hidden: auditFolderID, proof: fixed("admins_only"), admin: servesData, member: notFound}.row("GET /folders/{id}/permissions"),
		restSpec{method: http.MethodGet, path: "/dashboards", proof: starterTitle, control: controlTitle, list: true, admin: servesData, member: absentFromList}.row("GET /dashboards"),
		dashboard("", "", `"title"`),
		dashboard("/versions", "/versions", `"version"`),
		dashboard("/versions/{version}", "/versions/1", `"version"`),
		restSpec{
			method: http.MethodGet, target: func(s *stack) string {
				return "/dashboards/" + s.starters[0] + "/versions/1/panels/" + s.starterPanel + "/data"
			},
			hidden: starterID, proof: fixed(`"meta"`), admin: servesData, member: notFound,
		}.row("GET /dashboards/{id}/versions/{version}/panels/{panel_id}/data"),
		restSpec{method: http.MethodGet, path: "/search?" + queryOf("q", "audit"), proof: starterTitle, control: controlTitle, list: true, admin: servesData, member: absentFromList}.row("GET /search"),
		restSpec{method: http.MethodGet, path: "/activity", proof: starterTitle, control: controlTitle, list: true, admin: servesData, member: absentFromList}.row("GET /activity"),
	}
}

func restrictedRunRESTRows() []surfaceRow {
	restrictedRun := func(s *stack) string { return s.restrictedRun }
	plainRun := func(s *stack) string { return s.plainRun }
	restrictedChat := func(s *stack) string { return s.restrictedChat }
	revision := func(id text) text {
		return func(s *stack) string { return "/dashboards/" + s.controlBoard + "/revisions/" + id(s) }
	}
	return []surfaceRow{
		restSpec{
			method: http.MethodGet, target: func(s *stack) string { return "/dashboards/" + s.controlBoard + "/revisions" },
			proof: restrictedRun, control: plainRun, list: true, admin: servesData, member: absentFromList,
		}.row("GET /dashboards/{id}/revisions of a run that read the audit trail"),
		restSpec{method: http.MethodGet, target: revision(restrictedRun), hidden: restrictedRun, proof: restrictedRun, admin: servesData, member: notFound}.row("GET /dashboards/{id}/revisions/{revision_id} of a run that read the audit trail"),
		restSpec{method: http.MethodGet, target: revision(plainRun), proof: plainRun, admin: servesData, member: servesData}.row("GET /dashboards/{id}/revisions/{revision_id} of an ordinary run"),
		restSpec{method: http.MethodGet, target: func(s *stack) string { return "/observe/" + s.restrictedChat }, hidden: restrictedChat, proof: restrictedChat, admin: servesData, member: notFound}.row("GET /observe/{id} of a chat that read the audit trail"),
	}
}

func mcpRows() []surfaceRow {
	adminOnly := func(tool, proof string, args func(s *stack, _ string) map[string]any) surfaceRow {
		return toolSpec{tool: tool, args: args, proof: fixed(proof), admin: servesData, member: forbiddenRole}.row()
	}
	onAudit := func(tool, proof string, extra map[string]any) surfaceRow {
		return toolSpec{
			tool: tool, hidden: fixed(auditUID), proof: fixed(proof), admin: servesData, member: unknownSource,
			args: func(_ *stack, uid string) map[string]any {
				args := map[string]any{"datasource": uid}
				for k, v := range extra {
					args[k] = v
				}
				return args
			},
		}.row()
	}
	byDashboard := func(tool string, proof text) surfaceRow {
		return toolSpec{
			tool: tool, hidden: starterID, proof: proof, admin: servesData, member: notFound,
			args: func(_ *stack, id string) map[string]any { return map[string]any{"dashboard_id": id} },
		}.row()
	}
	return []surfaceRow{
		adminOnly("search_audit_events", marker, func(*stack, string) map[string]any { return map[string]any{"query": "actor_name:Ada"} }),
		adminOnly("get_audit_event", marker, func(s *stack, _ string) map[string]any { return map[string]any{"id": s.eventID} }),
		adminOnly("list_audit_facets", "Ada", func(*stack, string) map[string]any { return map[string]any{"field": "actor_name"} }),
		adminOnly("get_audit_histogram", "buckets", nil),
		adminOnly("aggregate_audit_events", "rows", func(*stack, string) map[string]any {
			return map[string]any{"query": `{"aggs":{"by":{"terms":{"field":"actor_name"}}}}`}
		}),
		adminOnly("list_audit_fields", "actor_name", nil),
		toolSpec{tool: "list_datasources", proof: fixed(auditUID), control: fixed(controlUID), list: true, admin: servesData, member: absentFromList}.row(),
		onAudit("get_schema", "tables", nil),
		onAudit("list_labels", "labels", nil),
		onAudit("recheck_datasource", auditUID, nil),
		onAudit("query_instant", "frames", map[string]any{"query": "actor_name:Ada", "from": "now-1h", "to": "now"}),
		toolSpec{tool: "list_folders", proof: fixed("Audit"), control: fixed(controlFolderName), list: true, admin: servesData, member: absentFromList}.row(),
		toolSpec{tool: "list_dashboards", proof: starterTitle, control: controlTitle, list: true, admin: servesData, member: absentFromList}.row(),
		byDashboard("get_dashboard", starterTitle),
		byDashboard("list_dashboard_versions", fixed("version")),
		toolSpec{
			tool: "search_workspace", proof: starterTitle, control: controlTitle, list: true, admin: servesData, member: absentFromList,
			args: func(*stack, string) map[string]any { return map[string]any{"q": "audit"} },
		}.row(),
		toolSpec{tool: "list_activity", proof: starterTitle, control: controlTitle, list: true, admin: servesData, member: absentFromList}.row(),
		toolSpec{
			tool: "get_run", label: " of a run that read the audit trail", hidden: func(s *stack) string { return s.restrictedRun }, proof: func(s *stack) string { return s.restrictedRun }, admin: servesData, member: notFound,
			args: func(_ *stack, id string) map[string]any { return map[string]any{"id": id} },
		}.row(),
		toolSpec{
			tool: "get_run", label: " of an ordinary run", proof: func(s *stack) string { return s.plainRun }, admin: servesData, member: servesData,
			args: func(s *stack, _ string) map[string]any { return map[string]any{"id": s.plainRun} },
		}.row(),
		toolSpec{
			tool: "get_chat", label: " of a chat that read the audit trail", hidden: func(s *stack) string { return s.restrictedChat }, proof: func(s *stack) string { return s.restrictedChat }, admin: servesData, member: notFound,
			args: func(_ *stack, id string) map[string]any { return map[string]any{"id": id} },
		}.row(),
		{
			name: "MCP tools/list",
			send: func(s *stack, c caller) observed {
				return observeTool(s.mcpPost(c, []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)))
			},
			proof: fixed("search_audit_events"), admin: servesData, member: servesData,
		},
	}
}

func allRows() []surfaceRow {
	return slices.Concat(auditRESTRows(), datasourceRESTRows(), workspaceRESTRows(), restrictedRunRESTRows(), mcpRows(), datasourceMutationRows())
}

func TestAuditSurfacesByRoleAndCredential(t *testing.T) {
	s := newStack(t)
	got := map[string]map[string]outcome{}
	for _, row := range allRows() {
		got[row.name] = map[string]outcome{}
		for _, c := range callers() {
			t.Run(row.name+"/"+c.name, func(t *testing.T) {
				s.t = t
				seen := row.send(s, c)
				result := seen.classify(row.proof(s), row.list)
				got[row.name][c.name] = result
				if want := row.expected(c); result != want {
					if !c.admin() && result == servesData {
						t.Fatalf("SECURITY: %s served audit data to %s: %.300s", row.name, c.name, seen.body)
					}
					t.Fatalf("%s as %s = %s, want %s; status %d code %q refusal %q body %.400s", row.name, c.name, result, want, seen.status, seen.code, seen.refusal, seen.body)
				}
				if row.control != nil && result != unauthenticated && result != forbiddenRole && !strings.Contains(seen.body, row.control(s)) {
					t.Fatalf("%s as %s lacks the control %q, so a hidden result would pass as an empty one: %.300s", row.name, c.name, row.control(s), seen.body)
				}
				if row.twin != nil && result == notFound {
					twin := row.twin(s, c)
					if twin.status != seen.status || twin.refusal != seen.refusal || twin.code != seen.code {
						t.Fatalf("%s as %s answers differently from a missing id: %d %q %q against %d %q %q", row.name, c.name, seen.status, seen.code, seen.refusal, twin.status, twin.code, twin.refusal)
					}
				}
			})
		}
	}
	logMatrix(t, got)
}

func logMatrix(t *testing.T, got map[string]map[string]outcome) {
	names := make([]string, 0, len(got))
	for name := range got {
		names = append(names, name)
	}
	sort.Strings(names)
	cs := callers()
	var b strings.Builder
	b.WriteString("\n| surface |")
	for _, c := range cs {
		b.WriteString(" " + c.name + " |")
	}
	b.WriteString("\n|---|")
	for range cs {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	for _, name := range names {
		b.WriteString("| " + name + " |")
		for _, c := range cs {
			b.WriteString(" " + string(got[name][c.name]) + " |")
		}
		b.WriteString("\n")
	}
	t.Log(b.String())
}

var outOfScope = map[string]bool{
	"POST /datasources":      true,
	"POST /datasources/test": true,
}

func TestEveryAuditAndDatasourceOperationHasARow(t *testing.T) {
	covered := map[string]bool{}
	for _, row := range allRows() {
		covered[row.name] = true
	}
	for path, item := range apitest.Load(t).Paths.Map() {
		if !strings.HasPrefix(path, "/audit/") && !strings.HasPrefix(path, "/datasources") {
			continue
		}
		for method := range item.Operations() {
			name := method + " " + path
			if !covered[name] && !outOfScope[name] {
				t.Errorf("%s has no row in the audit access matrix", name)
			}
		}
	}
}
