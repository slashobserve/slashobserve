package news

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/egress"
	"github.com/slashobserve/slashobserve/internal/logtest"
)

func init() { gin.SetMode(gin.TestMode) }

type fakeFetcher struct {
	resp egress.Response
	err  error
	got  egress.Request
}

func (f *fakeFetcher) Fetch(_ context.Context, req egress.Request) (egress.Response, error) {
	f.got = req
	return f.resp, f.err
}

type rig struct {
	fetcher *fakeFetcher
	logs    *logtest.Capture
	engine  *gin.Engine
}

func newRig(t *testing.T, fetcher *fakeFetcher) *rig {
	t.Helper()
	logger, logs := logtest.New()
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		if kind := c.GetHeader("X-Test-Kind"); kind != "" {
			p := auth.Principal{Kind: auth.PrincipalKind(kind), ID: "caller-1", Role: auth.RoleAdmin}
			c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), p))
		}
	})
	NewHandler(fetcher, logger).Register(auth.NewRoutes(api))
	return &rig{fetcher: fetcher, logs: logs, engine: engine}
}

func (r *rig) get(rawURL, kind string) *httptest.ResponseRecorder {
	target := "/api/v1/news"
	if rawURL != "" {
		target += "?url=" + url.QueryEscape(rawURL)
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if kind != "" {
		req.Header.Set("X-Test-Kind", kind)
	}
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	return rec
}

type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	return body
}

const privateFeedURL = "https://feeds.example.com/secret-path/feed.xml?token=abc123"

const sampleFeed = `<rss><channel><title>Blog</title>
<item><title>Relative</title><link>/posts/1</link><pubDate>Tue, 28 Oct 2025 09:30:00 +0200</pubDate><description>First post</description></item>
<item><title>Script link</title><link>javascript:alert(1)</link><description>Second post</description></item>
</channel></rss>`

func TestGetReturnsSanitizedItems(t *testing.T) {
	fetchedAt := time.Date(2025, 10, 29, 12, 0, 0, 0, time.FixedZone("x", 3600))
	fetcher := &fakeFetcher{resp: egress.Response{
		Body:      []byte(sampleFeed),
		FinalURL:  "https://feeds.example.com/final/feed.xml",
		FetchedAt: fetchedAt,
	}}
	rec := newRig(t, fetcher).get(privateFeedURL, "user")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	want := `{"title":"Blog","items":[` +
		`{"title":"Relative","link":"https://feeds.example.com/posts/1","published":"2025-10-28T07:30:00Z","summary":"First post"},` +
		`{"title":"Script link","link":null,"published":null,"summary":"Second post"}` +
		`],"fetched_at":"2025-10-29T11:00:00Z"}`
	if rec.Body.String() != want {
		t.Errorf("body = %s\nwant   %s", rec.Body, want)
	}
	if fetcher.got.URL != privateFeedURL || len(fetcher.got.Accept) == 0 || fetcher.got.MaxBytes != 1<<20 {
		t.Errorf("request = %+v", fetcher.got)
	}
}

func TestGetReturnsAnEmptyItemList(t *testing.T) {
	fetcher := &fakeFetcher{resp: egress.Response{Body: []byte("<rss><channel><title>T</title></channel></rss>"), FinalURL: "https://feeds.example.com/f"}}
	rec := newRig(t, fetcher).get(privateFeedURL, "user")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestGetMapsFetchErrors(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		status   int
		code     string
		message  string
		logsInfo bool
	}{
		{"invalid url", &egress.HostError{Host: "feeds.example.com", Err: egress.ErrInvalidURL}, 400, "invalid_feed_url", "Feed URLs must be https addresses with a domain name.", false},
		{"invalid url without a host", egress.ErrInvalidURL, 400, "invalid_feed_url", "Feed URLs must be https addresses with a domain name.", false},
		{"not allowed", &egress.HostError{Host: "feeds.example.com", Err: egress.ErrNotAllowed}, 403, "outbound_domain_not_allowed", "feeds.example.com is not on the outbound allowlist.", true},
		{"blocked address", &egress.HostError{Host: "internal.example.com", Err: egress.ErrBlockedAddress}, 502, "feed_unavailable", "internal.example.com points to a private or reserved address.", true},
		{"too large", &egress.HostError{Host: "feeds.example.com", Err: egress.ErrTooLarge}, 502, "feed_unavailable", "The feed is larger than 1 MiB.", true},
		{"wrong content type", &egress.HostError{Host: "feeds.example.com", Err: egress.ErrContentType}, 502, "feed_unavailable", "The address did not return an RSS or Atom feed.", true},
		{"unavailable", &egress.HostError{Host: "feeds.example.com", Err: egress.ErrUnavailable}, 502, "feed_unavailable", "The feed could not be fetched.", true},
		{"unavailable without a host", egress.ErrUnavailable, 502, "feed_unavailable", "The feed could not be fetched.", true},
		{"wrapped", fmt.Errorf("again: %w", &egress.HostError{Host: "feeds.example.com", Err: egress.ErrNotAllowed}), 403, "outbound_domain_not_allowed", "feeds.example.com is not on the outbound allowlist.", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, &fakeFetcher{err: tc.err})
			rec := r.get(privateFeedURL, "user")
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d, body %s", rec.Code, tc.status, rec.Body)
			}
			if body := decodeError(t, rec); body.Code != tc.code || body.Error != tc.message {
				t.Errorf("body = %+v", body)
			}
			if !tc.logsInfo {
				if strings.TrimSpace(r.logs.String()) != "" {
					t.Errorf("unexpected log: %s", r.logs)
				}
				return
			}
			record := r.logs.Find(t, "news feed fetch failed")
			if record["level"] != "INFO" || record["code"] != tc.code || record["actor_id"] != "caller-1" {
				t.Errorf("record = %v", record)
			}
			r.logs.AssertAbsent(t, "/secret-path", "token=", "abc123")
		})
	}
}

