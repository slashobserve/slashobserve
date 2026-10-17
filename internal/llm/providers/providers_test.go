package providers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/config"
	"github.com/slashobserve/slashobserve/internal/llm"
	"github.com/slashobserve/slashobserve/internal/llm/providers"
)

const (
	sentinel    = "leakcheck-7f3a91"
	systemText  = "You build dashboards."
	toolsSchema = `{"type":"object","properties":{"limit":{"type":"integer","minimum":1,"maximum":100},"name":{"type":"string","minLength":2},"ratio":{"type":"number","exclusiveMinimum":0.5}},"required":["limit"],"additionalProperties":false,"title":"Query","minProperties":1,"maxProperties":10}`
	toolsNormal = `{"additionalProperties":false,"maxProperties":10,"minProperties":1,"properties":{"limit":{"maximum":100,"minimum":1,"type":"integer"},"name":{"minLength":2,"type":"string"},"ratio":{"exclusiveMinimum":0.5,"type":"number"}},"required":["limit"],"title":"Query","type":"object"}`
)

type recorded struct {
	Path   string
	Header http.Header
	Body   []byte
}

type upstream struct {
	srv  *httptest.Server
	mu   sync.Mutex
	reqs []recorded
}

func newUpstream(t *testing.T, handler func(n int, w http.ResponseWriter, r *http.Request)) *upstream {
	t.Helper()
	u := &upstream{}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		u.mu.Lock()
		u.reqs = append(u.reqs, recorded{Path: r.URL.Path, Header: r.Header.Clone(), Body: body})
		n := len(u.reqs)
		u.mu.Unlock()
		handler(n, w, r)
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func replies(bodies ...string) func(int, http.ResponseWriter, *http.Request) {
	return func(n int, w http.ResponseWriter, _ *http.Request) {
		if n > len(bodies) {
			http.Error(w, "unexpected extra request", http.StatusTeapot)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, bodies[n-1])
	}
}

func (u *upstream) requests() []recorded {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]recorded(nil), u.reqs...)
}

func (u *upstream) url() *url.URL {
	parsed, err := url.Parse(u.srv.URL + "/")
	if err != nil {
		panic(err)
	}
	return parsed
}

type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type fakeCall struct {
	ID    string
	Name  string
	Input string
}

type flavor struct {
	name      string
	endTurn   func(text string) string
	withStop  func(stop string) string
	toolCalls func(calls ...fakeCall) string
	usage     func(input, output, cacheRead int64) string
	errorBody func(message string) string
	requestID string
	prefix    func(t *testing.T, body []byte) (tools, system []byte)
}

var anthropicFlavor = flavor{
	name: config.LLMProviderAnthropic,
	endTurn: func(text string) string {
		return fmt.Sprintf(`{"id":"msg_1","type":"message","role":"assistant","model":"test-model","content":[{"type":"text","text":%q}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`, text)
	},
	withStop: func(stop string) string {
		return fmt.Sprintf(`{"id":"msg_1","type":"message","role":"assistant","model":"test-model","content":[{"type":"text","text":"x"}],"stop_reason":%q,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`, stop)
	},
	toolCalls: func(calls ...fakeCall) string {
		blocks := []string{`{"type":"thinking","thinking":"hmm","signature":"sig-abc=="}`, `{"type":"redacted_thinking","data":"opaque-redacted=="}`, `{"type":"text","text":"calling"}`}
		for _, c := range calls {
			blocks = append(blocks, fmt.Sprintf(`{"type":"tool_use","id":%q,"name":%q,"input":%s}`, c.ID, c.Name, c.Input))
		}
		return fmt.Sprintf(`{"id":"msg_1","type":"message","role":"assistant","model":"test-model","content":[%s],"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`, strings.Join(blocks, ","))
	},
	usage: func(input, output, cacheRead int64) string {
		return fmt.Sprintf(`{"id":"msg_1","type":"message","role":"assistant","model":"test-model","content":[{"type":"text","text":"x"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":%d,"output_tokens":%d,"cache_read_input_tokens":%d,"cache_creation_input_tokens":7}}`, input, output, cacheRead)
	},
	errorBody: func(message string) string {
		return fmt.Sprintf(`{"type":"error","error":{"type":"authentication_error","message":%q}}`, message)
	},
	requestID: "request-id",
	prefix: func(t *testing.T, body []byte) ([]byte, []byte) {
		fields := decodeFields(t, body)
		return fields["tools"], fields["system"]
	},
}

