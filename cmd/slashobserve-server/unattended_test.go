package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestAServerWithNoIdentityProviderStartsAndTakesItsFileKey(t *testing.T) {
	dir := t.TempDir()
	key := "sok_" + strings.Repeat("A", 43)
	keyFile := filepath.Join(dir, "admin.key")
	accounts := filepath.Join(dir, "service-accounts.yaml")
	datasources := filepath.Join(dir, "datasources.yaml")
	for path, content := range map[string]string{
		keyFile:     key + "\n",
		accounts:    "serviceAccounts:\n  - name: bench-admin\n    role: admin\n    keyFile: " + keyFile + "\n",
		datasources: "datasources:\n  - uid: prom\n    type: prometheus\n    name: Prometheus\n    prometheus:\n      url: http://127.0.0.1:1\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	addr, exited := startServer(t, dbtest.New(t).URL(), map[string]string{
		"SLASHOBSERVE_OIDC_ISSUER_URL":            "",
		"SLASHOBSERVE_OIDC_CLIENT_ID":             "",
		"SLASHOBSERVE_OIDC_CLIENT_SECRET":         "",
		"SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS": "",
		"SLASHOBSERVE_SERVICE_ACCOUNTS_FILE":      accounts,
		"SLASHOBSERVE_DATASOURCES_FILE":           datasources,
	})
	base := "http://" + addr
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	type reply struct {
		status   int
		location string
		body     []byte
	}
	get := func(path, token string) (reply, bool) {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := noRedirect.Do(req)
		if err != nil {
			return reply{}, false
		}
		defer func() { _ = res.Body.Close() }()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return reply{}, false
		}
		return reply{status: res.StatusCode, location: res.Header.Get("Location"), body: body}, true
	}

	deadline := time.After(2 * time.Minute)
	for ready := false; !ready; {
		select {
		case err := <-exited:
			t.Fatalf("the server stopped before it was ready: %v %v", err, exitAttrs(err))
		case <-deadline:
			t.Fatal("the server was not ready in time")
		case <-time.After(100 * time.Millisecond):
			res, ok := get("/readyz", "")
			ready = ok && res.status == http.StatusOK
		}
	}
	if res, _ := get("/readyz", ""); !strings.Contains(string(res.body), `"phase":"ready"`) {
		t.Fatalf("/readyz of a serving server = %s, want phase ready", res.body)
	}

	var me struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
		Role string `json:"role"`
	}
	res, _ := get("/api/v1/me", key)
	if res.status != http.StatusOK {
		t.Fatalf("GET /api/v1/me with the file key = %d, want 200", res.status)
	}
	if err := json.Unmarshal(res.body, &me); err != nil {
		t.Fatal(err)
	}
	if me.Kind != "service_account" || me.Name != "bench-admin" || me.Role != "admin" {
		t.Fatalf("the file key acts as %+v, want the bench-admin service account with the admin role", me)
	}

	res, _ = get("/api/v1/auth/login", "")
	if res.status != http.StatusFound || res.location != "/login?error=sign_in_off" {
		t.Fatalf("browser sign-in = %d to %q, want a redirect to /login?error=sign_in_off", res.status, res.location)
	}

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := <-exited; err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}
