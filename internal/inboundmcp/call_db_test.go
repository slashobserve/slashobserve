package inboundmcp

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/datasource"
)

func (r *rig) approvedSource(t *testing.T, uid, secret string) {
	t.Helper()
	r.call(t, http.MethodPost, base, registration(uid, secret))
	r.discover(t, uid)
	r.call(t, http.MethodPost, base+"/"+uid+"/tools/approve-read-only", "")
}

func accessFor(t *testing.T, p auth.Principal) context.Context {
	t.Helper()
	return datasource.WithAccess(t.Context(), datasource.Access{By: p})
}

func TestAnApprovedToolOfARegisteredSourceIsListedAndCalled(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search the code"))
	r.approvedSource(t, "docs", "s3cret-value")

	offers, err := r.service.Offers(accessFor(t, r.admin))
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 1 || offers[0].UID != "docs" || offers[0].Host != fakeHost || len(offers[0].Tools) != 1 || offers[0].Tools[0].Name != "search" {
		t.Fatalf("offers = %+v", offers)
	}

	got, err := r.service.Call(accessFor(t, r.admin), "docs", "search", json.RawMessage(`{"query":"checkout"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "ok" || got.IsError || got.Cut {
		t.Fatalf("result = %+v", got)
	}
	r.fake.mu.Lock()
	defer r.fake.mu.Unlock()
	for _, header := range r.fake.headers {
		if header != "s3cret-value" {
			t.Errorf("the source saw the header %q", header)
		}
	}
	r.assertNoSecret(t, "s3cret-value")
}

func TestAnAdminsOnlySourceIsNeitherListedNorCalledForAViewer(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search the code"))
	r.approvedSource(t, "docs", "s3cret-value")
	viewer := auth.Principal{Kind: auth.KindUser, ID: "viewer", Name: "vi", Role: auth.RoleViewer}

	offers, err := r.service.Offers(accessFor(t, viewer))
	if err != nil || len(offers) != 0 {
		t.Fatalf("offers = %+v, err = %v", offers, err)
	}
	_, err = r.service.Call(accessFor(t, viewer), "docs", "search", json.RawMessage(`{"query":"x"}`))
	if failureOf(t, err).Code != tools.ContextCodeSourceNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestAChangedToolIsNotCalledUntilAnAdminApprovesItAgain(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search the code"))
	r.approvedSource(t, "docs", "s3cret-value")
	r.fake.setTools(readOnlyTool("search", "Search the code, and now something else"))
	r.discover(t, "docs")

	before := r.fake.requestCount()
	_, err := r.service.Call(accessFor(t, r.admin), "docs", "search", json.RawMessage(`{"query":"x"}`))
	if failureOf(t, err).Code != tools.ContextCodeToolNotApproved {
		t.Fatalf("err = %v", err)
	}
	if r.fake.requestCount() != before {
		t.Error("the source was called for a tool that changed")
	}
	offers, err := r.service.Offers(accessFor(t, r.admin))
	if err != nil || len(offers) != 0 {
		t.Fatalf("a source with no approved tool must not be offered: %+v %v", offers, err)
	}
}

func TestACallToASourceOffTheAllowlistIsReportedAsDown(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search the code"))
	r.approvedSource(t, "docs", "s3cret-value")
	r.database.Exec(t, `DELETE FROM outbound_domains`)

	before := r.fake.requestCount()
	_, err := r.service.Call(accessFor(t, r.admin), "docs", "search", json.RawMessage(`{"query":"x"}`))
	failure := failureOf(t, err)
	if failure.Code != tools.ContextCodeSourceDown || failure.Reason != FailureNotAllowed {
		t.Fatalf("failure = %+v", failure)
	}
	if r.fake.requestCount() != before {
		t.Error("the egress guard let a call reach a domain that is not allowed")
	}
}

func (r *rig) stateOf(t *testing.T, uid string) (string, string) {
	t.Helper()
	source, err := r.service.Get(t.Context(), uid)
	if err != nil {
		t.Fatal(err)
	}
	return source.State, source.Failure
}

func TestAFailedCallMarksTheSourceUnreachableAndTheNextGoodCallClearsIt(t *testing.T) {
	r := newRig(t, readOnlyTool("search", "Search the code"))
	r.approvedSource(t, "docs", "s3cret-value")
	if state, _ := r.stateOf(t, "docs"); state != StateReady {
		t.Fatalf("state after discovery = %q", state)
	}

	r.database.Exec(t, `DELETE FROM outbound_domains`)
	if _, err := r.service.Call(accessFor(t, r.admin), "docs", "search", json.RawMessage(`{"query":"x"}`)); failureOf(t, err).Code != tools.ContextCodeSourceDown {
		t.Fatalf("err = %v", err)
	}
	if state, failure := r.stateOf(t, "docs"); state != StateUnreachable || failure != FailureNotAllowed {
		t.Fatalf("after a failed call the source is %q (%q), want unreachable", state, failure)
	}

	r.allow(t, fakeHost)
	if _, err := r.service.Call(accessFor(t, r.admin), "docs", "search", json.RawMessage(`{"query":"x"}`)); err != nil {
		t.Fatal(err)
	}
	if state, failure := r.stateOf(t, "docs"); state != StateReady || failure != "" {
		t.Fatalf("after a good call the source is %q (%q), want ready", state, failure)
	}
}
