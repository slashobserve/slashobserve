package surface

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/slashobserve/slashobserve/api/apitest"
	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/audit"
)

var nonHTTPAuditSources = map[audit.Action]string{
	audit.ActionGap:     "the access event writer, when its buffer drops events",
	audit.ActionPrune:   "the audit_partitions job, when it removes old partitions",
	audit.ActionRefuse:  "the request middleware, for a 401, 403 or 429 on a route that declares no action",
	"auth.login_failed": "the OIDC callback, when it refuses a sign-in",
	"thread.agent_set":  "the asks service, when a message changes which agent a thread asks or turns it off",
	"chat.create":       "the run funnel, in the transaction of a chat's first turn",
	"chat.turn":         "the run funnel, in the transaction of every turn a chat takes",
	"job.run":           "the scheduler, in the transaction that finishes a job run",
	"run.list":          "the run lists, once per list request",
	"chat.list":         "the chat list, once per list request",

	"saved_prompt.share":   "the prompts handler, in the transaction of a change that makes a prompt shared",
	"saved_prompt.unshare": "the prompts handler, in the transaction of a change that makes a shared prompt personal",

	"agent.tool_call":          "the MCP handler, for each tool call except the claim tools",
	"mcp_source.call":          "the MCP sources service, for each call_mcp_tool call that reaches it, allowed or refused",
	"auth.credential_rejected": "authentication, for a bearer credential that matches no key or token",
	"auth.csrf_rejected":       "authentication, for a cookie request refused by the cross-origin check",
	"migration.apply":          "the server at start, once for each migration it applied",
	audit.ActionOAuthAuthorize: "the OAuth authorize endpoint, mounted outside /api/v1",
	audit.ActionOAuthRegister:  "the OAuth registration endpoint, mounted outside /api/v1",
	audit.ActionOAuthRevoke:    "the OAuth revocation endpoint, mounted outside /api/v1",
	audit.ActionOAuthExchange:  "the OAuth token endpoint, mounted outside /api/v1",
	"server.start":             "the server, once it is about to listen",
	"server.stop":              "the server, once it has drained requests and before the audit writer stops",
	"webhook.deliver":          "the webhook worker, for each delivery attempt",
}

var pendingAuditActions = []audit.Action{}

type declaredOperation struct {
	where       string
	file        string
	declaration audit.Declaration
}

func readAuditDeclarations(doc *openapi3.T) ([]declaredOperation, []string) {
	var declared []declaredOperation
	var problems []string
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			where := fmt.Sprintf("%s %s", strings.ToLower(method), path)
			file := specFile(item)
			d, ok, err := audit.ReadDeclaration(op)
			switch {
			case !ok:
				problems = append(problems, fmt.Sprintf("%s has no %s: add { action, target } or { none: <reason> } in %s", where, audit.Extension, file))
				continue
			case err != nil:
				problems = append(problems, fmt.Sprintf("%s: %s %v in %s", where, audit.Extension, err, file))
				continue
			}
			if name, ok := d.Target.PathParam(); ok && !strings.Contains(path, "{"+name+"}") {
				problems = append(problems, fmt.Sprintf("%s: %s target id names path.%s, which is not a parameter of the path, in %s", where, audit.Extension, name, file))
			}
			declared = append(declared, declaredOperation{where: where, file: file, declaration: d})
		}
	}
	slices.Sort(problems)
	return declared, problems
}

func TestEveryOperationDeclaresItsAuditAction(t *testing.T) {
	_, problems := readAuditDeclarations(apitest.Load(t))
	for _, p := range problems {
		t.Error(p)
	}
}

func TestEveryDeclaredAuditActionIsInTheCatalog(t *testing.T) {
	declared, _ := readAuditDeclarations(apitest.Load(t))
	for _, op := range declared {
		if op.declaration.Action == "" {
			continue
		}
		if _, ok := audit.Lookup(op.declaration.Action); !ok {
			t.Errorf("%s declares %s %q, which is not in the catalog in internal/audit/catalog.go (%s)", op.where, audit.Extension, op.declaration.Action, op.file)
		}
	}
}

func TestEveryCatalogActionHasASource(t *testing.T) {
	declared, _ := readAuditDeclarations(apitest.Load(t))
	sources := map[audit.Action]bool{}
	for _, op := range declared {
		sources[op.declaration.Action] = true
	}
	for _, typ := range activity.Types() {
		action, ok := activity.AuditAction(typ)
		if !ok {
			t.Errorf("activity type %q has no audit action in internal/activity/audit.go", typ)
		}
		sources[action] = true
	}
	for action := range nonHTTPAuditSources {
		sources[action] = true
	}
	for _, action := range audit.Actions() {
		pending := slices.Contains(pendingAuditActions, action)
		switch {
		case sources[action] && pending:
			t.Errorf("%s is recorded now: remove it from pendingAuditActions", action)
		case !sources[action] && !pending:
			t.Errorf("nothing records %s: name it in a route's %s, in an activity mapping or in nonHTTPAuditSources", action, audit.Extension)
		}
	}
	for _, action := range pendingAuditActions {
		if _, ok := audit.Lookup(action); !ok {
			t.Errorf("pending action %s is not in the catalog", action)
		}
	}
}

func TestFolderAndTeamActionsAreRecordedAndNotPending(t *testing.T) {
	for _, action := range []audit.Action{"folder.create", "folder.rename", "folder.delete", "folder.permissions_change", "team.create", "team.delete", "team.member_add", "team.member_remove"} {
		if _, ok := audit.Lookup(action); !ok {
			t.Errorf("%s is not in the catalog", action)
		}
		if slices.Contains(pendingAuditActions, action) {
			t.Errorf("%s is recorded inside its service's transaction and must not be pending", action)
		}
	}
}

func TestAuthAndSystemActionsAreNoLongerPending(t *testing.T) {
	recorded := []audit.Action{
		"agent.tool_call", "auth.credential_rejected", "auth.csrf_rejected", "auth.login_failed",
		"migration.apply", "oauth.authorize", "oauth.client_register", "oauth.revoke", "oauth.token",
		"server.start", "server.stop", "webhook.deliver",
		"service_account.create", "service_account.key_create", "service_account.key_revoke",
	}
	for _, action := range recorded {
		if slices.Contains(pendingAuditActions, action) {
			t.Errorf("%s is recorded and must not be in pendingAuditActions", action)
		}
	}
}

const publicTokenParam = "{token}"

func TestEveryUnrecordedRouteThatNamesAnIdDeclaresWhatARefusedCallIsAbout(t *testing.T) {
	declared, _ := readAuditDeclarations(apitest.Load(t))
	for _, op := range declared {
		_, path, _ := strings.Cut(op.where, " ")
		d := op.declaration
		if d.None == "" || !strings.Contains(path, "{") || strings.Contains(path, publicTokenParam) {
			continue
		}
		if _, named := d.Target.PathParam(); !named {
			t.Errorf("%s declares %s none and names an id in its path: add target { kind, id: path.<name> } so a refused call is recorded (%s); a public token is never a target", op.where, audit.Extension, op.file)
		}
	}
}
