package loadgen

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func utc(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func formatAll(ts []time.Time) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.UTC().Format(time.RFC3339))
	}
	return out
}

func TestMarkerTimesAreTheSixHourSlotsOfTheLastDay(t *testing.T) {
	cases := []struct {
		now  string
		want []string
	}{
		{"2026-11-06T07:30:00Z", []string{"2026-11-05T06:00:00Z", "2026-11-05T12:00:00Z", "2026-11-05T18:00:00Z", "2026-11-06T00:00:00Z", "2026-11-06T06:00:00Z"}},
		{"2026-11-06T06:00:00Z", []string{"2026-11-05T06:00:00Z", "2026-11-05T12:00:00Z", "2026-11-05T18:00:00Z", "2026-11-06T00:00:00Z", "2026-11-06T06:00:00Z"}},
		{"2026-11-06T05:59:59Z", []string{"2026-11-05T06:00:00Z", "2026-11-05T12:00:00Z", "2026-11-05T18:00:00Z", "2026-11-06T00:00:00Z"}},
		{"2026-11-06T08:00:00Z", []string{"2026-11-05T12:00:00Z", "2026-11-05T18:00:00Z", "2026-11-06T00:00:00Z", "2026-11-06T06:00:00Z"}},
		{"2026-11-06T14:00:00+05:30", []string{"2026-11-05T12:00:00Z", "2026-11-05T18:00:00Z", "2026-11-06T00:00:00Z", "2026-11-06T06:00:00Z"}},
	}
	for _, tc := range cases {
		if got := formatAll(markerTimes(utc(tc.now))); !slices.Equal(got, tc.want) {
			t.Errorf("markerTimes(%s) = %v, want %v", tc.now, got, tc.want)
		}
	}
}

func TestNextMarkerCheckFollowsTheNextSlot(t *testing.T) {
	cases := map[string]string{
		"2026-11-06T07:30:00Z": "2026-11-06T12:00:30Z",
		"2026-11-06T00:00:00Z": "2026-11-06T06:00:30Z",
		"2026-11-06T23:59:59Z": "2026-11-07T00:00:30Z",
	}
	for now, want := range cases {
		if got := nextMarkerCheck(utc(now)).Format(time.RFC3339); got != want {
			t.Errorf("nextMarkerCheck(%s) = %s, want %s", now, got, want)
		}
	}
}

