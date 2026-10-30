package assets

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

func (r *rig) get(route, rawURL, kind string) *httptest.ResponseRecorder {
	target := "/api/v1/assets/" + route
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

const privateURL = "https://cdn.example.com/secret-path/pic.png?token=abc123"

var (
	pngBody  = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	svgBody  = []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	pngReply = egress.Response{Body: pngBody, ContentType: "image/png", FinalURL: "https://cdn.example.com/secret-path/pic.png?token=abc123"}
)

func TestImageServesEachTypeWithTheVerifiedTypeAndSafeHeaders(t *testing.T) {
	cases := []struct {
		mediaType string
		body      []byte
	}{
		{"image/png", pngBody},
		{"image/jpeg", []byte("\xff\xd8\xff\xe0\x00")},
		{"image/gif", []byte("GIF89a\x01\x00")},
		{"image/webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")},
		{"image/avif", []byte("\x00\x00\x00\x1cftypavif\x00\x00\x00\x00")},
		{"image/svg+xml", svgBody},
	}
	for _, tc := range cases {
		t.Run(tc.mediaType, func(t *testing.T) {
			fetcher := &fakeFetcher{resp: egress.Response{Body: tc.body, ContentType: tc.mediaType, FinalURL: privateURL}}
			rec := newRig(t, fetcher).get("image", privateURL, "user")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
			}
			if rec.Body.String() != string(tc.body) {
				t.Errorf("body changed: %q", rec.Body)
			}
			want := map[string]string{
				"Content-Type":                 tc.mediaType,
				"Cache-Control":                "private, max-age=3600",
				"Content-Security-Policy":      "default-src 'none'; style-src 'unsafe-inline'; sandbox; frame-ancestors 'none'",
				"Cross-Origin-Resource-Policy": "same-origin",
			}
			for name, value := range want {
				if got := rec.Header().Get(name); got != value {
					t.Errorf("%s = %q, want %q", name, got, value)
				}
			}
			if fetcher.got.URL != privateURL || fetcher.got.MaxBytes != 5<<20 {
				t.Errorf("request = %+v", fetcher.got)
			}
			wantAccept := []string{"image/png", "image/jpeg", "image/gif", "image/webp", "image/avif", "image/svg+xml"}
			if strings.Join(fetcher.got.Accept, ",") != strings.Join(wantAccept, ",") {
				t.Errorf("accept = %v", fetcher.got.Accept)
			}
		})
	}
}

func TestImageRefusesABodyThatIsNotItsDeclaredType(t *testing.T) {
	cases := map[string]egress.Response{
		"html declared as png": {Body: []byte("<html><script>alert(1)</script></html>"), ContentType: "image/png", FinalURL: privateURL},
		"xml declared as svg":  {Body: []byte("<rss/>"), ContentType: "image/svg+xml", FinalURL: privateURL},
	}
	for name, resp := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, &fakeFetcher{resp: resp})
			rec := r.get("image", privateURL, "user")
			if rec.Code != http.StatusBadGateway {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
			}
			if got := decodeError(t, rec); got.Code != "asset_unavailable" || got.Error != "The address did not return a valid image." {
				t.Errorf("body = %+v", got)
			}
			record := r.logs.Find(t, "asset fetch failed")
			if record["host"] != "cdn.example.com" || record["code"] != "asset_unavailable" {
				t.Errorf("record = %v", record)
			}
			r.logs.AssertAbsent(t, "/secret-path", "token=", "abc123")
			if rec.Header().Get("Cache-Control") != "" {
				t.Errorf("a refusal was made cacheable: %v", rec.Header())
			}
		})
	}
}