var openAIFlavor = flavor{
	name: config.LLMProviderOpenAI,
	endTurn: func(text string) string {
		return fmt.Sprintf(`{"id":"c1","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":%q}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, text)
	},
	withStop: func(stop string) string {
		return fmt.Sprintf(`{"id":"c1","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"finish_reason":%q,"message":{"role":"assistant","content":"x"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, stop)
	},
	toolCalls: func(calls ...fakeCall) string {
		var parts []string
		for _, c := range calls {
			parts = append(parts, fmt.Sprintf(`{"id":%q,"type":"function","function":{"name":%q,"arguments":%q}}`, c.ID, c.Name, c.Input))
		}
		return fmt.Sprintf(`{"id":"c1","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":"calling","tool_calls":[%s]}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, strings.Join(parts, ","))
	},
	usage: func(input, output, cacheRead int64) string {
		return fmt.Sprintf(`{"id":"c1","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"x"}}],"usage":{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d,"prompt_tokens_details":{"cached_tokens":%d}}}`, input, output, input+output, cacheRead)
	},
	errorBody: func(message string) string {
		return fmt.Sprintf(`{"error":{"message":%q,"type":"invalid_request_error","param":null,"code":"invalid_api_key"}}`, message)
	},
	requestID: "x-request-id",
	prefix: func(t *testing.T, body []byte) ([]byte, []byte) {
		fields := decodeFields(t, body)
		var messages []json.RawMessage
		if err := json.Unmarshal(fields["messages"], &messages); err != nil || len(messages) == 0 {
			t.Fatalf("messages: %v in %s", err, body)
		}
		return fields["tools"], messages[0]
	},
}

var flavors = []flavor{anthropicFlavor, openAIFlavor}

func decodeFields(t *testing.T, body []byte) map[string][]byte {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("request body is not a JSON object: %v\n%s", err, body)
	}
	fields := make(map[string][]byte, len(raw))
	for k, v := range raw {
		fields[k] = v
	}
	return fields
}

func decodeObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("not a JSON object: %v\n%s", err, body)
	}
	return out
}

type options struct {
	timeout time.Duration
	key     *string
	logs    *logBuffer
}

func newProvider(t *testing.T, f flavor, up *upstream, opts options) (llm.Provider, *logBuffer) {
	t.Helper()
	if opts.timeout == 0 {
		opts.timeout = 5 * time.Second
	}
	if opts.logs == nil {
		opts.logs = &logBuffer{}
	}
	key := sentinel
	if opts.key != nil {
		key = *opts.key
	}
	provider, err := providers.New(config.LLM{
		Provider:       f.name,
		Model:          "test-model",
		APIKey:         key,
		BaseURL:        up.url(),
		Effort:         "medium",
		MaxTokens:      2048,
		RequestTimeout: opts.timeout,
	}, slog.New(slog.NewTextHandler(opts.logs, nil)))
	if err != nil {
		t.Fatalf("providers.New: %v", err)
	}
	return provider, opts.logs
}

func request(tools ...llm.ToolSpec) llm.Request {
	return llm.Request{System: systemText, Tools: tools, Prompt: "show errors", Context: "<context>none</context>"}
}

func queryTool() llm.ToolSpec {
	return llm.ToolSpec{Name: "query", Description: "Run a query", InputSchema: json.RawMessage(toolsSchema)}
}

func startNext(t *testing.T, p llm.Provider, req llm.Request) (llm.Session, llm.Turn) {
	t.Helper()
	session, err := p.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	turn, err := session.Next(context.Background(), nil)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	return session, turn
}

func TestToolSchemaKeepsNumbersAndIsByteStable(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			up := newUpstream(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, f.endTurn("ok"))
			})
			p, _ := newProvider(t, f, up, options{})
			for range 20 {
				startNext(t, p, request(queryTool()))
			}
			reqs := up.requests()
			if len(reqs) != 20 {
				t.Fatalf("got %d requests, want 20", len(reqs))
			}
			firstTools, firstSystem := f.prefix(t, reqs[0].Body)
			if bytes.Contains(firstTools, []byte(`"minimum":"1"`)) || !bytes.Contains(firstTools, []byte(`"minimum":1`)) {
				t.Fatalf("numeric keywords were not sent as numbers: %s", firstTools)
			}
			if !bytes.Contains(firstTools, []byte(`"exclusiveMinimum":0.5`)) {
				t.Fatalf("fractional keyword lost: %s", firstTools)
			}
			for i, r := range reqs[1:] {
				tools, system := f.prefix(t, r.Body)
				if !bytes.Equal(tools, firstTools) || !bytes.Equal(system, firstSystem) {
					t.Fatalf("request %d tools/system bytes differ from request 0:\n%s\n%s", i+1, tools, firstTools)
				}
			}
		})
	}
}

