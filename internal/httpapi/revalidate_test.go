package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func revalidatedRequest(t *testing.T, ifNoneMatch string, body any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/x", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		RevalidatedJSON(c, body)
	})
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestRevalidatedJSONSendsAPrivateTagThatVariesByCaller(t *testing.T) {
	rec := revalidatedRequest(t, "", gin.H{"a": 1})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-cache" {
		t.Errorf("Cache-Control = %q, want it to replace no-store with private, no-cache", got)
	}
	if got := rec.Header().Get("Vary"); got != "Authorization, Cookie" {
		t.Errorf("Vary = %q", got)
	}
	if rec.Header().Get("ETag") == "" || rec.Body.String() != `{"a":1}` {
		t.Errorf("etag %q body %q", rec.Header().Get("ETag"), rec.Body.String())
	}
}

func TestRevalidatedJSONAnswers304OnlyWhileTheBodyIsUnchanged(t *testing.T) {
	tag := revalidatedRequest(t, "", gin.H{"a": 1}).Header().Get("ETag")
	cases := []struct {
		name  string
		match string
		body  any
		want  int
	}{
		{"same body", tag, gin.H{"a": 1}, http.StatusNotModified},
		{"one of a list", `"other", ` + tag, gin.H{"a": 1}, http.StatusNotModified},
		{"strong form of the same tag", tag[2:], gin.H{"a": 1}, http.StatusNotModified},
		{"wildcard", "*", gin.H{"a": 1}, http.StatusNotModified},
		{"changed body", tag, gin.H{"a": 2}, http.StatusOK},
		{"other tag", `"nope"`, gin.H{"a": 1}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := revalidatedRequest(t, tc.match, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if tc.want == http.StatusNotModified && rec.Body.Len() != 0 {
				t.Errorf("304 carried a body %q", rec.Body.String())
			}
		})
	}
}