func TestGeoJSONRoundTripsLargeIntegersWithTheDeclaredHeaders(t *testing.T) {
	body := `{"type":"FeatureCollection","features":[{"type":"Feature","id":9007199254740993,"properties":null,"geometry":{"type":"Point","coordinates":[1,2]}}]}`
	fetcher := &fakeFetcher{resp: egress.Response{Body: []byte(body), ContentType: "text/plain", FinalURL: privateURL}}
	rec := newRig(t, fetcher).get("geojson", privateURL, "user")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"id":9007199254740993`) {
		t.Errorf("large integer lost: %s", rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("content type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, max-age=3600" {
		t.Errorf("cache control = %q", got)
	}
	wantAccept := "application/geo+json,application/json,text/plain"
	if strings.Join(fetcher.got.Accept, ",") != wantAccept || fetcher.got.MaxBytes != 5<<20 {
		t.Errorf("request = %+v", fetcher.got)
	}
}

func TestGeoJSONRefusesBodiesThatAreNotGeoJSON(t *testing.T) {
	for name, body := range map[string]string{
		"array":        `[1,2,3]`,
		"unknown type": `{"type":"Nope"}`,
		"html":         `<html></html>`,
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, &fakeFetcher{resp: egress.Response{Body: []byte(body), ContentType: "application/json", FinalURL: privateURL}})
			rec := r.get("geojson", privateURL, "user")
			if rec.Code != http.StatusBadGateway {
				t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
			}
			if got := decodeError(t, rec); got.Code != "asset_unavailable" || got.Error != "The address did not return a valid GeoJSON file." {
				t.Errorf("body = %+v", got)
			}
			r.logs.AssertAbsent(t, "/secret-path", "token=")
		})
	}
}

func TestFetchErrorsMapToTheDocumentedStatuses(t *testing.T) {
	host := func(kind error) error { return &egress.HostError{Host: "cdn.example.com", Err: kind} }
	cases := []struct {
		name     string
		err      error
		status   int
		code     string
		image    string
		geojson  string
		logsInfo bool
	}{
		{"invalid url", host(egress.ErrInvalidURL), 400, "invalid_asset_url", "Asset URLs must be https addresses with a domain name.", "Asset URLs must be https addresses with a domain name.", false},
		{"invalid url without a host", egress.ErrInvalidURL, 400, "invalid_asset_url", "Asset URLs must be https addresses with a domain name.", "Asset URLs must be https addresses with a domain name.", false},
		{"not allowed", host(egress.ErrNotAllowed), 403, "outbound_domain_not_allowed", "cdn.example.com is not on the outbound allowlist.", "cdn.example.com is not on the outbound allowlist.", true},
		{"blocked address", host(egress.ErrBlockedAddress), 502, "asset_unavailable", "cdn.example.com points to a private or reserved address.", "cdn.example.com points to a private or reserved address.", true},
		{"too large", host(egress.ErrTooLarge), 502, "asset_unavailable", "The file is larger than 5 MiB.", "The file is larger than 5 MiB.", true},
		{"wrong content type", host(egress.ErrContentType), 502, "asset_unavailable", "The address did not return an image.", "The address did not return a GeoJSON file.", true},
		{"unavailable", host(egress.ErrUnavailable), 502, "asset_unavailable", "The file could not be fetched.", "The file could not be fetched.", true},
		{"unavailable without a host", egress.ErrUnavailable, 502, "asset_unavailable", "The file could not be fetched.", "The file could not be fetched.", true},
		{"wrapped", fmt.Errorf("again: %w", host(egress.ErrNotAllowed)), 403, "outbound_domain_not_allowed", "cdn.example.com is not on the outbound allowlist.", "cdn.example.com is not on the outbound allowlist.", true},
	}
	for _, route := range []string{"image", "geojson"} {
		for _, tc := range cases {
			t.Run(route+" "+tc.name, func(t *testing.T) {
				r := newRig(t, &fakeFetcher{err: tc.err})
				rec := r.get(route, privateURL, "user")
				if rec.Code != tc.status {
					t.Fatalf("status = %d, want %d, body %s", rec.Code, tc.status, rec.Body)
				}
				message := tc.image
				if route == "geojson" {
					message = tc.geojson
				}
				if body := decodeError(t, rec); body.Code != tc.code || body.Error != message {
					t.Errorf("body = %+v, want %q", body, message)
				}
				if !tc.logsInfo {
					if strings.TrimSpace(r.logs.String()) != "" {
						t.Errorf("unexpected log: %s", r.logs)
					}
					return
				}
				record := r.logs.Find(t, "asset fetch failed")
				if record["level"] != "INFO" || record["code"] != tc.code || record["actor_id"] != "caller-1" {
					t.Errorf("record = %v", record)
				}
				r.logs.AssertAbsent(t, "/secret-path", "token=", "abc123")
			})
		}
	}
}

func TestURLParameterIsValidated(t *testing.T) {
	cases := map[string]string{
		"missing":  "",
		"too long": "https://cdn.example.com/" + strings.Repeat("a", 2000),
	}
	for _, route := range []string{"image", "geojson"} {
		for name, raw := range cases {
			t.Run(route+" "+name, func(t *testing.T) {
				fetcher := &fakeFetcher{}
				rec := newRig(t, fetcher).get(route, raw, "user")
				if rec.Code != http.StatusBadRequest || decodeError(t, rec).Code != "invalid_request" {
					t.Errorf("status = %d, body %s", rec.Code, rec.Body)
				}
				if fetcher.got.URL != "" {
					t.Errorf("fetched %q", fetcher.got.URL)
				}
			})
		}
	}
}

func TestServiceAccountsAreRefused(t *testing.T) {
	for _, route := range []string{"image", "geojson"} {
		t.Run(route, func(t *testing.T) {
			fetcher := &fakeFetcher{resp: pngReply}
			rec := newRig(t, fetcher).get(route, privateURL, string(auth.KindServiceAccount))
			if rec.Code != http.StatusForbidden || decodeError(t, rec).Code != "forbidden" {
				t.Errorf("status = %d, body %s", rec.Code, rec.Body)
			}
			if fetcher.got.URL != "" {
				t.Errorf("fetched %q", fetcher.got.URL)
			}
		})
	}
}

func TestUnknownFetchErrorIsAnInternalError(t *testing.T) {
	r := newRig(t, &fakeFetcher{err: fmt.Errorf("read outbound allowlist: %w", errors.New("connection refused to db.internal:5432"))})
	rec := r.get("image", privateURL, "user")
	if rec.Code != http.StatusInternalServerError || decodeError(t, rec).Code != "internal" {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	record := r.logs.Find(t, "fetch asset")
	if record["level"] != slog.LevelError.String() {
		t.Errorf("level = %v", record["level"])
	}
	r.logs.AssertSafeFailure(t, record, "internal", "db.internal", "/secret-path", "token=")
}

func TestGoneClientIsNotAFailure(t *testing.T) {
	r := newRig(t, &fakeFetcher{err: context.Canceled})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets/image?url="+url.QueryEscape(privateURL), nil).WithContext(ctx)
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