func TestToolsAndSystemAreByteIdenticalAcrossTurns(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			up := newUpstream(t, replies(
				f.toolCalls(fakeCall{"call_1", "query", `{"limit":5}`}),
				f.toolCalls(fakeCall{"call_2", "query", `{"limit":6}`}),
				f.endTurn("done"),
			))
			p, _ := newProvider(t, f, up, options{})
			session, turn := startNext(t, p, request(queryTool()))
			for _, id := range []string{"call_1", "call_2"} {
				if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].ID != id {
					t.Fatalf("unexpected tool calls: %+v", turn.ToolCalls)
				}
				var err error
				turn, err = session.Next(context.Background(), []llm.ToolResult{{CallID: id, Content: "rows"}})
				if err != nil {
					t.Fatalf("Next: %v", err)
				}
			}
			reqs := up.requests()
			tools0, system0 := f.prefix(t, reqs[0].Body)
			for i, r := range reqs[1:] {
				tools, system := f.prefix(t, r.Body)
				if !bytes.Equal(tools, tools0) || !bytes.Equal(system, system0) {
					t.Fatalf("turn %d prefix bytes changed", i+1)
				}
			}
		})
	}
}

func TestAnthropicGoldenRequest(t *testing.T) {
	up := newUpstream(t, replies(anthropicFlavor.endTurn("ok")))
	p, _ := newProvider(t, anthropicFlavor, up, options{})
	startNext(t, p, request(queryTool()))

	want := `{
		"model":"test-model","max_tokens":2048,
		"output_config":{"effort":"medium"},
		"system":[{"type":"text","text":"You build dashboards.","cache_control":{"type":"ephemeral"}}],
		"tools":[{"name":"query","description":"Run a query","input_schema":` + toolsNormal + `}],
		"messages":[{"role":"user","content":[{"type":"text","text":"<context>none</context>\n\n<request>\nshow errors\n</request>","cache_control":{"type":"ephemeral"}}]}]
	}`
	got := up.requests()[0]
	if !strings.HasSuffix(got.Path, "/v1/messages") {
		t.Fatalf("path = %q", got.Path)
	}
	assertJSONEqual(t, got.Body, want)
	tools, _ := anthropicFlavor.prefix(t, got.Body)
	if !strings.Contains(string(tools), `"input_schema":`+toolsNormal) {
		t.Fatalf("input_schema is not the normalized schema bytes: %s", tools)
	}
}

func TestOpenAIGoldenRequest(t *testing.T) {
	up := newUpstream(t, replies(openAIFlavor.endTurn("ok")))
	p, _ := newProvider(t, openAIFlavor, up, options{})
	startNext(t, p, request(queryTool()))

	want := `{
		"model":"test-model","max_completion_tokens":2048,
		"messages":[
			{"role":"system","content":"You build dashboards."},
			{"role":"user","content":"<context>none</context>\n\n<request>\nshow errors\n</request>"}
		],
		"tools":[{"type":"function","function":{"name":"query","description":"Run a query","parameters":` + toolsNormal + `}}]
	}`
	got := up.requests()[0]
	if !strings.HasSuffix(got.Path, "/chat/completions") {
		t.Fatalf("path = %q", got.Path)
	}
	assertJSONEqual(t, got.Body, want)
	if got.Header.Get("Authorization") != "Bearer "+sentinel {
		t.Fatalf("Authorization = %q", got.Header.Get("Authorization"))
	}
}

func assertJSONEqual(t *testing.T, got []byte, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("got is not JSON: %v\n%s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("want is not JSON: %v", err)
	}
	gb, _ := json.MarshalIndent(g, "", " ")
	wb, _ := json.MarshalIndent(w, "", " ")
	if !bytes.Equal(gb, wb) {
		t.Fatalf("request body mismatch\ngot:\n%s\nwant:\n%s", gb, wb)
	}
}

