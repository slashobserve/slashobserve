package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

const mixedFrames = `{"frames":[{"ref":"A","name":"logs","labels":{"app":"shop"},"fields":[
	{"name":"Time","type":"time","values":[1762000000000,1762000001000]},
	{"name":"Body","type":"string","values":["first","second"]},
	{"name":"Count","type":"number","values":[3,null]},
	{"name":"Ok","type":"bool","values":[true,false]},
	{"name":"Attributes","type":"other","values":[{"k":"v"},null]}
]}],"meta":{}}`

func TestExploreFramesDecodeMixedFieldTypes(t *testing.T) {
	body, err := decode[exploreResponse]([]byte(mixedFrames))
	if err != nil {
		t.Fatal(err)
	}
	if len(body.Frames) != 1 {
		t.Fatalf("frames %+v", body.Frames)
	}
	f := body.Frames[0]
	if f.Ref != "A" || f.Name != "logs" || f.Labels["app"] != "shop" || f.rows() != 2 {
		t.Fatalf("frame %+v", f)
	}
	if ms, ok := f.numberAt(0, 1); !ok || ms != 1762000001000 {
		t.Errorf("time %v %v", ms, ok)
	}
	if got := f.stringAt(1, 0); got != "first" {
		t.Errorf("body %q", got)
	}
	if _, ok := f.numberAt(2, 1); ok {
		t.Error("a null number reads as a number")
	}
	if got := f.stringAt(3, 0); got != "true" {
		t.Errorf("bool %q", got)
	}
	if got := f.stringAt(4, 0); got != `{"k":"v"}` {
		t.Errorf("object %q", got)
	}
	if got := f.stringAt(4, 1); got != "" {
		t.Errorf("null %q", got)
	}
	if got := f.stringAt(1, 5); got != "" {
		t.Errorf("past the end %q", got)
	}
}

func TestFieldIndexMatchesNamesCaseInsensitivelyInOrder(t *testing.T) {
	body, err := decode[exploreResponse]([]byte(mixedFrames))
	if err != nil {
		t.Fatal(err)
	}
	f := body.Frames[0]
	cases := []struct {
		names []string
		want  int
	}{
		{[]string{"body"}, 1},
		{[]string{"BODY"}, 1},
		{[]string{"message", "count"}, 2},
		{[]string{"count", "body"}, 2},
		{[]string{"missing"}, -1},
	}
	for _, tc := range cases {
		if got := fieldIndex(f, tc.names...); got != tc.want {
			t.Errorf("fieldIndex(%v) = %d, want %d", tc.names, got, tc.want)
		}
	}
	if got := firstOfType(f, "time"); got != 0 {
		t.Errorf("first time field %d", got)
	}
	if got := firstOfType(f, "string"); got != 1 {
		t.Errorf("first string field %d", got)
	}
	if got := firstOfType(f, "missing"); got != -1 {
		t.Errorf("missing type %d", got)
	}
}

func TestExplorePostsTheQueryToTheDatasource(t *testing.T) {
	var path, raw string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.EscapedPath()
		data, _ := io.ReadAll(r.Body)
		raw = string(data)
		_, _ = io.WriteString(w, mixedFrames)
	}))
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	c, err := (&app{version: "test"}).clientFor(base, "k")
	if err != nil {
		t.Fatal(err)
	}
	_, body, err := explore(t.Context(), c, "loki main", exploreRequest{Query: `{app="shop"}`, Kind: "range", From: "now-1h", To: "now", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/api/v1/datasources/loki%20main/query" || len(body.Frames) != 1 {
		t.Fatalf("path %q, frames %d", path, len(body.Frames))
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		t.Fatal(err)
	}
	if sent["query"] != `{app="shop"}` || sent["kind"] != "range" || sent["from"] != "now-1h" || sent["to"] != "now" || sent["limit"] != float64(10) {
		t.Fatalf("sent %s", raw)
	}
}
