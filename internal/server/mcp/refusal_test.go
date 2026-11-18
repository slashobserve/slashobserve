package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUniformRefusalGivesEveryRefusalACodeAndAMessage(t *testing.T) {
	cases := map[string]struct{ in, code, message string }{
		"already uniform":       {`{"code":"not_found","message":"gone"}`, "not_found", "gone"},
		"error becomes message": {`{"error":"bad base","code":"invalid_request"}`, "invalid_request", "bad base"},
		"no message":            {`{"code":"version_conflict","latest_version":4}`, "version_conflict", "the call was refused with code version_conflict; the other fields say why"},
		"no code":               {`{"message":"boom"}`, refusalCodeUnknown, "boom"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var got map[string]any
			if err := json.Unmarshal(uniformRefusal([]byte(c.in)), &got); err != nil {
				t.Fatal(err)
			}
			if got["code"] != c.code || got["message"] != c.message {
				t.Fatalf("got %v", got)
			}
			if _, ok := got["error"]; ok {
				t.Fatalf("error key kept: %v", got)
			}
		})
	}
	extra := string(uniformRefusal([]byte(`{"code":"version_conflict","latest_version":4}`)))
	if !json.Valid([]byte(extra)) || !strings.Contains(extra, `"latest_version":4`) {
		t.Fatalf("other fields lost: %s", extra)
	}
	if got := string(uniformRefusal([]byte(`not json`))); got != "not json" {
		t.Fatalf("non-object changed: %s", got)
	}
}
