package cli

import (
	"net/http"
	"strings"
	"testing"
)

func TestSchemaListsTablesThenColumnsAndSaysHowToPageOn(t *testing.T) {
	server, seen := recordingServer(t, `{"tables":[{"name":"otel_logs"},{"name":"otel_traces","kind":"MergeTree"}],"truncated":true,"truncated_by":"limit","next_offset":2}`)
	stdout, stderr, code := runWith(server, "datasources", "schema", "ch")
	if code != 0 || seen.path != "/api/v1/datasources/ch/schema" || seen.query != "" {
		t.Fatalf("exit %d, call %+v, stderr %q", code, seen, stderr)
	}
	if !strings.Contains(stdout, "otel_logs") || !strings.Contains(stdout, "MergeTree") || !strings.Contains(stderr, "--offset 2") {
		t.Fatalf("stdout %q, stderr %q", stdout, stderr)
	}

	server, seen = recordingServer(t, `{"table":"otel_logs","columns":[{"name":"Body","type":"String"},{"name":"Timestamp"}],"truncated":false}`)
	stdout, _, code = runWith(server, "datasources", "schema", "ch", "--table", "otel_logs", "--offset", "5")
	if code != 0 || seen.query != "offset=5&table=otel_logs" || !strings.Contains(stdout, "Body") || !strings.Contains(stdout, "String") {
		t.Fatalf("exit %d, call %+v, stdout %q", code, seen, stdout)
	}
}

func TestSchemaOfATempoDatasourcePrintsServicesSpanNamesAndAttributes(t *testing.T) {
	server, _ := recordingServer(t, `{"datasource":"tempo","columns":["service","span_name"],"rows":[["checkout","POST /pay"]],"attributes":["http.method"],"truncated":false}`)
	stdout, _, code := runWith(server, "datasources", "schema", "tempo")
	if code != 0 || !strings.Contains(stdout, "checkout") || !strings.Contains(stdout, "POST /pay") || !strings.Contains(stdout, "http.method") {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestSampleSendsTheTableAndLimitAndPrintsTheRows(t *testing.T) {
	server, seen := recordingServer(t, `{"table":"otel_logs","columns":["Body","Severity"],"rows":[["payment failed",null],["ok","INFO"]],"truncated":true}`)
	stdout, stderr, code := runWith(server, "datasources", "sample", "ch", "--table", "otel_logs", "--limit", "2")
	if code != 0 || seen.path != "/api/v1/datasources/ch/sample" || seen.query != "limit=2&table=otel_logs" {
		t.Fatalf("exit %d, call %+v, stderr %q", code, seen, stderr)
	}
	if !strings.Contains(stdout, "Body") || !strings.Contains(stdout, "payment failed") || !strings.Contains(stdout, "INFO") || !strings.Contains(stderr, "cut") {
		t.Fatalf("stdout %q, stderr %q", stdout, stderr)
	}
}

func TestSchemaAndSampleRefuseBadFlagsBeforeCallingTheServer(t *testing.T) {
	for _, args := range [][]string{
		{"datasources", "schema", "ch", "--offset", "-1"},
		{"datasources", "sample", "ch", "--limit", "0"},
		{"datasources", "sample", "ch", "--limit", "21"},
		{"datasources", "schema"},
		{"datasources", "sample"},
	} {
		_, _, stderr, code := runAgainst(t, http.StatusOK, `{}`, args...)
		if code != 2 || stderr == "" {
			t.Fatalf("%v: exit %d, stderr %q", args, code, stderr)
		}
	}
}
