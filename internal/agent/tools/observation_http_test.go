package tools

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/observations"
)

var scriptCaller = auth.Principal{Kind: auth.KindServiceAccount, ID: "88888888-8888-4888-8888-888888888888", Name: "ci", Role: auth.RoleEditor}

type blockCall struct {
	engine *gin.Engine
	who    auth.Principal
}

func (b *blockCall) send(method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	b.engine.ServeHTTP(rec, req)
	return rec
}

func newBlockCall(t *testing.T, r *observationRig) *blockCall {
	t.Helper()
	r.db.Exec(t, `INSERT INTO service_accounts (id, name) VALUES ($1::uuid, $2)`, scriptCaller.ID, scriptCaller.Name)
	gin.SetMode(gin.TestMode)
	b := &blockCall{engine: gin.New(), who: scriptCaller}
	handler := observations.NewHandler(r.service, slog.New(slog.DiscardHandler), observations.WithPanelChecker(r.registry))
	handler.Register(auth.NewRoutes(b.engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), b.who))
	})))
	return b
}

func restPanelBlock(panel string) string {
	return fmt.Sprintf(`{"kind":"panel","body":{"panel":%s}}`, panel)
}

func TestAKeyThatAddsAPanelOverRESTMeetsTheCheckAddBlockRuns(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)
	b := newBlockCall(t, r)
	blocks := "/api/v1/observations/" + id + "/blocks"

	rec := b.send(http.MethodPost, blocks, restPanelBlock(evidencePanel("Nothing", "empty")))
	var refused struct {
		Code   string `json:"code"`
		Issues []struct {
			Path string `json:"path"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &refused); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusUnprocessableEntity || refused.Code != codeInvalidSpec || len(refused.Issues) == 0 || !strings.HasPrefix(refused.Issues[0].Path, "/panels/0") {
		t.Fatalf("a panel with no data: %d %s", rec.Code, rec.Body)
	}
	if page := r.view(t, editorCaller, id); len(page.Blocks) != 0 {
		t.Fatalf("a refused panel left blocks behind: %#v", page.Blocks)
	}

	if rec := b.send(http.MethodPost, blocks, restPanelBlock(evidencePanel("Errors", "up"))); rec.Code != http.StatusCreated {
		t.Fatalf("a panel with data: %d %s", rec.Code, rec.Body)
	}
	if rec := b.send(http.MethodPost, blocks, `{"kind":"text","body":{"markdown":"note"}}`); rec.Code != http.StatusCreated {
		t.Fatalf("a text block: %d %s", rec.Code, rec.Body)
	}
}

func TestAKeyThatEditsAPanelOverRESTMeetsTheCheckUpdateBlockRuns(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)
	good := r.written(t, editorCaller, nameAddBlock, panelBlockInput(id, evidencePanel("Errors", "up")))
	b := newBlockCall(t, r)
	path := "/api/v1/blocks/" + good.BlockID

	rec := b.send(http.MethodPatch, path, fmt.Sprintf(`{"version":1,"body":{"panel":%s}}`, evidencePanel("Nothing", "empty")))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("an edit to a panel with no data: %d %s", rec.Code, rec.Body)
	}
	if rec := b.send(http.MethodPatch, path, fmt.Sprintf(`{"version":1,"body":{"panel":%s}}`, evidencePanel("Latency", "up"))); rec.Code != http.StatusOK {
		t.Fatalf("an edit to a panel with data: %d %s", rec.Code, rec.Body)
	}
}

func TestAPersonsSessionIsNotHeldToTheCheckWhenAddingAPanel(t *testing.T) {
	r := newObservationRig(t)
	id := r.start(t, editorCaller)
	b := newBlockCall(t, r)
	b.who = editorCaller
	rec := b.send(http.MethodPost, "/api/v1/observations/"+id+"/blocks", restPanelBlock(evidencePanel("Nothing", "empty")))
	if rec.Code != http.StatusCreated {
		t.Fatalf("a person's panel with no data: %d %s", rec.Code, rec.Body)
	}
}
