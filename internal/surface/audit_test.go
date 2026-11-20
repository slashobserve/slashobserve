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
}

var pendingAuditActions = []audit.Action{
	"agent.tool_call",
	"auth.credential_rejected",
	"auth.csrf_rejected",
	"block.add",
	"block.edit",
	"block.remove",
	"chat.create",
	"chat.delete",
	"chat.turn",
	"dashboard.export",
	"datasource.audience_change",
	"finding.apply",
	"folder.move",
	"logs.export",
	"migration.apply",
	"oauth.authorize",
	"oauth.client_register",
	"oauth.revoke",
	"oauth.token",
	"observation.arrange",
	"run.share",
	"run.withdraw",
	"server.start",
	"server.stop",
	"settings.change",
	"thread.agent_set",
	"webhook.deliver",
}

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
