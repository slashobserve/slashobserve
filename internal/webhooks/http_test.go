package webhooks

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/egress"
)

type httpRig struct {
	*rig
	engine *gin.Engine
	router routers.Router
	caller auth.Principal
}

var adminKey = auth.Principal{Kind: auth.KindServiceAccount, ID: "3f1c2a52-8c5e-4a53-9a3e-1d2d4c0a9b11", Name: "ci", Role: auth.RoleAdmin}

func newHTTPRig(t *testing.T, base *rig) *httpRig {
	t.Helper()
	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(openapi3.FormatOfStringForUUIDOfRFC9562))
	doc, err := api.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	h := &httpRig{rig: base, router: router, caller: adminOne}
	gin.SetMode(gin.TestMode)
	h.engine = gin.New()
	NewHandler(base.svc, discard()).Register(auth.NewRoutes(h.engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), h.caller))
	})))
	return h
}

func (h *httpRig) call(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.engine.ServeHTTP(rec, req)

	check := httptest.NewRequest(method, path, strings.NewReader(body))
	check.Header = req.Header.Clone()
	route, params, err := h.router.FindRoute(check)
	if err != nil {
		t.Fatalf("%s %s is not documented: %v", method, path, err)
	}
	options := &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, IncludeResponseStatus: true, SkipSettingDefaults: true}
	input := &openapi3filter.RequestValidationInput{Request: check, PathParams: params, Route: route, Options: options}
	if err := openapi3filter.ValidateRequest(t.Context(), input); err != nil && rec.Code < 400 {
		t.Fatalf("%s %s breaks the document but answered %d: %v", method, path, rec.Code, err)
	}
	check.Body = io.NopCloser(bytes.NewReader(nil))
	response := &openapi3filter.ResponseValidationInput{RequestValidationInput: input, Status: rec.Code, Header: rec.Header(), Options: options}
	response.SetBodyBytes(rec.Body.Bytes())
	if err := openapi3filter.ValidateResponse(t.Context(), response); err != nil {
		t.Fatalf("%s %s answered %d against the document: %v\n%s", method, path, rec.Code, err, rec.Body)
	}
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return out
}

func expect(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	got := ""
	if code != "" {
		got = decode[struct {
			Code string `json:"code"`
		}](t, rec).Code
	}
	if rec.Code != status || got != code {
		t.Fatalf("answered %d %s, want %d %s", rec.Code, rec.Body, status, code)
	}
}

type webhookBody struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	Owner   struct {
		ID string `json:"id"`
	} `json:"owner"`
	ConsecutiveFailures int     `json:"consecutive_failures"`
	DisabledReason      *string `json:"disabled_reason"`
}

type withSecret struct {
	Webhook webhookBody `json:"webhook"`
	Secret  string      `json:"secret"`
}

const createBody = `{"name":"Pipeline","url":"` + receiverURL + `","event_types":["finding.created","run.succeeded"]}`

