package grafanaconn_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/egress/allowlist"
	"github.com/slashobserve/slashobserve/internal/grafanaconn"
)

func TestConnectionStoresTheTokenEncryptedAndAllowsTheHost(t *testing.T) {
	r := newRig(t)
	conn, err := r.svc.Create(t.Context(), r.admin, grafanaconn.Input{Name: " Production ", URL: "https://Grafana.Example.com/", Token: grafanaToken})
	if err != nil {
		t.Fatal(err)
	}
	if conn.Name != "Production" || conn.URL != grafanaURL || conn.CreatedBy != r.admin.ID || conn.LastTest != nil || conn.LastSync != nil {
		t.Fatalf("connection %+v", conn)
	}
	var sealed []byte
	if err := r.db.SQLDB(t).QueryRowContext(t.Context(), "SELECT token_ciphertext FROM grafana_connections WHERE id = $1", conn.ID).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if len(sealed) <= len(grafanaToken) || bytes.Contains(sealed, []byte(grafanaToken)) || bytes.Contains(sealed, []byte("glsa_")) {
		t.Fatalf("the stored bytes hold the token: %q", sealed)
	}
	list, err := r.domains.Allowed(t.Context())
	if err != nil || !list.Allows(grafanaHost) {
		t.Fatalf("allowlist %v, err %v", list, err)
	}

	second, err := r.svc.Create(t.Context(), r.admin, grafanaconn.Input{Name: "Copy", URL: grafanaURL + "/grafana", Token: grafanaToken})
	if err != nil || second.URL != grafanaURL+"/grafana" {
		t.Fatalf("a second connection to an allowed host: %+v, err %v", second, err)
	}
	all, err := r.svc.List(t.Context())
	if err != nil || len(all) != 2 || all[0].ID != conn.ID || all[1].ID != second.ID {
		t.Fatalf("list %+v, err %v", all, err)
	}
}

func TestConnectionRefusesBadInput(t *testing.T) {
	r := newRig(t)
	cases := map[string]grafanaconn.Input{
		"empty name":       {Name: " ", URL: grafanaURL, Token: grafanaToken},
		"long name":        {Name: strings.Repeat("n", 101), URL: grafanaURL, Token: grafanaToken},
		"ftp url":          {Name: "x", URL: "ftp://" + grafanaHost, Token: grafanaToken},
		"user info":        {Name: "x", URL: "https://admin:pw@" + grafanaHost, Token: grafanaToken},
		"query":            {Name: "x", URL: grafanaURL + "/?orgId=1", Token: grafanaToken},
		"fragment":         {Name: "x", URL: grafanaURL + "/#top", Token: grafanaToken},
		"ip literal":       {Name: "x", URL: "http://10.0.0.5:3000", Token: grafanaToken},
		"single label":     {Name: "x", URL: "http://grafana:3000", Token: grafanaToken},
		"port zero":        {Name: "x", URL: grafanaURL + ":0", Token: grafanaToken},
		"port too high":    {Name: "x", URL: grafanaURL + ":65536", Token: grafanaToken},
		"empty token":      {Name: "x", URL: grafanaURL, Token: ""},
		"token with space": {Name: "x", URL: grafanaURL, Token: "glsa one"},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := r.svc.Create(t.Context(), r.admin, in)
			var invalid *grafanaconn.InvalidError
			if !errors.As(err, &invalid) {
				t.Fatalf("error %v, want InvalidError", err)
			}
		})
	}
	if n := r.count(t, "SELECT count(*) FROM grafana_connections"); n != 0 {
		t.Fatalf("%d connections stored", n)
	}
	if n := r.count(t, "SELECT count(*) FROM outbound_domains"); n != 0 {
		t.Fatalf("%d domains allowed by refused input", n)
	}
}

func TestConnectionAcceptsPlainHTTPOnAnyPort(t *testing.T) {
	r := newRig(t)
	conn, err := r.svc.Create(t.Context(), r.admin, grafanaconn.Input{Name: "Local", URL: "http://grafana.internal.example.com:3000", Token: grafanaToken})
	if err != nil || conn.URL != "http://grafana.internal.example.com:3000" {
		t.Fatalf("connection %+v, err %v", conn, err)
	}
}

func TestSecretKeyIsRequiredToStoreAToken(t *testing.T) {
	r := newRigWith(t, rigOptions{})
	_, err := r.svc.Create(t.Context(), r.admin, grafanaconn.Input{Name: "Production", URL: grafanaURL, Token: grafanaToken})
	if !errors.Is(err, grafanaconn.ErrNoSecretKey) || !strings.Contains(err.Error(), "SLASHOBSERVE_SECRET_KEY") {
		t.Fatalf("error %v, want ErrNoSecretKey naming the variable", err)
	}
	if n := r.count(t, "SELECT count(*) FROM outbound_domains"); n != 0 {
		t.Fatalf("%d domains allowed without a key", n)
	}
}

func TestSecretFromAnotherKeyCannotBeRead(t *testing.T) {
	r := newRig(t)
	conn := r.connect(t)
	other := grafanaconn.New(t.Context(), grafanaconn.Deps{
		DB: r.db.DB, Egress: egress.New(r.domains, egress.Options{UserAgent: "TestAgent/1"}), Domains: r.domains,
		SecretKey: []byte("another key of thirty-two bytes!"), Logger: discard(),
	})
	if _, err := other.Test(t.Context(), uuidOf(t, conn.ID)); !errors.Is(err, grafanaconn.ErrUnreadableToken) {
		t.Fatalf("error %v, want ErrUnreadableToken", err)
	}
	if got := r.grafana.authHeaders(); len(got) != 0 {
		t.Fatalf("grafana saw %d requests", len(got))
	}
}

