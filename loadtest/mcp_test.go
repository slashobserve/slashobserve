package main

import (
	"strings"
	"testing"
)

func TestRPCFailure(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		wantCode    string
		wantMessage string
	}{
		{"success", `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{}"}]}}`, "", ""},
		{"tool error", `{"jsonrpc":"2.0","id":1,"result":{"isError":true,"content":[{"type":"text","text":"{\"code\":\"datasource_unavailable\"}"}]}}`, "tool_error", `{"code":"datasource_unavailable"}`},
		{"rpc error", `{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"bad params"}}`, "rpc_-32602", "bad params"},
		{"event stream", "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"isError\":true,\"content\":[{\"text\":\"boom\"}]}}\n\n", "tool_error", "boom"},
		{"no result", `{"jsonrpc":"2.0","id":1}`, "no_result", ""},
		{"not json", `nope`, "unreadable", "nope"},
	}
	for _, tt := range tests {
		code, message := rpcFailure([]byte(tt.payload))
		if code != tt.wantCode || message != tt.wantMessage {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", tt.name, code, message, tt.wantCode, tt.wantMessage)
		}
	}
}

func TestShedByServerIsOnlyTheBusyAnswer(t *testing.T) {
	busy := reply{status: 503, body: []byte(`{"code":"busy","message":"server is busy"}`)}
	if !shedByServer(busy) {
		t.Fatal("a 503 with code busy is the server shedding load")
	}
	for name, r := range map[string]reply{
		"other 503":     {status: 503, body: []byte(`{"code":"shutting_down"}`)},
		"429":           {status: 429, body: []byte(`{"code":"too_many_requests"}`)},
		"busy with 200": {status: 200, body: []byte(`{"code":"busy"}`)},
		"502":           {status: 502},
	} {
		if shedByServer(r) {
			t.Errorf("%s must not count as shed load", name)
		}
	}
}

func TestShedGate(t *testing.T) {
	tests := []struct {
		name     string
		replicas int
		shed     int
		wantPass bool
		contains string
	}{
		{"one server above the cap sheds as expected", 1, 100, true, "expected"},
		{"two servers inside the cap shed nothing", 2, 0, true, "20 calls in flight per server"},
		{"two servers inside the cap must not shed", 2, 1, false, "1 shed"},
	}
	for _, tt := range tests {
		cfg := &config{mcpClients: 10, mcpInFlight: 4, replicas: make([]string, tt.replicas)}
		m := &mcpRun{e: &env{cfg: cfg}}
		g := m.shedGate(tt.shed)
		if g.Pass != tt.wantPass || !strings.Contains(g.Detail, tt.contains) {
			t.Errorf("%s: got %+v", tt.name, g)
		}
	}
}

func TestShare(t *testing.T) {
	if got := share(1, 4); got != 25 {
		t.Errorf("share(1, 4) = %v, want 25", got)
	}
	if got := share(0, 0); got != 0 {
		t.Errorf("share(0, 0) = %v, want 0", got)
	}
}