func parseSignature(t *testing.T, header string) (time.Time, string) {
	t.Helper()
	stamp, mac, ok := strings.Cut(header, ",")
	if !ok || !strings.HasPrefix(stamp, "t=") || !strings.HasPrefix(mac, "v1=") {
		t.Fatalf("signature header %q", header)
	}
	unix, err := strconv.ParseInt(strings.TrimPrefix(stamp, "t="), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return time.Unix(unix, 0), strings.TrimPrefix(mac, "v1=")
}

func TestHTTPEditorsAndViewersAreRefusedEverywhere(t *testing.T) {
	h := newHTTPRig(t, newRig(t))
	created := decode[withSecret](t, h.call(t, http.MethodPost, "/api/v1/webhooks", createBody))
	base := "/api/v1/webhooks/" + created.Webhook.ID
	for _, role := range []auth.Role{auth.RoleEditor, auth.RoleCommenter, auth.RoleViewer} {
		h.caller = auth.Principal{Kind: auth.KindUser, ID: editorOnly.ID, Name: "Edie", Role: role}
		for _, tc := range []struct{ method, path, body string }{
			{http.MethodGet, "/api/v1/webhooks", ""},
			{http.MethodPost, "/api/v1/webhooks", createBody},
			{http.MethodPatch, base, `{"enabled":false}`},
			{http.MethodDelete, base, ""},
			{http.MethodPost, base + "/secret", ""},
			{http.MethodPost, base + "/test", ""},
			{http.MethodGet, base + "/deliveries", ""},
		} {
			expect(t, h.call(t, tc.method, tc.path, tc.body), http.StatusForbidden, "forbidden_role")
		}
	}
	if got := len(h.sender.all()); got != 0 {
		t.Fatalf("a refused caller caused %d requests to the receiver", got)
	}
}

func TestHTTPAnAdminServiceAccountKeyMayManageWebhooks(t *testing.T) {
	h := newHTTPRig(t, newRig(t))
	h.caller = adminKey
	h.principals.set(adminKey)

	rec := h.call(t, http.MethodPost, "/api/v1/webhooks", createBody)

	expect(t, rec, http.StatusCreated, "")
	if got := decode[withSecret](t, rec).Webhook.Owner.ID; got != adminKey.ID {
		t.Fatalf("owner %s, want the key's account", got)
	}
}

func TestHTTPCreateShowsTheSecretOnceAndItVerifiesTheTestDelivery(t *testing.T) {
	h := newHTTPRig(t, newRig(t))

	rec := h.call(t, http.MethodPost, "/api/v1/webhooks", createBody)
	expect(t, rec, http.StatusCreated, "")
	created := decode[withSecret](t, rec)
	if !strings.HasPrefix(created.Secret, "whsec_") || len(created.Secret) != 49 || !created.Webhook.Enabled {
		t.Fatalf("created %s", rec.Body)
	}

	for _, path := range []string{"/api/v1/webhooks", "/api/v1/webhooks/" + created.Webhook.ID + "/deliveries"} {
		if body := h.call(t, http.MethodGet, path, "").Body.String(); strings.Contains(body, created.Secret) {
			t.Fatalf("GET %s returned the secret", path)
		}
	}
	var sealedBytes []byte
	if err := h.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT secret_ciphertext FROM webhooks`).Scan(&sealedBytes); err != nil || bytes.Contains(sealedBytes, []byte(created.Secret)) {
		t.Fatalf("stored secret is readable: %v", err)
	}

	tested := h.call(t, http.MethodPost, "/api/v1/webhooks/"+created.Webhook.ID+"/test", "")
	expect(t, tested, http.StatusOK, "")
	result := decode[struct {
		Delivery struct {
			Outcome     string `json:"outcome"`
			EventType   string `json:"event_type"`
			ActivitySeq *int64 `json:"activity_seq"`
			StatusCode  *int   `json:"status_code"`
		} `json:"delivery"`
	}](t, tested).Delivery
	if result.Outcome != "delivered" || result.EventType != "webhook.test" || result.ActivitySeq != nil || result.StatusCode == nil || *result.StatusCode != 204 {
		t.Fatalf("test result %s", tested.Body)
	}
	sent := h.sender.all()
	if len(sent) != 1 {
		t.Fatalf("%d requests, want the one test", len(sent))
	}
	at, mac := parseSignature(t, sent[0].Header.Get("X-SlashObserve-Signature"))
	if want := signature(created.Secret, at, sent[0].Body); !strings.HasSuffix(want, mac) {
		t.Fatalf("the created secret does not verify the test delivery signature")
	}
	if sent[0].Header.Get("X-SlashObserve-Event") != "webhook.test" || !strings.Contains(string(sent[0].Body), `"event":null`) {
		t.Fatalf("test request %+v %s", sent[0].Header, sent[0].Body)
	}
}

func TestHTTPTestWorksOnADisabledWebhookAndChangesNothingElse(t *testing.T) {
	h := newHTTPRig(t, newRig(t))
	created := decode[withSecret](t, h.call(t, http.MethodPost, "/api/v1/webhooks", createBody))
	h.call(t, http.MethodPatch, "/api/v1/webhooks/"+created.Webhook.ID, `{"enabled":false}`)
	before := h.row(t, created.Webhook.ID)
	h.sender.script(func(int, call) (egress.SendResult, error) { return egress.SendResult{Status: 500}, nil })

	rec := h.call(t, http.MethodPost, "/api/v1/webhooks/"+created.Webhook.ID+"/test", "")

	expect(t, rec, http.StatusOK, "")
	if !strings.Contains(rec.Body.String(), `"outcome":"failed"`) || !strings.Contains(rec.Body.String(), `"error_code":"status"`) {
		t.Fatalf("test result %s", rec.Body)
	}
	after := h.row(t, created.Webhook.ID)
	if after.ConsecutiveFailures != before.ConsecutiveFailures || after.CursorSeq != before.CursorSeq || after.DisabledReason != before.DisabledReason || !after.NextAttemptAt.Time.Equal(before.NextAttemptAt.Time) {
		t.Fatalf("a test changed the webhook: %+v to %+v", before, after)
	}
}

func TestHTTPWithoutASecretKeyCreateAnswersSecretKeyMissing(t *testing.T) {
	h := newHTTPRig(t, newRigWithKey(t, nil))

	expect(t, h.call(t, http.MethodPost, "/api/v1/webhooks", createBody), http.StatusConflict, "secret_key_missing")

	var count int
	if err := h.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM webhooks`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("%d webhooks stored, err %v", count, err)
	}
}