func TestMissingKeepsSlotsWithoutAMarker(t *testing.T) {
	slots := []time.Time{utc("2026-11-05T12:00:00Z"), utc("2026-11-05T18:00:00Z"), utc("2026-11-06T00:00:00Z")}
	cases := []struct {
		name     string
		existing []time.Time
		want     []string
	}{
		{"none posted", nil, []string{"2026-11-05T12:00:00Z", "2026-11-05T18:00:00Z", "2026-11-06T00:00:00Z"}},
		{"one posted in another zone", []time.Time{utc("2026-11-05T23:30:00+05:30")}, []string{"2026-11-05T12:00:00Z", "2026-11-06T00:00:00Z"}},
		{"a marker off the slot", []time.Time{utc("2026-11-05T18:00:01Z")}, []string{"2026-11-05T12:00:00Z", "2026-11-05T18:00:00Z", "2026-11-06T00:00:00Z"}},
		{"all posted", slots, nil},
	}
	for _, tc := range cases {
		if got := formatAll(missing(slots, tc.existing)); !slices.Equal(got, tc.want) {
			t.Errorf("%s: missing = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestMarkerTextAndServiceAreChosenFromTheSeed(t *testing.T) {
	pattern := regexp.MustCompile(`^Deploy (frontend|checkout|payments) v1\.\d{1,2}\.\d: [a-z ]+$`)
	slot := utc("2026-11-06T06:00:00Z")
	service := markerService(42, slot)
	if !slices.Contains(markerServices, service) || markerService(42, slot) != service {
		t.Fatalf("markerService = %q, want the same shop service on every call", service)
	}
	text := markerText(42, service, slot)
	if !pattern.MatchString(text) || !strings.Contains(text, "Deploy "+service+" ") || markerText(42, service, slot) != text {
		t.Fatalf("markerText = %q, want a stable deploy line for %s", text, service)
	}
	texts := map[string]bool{}
	for i := range 20 {
		at := slot.Add(time.Duration(i) * markerEvery)
		texts[markerText(42, markerService(42, at), at)] = true
	}
	if len(texts) < 10 {
		t.Fatalf("20 slots gave only %d distinct texts", len(texts))
	}
}

type fakeAnnotations struct {
	mu       sync.Mutex
	existing []string
	posted   []map[string]any
	auth     []string
	status   int
}

func (f *fakeAnnotations) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/annotations":
		if r.URL.Query().Get("tags") != "deploy" || r.URL.Query().Get("from") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var list []map[string]string
		for _, at := range f.existing {
			list = append(list, map[string]string{"time_from": at})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"annotations": list, "next_cursor": nil})
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/annotations":
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.posted = append(f.posted, body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func markerTestClient(t *testing.T, base, token string) markerClient {
	t.Helper()
	tokenFile := filepath.Join(t.TempDir(), "loadgen.key")
	if token != "" {
		if err := os.WriteFile(tokenFile, []byte(token+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return markerClient{base: base, tokenFile: tokenFile, seed: 42, http: &http.Client{Timeout: time.Second}}
}

func TestSyncPostsOnlyTheMissingMarkers(t *testing.T) {
	fake := &fakeAnnotations{existing: []string{"2026-11-05T18:00:00Z", "2026-11-06T00:00:00Z"}}
	server := httptest.NewServer(fake)
	defer server.Close()
	c := markerTestClient(t, server.URL, "test-token")

	posted, err := c.sync(t.Context(), utc("2026-11-06T07:30:00Z"))
	if err != nil || posted != 3 {
		t.Fatalf("sync = %d, %v, want 3 posted", posted, err)
	}
	var times []string
	for _, p := range fake.posted {
		times = append(times, p["time_from"].(string))
		tags := p["tags"].([]any)
		if len(tags) != 2 || tags[0] != "deploy" || !slices.Contains(markerServices, tags[1].(string)) {
			t.Errorf("tags = %v, want deploy and a shop service", tags)
		}
		if !strings.HasPrefix(p["text"].(string), "Deploy "+tags[1].(string)+" ") {
			t.Errorf("text %q does not name the tagged service", p["text"])
		}
		if _, global := p["dashboard_id"]; global {
			t.Errorf("marker %v has a dashboard_id, want a global marker", p)
		}
	}
	if want := []string{"2026-11-05T06:00:00Z", "2026-11-05T12:00:00Z", "2026-11-06T06:00:00Z"}; !slices.Equal(times, want) {
		t.Fatalf("posted %v, want %v", times, want)
	}
	for _, header := range fake.auth {
		if header != "Bearer test-token" {
			t.Fatalf("Authorization = %q, want the token from the file", header)
		}
	}
}

func TestSyncFailures(t *testing.T) {
	refused := &fakeAnnotations{status: http.StatusUnauthorized}
	refusing := httptest.NewServer(refused)
	defer refusing.Close()
	gone := httptest.NewServer(http.NotFoundHandler())
	goneURL := gone.URL
	gone.Close()

	cases := []struct {
		name     string
		client   markerClient
		notReady bool
		reason   string
	}{
		{"token file not written yet", markerTestClient(t, refusing.URL, ""), true, ""},
		{"server not listening", markerTestClient(t, goneURL, "test-token"), true, ""},
		{"server refuses the key", markerTestClient(t, refusing.URL, "test-token"), false, "unexpected_status"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.client.sync(t.Context(), utc("2026-11-06T07:30:00Z"))
			var failure *markerFailure
			switch {
			case tc.notReady && !errors.Is(err, errNotReady):
				t.Fatalf("got %v, want not ready", err)
			case !tc.notReady && (!errors.As(err, &failure) || failure.reason != tc.reason):
				t.Fatalf("got %v, want %s", err, tc.reason)
			}
			if err != nil && strings.Contains(err.Error(), "test-token") {
				t.Fatalf("the error holds the token: %v", err)
			}
		})
	}
}

func markerEnv(observeURL string) map[string]string {
	return map[string]string{"LOADGEN_OBSERVE_URL": observeURL, "LOADGEN_OBSERVE_TOKEN_FILE": "/demo-keys/loadgen.key"} //nolint:gosec // a file path, not a credential
}

func TestConfigReadsTheMarkerSettings(t *testing.T) {
	base := map[string]string{"LOADGEN_TARGET": "http://frontend-0:8080"}
	with := func(extra map[string]string) func(string) (string, bool) {
		return func(k string) (string, bool) {
			if v, ok := extra[k]; ok {
				return v, true
			}
			v, ok := base[k]
			return v, ok
		}
	}
	cfg, err := ConfigFromEnv(with(nil))
	if err != nil || cfg.ObserveURL != "" {
		t.Fatalf("without the URL got %+v, %v, want markers off", cfg, err)
	}
	cfg, err = ConfigFromEnv(with(markerEnv("")))
	if err != nil || cfg.ObserveURL != "" {
		t.Fatalf("with an empty URL got %+v, %v, want markers off", cfg, err)
	}
	cfg, err = ConfigFromEnv(with(markerEnv("http://server:8080/")))
	if err != nil || cfg.ObserveURL != "http://server:8080" || cfg.ObserveTokenFile != "/demo-keys/loadgen.key" {
		t.Fatalf("got %+v, %v, want the URL and token file", cfg, err)
	}
	for _, bad := range []map[string]string{
		{"LOADGEN_OBSERVE_URL": "http://server:8080"},
		{"LOADGEN_OBSERVE_URL": "server:8080", "LOADGEN_OBSERVE_TOKEN_FILE": "/k"},
		{"LOADGEN_OBSERVE_URL": "ftp://server", "LOADGEN_OBSERVE_TOKEN_FILE": "/k"},
	} {
		if _, err := ConfigFromEnv(with(bad)); err == nil {
			t.Errorf("%v: got no error, want one", bad)
		}
	}
}