func TestGetRejectsBadFeedBodies(t *testing.T) {
	for name, body := range map[string]string{
		"html page": "<!DOCTYPE html><html><body>Hello</body></html>",
		"truncated": "<rss><channel><item><title>Cut",
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, &fakeFetcher{resp: egress.Response{Body: []byte(body), FinalURL: "https://feeds.example.com/secret-path/feed.xml?token=abc123"}})
			rec := r.get(privateFeedURL, "user")
			if rec.Code != http.StatusBadGateway {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
			}
			if got := decodeError(t, rec); got.Code != "feed_invalid" || got.Error != "The address did not return a valid RSS or Atom feed." {
				t.Errorf("body = %+v", got)
			}
			record := r.logs.Find(t, "news feed fetch failed")
			if record["host"] != "feeds.example.com" || record["code"] != "feed_invalid" {
				t.Errorf("record = %v", record)
			}
			r.logs.AssertAbsent(t, "/secret-path", "token=")
		})
	}
}

func TestGetValidatesTheURLParameter(t *testing.T) {
	cases := map[string]string{
		"missing":  "",
		"too long": "https://feeds.example.com/" + strings.Repeat("a", 2000),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			fetcher := &fakeFetcher{}
			rec := newRig(t, fetcher).get(raw, "user")
			if rec.Code != http.StatusBadRequest || decodeError(t, rec).Code != "invalid_request" {
				t.Errorf("status = %d, body %s", rec.Code, rec.Body)
			}
			if fetcher.got.URL != "" {
				t.Errorf("fetched %q", fetcher.got.URL)
			}
		})
	}
}

func TestGetRefusesServiceAccounts(t *testing.T) {
	fetcher := &fakeFetcher{}
	rec := newRig(t, fetcher).get(privateFeedURL, string(auth.KindServiceAccount))
	if rec.Code != http.StatusForbidden || decodeError(t, rec).Code != "forbidden" {
		t.Errorf("status = %d, body %s", rec.Code, rec.Body)
	}
	if fetcher.got.URL != "" {
		t.Errorf("fetched %q", fetcher.got.URL)
	}
}

func TestGetUnknownFetchErrorIsAnInternalError(t *testing.T) {
	r := newRig(t, &fakeFetcher{err: fmt.Errorf("read outbound allowlist: %w", errors.New("connection refused to db.internal:5432"))})
	rec := r.get(privateFeedURL, "user")
	if rec.Code != http.StatusInternalServerError || decodeError(t, rec).Code != "internal" {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	record := r.logs.Find(t, "fetch news feed")
	if record["level"] != slog.LevelError.String() {
		t.Errorf("level = %v", record["level"])
	}
	r.logs.AssertSafeFailure(t, record, "internal", "db.internal", "/secret-path", "token=")
}

func TestGetGoneClientIsNotAFailure(t *testing.T) {
	r := newRig(t, &fakeFetcher{err: context.Canceled})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/news?url="+url.QueryEscape(privateFeedURL), nil).WithContext(ctx)
	req.Header.Set("X-Test-Kind", "user")
	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	if rec.Code != 499 {
		t.Errorf("status = %d", rec.Code)
	}
	for _, record := range r.logs.Records(t) {
		if record["level"] == slog.LevelError.String() {
			t.Errorf("error logged: %v", record)
		}
	}
}
