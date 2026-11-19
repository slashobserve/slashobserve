package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func TestWebRoutesAnswerJSONNotFoundForServerPaths(t *testing.T) {
	for _, path := range []string{"/api/nope", "/api", "/.well-known/openid-configuration", "/.well-known", "/oauth/nope", "/oauth"} {
		t.Run(path, func(t *testing.T) {
			rec := serveWeb(t, Redirects{}, http.MethodGet, path)
			if rec.Code != http.StatusNotFound || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
				t.Fatalf("GET %s = %d %q, want a JSON 404", path, rec.Code, rec.Header().Get("Content-Type"))
			}
			if !strings.Contains(rec.Body.String(), `"code":"not_found"`) {
				t.Fatalf("body %q lacks the not_found code", rec.Body.String())
			}
		})
	}
}

func TestWebRoutesStillServeTheAppForConnectAndOtherPaths(t *testing.T) {
	for _, path := range []string{"/connect/x", "/oauthish", "/settings"} {
		t.Run(path, func(t *testing.T) {
			rec := serveWeb(t, Redirects{}, http.MethodGet, path)
			if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
				t.Fatalf("GET %s answered JSON: %s", path, rec.Body.String())
			}
			if rec.Code != http.StatusOK && rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("GET %s = %d, want the app or the not-built text", path, rec.Code)
			}
		})
	}
}

const bundleSource = "export const answer = 42;\n"

func serveBundle(t *testing.T, target, acceptEncoding string) *httptest.ResponseRecorder {
	t.Helper()
	dist := fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html>")},
		"assets/app-abc.js":    {Data: []byte(strings.Repeat(bundleSource, 100))},
		"assets/app-abc.js.br": {Data: []byte("brotli-bytes")},
		"assets/plain-abc.js":  {Data: []byte(strings.Repeat(bundleSource, 100))},
		"logo.svg":             {Data: []byte("<svg/>")},
		"logo.svg.br":          {Data: []byte("brotli-bytes")},
	}
	engine := gin.New()
	registerWebFiles(engine, Redirects{}, dist)
	handler, err := compressReads(engine)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestAnAssetWithAPrecompressedFileIsServedBrotliToABrowserThatAsks(t *testing.T) {
	rec := serveBundle(t, "/assets/app-abc.js", "gzip, deflate, br")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Encoding") != "br" || rec.Body.String() != "brotli-bytes" {
		t.Fatalf("got %d %q %q", rec.Code, rec.Header().Get("Content-Encoding"), rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Fatalf("Content-Type %q is not JavaScript", got)
	}
	if got := rec.Header().Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
		t.Fatalf("Vary %q lacks Accept-Encoding", got)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Fatalf("Cache-Control %q lost immutable", got)
	}
}

func TestAnAssetIsNotServedBrotliWhenTheBrowserDoesNotAskOrRefusesIt(t *testing.T) {
	for _, accept := range []string{"", "gzip", "identity", "br;q=0, gzip", "brx"} {
		t.Run(accept, func(t *testing.T) {
			rec := serveBundle(t, "/assets/app-abc.js", accept)
			if rec.Header().Get("Content-Encoding") == "br" || strings.Contains(rec.Body.String(), "brotli-bytes") {
				t.Fatalf("Accept-Encoding %q got brotli", accept)
			}
			if rec.Header().Get("Vary") == "" {
				t.Fatalf("Vary is empty, so a cache could hand the plain copy to a brotli browser")
			}
		})
	}
}

func TestAnAssetWithoutAPrecompressedFileFallsBackToGzip(t *testing.T) {
	rec := serveBundle(t, "/assets/plain-abc.js", "br, gzip")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("got %d %q, want gzip", rec.Code, rec.Header().Get("Content-Encoding"))
	}
}

func TestOnlyFilesUnderAssetsAreServedPrecompressed(t *testing.T) {
	rec := serveBundle(t, "/logo.svg", "br")
	if rec.Header().Get("Content-Encoding") == "br" {
		t.Fatalf("a top-level file was served brotli")
	}
}

func TestThePrecompressedCopyIsNeverServedAsAFileOfItsOwn(t *testing.T) {
	rec := serveBundle(t, "/assets/app-abc.js.br", "br")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET the .br file = %d, want 404", rec.Code)
	}
}
