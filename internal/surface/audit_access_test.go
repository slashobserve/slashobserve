package surface

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type outcome string

const (
	servesData      outcome = "200 with data"
	absentFromList  outcome = "absent from the list"
	forbiddenRole   outcome = "403 forbidden_role"
	notFound        outcome = "404 not found"
	unauthenticated outcome = "401"
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
	name   string
	send   func(s *stack, c caller) observed
	proof  func(s *stack) string
	list   bool
	admin  outcome
	member outcome
}

func (r surfaceRow) expected(c caller) outcome {
	switch {
	case c.kind == anonymousCaller:
		return unauthenticated
	case c.admin():
		return r.admin
	}
	return r.member
}

func restRow(name, method string, target func(s *stack) string, body func(s *stack) string, proof func(s *stack) string, list bool, admin, member outcome) surfaceRow {
	return surfaceRow{
		name: name,
		send: func(s *stack, c caller) observed {
			payload := ""
			if body != nil {
				payload = body(s)
			}
			return observeREST(s.rest(c, method, target(s), payload))
		},
		proof: proof, list: list, admin: admin, member: member,
	}
}

func fixed(v string) func(*stack) string { return func(*stack) string { return v } }

func queryOf(pairs ...string) string {
	values := url.Values{}
	for i := 0; i < len(pairs); i += 2 {
		values.Set(pairs[i], pairs[i+1])
	}
	return values.Encode()
}

func auditRESTRows() []surfaceRow {
	adminOnly := func(name, target string, proof string) surfaceRow {
		return restRow(name, http.MethodGet, fixed(target), nil, fixed(proof), false, servesData, forbiddenRole)
	}
	return []surfaceRow{
		adminOnly("GET /audit/events", "/audit/events", marker),
		restRow("GET /audit/events/{id}", http.MethodGet, func(s *stack) string { return "/audit/events/" + s.eventID }, nil, fixed(marker), false, servesData, forbiddenRole),
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

func allRows() []surfaceRow {
	return auditRESTRows()
}

func TestAuditSurfacesByRoleAndCredential(t *testing.T) {
	s := newStack(t)
	for _, row := range allRows() {
		for _, c := range callers() {
			t.Run(row.name+"/"+c.name, func(t *testing.T) {
				s.t = t
				got := row.send(s, c).classify(row.proof(s), row.list)
				if want := row.expected(c); got != want {
					if !c.admin() && got == servesData {
						t.Fatalf("SECURITY: %s served audit data to %s", row.name, c.name)
					}
					t.Fatalf("%s as %s = %s, want %s", row.name, c.name, got, want)
				}
			})
		}
	}
}
