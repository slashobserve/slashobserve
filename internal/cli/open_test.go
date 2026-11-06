package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fakeOpener(t *testing.T, exitCode int) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake opener is a shell script")
	}
	dir := t.TempDir()
	record := filepath.Join(dir, "opened")
	name, _ := opener(runtime.GOOS, "")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\nexit %d\n", record, exitCode)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil { //nolint:gosec // the fake opener must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return record
}

func opened(t *testing.T, record string) string {
	t.Helper()
	data, err := os.ReadFile(record) //nolint:gosec // a file in the test's temp dir
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

func TestOpenerPerPlatform(t *testing.T) {
	cases := []struct {
		goos, name string
		args       []string
	}{
		{"darwin", "open", []string{"https://o/d/1"}},
		{"linux", "xdg-open", []string{"https://o/d/1"}},
		{"freebsd", "xdg-open", []string{"https://o/d/1"}},
		{"windows", "rundll32", []string{"url.dll,FileProtocolHandler", "https://o/d/1"}},
	}
	for _, tc := range cases {
		name, args := opener(tc.goos, "https://o/d/1")
		if name != tc.name || strings.Join(args, " ") != strings.Join(tc.args, " ") {
			t.Errorf("%s: %s %v, want %s %v", tc.goos, name, args, tc.name, tc.args)
		}
	}
}

func TestOpenRunsThePlatformOpener(t *testing.T) {
	record := fakeOpener(t, 0)
	if err := openInBrowser(runtime.GOOS, "https://o.example.com/d/1/shop"); err != nil {
		t.Fatal(err)
	}
	if got := opened(t, record); got != "https://o.example.com/d/1/shop" {
		t.Fatalf("opened %q", got)
	}
}

func TestOpenRefusesAddressesThatAreNotWebPages(t *testing.T) {
	record := fakeOpener(t, 0)
	for _, address := range []string{"file:///etc/passwd", "javascript:alert(1)", "/d/1", "https:///d/1", "-a Calculator"} {
		if err := openInBrowser(runtime.GOOS, address); err == nil {
			t.Errorf("%q was opened", address)
		}
	}
	if got := opened(t, record); got != "" {
		t.Fatalf("the opener ran with %q", got)
	}
}

func TestOpenNeverFailsTheCommand(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T)
		want  string
	}{
		{"missing opener", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("PATH lookup differs on windows")
			}
			t.Setenv("PATH", t.TempDir())
		}, "was not found"},
		{"failing opener", func(t *testing.T) { fakeOpener(t, 3) }, "failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)
			var stdout, stderr bytes.Buffer
			a := &app{open: true}
			a.openLink(&output{out: &stdout, err: &stderr}, "https://o.example.com/d/1")
			if !strings.Contains(stderr.String(), "could not open the browser") || !strings.Contains(stderr.String(), tc.want) || stdout.Len() != 0 {
				t.Fatalf("stdout %q, stderr %q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestOpenAfterWaitOpensTheTitledAddress(t *testing.T) {
	record := fakeOpener(t, 0)
	const id = "bbbbbbbb-2222-3333-4444-555555555555"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/observe/" + id:
			_, _ = fmt.Fprintf(w, `{"request":{"id":"%s","status":"succeeded","dashboard_id":"%s","result_version":2,"url":"%s/d/%s","steps":[]}}`, id, id, server.URL, id)
		case "/api/v1/dashboards/" + id:
			_, _ = fmt.Fprintf(w, `{"dashboard":{"id":"%s","title":"Checkout health"},"version":{"version":2,"spec":{"panels":[]}}}`, id)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"error":"not found","code":"not_found"}`)
		}
	}))
	t.Cleanup(server.Close)
	var stdout, stderr bytes.Buffer
	code := Main([]string{"--server", server.URL, "--token", "k", "status", id, "--wait", "--open"}, IO{In: strings.NewReader(""), Out: &stdout, Err: &stderr}, "test")
	want := server.URL + "/d/" + id + "/checkout-health"
	if code != 0 || strings.TrimSpace(stdout.String()) != want || !strings.Contains(stderr.String(), "Saved version 2") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	if got := opened(t, record); got != want {
		t.Fatalf("opened %q, want %q", got, want)
	}
}

func TestOpenCommandOpensTheServerOrADashboard(t *testing.T) {
	const server = "https://observe.example.com"
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"server", nil, server},
		{"dashboard id", []string{testDashboard}, server + "/d/" + testDashboard},
		{"grafana uid", []string{"checkout-overview_2"}, server + "/d/checkout-overview_2"},
		{"address on the server", []string{"https://observe.example.com:443/f/abc?x=1"}, server + "/f/abc?x=1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			record := fakeOpener(t, 0)
			stdout, stderr, code := runCLI(append([]string{"--server", server, "open"}, tc.args...)...)
			if code != 0 || strings.TrimSpace(stdout) != tc.want || stderr != "" {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
			}
			if got := opened(t, record); got != tc.want {
				t.Fatalf("opened %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOpenCommandPrintsJSON(t *testing.T) {
	fakeOpener(t, 0)
	stdout, _, code := runCLI("--server", "https://observe.example.com", "--json", "open", testDashboard)
	if code != 0 || strings.TrimSpace(stdout) != `{"url":"https://observe.example.com/d/`+testDashboard+`"}` {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestOpenCommandRefusesAddressesOffTheServer(t *testing.T) {
	for _, target := range []string{"https://evil.example.com/d/1", "http://observe.example.com/d/1", "javascript:alert(1)", "not a uid"} {
		t.Run(target, func(t *testing.T) {
			record := fakeOpener(t, 0)
			_, stderr, code := runCLI("--server", "https://observe.example.com", "open", target)
			if code != 2 || stderr == "" {
				t.Fatalf("exit %d, stderr %q", code, stderr)
			}
			if got := opened(t, record); got != "" {
				t.Fatalf("opened %q", got)
			}
		})
	}
	_, stderr, _ := runCLI("--server", "https://observe.example.com", "open", "https://evil.example.com/d/1")
	if !strings.Contains(stderr, "https://evil.example.com/d/1 is not on https://observe.example.com") {
		t.Fatalf("stderr %q", stderr)
	}
}

func TestOpenCommandFailsWhenTheBrowserDoesNotOpen(t *testing.T) {
	fakeOpener(t, 3)
	stdout, stderr, code := runCLI("--server", "https://observe.example.com", "open")
	if code != 1 || strings.TrimSpace(stdout) != "https://observe.example.com" || !strings.Contains(stderr, "could not open the browser") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}