func TestAnthropicHistoryRoundTrip(t *testing.T) {
	up := newUpstream(t, replies(
		anthropicFlavor.toolCalls(
			fakeCall{"toolu_a", "query", `{"limit":1}`},
			fakeCall{"toolu_b", "query", `{"limit":2}`},
		),
		anthropicFlavor.toolCalls(fakeCall{"toolu_c", "query", `{"limit":3}`}),
		anthropicFlavor.endTurn("done"),
	))
	p, _ := newProvider(t, anthropicFlavor, up, options{})
	session, turn := startNext(t, p, request(queryTool()))
	if len(turn.ToolCalls) != 2 || turn.Stop != llm.StopToolUse {
		t.Fatalf("turn = %+v", turn)
	}
	turn, err := session.Next(context.Background(), []llm.ToolResult{
		{CallID: "toolu_a", Content: "first"},
		{CallID: "toolu_b", Content: "boom", IsError: true},
	})
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if _, err := session.Next(context.Background(), []llm.ToolResult{{CallID: "toolu_c", Content: "third"}}); err != nil {
		t.Fatalf("Next: %v", err)
	}

	reqs := up.requests()
	second := decodeObject(t, reqs[1].Body)["messages"].([]any)
	if len(second) != 3 {
		t.Fatalf("second request has %d messages", len(second))
	}
	assistant := second[1].(map[string]any)
	if assistant["role"] != "assistant" {
		t.Fatalf("message 1 role = %v", assistant["role"])
	}
	content := assistant["content"].([]any)
	wantTypes := []string{"thinking", "redacted_thinking", "text", "tool_use", "tool_use"}
	if len(content) != len(wantTypes) {
		t.Fatalf("assistant content = %v", content)
	}
	for i, typ := range wantTypes {
		if content[i].(map[string]any)["type"] != typ {
			t.Fatalf("assistant block %d type = %v, want %s", i, content[i].(map[string]any)["type"], typ)
		}
	}
	if content[0].(map[string]any)["signature"] != "sig-abc==" || content[0].(map[string]any)["thinking"] != "hmm" {
		t.Fatalf("thinking block changed: %v", content[0])
	}
	if content[1].(map[string]any)["data"] != "opaque-redacted==" {
		t.Fatalf("redacted_thinking block changed: %v", content[1])
	}
	results := second[2].(map[string]any)["content"].([]any)
	if len(results) != 2 {
		t.Fatalf("tool results = %v", results)
	}
	first, secondResult := results[0].(map[string]any), results[1].(map[string]any)
	if first["tool_use_id"] != "toolu_a" || secondResult["tool_use_id"] != "toolu_b" {
		t.Fatalf("tool result order = %v", results)
	}
	if first["is_error"] == true || secondResult["is_error"] != true {
		t.Fatalf("is_error flags = %v, %v", first["is_error"], secondResult["is_error"])
	}

	third := decodeObject(t, reqs[2].Body)["messages"].([]any)
	if len(third) != 5 {
		t.Fatalf("third request has %d messages", len(third))
	}
	if !bytes.Contains(reqs[2].Body, []byte("sig-abc==")) {
		t.Fatal("older thinking block dropped from later history")
	}
	for i, r := range reqs {
		if n := bytes.Count(r.Body, []byte(`"cache_control"`)); n != 2 {
			t.Fatalf("request %d has %d cache breakpoints, want 2 (system and newest message)", i, n)
		}
		messages := decodeObject(t, r.Body)["messages"].([]any)
		lastBlocks := messages[len(messages)-1].(map[string]any)["content"].([]any)
		if lastBlocks[len(lastBlocks)-1].(map[string]any)["cache_control"] == nil {
			t.Fatalf("request %d: newest message has no cache breakpoint", i)
		}
		for _, m := range messages[:len(messages)-1] {
			for _, b := range m.(map[string]any)["content"].([]any) {
				if b.(map[string]any)["cache_control"] != nil {
					t.Fatalf("request %d: older message carries a cache breakpoint", i)
				}
			}
		}
	}
}

func TestOpenAIHistoryRoundTrip(t *testing.T) {
	up := newUpstream(t, replies(
		openAIFlavor.toolCalls(
			fakeCall{"call_a", "query", `{"limit":1}`},
			fakeCall{"call_b", "query", `{"limit":2}`},
		),
		openAIFlavor.endTurn("done"),
	))
	p, _ := newProvider(t, openAIFlavor, up, options{})
	session, _ := startNext(t, p, request(queryTool()))
	if _, err := session.Next(context.Background(), []llm.ToolResult{
		{CallID: "call_a", Content: "first"},
		{CallID: "call_b", Content: "boom", IsError: true},
	}); err != nil {
		t.Fatalf("Next: %v", err)
	}

	messages := decodeObject(t, up.requests()[1].Body)["messages"].([]any)
	if len(messages) != 5 {
		t.Fatalf("second request has %d messages: %v", len(messages), messages)
	}
	assistant := messages[2].(map[string]any)
	calls := assistant["tool_calls"].([]any)
	if assistant["role"] != "assistant" || len(calls) != 2 {
		t.Fatalf("assistant message = %v", assistant)
	}
	for i, id := range []string{"call_a", "call_b"} {
		call := calls[i].(map[string]any)
		if call["id"] != id || call["type"] != "function" {
			t.Fatalf("tool call %d = %v", i, call)
		}
	}
	a, b := messages[3].(map[string]any), messages[4].(map[string]any)
	if a["role"] != "tool" || a["tool_call_id"] != "call_a" || a["content"] != "first" {
		t.Fatalf("first tool message = %v", a)
	}
	if b["tool_call_id"] != "call_b" || b["content"] != "ERROR: boom" {
		t.Fatalf("second tool message = %v", b)
	}
}