func TestHTTPAHostOffTheAllowlistIsRefusedOnCreateAndOnPatch(t *testing.T) {
	h := newHTTPRig(t, newRig(t))
	off := `{"name":"Elsewhere","url":"https://elsewhere.example.test/in","event_types":["finding.created"]}`

	expect(t, h.call(t, http.MethodPost, "/api/v1/webhooks", off), http.StatusForbidden, "outbound_domain_not_allowed")

	created := decode[withSecret](t, h.call(t, http.MethodPost, "/api/v1/webhooks", createBody))
	expect(t, h.call(t, http.MethodPatch, "/api/v1/webhooks/"+created.Webhook.ID, `{"url":"https://elsewhere.example.test/in"}`), http.StatusForbidden, "outbound_domain_not_allowed")
	allowed, err := h.allow.Allowed(t.Context())
	if err != nil || allowed.Allows("elsewhere.example.test") {
		t.Fatalf("a webhook changed the allowlist: %v", err)
	}
}

func TestHTTPValidationErrors(t *testing.T) {
	h := newHTTPRig(t, newRig(t))
	for name, body := range map[string]string{
		"missing fields":   `{"name":"x"}`,
		"http":             `{"name":"x","url":"http://hooks.example.test/in","event_types":["finding.created"]}`,
		"unknown type":     `{"name":"x","url":"` + receiverURL + `","event_types":["nope.nope"]}`,
		"empty types":      `{"name":"x","url":"` + receiverURL + `","event_types":[]}`,
		"duplicate types":  `{"name":"x","url":"` + receiverURL + `","event_types":["finding.created","finding.created"]}`,
		"unknown property": `{"name":"x","url":"` + receiverURL + `","event_types":["finding.created"],"secret":"mine"}`,
	} {
		rec := h.call(t, http.MethodPost, "/api/v1/webhooks", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: answered %d %s", name, rec.Code, rec.Body)
		}
	}
	expect(t, h.call(t, http.MethodPatch, "/api/v1/webhooks/not-a-uuid", `{}`), http.StatusBadRequest, "invalid_request")
	expect(t, h.call(t, http.MethodPatch, "/api/v1/webhooks/00000000-0000-4000-8000-00000000ffff", `{"enabled":true}`), http.StatusNotFound, "not_found")
	expect(t, h.call(t, http.MethodDelete, "/api/v1/webhooks/00000000-0000-4000-8000-00000000ffff", ""), http.StatusNotFound, "not_found")
	expect(t, h.call(t, http.MethodPost, "/api/v1/webhooks/00000000-0000-4000-8000-00000000ffff/secret", ""), http.StatusNotFound, "not_found")
	expect(t, h.call(t, http.MethodPost, "/api/v1/webhooks/00000000-0000-4000-8000-00000000ffff/test", ""), http.StatusNotFound, "not_found")
	expect(t, h.call(t, http.MethodGet, "/api/v1/webhooks/00000000-0000-4000-8000-00000000ffff/deliveries", ""), http.StatusNotFound, "not_found")
}