func TestSecretIsBoundToItsConnection(t *testing.T) {
	r := newRig(t)
	first := r.connect(t)
	second, err := r.svc.Create(t.Context(), r.admin, grafanaconn.Input{Name: "Other", URL: grafanaURL, Token: "glsa_other"})
	if err != nil {
		t.Fatal(err)
	}
	r.db.Exec(t, "UPDATE grafana_connections SET token_ciphertext = (SELECT token_ciphertext FROM grafana_connections WHERE id = $1) WHERE id = $2", first.ID, second.ID)
	if _, err := r.svc.Test(t.Context(), uuidOf(t, second.ID)); !errors.Is(err, grafanaconn.ErrUnreadableToken) {
		t.Fatalf("error %v, want ErrUnreadableToken for a token moved between connections", err)
	}
}

func TestConnectionTestStoresTheCounts(t *testing.T) {
	r := newRig(t)
	r.grafana.folders = []fakeFolder{{uid: "f-plat", title: "Platform"}, {uid: "f-k8s", title: "Kubernetes", parent: "f-plat"}, {uid: "f-apps", title: "Apps"}}
	r.grafana.add(&fakeDashboard{uid: "d1", title: "One", folder: "f-apps", model: simpleModel("graf-prom", "prometheus"), version: 1})
	r.grafana.add(&fakeDashboard{uid: "d2", title: "Two", model: simpleModel("graf-prom", "prometheus"), version: 1})
	conn := r.connect(t)

	tested, err := r.svc.Test(t.Context(), uuidOf(t, conn.ID))
	if err != nil {
		t.Fatal(err)
	}
	if tested.LastTest == nil || !tested.LastTest.OK || tested.LastTest.Message != "Connected: 2 top-level folders, 2 dashboards." {
		t.Fatalf("last test %+v", tested.LastTest)
	}
	for _, header := range r.grafana.authHeaders() {
		if header != "Bearer "+grafanaToken {
			t.Fatalf("grafana saw Authorization %q", header)
		}
	}
	again, err := r.svc.Get(t.Context(), uuidOf(t, conn.ID))
	if err != nil || again.LastTest == nil || again.LastTest.Message != tested.LastTest.Message {
		t.Fatalf("stored test %+v, err %v", again.LastTest, err)
	}
}

func TestConnectionTestRecordsARefusedToken(t *testing.T) {
	r := newRig(t)
	r.grafana.token = "glsa_rotated"
	conn := r.connect(t)
	tested, err := r.svc.Test(t.Context(), uuidOf(t, conn.ID))
	if err != nil {
		t.Fatal(err)
	}
	if tested.LastTest == nil || tested.LastTest.OK || tested.LastTest.Message != "Grafana refused the token (status 401)." {
		t.Fatalf("last test %+v", tested.LastTest)
	}
	if strings.Contains(tested.LastTest.Message, grafanaToken) {
		t.Fatal("the message holds the token")
	}
}

func TestConnectionUpdateNeedsTheTokenForANewURL(t *testing.T) {
	r := newRig(t)
	conn := r.connect(t)
	if _, err := r.svc.Test(t.Context(), uuidOf(t, conn.ID)); err != nil {
		t.Fatal(err)
	}
	newURL := "https://grafana2.example.com"
	if _, err := r.svc.Update(t.Context(), r.admin, uuidOf(t, conn.ID), grafanaconn.Patch{URL: &newURL}); !errors.Is(err, grafanaconn.ErrTokenRequired) {
		t.Fatalf("error %v, want ErrTokenRequired", err)
	}
	name := "Renamed"
	renamed, err := r.svc.Update(t.Context(), r.admin, uuidOf(t, conn.ID), grafanaconn.Patch{Name: &name})
	if err != nil || renamed.Name != "Renamed" || renamed.LastTest == nil {
		t.Fatalf("renamed %+v, err %v; want the test kept", renamed, err)
	}
	token := "glsa_new"
	moved, err := r.svc.Update(t.Context(), r.admin, uuidOf(t, conn.ID), grafanaconn.Patch{URL: &newURL, Token: &token})
	if err != nil || moved.URL != newURL || moved.LastTest != nil {
		t.Fatalf("moved %+v, err %v; want the new url and no test", moved, err)
	}
	list, err := r.domains.Allowed(t.Context())
	if err != nil || !list.Allows("grafana2.example.com") {
		t.Fatalf("allowlist %v, err %v", list, err)
	}
}

func TestConnectionDeleteForgetsIt(t *testing.T) {
	r := newRig(t)
	conn := r.connect(t)
	if err := r.svc.Delete(t.Context(), r.admin, uuidOf(t, conn.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Get(t.Context(), uuidOf(t, conn.ID)); !errors.Is(err, grafanaconn.ErrNotFound) {
		t.Fatalf("get after delete: %v", err)
	}
	if err := r.svc.Delete(t.Context(), r.admin, uuidOf(t, conn.ID)); !errors.Is(err, grafanaconn.ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestConnectionToAHostTheLockedListRefuses(t *testing.T) {
	r := newRigWith(t, rigOptions{secretKey: secretKey, lock: allowlist.Lock{Locked: true}})
	_, err := r.svc.Create(t.Context(), r.admin, grafanaconn.Input{Name: "Production", URL: grafanaURL, Token: grafanaToken})
	var hostErr *egress.HostError
	if !errors.Is(err, egress.ErrNotAllowed) || !errors.As(err, &hostErr) || hostErr.Host != grafanaHost {
		t.Fatalf("error %v, want the host not allowed", err)
	}
}
