package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func servicesServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/datasources":
			_, _ = io.WriteString(w, `{"datasources":[{"uid":"prom","type":"prometheus","name":"Prom","healthy":true},{"uid":"lk","type":"loki","name":"Loki","healthy":true},{"uid":"tp","type":"tempo","name":"Tempo","healthy":true},{"uid":"ch","type":"clickhouse","name":"CH","healthy":true}]}`)
		case "/api/v1/datasources/prom/services":
			_, _ = io.WriteString(w, `{"datasource":"prom","signal":"metrics","field":"job","window_seconds":3600,"services":["checkout-job","shop"],"truncated":false}`)
		case "/api/v1/datasources/lk/services":
			_, _ = io.WriteString(w, `{"datasource":"lk","signal":"logs","field":"service_name","window_seconds":3600,"services":["checkout","shop"],"truncated":false}`)
		case "/api/v1/datasources/tp/services":
			w.WriteHeader(http.StatusGatewayTimeout)
			_, _ = io.WriteString(w, `{"error":"the query timed out","code":"query_timeout"}`)
		case "/api/v1/datasources/ch/services":
			if r.URL.Query().Get("signal") == "traces" {
				_, _ = io.WriteString(w, `{"datasource":"ch","signal":"traces","field":"ServiceName","window_seconds":3600,"services":["frontend"],"truncated":false}`)
				return
			}
			_, _ = io.WriteString(w, `{"datasource":"ch","signal":"logs","field":"ServiceName","window_seconds":3600,"services":["Checkout-API","a1","a2","a3","a4","a5","a6","a7","a8","a9"],"truncated":false}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"datasource not found","code":"not_found"}`)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestServicesOfOneDatasourcePrintsNamesAndWhereTheyLive(t *testing.T) {
	server, seen := recordingServer(t, `{"datasource":"lk","signal":"logs","field":"service_name","window_seconds":900,"services":["checkout","shop"],"truncated":true}`)
	stdout, stderr, code := runWith(server, "datasources", "services", "lk", "--window", "15m", "--service", "CHECK")
	if code != 0 || seen.path != "/api/v1/datasources/lk/services" || seen.query != "window=15m" {
		t.Fatalf("exit %d, call %+v, stderr %q", code, seen, stderr)
	}
	if strings.TrimSpace(stdout) != "checkout" || !strings.Contains(stderr, "cut at 2 services") || !strings.Contains(stderr, "service_name over the last 15m") {
		t.Fatalf("stdout %q, stderr %q", stdout, stderr)
	}
}

func TestServicesWithoutAUIDAsksEverySourceThatHoldsServices(t *testing.T) {
	stdout, stderr, code := runWith(servicesServer(t), "datasources", "services")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for _, want := range []string{"lk", "service_name", "checkout, shop", "prom", "metrics", "job", "checkout-job, shop", "tp", "unavailable", "ch", "traces", "frontend", "+2 more"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}

func TestServicesSearchKeepsSourcesHoldingTheServiceAndTheOnesThatCouldNotAnswer(t *testing.T) {
	stdout, _, code := runWith(servicesServer(t), "datasources", "services", "--service", "checkout", "--signal", "logs")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "lk") || !strings.Contains(stdout, "Checkout-API") || strings.Contains(stdout, "frontend") {
		t.Fatalf("stdout:\n%s", stdout)
	}
	stdout, _, _ = runWith(servicesServer(t), "datasources", "services", "--service", "nothing-like-this", "--signal", "logs")
	if strings.Contains(stdout, "lk") {
		t.Fatalf("stdout:\n%s", stdout)
	}
}

func TestServicesJSONListsSources(t *testing.T) {
	stdout, _, code := runWith(servicesServer(t), "--json", "datasources", "services", "--signal", "traces")
	if code != 0 || !strings.Contains(stdout, `"sources"`) || !strings.Contains(stdout, `"frontend"`) || !strings.Contains(stdout, `"unavailable"`) {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestServicesRefusesTwoUIDs(t *testing.T) {
	_, _, stderr, code := runAgainst(t, http.StatusOK, `{}`, "datasources", "services", "a", "b")
	if code != 2 || stderr == "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}