func TestStopReasons(t *testing.T) {
	type tc struct {
		name    string
		body    string
		want    llm.StopReason
		wantErr bool
		calls   int
	}
	call := fakeCall{"call_1", "query", `{"limit":1}`}
	anthropicCases := []tc{
		{"end_turn", anthropicFlavor.withStop("end_turn"), llm.StopEndTurn, false, 0},
		{"stop_sequence", anthropicFlavor.withStop("stop_sequence"), llm.StopEndTurn, false, 0},
		{"max_tokens", anthropicFlavor.withStop("max_tokens"), llm.StopMaxTokens, false, 0},
		{"context window exceeded", anthropicFlavor.withStop("model_context_window_exceeded"), llm.StopMaxTokens, false, 0},
		{"refusal", anthropicFlavor.withStop("refusal"), llm.StopRefusal, false, 0},
		{"tool_use", anthropicFlavor.toolCalls(call), llm.StopToolUse, false, 1},
		{"max_tokens with tool calls", strings.Replace(anthropicFlavor.toolCalls(call), `"stop_reason":"tool_use"`, `"stop_reason":"max_tokens"`, 1), llm.StopMaxTokens, false, 1},
		{"pause_turn", anthropicFlavor.withStop("pause_turn"), "", true, 0},
		{"unknown", anthropicFlavor.withStop("something_new"), "", true, 0},
	}
	openAIMessage := func(finish, message string) string {
		return fmt.Sprintf(`{"id":"c1","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"finish_reason":%q,"message":%s}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, finish, message)
	}
	oneCall := `{"id":"call_1","type":"function","function":{"name":"query","arguments":"{\"limit\":1}"}}`
	openAICases := []tc{
		{"stop", openAIFlavor.withStop("stop"), llm.StopEndTurn, false, 0},
		{"length", openAIFlavor.withStop("length"), llm.StopMaxTokens, false, 0},
		{"content_filter", openAIFlavor.withStop("content_filter"), llm.StopRefusal, false, 0},
		{"length with tool calls", strings.Replace(openAIFlavor.toolCalls(call), `"finish_reason":"tool_calls"`, `"finish_reason":"length"`, 1), llm.StopMaxTokens, false, 1},
		{"tool_calls", openAIFlavor.toolCalls(call), llm.StopToolUse, false, 1},
		{"stop with tool calls", openAIMessage("stop", `{"role":"assistant","content":null,"tool_calls":[`+oneCall+`]}`), llm.StopToolUse, false, 1},
		{"tool_calls without calls", openAIMessage("tool_calls", `{"role":"assistant","content":"hi"}`), llm.StopEndTurn, false, 0},
		{"refusal field", openAIMessage("stop", `{"role":"assistant","content":null,"refusal":"I cannot help"}`), llm.StopRefusal, false, 0},
		{"missing tool call type", openAIMessage("tool_calls", `{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","function":{"name":"query","arguments":"{\"limit\":1}"}}]}`), llm.StopToolUse, false, 1},
		{"custom tool call", openAIMessage("tool_calls", `{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"custom","custom":{"name":"x","input":"y"}}]}`), "", true, 0},
		{"unknown", openAIFlavor.withStop("something_new"), llm.StopEndTurn, false, 0},
		{"eos", openAIFlavor.withStop("eos"), llm.StopEndTurn, false, 0},
		{"empty", openAIFlavor.withStop(""), llm.StopEndTurn, false, 0},
		{"null", strings.Replace(openAIFlavor.withStop("stop"), `"finish_reason":"stop"`, `"finish_reason":null`, 1), llm.StopEndTurn, false, 0},
		{"unknown with refusal", openAIMessage("eos", `{"role":"assistant","content":null,"refusal":"I cannot help"}`), llm.StopRefusal, false, 0},
		{"unknown with tool calls", openAIMessage("eos", `{"role":"assistant","content":null,"tool_calls":[`+oneCall+`]}`), llm.StopToolUse, false, 1},
	}
	run := func(f flavor, cases []tc) {
		for _, c := range cases {
			t.Run(f.name+"/"+c.name, func(t *testing.T) {
				up := newUpstream(t, replies(c.body, f.endTurn("after")))
				p, _ := newProvider(t, f, up, options{})
				session, err := p.Start(context.Background(), request(queryTool()))
				if err != nil {
					t.Fatalf("Start: %v", err)
				}
				turn, err := session.Next(context.Background(), nil)
				if c.wantErr {
					if !errors.Is(err, llm.ErrProvider) {
						t.Fatalf("err = %v, want ErrProvider", err)
					}
					return
				}
				if err != nil {
					t.Fatalf("Next: %v", err)
				}
				if turn.Stop != c.want || len(turn.ToolCalls) != c.calls {
					t.Fatalf("stop = %q with %d calls, want %q with %d", turn.Stop, len(turn.ToolCalls), c.want, c.calls)
				}
				if c.calls > 0 {
					if _, err := session.Next(context.Background(), []llm.ToolResult{{CallID: "call_1", Content: "ok"}}); err != nil {
						t.Fatalf("follow-up Next: %v", err)
					}
					follow := string(up.requests()[1].Body)
					if strings.Contains(follow, `"tool_calls":[{}]`) || strings.Contains(follow, `"type":""`) {
						t.Fatalf("history carries a malformed tool call: %s", follow)
					}
				}
			})
		}
	}
	run(anthropicFlavor, anthropicCases)
	run(openAIFlavor, openAICases)
}

func TestUsage(t *testing.T) {
	cases := []struct {
		f    flavor
		body string
		want llm.Usage
	}{
		{anthropicFlavor, anthropicFlavor.usage(100, 20, 30), llm.Usage{InputTokens: 100, OutputTokens: 20, CacheReadTokens: 30, CacheWriteTokens: 7, Calls: 1}},
		{openAIFlavor, openAIFlavor.usage(100, 20, 30), llm.Usage{InputTokens: 70, OutputTokens: 20, CacheReadTokens: 30, Calls: 1}},
	}
	for _, c := range cases {
		t.Run(c.f.name, func(t *testing.T) {
			up := newUpstream(t, replies(c.body))
			p, _ := newProvider(t, c.f, up, options{})
			_, turn := startNext(t, p, request())
			if turn.Usage != c.want {
				t.Fatalf("usage = %+v, want %+v", turn.Usage, c.want)
			}
		})
	}
}

func TestOpenAIToolCallArguments(t *testing.T) {
	cases := []struct {
		name, arguments, want string
	}{
		{"empty", "", `{}`},
		{"whitespace", "  \n", `{}`},
		{"object", `{"limit":1}`, `{"limit":1}`},
		{"invalid kept raw", `{"limit":`, `{"limit":`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			up := newUpstream(t, replies(openAIFlavor.toolCalls(fakeCall{"call_1", "query", c.arguments}), openAIFlavor.endTurn("done")))
			p, _ := newProvider(t, openAIFlavor, up, options{})
			session, turn := startNext(t, p, request(queryTool()))
			if got := string(turn.ToolCalls[0].Input); got != c.want {
				t.Fatalf("Input = %q, want %q", got, c.want)
			}
			if c.want == `{}` || c.name == "object" {
				if _, err := json.Marshal(turn); err != nil {
					t.Fatalf("turn does not marshal: %v", err)
				}
			}
			if _, err := session.Next(context.Background(), []llm.ToolResult{{CallID: "call_1", Content: "ok"}}); err != nil {
				t.Fatalf("Next: %v", err)
			}
			messages := decodeObject(t, up.requests()[1].Body)["messages"].([]any)
			sent := messages[2].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)["arguments"]
			if sent != c.want {
				t.Fatalf("history arguments = %v, want %q", sent, c.want)
			}
		})
	}
}

func TestEmptyToolsAndSystemAreOmitted(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			up := newUpstream(t, replies(f.endTurn("ok")))
			p, _ := newProvider(t, f, up, options{})
			startNext(t, p, llm.Request{Prompt: "hello"})
			fields := decodeFields(t, up.requests()[0].Body)
			if _, ok := fields["tools"]; ok {
				t.Fatalf("empty tool list was sent: %s", fields["tools"])
			}
			if _, ok := fields["system"]; ok {
				t.Fatalf("empty system was sent: %s", fields["system"])
			}
			if f.name == config.LLMProviderOpenAI {
				messages := decodeObject(t, up.requests()[0].Body)["messages"].([]any)
				if len(messages) != 1 || messages[0].(map[string]any)["role"] != "user" {
					t.Fatalf("messages = %v, want a single user message", messages)
				}
			}
		})
	}
}

func TestInvalidToolSchemasAreRejectedAtStart(t *testing.T) {
	schemas := map[string]string{
		"not an object":    `[1]`,
		"wrong type":       `{"type":"string"}`,
		"trailing object":  `{"type":"object"} {"extra":1}`,
		"trailing garbage": `{"type":"object"} x`,
		"bad properties":   `{"type":"object","properties":[]}`,
		"bad required":     `{"type":"object","required":[1]}`,
	}
	for _, f := range flavors {
		for name, schema := range schemas {
			t.Run(f.name+"/"+name, func(t *testing.T) {
				up := newUpstream(t, replies(f.endTurn("ok")))
				p, _ := newProvider(t, f, up, options{})
				_, err := p.Start(context.Background(), request(llm.ToolSpec{Name: "query", InputSchema: json.RawMessage(schema)}))
				if err == nil {
					t.Fatal("Start accepted an invalid schema")
				}
				if n := len(up.requests()); n != 0 {
					t.Fatalf("%d requests sent", n)
				}
			})
		}
	}
}

func TestSessionRejectsMismatchedResultsWithoutCallingTheModel(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			up := newUpstream(t, replies(f.toolCalls(fakeCall{"call_1", "query", `{"limit":1}`}), f.endTurn("done")))
			p, _ := newProvider(t, f, up, options{})
			session, _ := startNext(t, p, request(queryTool()))
			for name, results := range map[string][]llm.ToolResult{
				"none":  nil,
				"extra": {{CallID: "call_1"}, {CallID: "call_2"}},
				"wrong": {{CallID: "other"}},
			} {
				if _, err := session.Next(context.Background(), results); err == nil {
					t.Fatalf("%s: Next accepted mismatched results", name)
				}
			}
			if n := len(up.requests()); n != 1 {
				t.Fatalf("%d requests sent, want 1", n)
			}
		})
	}
}

func TestUpstreamErrorBodyNeverLeaks(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			up := newUpstream(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set(f.requestID, "req_0123abc")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, f.errorBody("invalid key "+sentinel+" for "+systemText))
			})
			p, logs := newProvider(t, f, up, options{})
			session, err := p.Start(context.Background(), request(queryTool()))
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			_, err = session.Next(context.Background(), nil)
			if !errors.Is(err, llm.ErrProvider) {
				t.Fatalf("err = %v, want ErrProvider", err)
			}
			if !strings.Contains(err.Error(), "401") {
				t.Fatalf("error lacks the status: %v", err)
			}
			logged := logs.String()
			for _, secret := range []string{sentinel, "invalid key", systemText, "show errors", up.srv.URL, "127.0.0.1"} {
				if strings.Contains(err.Error(), secret) || strings.Contains(logged, secret) {
					t.Fatalf("%q leaked\nerror: %v\nlog: %s", secret, err, logged)
				}
			}
			for _, want := range []string{"status=401", "request_id=req_0123abc", "error_type=" + errorType(f)} {
				if !strings.Contains(logged, want) {
					t.Fatalf("log lacks %q: %s", want, logged)
				}
			}
		})
	}
}

func errorType(f flavor) string {
	if f.name == config.LLMProviderAnthropic {
		return "authentication_error"
	}
	return "invalid_request_error"
}

func TestRequestTimeoutAndCancellation(t *testing.T) {
	block := func(_ int, _ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}
	for _, f := range flavors {
		t.Run(f.name+"/our timeout", func(t *testing.T) {
			up := newUpstream(t, block)
			p, logs := newProvider(t, f, up, options{timeout: 200 * time.Millisecond})
			session, _ := p.Start(context.Background(), request())
			_, err := session.Next(context.Background(), nil)
			if !errors.Is(err, llm.ErrProvider) || !strings.Contains(err.Error(), "timed out") {
				t.Fatalf("err = %v, want an ErrProvider timeout", err)
			}
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				t.Fatalf("our own timeout must not look like a caller context error: %v", err)
			}
			if !strings.Contains(logs.String(), "timed out") {
				t.Fatalf("log = %s", logs.String())
			}
		})
		t.Run(f.name+"/caller deadline", func(t *testing.T) {
			up := newUpstream(t, block)
			p, _ := newProvider(t, f, up, options{timeout: 5 * time.Second})
			session, _ := p.Start(context.Background(), request())
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			_, err := session.Next(ctx, nil)
			if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, llm.ErrProvider) {
				t.Fatalf("err = %v, want the caller's deadline", err)
			}
		})
		t.Run(f.name+"/caller cancel", func(t *testing.T) {
			up := newUpstream(t, block)
			p, _ := newProvider(t, f, up, options{timeout: 5 * time.Second})
			session, _ := p.Start(context.Background(), request())
			ctx, cancel := context.WithCancel(context.Background())
			time.AfterFunc(100*time.Millisecond, cancel)
			_, err := session.Next(ctx, nil)
			if !errors.Is(err, context.Canceled) || errors.Is(err, llm.ErrProvider) {
				t.Fatalf("err = %v, want the caller's cancellation", err)
			}
		})
		t.Run(f.name+"/timeout bounds retries", func(t *testing.T) {
			up := newUpstream(t, func(_ int, w http.ResponseWriter, r *http.Request) {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(300 * time.Millisecond):
				}
				w.Header().Set("retry-after-ms", "200")
				w.WriteHeader(http.StatusServiceUnavailable)
			})
			p, _ := newProvider(t, f, up, options{timeout: 700 * time.Millisecond})
			session, _ := p.Start(context.Background(), request())
			start := time.Now()
			_, err := session.Next(context.Background(), nil)
			elapsed := time.Since(start)
			if !errors.Is(err, llm.ErrProvider) || !strings.Contains(err.Error(), "timed out") {
				t.Fatalf("err = %v after %s, want an ErrProvider timeout", err, elapsed)
			}
			if elapsed > 1100*time.Millisecond {
				t.Fatalf("Next took %s with a 700ms request timeout", elapsed)
			}
			if n := len(up.requests()); n < 2 {
				t.Fatalf("%d attempts, want the SDK to retry inside the budget", n)
			}
		})
		t.Run(f.name+"/retries inside the budget", func(t *testing.T) {
			up := newUpstream(t, func(n int, w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if n == 1 {
					w.Header().Set("retry-after-ms", "10")
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}
				_, _ = io.WriteString(w, f.endTurn("recovered"))
			})
			p, _ := newProvider(t, f, up, options{timeout: 3 * time.Second})
			_, turn := startNext(t, p, request())
			if turn.Text != "recovered" || len(up.requests()) != 2 {
				t.Fatalf("turn = %+v after %d requests", turn, len(up.requests()))
			}
		})
	}
}

func TestProviderEnvironmentVariablesAreIgnored(t *testing.T) {
	decoy := newUpstream(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		t.Error("a request reached the host named by a provider environment variable")
		http.Error(w, "decoy", http.StatusTeapot)
	})
	for _, name := range []string{
		"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL",
		"OPENAI_API_KEY", "OPENAI_BASE_URL", "OPENAI_ORG_ID", "OPENAI_PROJECT_ID",
	} {
		value := "env-" + strings.ToLower(name)
		if strings.HasSuffix(name, "_BASE_URL") {
			value = decoy.srv.URL
		}
		t.Setenv(name, value)
	}
	t.Run("anthropic", func(t *testing.T) {
		up := newUpstream(t, replies(anthropicFlavor.endTurn("ok")))
		p, _ := newProvider(t, anthropicFlavor, up, options{})
		startNext(t, p, request())
		h := up.requests()[0].Header
		if h.Get("X-Api-Key") != sentinel || h.Get("Authorization") != "" {
			t.Fatalf("x-api-key = %q, authorization = %q", h.Get("X-Api-Key"), h.Get("Authorization"))
		}
	})
	t.Run("openai", func(t *testing.T) {
		up := newUpstream(t, replies(openAIFlavor.endTurn("ok"), openAIFlavor.endTurn("ok")))
		p, _ := newProvider(t, openAIFlavor, up, options{})
		startNext(t, p, request())
		empty := ""
		p, _ = newProvider(t, openAIFlavor, up, options{key: &empty})
		startNext(t, p, request())

		reqs := up.requests()
		if got := reqs[0].Header.Get("Authorization"); got != "Bearer "+sentinel {
			t.Fatalf("Authorization = %q, want the configured key", got)
		}
		if got := reqs[1].Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization = %q with no configured key, want none", got)
		}
		for _, r := range reqs {
			for _, h := range []string{"OpenAI-Organization", "OpenAI-Project"} {
				if r.Header.Get(h) != "" {
					t.Fatalf("header %s = %q, want none", h, r.Header.Get(h))
				}
			}
		}
	})
}
