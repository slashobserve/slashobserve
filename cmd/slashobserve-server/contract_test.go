package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/db/dbtest"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	l, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func TestTheRealRouterMatchesTheAPIDocument(t *testing.T) {
	addr := freeLoopbackAddr(t)
	for name, value := range map[string]string{
		"SLASHOBSERVE_HTTP_ADDR":                  addr,
		"SLASHOBSERVE_DATABASE_URL":               dbtest.New(t).URL(),
		"SLASHOBSERVE_PUBLIC_URL":                 "http://" + addr,
		"SLASHOBSERVE_OIDC_ISSUER_URL":            "http://127.0.0.1:1/dex",
		"SLASHOBSERVE_OIDC_CLIENT_ID":             "slashobserve",
		"SLASHOBSERVE_OIDC_CLIENT_SECRET":         "unused",
		"SLASHOBSERVE_AUTH_ALLOWED_EMAIL_DOMAINS": "example.com",
		"SLASHOBSERVE_SECRET_KEY":                 "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"SLASHOBSERVE_API_CONTRACT_CHECK":         "true",
		"SLASHOBSERVE_LOG_LEVEL":                  "error",
	} {
		t.Setenv(name, value)
	}

	exited := make(chan error, 1)
	go func() { exited <- run() }()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(2 * time.Minute)
	for {
		select {
		case err := <-exited:
			t.Fatalf("the server stopped before it served: %v %v", err, exitAttrs(err))
		case <-deadline:
			t.Fatal("the server did not serve in time")
		case <-ticker.C:
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+"/healthz", nil)
			if err != nil {
				t.Fatal(err)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				continue
			}
			_ = res.Body.Close()
			if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
			if err := <-exited; err != nil {
				t.Fatal(fmt.Errorf("shutdown: %w", err))
			}
			return
		}
	}
}
