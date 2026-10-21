package imports_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/imports"
)

const principalHeader = "X-Test-Principal"

type typeGateStub struct {
	promStub
	armed   *atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func (g typeGateStub) Type() string {
	if g.armed.Load() {
		g.entered <- struct{}{}
		<-g.release
	}
	return g.promStub.Type()
}

func newRouter(svc *imports.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	imports.NewHandler(svc).Register(r.Group("/api/v1", func(c *gin.Context) {
		who := ada
		if n, err := strconv.Atoi(c.GetHeader(principalHeader)); err == nil {
			who = principal(n)
		}
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), who))
	}))
	return r
}

func post(r http.Handler, path string, principalNumber int, body any) *httptest.ResponseRecorder {
	raw, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	if principalNumber >= 0 {
		req.Header.Set(principalHeader, strconv.Itoa(principalNumber))
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("not an error body: %s", rec.Body)
	}
	return body.Code
}

func TestTheRoutesRefuseABadMappingWith400(t *testing.T) {
	r := newRig(t)
	router := newRouter(r.imports)
	tooMany := map[string]string{}
	for i := range imports.MaxMappingEntries + 1 {
		tooMany["key"+strconv.Itoa(i)] = "prom"
	}
	for name, mapping := range map[string]map[string]string{
		"51 entries":       tooMany,
		"a 201 byte key":   {strings.Repeat("k", 201): "prom"},
		"a 201 byte value": {"${DS}": strings.Repeat("v", 201)},
		"an empty value":   {"${DS}": ""},
		"an empty key":     {"": "prom"},
	} {
		for _, path := range []string{"/api/v1/imports/preview", "/api/v1/imports"} {
			rec := post(router, path, -1, map[string]any{"json": string(grafanaJSON("Shop", "prom", 1)), "mapping": mapping})
			if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_request" {
				t.Errorf("%s %s: %d %s", name, path, rec.Code, rec.Body)
			}
		}
	}
	assertNothingSaved(t, r)
}

func TestThePreviewAcceptsAMappingAtTheLimits(t *testing.T) {
	router := newRouter(newRig(t).imports)
	atLimit := map[string]string{strings.Repeat("k", 200): strings.Repeat("v", 200)}
	for i := range imports.MaxMappingEntries - 1 {
		atLimit["key"+strconv.Itoa(i)] = "prom"
	}
	rec := post(router, "/api/v1/imports/preview", -1, map[string]any{"json": string(grafanaJSON("Shop", "prom", 1)), "mapping": atLimit})
	if rec.Code == http.StatusBadRequest {
		t.Fatalf("a mapping at the limits: %d %s", rec.Code, rec.Body)
	}
}

func TestPreviewSlotsRefuseWith429AndFree(t *testing.T) {
	gate := typeGateStub{armed: new(atomic.Bool), entered: make(chan struct{}, 64), release: make(chan struct{})}
	router := newRouter(newRigWith(t, gate).imports)
	gate.armed.Store(true)
	body := map[string]any{"json": string(grafanaJSON("Shop", "prom", 1))}

	start := func(n int) <-chan int {
		done := make(chan int, 1)
		go func() { done <- post(router, "/api/v1/imports/preview", n, body).Code }()
		select {
		case <-gate.entered:
		case <-time.After(10 * time.Second):
			t.Fatalf("preview for principal %d never started converting", n)
		}
		return done
	}
	refused := func(n int) {
		t.Helper()
		answered := make(chan *httptest.ResponseRecorder, 1)
		go func() { answered <- post(router, "/api/v1/imports/preview", n, body) }()
		select {
		case rec := <-answered:
			if rec.Code != http.StatusTooManyRequests || errorCode(t, rec) != "too_many_imports" {
				t.Fatalf("preview for principal %d: %d %s, want 429 too_many_imports", n, rec.Code, rec.Body)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("preview for principal %d was not refused", n)
		}
	}

	running := []<-chan int{start(0), start(0)}
	refused(0)
	running = append(running, start(1), start(2))
	refused(3)
	refused(0)

	close(gate.release)
	for i, done := range running {
		if code := <-done; code != http.StatusOK {
			t.Errorf("preview %d: %d", i, code)
		}
	}
	for _, n := range []int{0, 3} {
		if rec := post(router, "/api/v1/imports/preview", n, body); rec.Code != http.StatusOK {
			t.Errorf("preview for principal %d after the slots freed: %d %s", n, rec.Code, rec.Body)
		}
	}
}

func TestPreviewsDoNotTakeImportSlots(t *testing.T) {
	gate := typeGateStub{armed: new(atomic.Bool), entered: make(chan struct{}, 64), release: make(chan struct{})}
	r := newRigWith(t, gate)
	router := newRouter(r.imports)
	gate.armed.Store(true)
	done := make(chan int, 1)
	go func() {
		done <- post(router, "/api/v1/imports/preview", 0, map[string]any{"json": string(grafanaJSON("Shop", "prom", 1))}).Code
	}()
	<-gate.entered
	gate.armed.Store(false)
	rec := post(router, "/api/v1/imports", 0, map[string]any{"json": string(grafanaJSON("Shop", "prom", 1))})
	if rec.Code != http.StatusCreated {
		t.Fatalf("import while the same principal previews: %d %s", rec.Code, rec.Body)
	}
	close(gate.release)
	if code := <-done; code != http.StatusOK {
		t.Fatalf("preview: %d", code)
	}
}