func TestHTTPTheTwentyFirstWebhookIsRefused(t *testing.T) {
	h := newHTTPRig(t, newRig(t))
	for range maxWebhooks {
		expect(t, h.call(t, http.MethodPost, "/api/v1/webhooks", createBody), http.StatusCreated, "")
	}

	expect(t, h.call(t, http.MethodPost, "/api/v1/webhooks", createBody), http.StatusUnprocessableEntity, "too_many_webhooks")

	listed := decode[struct {
		Webhooks []webhookBody `json:"webhooks"`
	}](t, h.call(t, http.MethodGet, "/api/v1/webhooks", ""))
	if len(listed.Webhooks) != maxWebhooks {
		t.Fatalf("%d webhooks listed", len(listed.Webhooks))
	}
}

func TestHTTPPatchByAnotherAdminMovesTheOwnerAndEnableResetsFailures(t *testing.T) {
	h := newHTTPRig(t, newRig(t))
	created := decode[withSecret](t, h.call(t, http.MethodPost, "/api/v1/webhooks", createBody))
	if created.Webhook.Owner.ID != adminOne.ID {
		t.Fatalf("owner %s", created.Webhook.Owner.ID)
	}
	base := "/api/v1/webhooks/" + created.Webhook.ID
	h.db.Exec(t, `UPDATE webhooks SET consecutive_failures = 50, disabled_at = now(), disabled_reason = 'failing' WHERE id = $1::uuid`, created.Webhook.ID)

	h.caller = adminTwo
	rec := h.call(t, http.MethodPatch, base, `{"name":"Renamed"}`)
	expect(t, rec, http.StatusOK, "")
	got := decode[webhookBody](t, rec)
	if got.Owner.ID != adminTwo.ID || got.Enabled || got.ConsecutiveFailures != 50 || got.DisabledReason == nil || *got.DisabledReason != "failing" {
		t.Fatalf("after a rename by another admin %s", rec.Body)
	}

	h.caller = adminOne
	rec = h.call(t, http.MethodPatch, base, `{"enabled":true}`)
	expect(t, rec, http.StatusOK, "")
	got = decode[webhookBody](t, rec)
	if !got.Enabled || got.ConsecutiveFailures != 0 || got.DisabledReason != nil || got.Owner.ID != adminOne.ID {
		t.Fatalf("after enabling %s", rec.Body)
	}
	rec = h.call(t, http.MethodPatch, base, `{"enabled":false}`)
	if got = decode[webhookBody](t, rec); got.Enabled || got.DisabledReason == nil || *got.DisabledReason != "manual" {
		t.Fatalf("after disabling %s", rec.Body)
	}
}

func TestHTTPChangingTheAddressOrTypesKeepsTheCursorAndResetsFailuresOnANewAddress(t *testing.T) {
	r := newRig(t)
	h := newHTTPRig(t, r)
	r.allow.set(t, receiverHost, "other.example.test")
	created := decode[withSecret](t, h.call(t, http.MethodPost, "/api/v1/webhooks", createBody))
	before := h.row(t, created.Webhook.ID)
	h.db.Exec(t, `UPDATE webhooks SET consecutive_failures = 7, next_attempt_at = now() + interval '1 hour' WHERE id = $1::uuid`, created.Webhook.ID)

	rec := h.call(t, http.MethodPatch, "/api/v1/webhooks/"+created.Webhook.ID, `{"url":"https://other.example.test/hook","event_types":["run.failed"]}`)

	expect(t, rec, http.StatusOK, "")
	after := h.row(t, created.Webhook.ID)
	if after.Url != "https://other.example.test/hook" || after.ConsecutiveFailures != 0 || after.CursorSeq != before.CursorSeq || after.CursorXid != before.CursorXid || len(after.EventTypes) != 1 {
		t.Fatalf("after the change %+v", after)
	}
	if time.Until(after.NextAttemptAt.Time) > time.Minute {
		t.Fatalf("next attempt still %v away after an address change", time.Until(after.NextAttemptAt.Time))
	}
}

func TestHTTPRotatingReplacesTheSecret(t *testing.T) {
	h := newHTTPRig(t, newRig(t))
	created := decode[withSecret](t, h.call(t, http.MethodPost, "/api/v1/webhooks", createBody))
	h.caller = adminTwo

	rec := h.call(t, http.MethodPost, "/api/v1/webhooks/"+created.Webhook.ID+"/secret", "")

	expect(t, rec, http.StatusOK, "")
	rotated := decode[withSecret](t, rec)
	if rotated.Secret == created.Secret || !strings.HasPrefix(rotated.Secret, "whsec_") || rotated.Webhook.Owner.ID != adminTwo.ID {
		t.Fatalf("rotated %s", rec.Body)
	}
	h.call(t, http.MethodPost, "/api/v1/webhooks/"+created.Webhook.ID+"/test", "")
	sent := h.sender.all()[0]
	at, mac := parseSignature(t, sent.Header.Get("X-SlashObserve-Signature"))
	if !strings.HasSuffix(signature(rotated.Secret, at, sent.Body), mac) || strings.HasSuffix(signature(created.Secret, at, sent.Body), mac) {
		t.Fatal("the new secret does not sign, or the old one still does")
	}
}

func TestHTTPDeletingStopsDeliveryAndRemovesAttempts(t *testing.T) {
	h := newHTTPRig(t, newRig(t))
	created := decode[withSecret](t, h.call(t, http.MethodPost, "/api/v1/webhooks", createBody))
	h.call(t, http.MethodPost, "/api/v1/webhooks/"+created.Webhook.ID+"/test", "")

	expect(t, h.call(t, http.MethodDelete, "/api/v1/webhooks/"+created.Webhook.ID, ""), http.StatusNoContent, "")

	var left int
	if err := h.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT (SELECT count(*) FROM webhooks) + (SELECT count(*) FROM webhook_deliveries)`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("%d rows left, err %v", left, err)
	}
}

func TestHTTPDeliveriesPageNewestFirstWithACursor(t *testing.T) {
	h := newHTTPRig(t, newRig(t))
	created := decode[withSecret](t, h.call(t, http.MethodPost, "/api/v1/webhooks", createBody))
	base := "/api/v1/webhooks/" + created.Webhook.ID
	for range 3 {
		h.call(t, http.MethodPost, base+"/test", "")
	}
	type page struct {
		Deliveries []struct {
			ID string `json:"id"`
		} `json:"deliveries"`
		NextCursor *string `json:"next_cursor"`
	}

	first := decode[page](t, h.call(t, http.MethodGet, base+"/deliveries?limit=2", ""))
	if len(first.Deliveries) != 2 || first.NextCursor == nil {
		t.Fatalf("first page %+v", first)
	}
	second := decode[page](t, h.call(t, http.MethodGet, base+"/deliveries?limit=2&cursor="+*first.NextCursor, ""))
	if len(second.Deliveries) != 1 || second.NextCursor != nil {
		t.Fatalf("second page %+v", second)
	}
	ids := []string{first.Deliveries[0].ID, first.Deliveries[1].ID, second.Deliveries[0].ID}
	for i := 1; i < len(ids); i++ {
		a, _ := strconv.Atoi(ids[i-1])
		b, _ := strconv.Atoi(ids[i])
		if a <= b {
			t.Fatalf("ids %v are not newest first", ids)
		}
	}
	expect(t, h.call(t, http.MethodGet, base+"/deliveries?cursor=garbage", ""), http.StatusBadRequest, "invalid_request")
	expect(t, h.call(t, http.MethodGet, base+"/deliveries?limit=101", ""), http.StatusBadRequest, "invalid_request")
}

func TestHTTPAnEnabledWebhookCreatedNowSendsOnlyLaterEvents(t *testing.T) {
	r := newRig(t)
	r.append(t, activity.TypeFindingCreated)
	h := newHTTPRig(t, r)
	expect(t, h.call(t, http.MethodPost, "/api/v1/webhooks", createBody), http.StatusCreated, "")
	later := r.append(t, activity.TypeFindingCreated)

	r.pass(t)

	if got, want := seqsSent(t, r.sender.all()), []int64{later}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("sent %v, want only %v", got, want)
	}
}
