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
	"os"
	"path/filepath"
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
		writeReply(w, bodies[n-1])
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
	timeout   time.Duration
	idle      time.Duration
	noTimeout bool
	maxTokens int
	effort    string
	key       *string
	logs      *logBuffer

	logProviderErrors bool
}

func newProvider(t *testing.T, f flavor, up *upstream, opts options) (llm.Provider, *logBuffer) {
	t.Helper()
	if opts.timeout == 0 && !opts.noTimeout {
		opts.timeout = 5 * time.Second
	}
	if opts.maxTokens == 0 {
		opts.maxTokens = 2048
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
		Effort:         opts.effort,
		MaxTokens:      opts.maxTokens,
		RequestTimeout: opts.timeout,
		IdleTimeout:    opts.idle,

		LogProviderErrors: opts.logProviderErrors,
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
				writeReply(w, f.endTurn("ok"))
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
		"model":"test-model","max_tokens":2048,"stream":true,
		"output_config":{"effort":"high"},
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
		"model":"test-model","max_completion_tokens":2048,"stream":true,"stream_options":{"include_usage":true},
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

func TestReasoningEffortReachesTheProviderOnlyWhenSet(t *testing.T) {
	tests := []struct {
		name   string
		flavor flavor
		effort string
		want   string
	}{
		{"anthropic unset defaults to medium and the planning call steps it up", anthropicFlavor, "", `"output_config":{"effort":"high"}`},
		{"anthropic low steps up to medium", anthropicFlavor, "low", `"output_config":{"effort":"medium"}`},
		{"anthropic high stays", anthropicFlavor, "high", `"output_config":{"effort":"high"}`},
		{"openai low steps up to medium", openAIFlavor, "low", `"reasoning_effort":"medium"`},
		{"openai medium steps up to high", openAIFlavor, "medium", `"reasoning_effort":"high"`},
		{"openai none stays", openAIFlavor, "none", `"reasoning_effort":"none"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := newUpstream(t, replies(tt.flavor.endTurn("ok")))
			p, _ := newProvider(t, tt.flavor, up, options{effort: tt.effort})
			startNext(t, p, request(queryTool()))
			if body := up.requests()[0].Body; !bytes.Contains(body, []byte(tt.want)) {
				t.Fatalf("request body lacks %s: %s", tt.want, body)
			}
		})
	}

	t.Run("openai unset sends no effort", func(t *testing.T) {
		up := newUpstream(t, replies(openAIFlavor.endTurn("ok")))
		p, _ := newProvider(t, openAIFlavor, up, options{})
		startNext(t, p, request(queryTool()))
		if body := up.requests()[0].Body; bytes.Contains(body, []byte("reasoning_effort")) {
			t.Fatalf("an unset effort must send nothing: %s", body)
		}
	})
}

func tuningOf(t *testing.T, f flavor, body []byte) (effort string, maxTokens int) {
	t.Helper()
	fields := decodeFields(t, body)
	if f.name == config.LLMProviderAnthropic {
		var cfg struct {
			Effort string `json:"effort"`
		}
		if err := json.Unmarshal(fields["output_config"], &cfg); err != nil {
			t.Fatalf("output_config: %v", err)
		}
		if err := json.Unmarshal(fields["max_tokens"], &maxTokens); err != nil {
			t.Fatalf("max_tokens: %v", err)
		}
		return cfg.Effort, maxTokens
	}
	if raw, ok := fields["reasoning_effort"]; ok {
		if err := json.Unmarshal(raw, &effort); err != nil {
			t.Fatalf("reasoning_effort: %v", err)
		}
	}
	if err := json.Unmarshal(fields["max_completion_tokens"], &maxTokens); err != nil {
		t.Fatalf("max_completion_tokens: %v", err)
	}
	return effort, maxTokens
}

func TestThePlanningCallRaisesEffortAndLaterCallsLowerMaxTokens(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			up := newUpstream(t, replies(
				f.toolCalls(fakeCall{"call_1", "query", `{"limit":1}`}),
				f.endTurn("done"),
				f.endTurn("resumed"),
			))
			p, _ := newProvider(t, f, up, options{effort: "medium", maxTokens: 16000})
			session, _ := startNext(t, p, request(queryTool()))
			if _, err := session.Next(context.Background(), []llm.ToolResult{{CallID: "call_1", Content: "rows"}}); err != nil {
				t.Fatalf("Next: %v", err)
			}
			resumedRequest := request(queryTool())
			resumedRequest.BuildSaves = 3
			startNext(t, p, resumedRequest)

			want := []struct {
				effort string
				max    int
			}{{"high", 16000}, {"medium", 8192}, {"medium", 8192}}
			reqs := up.requests()
			if len(reqs) != len(want) {
				t.Fatalf("%d requests, want %d", len(reqs), len(want))
			}
			for i, w := range want {
				effort, maxTokens := tuningOf(t, f, reqs[i].Body)
				if effort != w.effort || maxTokens != w.max {
					t.Errorf("request %d: effort %q and max tokens %d, want %q and %d", i, effort, maxTokens, w.effort, w.max)
				}
			}
		})
	}
}

func TestACutOffAtTheLoweredCapIsResentAtTheConfiguredCap(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			stop := map[string]string{config.LLMProviderAnthropic: "max_tokens", config.LLMProviderOpenAI: "length"}[f.name]
			up := newUpstream(t, replies(
				f.toolCalls(fakeCall{"call_1", "query", `{"limit":1}`}),
				f.withStop(stop),
				f.endTurn("complete"),
			))
			p, _ := newProvider(t, f, up, options{effort: "medium", maxTokens: 16000})
			ctx, trace := llm.WithCallTrace(context.Background())
			session, err := p.Start(ctx, request(queryTool()))
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			if _, err := session.Next(ctx, nil); err != nil {
				t.Fatalf("Next: %v", err)
			}
			*trace = llm.CallTrace{}
			turn, err := session.Next(ctx, []llm.ToolResult{{CallID: "call_1", Content: "rows"}})
			if err != nil || turn.Text != "complete" || turn.Stop != llm.StopEndTurn {
				t.Fatalf("turn = %+v, err %v, want the resent answer", turn, err)
			}
			if trace.Attempts != 2 || turn.Usage.Calls != 1 || turn.Usage.OutputTokens != 2 {
				t.Fatalf("attempts = %d, usage = %+v, want 2 attempts and one call carrying both calls' tokens", trace.Attempts, turn.Usage)
			}
			reqs := up.requests()
			if len(reqs) != 3 {
				t.Fatalf("%d requests, want 3", len(reqs))
			}
			lowered, loweredMax := tuningOf(t, f, reqs[1].Body)
			resent, resentMax := tuningOf(t, f, reqs[2].Body)
			if loweredMax != 8192 || resentMax != 16000 || lowered != resent {
				t.Fatalf("lowered %q/%d and resent %q/%d, want 8192 then 16000 at the same effort", lowered, loweredMax, resent, resentMax)
			}
			if !bytes.Equal(decodeFields(t, reqs[1].Body)["messages"], decodeFields(t, reqs[2].Body)["messages"]) {
				t.Fatal("the resend sent different messages")
			}
		})
	}

	t.Run("a cut-off at the configured cap is returned as it is", func(t *testing.T) {
		up := newUpstream(t, replies(openAIFlavor.withStop("length")))
		p, _ := newProvider(t, openAIFlavor, up, options{maxTokens: 16000})
		_, turn := startNext(t, p, request())
		if turn.Stop != llm.StopMaxTokens || len(up.requests()) != 1 {
			t.Fatalf("stop %q after %d requests, want max_tokens after one", turn.Stop, len(up.requests()))
		}
	})
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
				if c.calls > 0 && c.want != llm.StopMaxTokens {
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

func TestAMaxTokensTurnIsNotKeptInHistory(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			up := newUpstream(t, replies(
				f.toolCalls(fakeCall{"call_1", "query", `{"limit":1}`}),
				f.withStop(map[string]string{config.LLMProviderAnthropic: "max_tokens", config.LLMProviderOpenAI: "length"}[f.name]),
				f.endTurn("done"),
			))
			p, _ := newProvider(t, f, up, options{})
			session, _ := startNext(t, p, request(queryTool()))
			results := []llm.ToolResult{{CallID: "call_1", Content: "rows"}}
			cut, err := session.Next(context.Background(), results)
			if err != nil || cut.Stop != llm.StopMaxTokens {
				t.Fatalf("cut turn = %+v, err %v", cut, err)
			}
			if _, err := session.Next(context.Background(), results); err != nil {
				t.Fatalf("Next after the cut-off: %v", err)
			}
			reqs := up.requests()
			if !bytes.Equal(reqs[1].Body, reqs[2].Body) {
				t.Fatalf("the call after a cut-off sent different messages:\n%s\n%s", reqs[1].Body, reqs[2].Body)
			}
		})
	}
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
			var callErr *llm.CallError
			if !errors.As(err, &callErr) {
				t.Fatalf("err = %T, want a CallError", err)
			}
			want := llm.CallError{Failure: llm.ProviderError, Provider: f.name, Status: 401, Type: errorType(f), Code: errorCode(f), RequestID: "req_0123abc"}
			got := *callErr
			if got.Failure != want.Failure || got.Provider != want.Provider || got.Status != want.Status || got.Type != want.Type || got.Code != want.Code || got.Param != "" || got.RequestID != want.RequestID {
				t.Fatalf("CallError = %+v, want %+v", got, want)
			}
			logged := logs.String()
			for _, secret := range []string{sentinel, "invalid key", systemText, "show errors", up.srv.URL, "127.0.0.1"} {
				if strings.Contains(err.Error(), secret) || strings.Contains(logged, secret) || strings.Contains(fmt.Sprintf("%+v", got), secret) {
					t.Fatalf("%q leaked\nerror: %v\nlog: %s", secret, err, logged)
				}
			}
		})
	}
}

func failedCall(t *testing.T, f flavor, message string, opts options) (*llm.CallError, *logBuffer) {
	t.Helper()
	up := newUpstream(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, f.errorBody(message))
	})
	p, logs := newProvider(t, f, up, opts)
	session, err := p.Start(context.Background(), request(queryTool()))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, err = session.Next(context.Background(), nil)
	var callErr *llm.CallError
	if !errors.As(err, &callErr) {
		t.Fatalf("err = %v, want a CallError", err)
	}
	return callErr, logs
}

func TestProviderErrorMessageIsEmptyWhenLoggingIsOff(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			callErr, _ := failedCall(t, f, "echoed "+systemText+" "+sentinel, options{})
			if callErr.ProviderMessage != "" {
				t.Fatalf("ProviderMessage = %q, want empty", callErr.ProviderMessage)
			}
		})
	}
}

func TestProviderErrorMessageIsScrubbedWhenLoggingIsOn(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			callErr, logs := failedCall(t, f, "rejected "+sentinel+" near "+systemText+"\nsecond line", options{logProviderErrors: true})
			want := "rejected [redacted] near " + systemText + " second line"
			if callErr.ProviderMessage != want {
				t.Fatalf("ProviderMessage = %q, want %q", callErr.ProviderMessage, want)
			}
			for _, text := range []string{callErr.Error(), fmt.Sprintf("%+v", callErr.Error()), logs.String()} {
				if strings.Contains(text, systemText) || strings.Contains(text, sentinel) {
					t.Fatalf("the provider message leaked into %q", text)
				}
			}
		})
	}
}

func TestProviderErrorMessageIsEmptyWhenTheBodyHasNone(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			up := newUpstream(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "plain text "+sentinel, http.StatusBadRequest)
			})
			p, _ := newProvider(t, f, up, options{logProviderErrors: true})
			session, _ := p.Start(context.Background(), request())
			_, err := session.Next(context.Background(), nil)
			var callErr *llm.CallError
			if !errors.As(err, &callErr) {
				t.Fatalf("err = %v, want a CallError", err)
			}
			if callErr.ProviderMessage != "" {
				t.Fatalf("ProviderMessage = %q, want empty", callErr.ProviderMessage)
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

func errorCode(f flavor) string {
	if f.name == config.LLMProviderAnthropic {
		return ""
	}
	return "invalid_api_key"
}

func TestProviderFailuresAreTypedAndAttemptsCounted(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		failure      llm.CallFailure
		wantAttempts int
	}{
		{"rate limited after retries", http.StatusTooManyRequests, llm.RateLimited, 4},
		{"unavailable after retries", http.StatusServiceUnavailable, llm.ProviderError, 4},
		{"bad request is not retried", http.StatusBadRequest, llm.ProviderError, 1},
	}
	for _, f := range flavors {
		for _, tc := range cases {
			t.Run(f.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				up := newUpstream(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Retry-After", "1")
					w.WriteHeader(tc.status)
					_, _ = io.WriteString(w, f.errorBody("provider says "+sentinel))
				})
				p, _ := newProvider(t, f, up, options{timeout: 30 * time.Second})
				ctx, trace := llm.WithCallTrace(context.Background())
				session, _ := p.Start(ctx, request())
				_, err := session.Next(ctx, nil)
				var callErr *llm.CallError
				if !errors.As(err, &callErr) {
					t.Fatalf("err = %v, want a CallError", err)
				}
				if callErr.Failure != tc.failure || callErr.Status != tc.status {
					t.Fatalf("CallError = %+v, want %s with status %d", callErr, tc.failure, tc.status)
				}
				if trace.Attempts != tc.wantAttempts || len(up.requests()) != tc.wantAttempts {
					t.Fatalf("attempts = %d, requests = %d, want %d", trace.Attempts, len(up.requests()), tc.wantAttempts)
				}
				if trace.Status != tc.status || trace.RetryAfter != "1" {
					t.Fatalf("trace = %+v, want status %d and retry-after 1", *trace, tc.status)
				}
				if callErr.RetryAfter != time.Second {
					t.Fatalf("CallError.RetryAfter = %s, want 1s", callErr.RetryAfter)
				}
			})
		}
	}
}

func TestASilentUpstreamFailsTheAttemptAtTheIdleTimeout(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			t.Parallel()
			silent := make(chan time.Duration, 8)
			up := newUpstream(t, func(_ int, w http.ResponseWriter, r *http.Request) {
				start := time.Now()
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				silent <- time.Since(start)
			})
			p, logs := newProvider(t, f, up, options{timeout: 30 * time.Second, idle: 200 * time.Millisecond})
			session, _ := p.Start(context.Background(), request())
			_, err := session.Next(context.Background(), nil)
			var callErr *llm.CallError
			if !errors.As(err, &callErr) || callErr.Failure != llm.CallTimeout || !callErr.Idle {
				t.Fatalf("err = %v, want an idle call_timeout", err)
			}
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				t.Fatalf("an idle timeout must not look like a caller context error: %v", err)
			}
			for i := range 4 {
				select {
				case took := <-silent:
					if took < 150*time.Millisecond || took > 2*time.Second {
						t.Fatalf("attempt %d was cut after %s, want about the 200ms idle timeout", i, took)
					}
				case <-time.After(5 * time.Second):
					t.Fatalf("attempt %d never ended on the upstream side, want 4 attempts", i)
				}
			}
			if n := len(up.requests()); n != 4 {
				t.Fatalf("%d requests, want 4", n)
			}
			if strings.Contains(logs.String(), sentinel) {
				t.Fatal("the api key leaked into the logs")
			}
		})
	}
}

func TestASlowButStreamingUpstreamIsNotIdle(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			up := newUpstream(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				pieces := []string{": keep-alive\n\n", ": keep-alive\n\n", ": keep-alive\n\n", ": keep-alive\n\n", ": keep-alive\n\n", ": keep-alive\n\n"}
				pieces = append(pieces, strings.SplitAfter(streamOf(f.endTurn("slow")), "\n\n")...)
				for _, piece := range pieces {
					_, _ = io.WriteString(w, piece)
					w.(http.Flusher).Flush()
					time.Sleep(100 * time.Millisecond)
				}
			})
			p, _ := newProvider(t, f, up, options{idle: 300 * time.Millisecond})
			start := time.Now()
			_, turn := startNext(t, p, request())
			if turn.Text != "slow" {
				t.Fatalf("turn = %+v", turn)
			}
			if took := time.Since(start); took < 900*time.Millisecond {
				t.Fatalf("the stream finished in %s, too fast to prove the watch stayed quiet", took)
			}
		})
	}
}

func TestAnIdleFirstAttemptIsRetriedAndSucceeds(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			up := newUpstream(t, func(n int, w http.ResponseWriter, r *http.Request) {
				if n == 1 {
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
					<-r.Context().Done()
					return
				}
				writeReply(w, f.endTurn("second try"))
			})
			p, _ := newProvider(t, f, up, options{idle: 200 * time.Millisecond})
			ctx, trace := llm.WithCallTrace(context.Background())
			session, _ := p.Start(ctx, request())
			turn, err := session.Next(ctx, nil)
			if err != nil || turn.Text != "second try" {
				t.Fatalf("turn = %+v, err %v", turn, err)
			}
			if trace.Attempts != 2 || len(up.requests()) != 2 {
				t.Fatalf("attempts = %d, requests = %d, want 2", trace.Attempts, len(up.requests()))
			}
		})
	}
}

func TestABadRequestIsNotRetried(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			up := newUpstream(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, f.errorBody("nope"))
			})
			p, _ := newProvider(t, f, up, options{})
			ctx, trace := llm.WithCallTrace(context.Background())
			session, _ := p.Start(ctx, request())
			start := time.Now()
			if _, err := session.Next(ctx, nil); err == nil {
				t.Fatal("Next accepted a 400")
			}
			if trace.Attempts != 1 || len(up.requests()) != 1 || time.Since(start) > 400*time.Millisecond {
				t.Fatalf("attempts = %d, requests = %d after %s, want one attempt and no wait", trace.Attempts, len(up.requests()), time.Since(start))
			}
		})
	}
}

func TestCallTraceCountsAttemptsOfASuccessfulCall(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			up := newUpstream(t, func(n int, w http.ResponseWriter, _ *http.Request) {
				if n == 1 {
					w.WriteHeader(http.StatusBadGateway)
					return
				}
				writeReply(w, f.endTurn("ok"))
			})
			p, _ := newProvider(t, f, up, options{})
			ctx, trace := llm.WithCallTrace(context.Background())
			session, _ := p.Start(ctx, request())
			if _, err := session.Next(ctx, nil); err != nil {
				t.Fatalf("Next: %v", err)
			}
			if trace.Attempts != 2 || trace.Status != http.StatusOK || trace.RetryAfter != "" {
				t.Fatalf("trace = %+v, want 2 attempts ending in 200", *trace)
			}
		})
	}
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
			p, _ := newProvider(t, f, up, options{timeout: 200 * time.Millisecond})
			ctx, trace := llm.WithCallTrace(context.Background())
			session, _ := p.Start(ctx, request())
			_, err := session.Next(ctx, nil)
			if !errors.Is(err, llm.ErrProvider) || !strings.Contains(err.Error(), "timed out") {
				t.Fatalf("err = %v, want an ErrProvider timeout", err)
			}
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				t.Fatalf("our own timeout must not look like a caller context error: %v", err)
			}
			var callErr *llm.CallError
			if !errors.As(err, &callErr) || callErr.Failure != llm.CallTimeout {
				t.Fatalf("err = %v, want a call_timeout CallError", err)
			}
			if trace.Attempts != 1 {
				t.Fatalf("attempts = %d, want 1", trace.Attempts)
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
				w.WriteHeader(http.StatusServiceUnavailable)
			})
			p, _ := newProvider(t, f, up, options{timeout: 700 * time.Millisecond})
			session, _ := p.Start(context.Background(), request())
			start := time.Now()
			_, err := session.Next(context.Background(), nil)
			elapsed := time.Since(start)
			var callErr *llm.CallError
			if !errors.As(err, &callErr) || callErr.Status != http.StatusServiceUnavailable {
				t.Fatalf("err = %v after %s, want the last attempt's 503", err, elapsed)
			}
			if elapsed > 1100*time.Millisecond {
				t.Fatalf("Next took %s with a 700ms request timeout", elapsed)
			}
			if n := len(up.requests()); n != 1 {
				t.Fatalf("%d attempts, want one: the next wait would pass the request timeout", n)
			}
		})
		t.Run(f.name+"/retries inside the budget", func(t *testing.T) {
			up := newUpstream(t, func(n int, w http.ResponseWriter, _ *http.Request) {
				if n == 1 {
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}
				writeReply(w, f.endTurn("recovered"))
			})
			p, _ := newProvider(t, f, up, options{timeout: 3 * time.Second})
			_, turn := startNext(t, p, request())
			if turn.Text != "recovered" || len(up.requests()) != 2 {
				t.Fatalf("turn = %+v after %d requests", turn, len(up.requests()))
			}
		})
	}
}

func TestZeroRequestTimeoutSetsNoDeadline(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name+"/slow call with a large output succeeds", func(t *testing.T) {
			up := newUpstream(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
				time.Sleep(300 * time.Millisecond)
				writeReply(w, f.endTurn("done"))
			})
			p, _ := newProvider(t, f, up, options{noTimeout: true, maxTokens: 64000})
			_, turn := startNext(t, p, request())
			if turn.Text != "done" {
				t.Fatalf("turn = %+v", turn)
			}
		})
		t.Run(f.name+"/caller cancel still ends the call", func(t *testing.T) {
			up := newUpstream(t, func(_ int, _ http.ResponseWriter, r *http.Request) {
				<-r.Context().Done()
			})
			p, _ := newProvider(t, f, up, options{noTimeout: true})
			session, _ := p.Start(context.Background(), request())
			ctx, cancel := context.WithCancel(context.Background())
			time.AfterFunc(100*time.Millisecond, cancel)
			_, err := session.Next(ctx, nil)
			if !errors.Is(err, context.Canceled) || errors.Is(err, llm.ErrProvider) {
				t.Fatalf("err = %v, want the caller's cancellation", err)
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

func TestReplayProviderCapsOutputAtTheConfiguredMaxTokens(t *testing.T) {
	dir := t.TempDir()
	input := `{"v":"` + strings.Repeat("a", 3192) + `"}`
	fixture := `{"prompt":"show errors","turns":[{"tool_calls":[{"name":"t","input":` + input + `}]}]}`
	if err := os.WriteFile(filepath.Join(dir, "f.json"), []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		maxTokens int
		wantStop  llm.StopReason
	}{{999, llm.StopMaxTokens}, {1000, llm.StopToolUse}} {
		provider, err := providers.New(config.LLM{Provider: config.LLMProviderReplay, ReplayDir: dir, MaxTokens: c.maxTokens}, slog.New(slog.DiscardHandler))
		if err != nil {
			t.Fatalf("providers.New: %v", err)
		}
		session, err := provider.Start(t.Context(), llm.Request{Prompt: "show errors"})
		if err != nil {
			t.Fatal(err)
		}
		turn, err := session.Next(t.Context(), nil)
		if err != nil || turn.Stop != c.wantStop {
			t.Fatalf("max tokens %d: turn stop = %q, err %v, want %q", c.maxTokens, turn.Stop, err, c.wantStop)
		}
	}
}

func writeReply(w http.ResponseWriter, body string) {
	stream := streamOf(body)
	if stream == "" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = io.WriteString(w, stream)
}

func streamOf(body string) string {
	switch {
	case strings.Contains(body, `"object":"chat.completion"`):
		return openAIStream(body)
	case strings.Contains(body, `"type":"message","role":"assistant"`):
		return anthropicStream(body)
	}
	return ""
}

func halves(text string) []string {
	runes := []rune(text)
	if len(runes) < 2 {
		return []string{text}
	}
	return []string{string(runes[:len(runes)/2]), string(runes[len(runes)/2:])}
}

func mustMarshal(v any) string {
	out, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(out)
}

func mustUnquote(raw json.RawMessage) string {
	var out string
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(err)
	}
	return out
}

type sseBody struct{ strings.Builder }

func (b *sseBody) event(name string, data any) {
	fmt.Fprintf(&b.Builder, "event: %s\ndata: %s\n\n", name, mustMarshal(data))
}

func (b *sseBody) data(data any) {
	fmt.Fprintf(&b.Builder, "data: %s\n\n", mustMarshal(data))
}

func anthropicStream(body string) string {
	var m struct {
		ID         string                       `json:"id"`
		Model      string                       `json:"model"`
		Content    []map[string]json.RawMessage `json:"content"`
		StopReason string                       `json:"stop_reason"`
		Usage      map[string]any               `json:"usage"`
	}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		panic(err)
	}
	var out sseBody
	startUsage := map[string]any{}
	for k, v := range m.Usage {
		startUsage[k] = v
	}
	startUsage["output_tokens"] = 1
	out.event("message_start", map[string]any{"type": "message_start", "message": map[string]any{
		"id": m.ID, "type": "message", "role": "assistant", "model": m.Model,
		"content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": startUsage,
	}})
	for i, block := range m.Content {
		started := map[string]any{}
		for k, v := range block {
			started[k] = v
		}
		var deltas []map[string]any
		switch mustUnquote(block["type"]) {
		case "text":
			started["text"] = ""
			for _, piece := range halves(mustUnquote(block["text"])) {
				deltas = append(deltas, map[string]any{"type": "text_delta", "text": piece})
			}
		case "thinking":
			started["thinking"], started["signature"] = "", ""
			for _, piece := range halves(mustUnquote(block["thinking"])) {
				deltas = append(deltas, map[string]any{"type": "thinking_delta", "thinking": piece})
			}
			for _, piece := range halves(mustUnquote(block["signature"])) {
				deltas = append(deltas, map[string]any{"type": "signature_delta", "signature": piece})
			}
		case "tool_use":
			started["input"] = map[string]any{}
			for _, piece := range halves(string(block["input"])) {
				deltas = append(deltas, map[string]any{"type": "input_json_delta", "partial_json": piece})
			}
		}
		out.event("content_block_start", map[string]any{"type": "content_block_start", "index": i, "content_block": started})
		for _, d := range deltas {
			out.event("content_block_delta", map[string]any{"type": "content_block_delta", "index": i, "delta": d})
		}
		out.event("content_block_stop", map[string]any{"type": "content_block_stop", "index": i})
	}
	out.event("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": m.StopReason, "stop_sequence": nil},
		"usage": map[string]any{"output_tokens": m.Usage["output_tokens"]},
	})
	out.event("message_stop", map[string]any{"type": "message_stop"})
	return out.String()
}

func openAIStream(body string) string {
	var m struct {
		ID      string `json:"id"`
		Created int    `json:"created"`
		Model   string `json:"model"`
		Choices []struct {
			FinishReason *string `json:"finish_reason"`
			Message      struct {
				Content   *string                      `json:"content"`
				Refusal   string                       `json:"refusal"`
				ToolCalls []map[string]json.RawMessage `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		panic(err)
	}
	chunk := func(choices []any) map[string]any {
		return map[string]any{"id": m.ID, "object": "chat.completion.chunk", "created": m.Created, "model": m.Model, "choices": choices}
	}
	delta := func(d any, finish any) map[string]any {
		return chunk([]any{map[string]any{"index": 0, "delta": d, "finish_reason": finish}})
	}
	var out sseBody
	choice := m.Choices[0]
	out.data(delta(map[string]any{"role": "assistant"}, nil))
	if choice.Message.Content != nil {
		for _, piece := range halves(*choice.Message.Content) {
			out.data(delta(map[string]any{"content": piece}, nil))
		}
	}
	if choice.Message.Refusal != "" {
		out.data(delta(map[string]any{"refusal": choice.Message.Refusal}, nil))
	}
	for i, call := range choice.Message.ToolCalls {
		head := map[string]any{"index": i}
		for _, k := range []string{"id", "type"} {
			if v, ok := call[k]; ok {
				head[k] = v
			}
		}
		var function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}
		if raw, ok := call["function"]; ok {
			if err := json.Unmarshal(raw, &function); err != nil {
				panic(err)
			}
			head["function"] = map[string]any{"name": function.Name, "arguments": ""}
		}
		out.data(delta(map[string]any{"tool_calls": []any{head}}, nil))
		if function.Arguments == "" {
			continue
		}
		for _, piece := range halves(function.Arguments) {
			out.data(delta(map[string]any{"tool_calls": []any{map[string]any{"index": i, "function": map[string]any{"arguments": piece}}}}, nil))
		}
	}
	var finish any
	if choice.FinishReason != nil {
		finish = *choice.FinishReason
	}
	out.data(delta(map[string]any{}, finish))
	final := chunk([]any{})
	final["usage"] = m.Usage
	out.data(final)
	_, _ = out.WriteString("data: [DONE]\n\n")
	return out.String()
}

func sentMessages(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, m := range decodeObject(t, body)["messages"].([]any) {
		out = append(out, m.(map[string]any))
	}
	return out
}

func TestAnthropicCompactKeepsTheTailAndTheLastThinking(t *testing.T) {
	var bodies []string
	for i := 1; i <= 6; i++ {
		bodies = append(bodies, anthropicFlavor.toolCalls(fakeCall{fmt.Sprintf("toolu_%d", i), "query", fmt.Sprintf(`{"limit":%d}`, i)}))
	}
	bodies = append(bodies, anthropicFlavor.endTurn("done"))
	up := newUpstream(t, replies(bodies...))
	p, _ := newProvider(t, anthropicFlavor, up, options{})
	session, err := p.Start(context.Background(), request(queryTool()))
	if err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("r", 5000)
	for i := 1; i <= 6; i++ {
		var results []llm.ToolResult
		if i > 1 {
			results = []llm.ToolResult{{CallID: fmt.Sprintf("toolu_%d", i-1), Content: big}}
		}
		if _, err := session.Next(context.Background(), results); err != nil {
			t.Fatalf("turn %d: %v", i, err)
		}
	}

	if err := session.Compact(llm.Compaction{Message: "SHORTENED", TailTurns: 4, TailBytes: 1 << 20, ResultBytes: 100}); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	fresh := strings.Repeat("n", 5000)
	if _, err := session.Next(context.Background(), []llm.ToolResult{{CallID: "toolu_6", Content: fresh}}); err != nil {
		t.Fatalf("Next after Compact: %v", err)
	}

	messages := sentMessages(t, up.requests()[6].Body)
	if len(messages) != 9 {
		t.Fatalf("%d messages, want the new message plus four turns", len(messages))
	}
	first := messages[0]["content"].([]any)[0].(map[string]any)
	if messages[0]["role"] != "user" || first["text"] != "SHORTENED" {
		t.Fatalf("first message = %v", messages[0])
	}
	for turn := 3; turn <= 6; turn++ {
		assistant := messages[1+(turn-3)*2]
		results := messages[2+(turn-3)*2]["content"].([]any)[0].(map[string]any)
		blocks := assistant["content"].([]any)
		var types []string
		for _, b := range blocks {
			types = append(types, b.(map[string]any)["type"].(string))
		}
		wantTypes := "text,tool_use"
		if turn == 6 {
			wantTypes = "thinking,redacted_thinking,text,tool_use"
		}
		if strings.Join(types, ",") != wantTypes {
			t.Fatalf("turn %d assistant blocks = %v, want %s", turn, types, wantTypes)
		}
		if id := blocks[len(blocks)-1].(map[string]any)["id"]; id != fmt.Sprintf("toolu_%d", turn) {
			t.Fatalf("turn %d tool_use id = %v", turn, id)
		}
		if results["tool_use_id"] != fmt.Sprintf("toolu_%d", turn) {
			t.Fatalf("turn %d tool_result id = %v", turn, results["tool_use_id"])
		}
		text := results["content"].([]any)[0].(map[string]any)["text"].(string)
		if turn < 6 && !strings.Contains(text, "[cut from 5000 bytes") {
			t.Fatalf("turn %d result was not cut: %d bytes", turn, len(text))
		}
		if turn == 6 && text != fresh {
			t.Fatalf("the pending results were changed: %d bytes", len(text))
		}
	}
	body := string(up.requests()[6].Body)
	if strings.Count(body, "sig-abc==") != 1 || strings.Count(body, "opaque-redacted==") != 1 {
		t.Fatalf("thinking must stay only on the last assistant turn:\n%s", body)
	}
}

func TestOpenAICompactKeepsToolCallsWithTheirResults(t *testing.T) {
	up := newUpstream(t, replies(
		openAIFlavor.toolCalls(fakeCall{"call_1", "query", `{"limit":1}`}),
		openAIFlavor.toolCalls(fakeCall{"call_2a", "query", `{"limit":2}`}, fakeCall{"call_2b", "query", `{"limit":2}`}),
		openAIFlavor.toolCalls(fakeCall{"call_3", "query", `{"limit":3}`}),
		openAIFlavor.toolCalls(fakeCall{"call_4", "query", `{"limit":4}`}),
		openAIFlavor.endTurn("done"),
	))
	p, _ := newProvider(t, openAIFlavor, up, options{})
	session, _ := startNext(t, p, request(queryTool()))
	big := strings.Repeat("r", 5000)
	for _, results := range [][]llm.ToolResult{
		{{CallID: "call_1", Content: big}},
		{{CallID: "call_2a", Content: big}, {CallID: "call_2b", Content: big, IsError: true}},
		{{CallID: "call_3", Content: big}},
	} {
		if _, err := session.Next(context.Background(), results); err != nil {
			t.Fatal(err)
		}
	}

	if err := session.Compact(llm.Compaction{Message: "SHORTENED", TailTurns: 3, TailBytes: 1 << 20, ResultBytes: 100}); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if _, err := session.Next(context.Background(), []llm.ToolResult{{CallID: "call_4", Content: "fresh"}}); err != nil {
		t.Fatalf("Next after Compact: %v", err)
	}

	messages := sentMessages(t, up.requests()[4].Body)
	var shape []string
	for _, m := range messages {
		switch m["role"] {
		case "assistant":
			var ids []string
			for _, c := range m["tool_calls"].([]any) {
				ids = append(ids, c.(map[string]any)["id"].(string))
			}
			shape = append(shape, "assistant:"+strings.Join(ids, "+"))
		case "tool":
			shape = append(shape, "tool:"+m["tool_call_id"].(string))
		default:
			shape = append(shape, m["role"].(string))
		}
	}
	want := "system,user,assistant:call_2a+call_2b,tool:call_2a,tool:call_2b,assistant:call_3,tool:call_3,assistant:call_4,tool:call_4"
	if strings.Join(shape, ",") != want {
		t.Fatalf("shape = %s\nwant    %s", strings.Join(shape, ","), want)
	}
	if messages[1]["content"] != "SHORTENED" {
		t.Fatalf("new message = %v", messages[1])
	}
	cut := messages[3]["content"].(string)
	if !strings.Contains(cut, "[cut from 5000 bytes") || len(cut) > 300 {
		t.Fatalf("older result not cut: %d bytes", len(cut))
	}
	if label := messages[4]["content"].(string); !strings.HasPrefix(label, "ERROR: ") {
		t.Fatalf("the error label was lost: %q", label)
	}
	if messages[8]["content"] != "fresh" {
		t.Fatalf("pending result = %v", messages[8]["content"])
	}
}

func TestCompactAtTheFirstCallKeepsOnlyTheNewMessage(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			up := newUpstream(t, replies(f.endTurn("done")))
			p, _ := newProvider(t, f, up, options{effort: "medium", maxTokens: 16000})
			session, err := p.Start(context.Background(), request(queryTool()))
			if err != nil {
				t.Fatal(err)
			}
			if err := session.Compact(llm.Compaction{Message: "SHORTENED", TailTurns: 4, TailBytes: 1 << 20, ResultBytes: 100}); err != nil {
				t.Fatalf("Compact: %v", err)
			}
			if _, err := session.Next(context.Background(), nil); err != nil {
				t.Fatalf("Next: %v", err)
			}

			body := up.requests()[0].Body
			messages := sentMessages(t, body)
			wantMessages := 1
			if f.name == config.LLMProviderOpenAI {
				wantMessages = 2
			}
			if len(messages) != wantMessages || !bytes.Contains(body, []byte("SHORTENED")) || bytes.Contains(body, []byte("show errors")) {
				t.Fatalf("messages = %v", messages)
			}
			if effort, maxTokens := tuningOf(t, f, body); effort != "high" || maxTokens != 16000 {
				t.Fatalf("a compaction before the first reply must keep it the planning call: effort %q, max tokens %d", effort, maxTokens)
			}
		})
	}
}

func TestCompactAfterAPlainAnswerDropsTheAnswerSoTheNextCallDoesNotEndOnTheAssistant(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			up := newUpstream(t, replies(
				f.toolCalls(fakeCall{"call_1", "query", `{"limit":1}`}),
				f.endTurn("ALLDONE"),
				f.endTurn("continuing"),
			))
			p, _ := newProvider(t, f, up, options{})
			session, _ := startNext(t, p, request(queryTool()))
			if _, err := session.Next(context.Background(), []llm.ToolResult{{CallID: "call_1", Content: "result"}}); err != nil {
				t.Fatal(err)
			}
			if err := session.Compact(llm.Compaction{Message: "SHORTENED", TailTurns: 4, TailBytes: 1 << 20, ResultBytes: 100}); err != nil {
				t.Fatalf("Compact: %v", err)
			}
			if _, err := session.Next(context.Background(), nil); err != nil {
				t.Fatalf("Next after Compact: %v", err)
			}

			body := up.requests()[2].Body
			messages := sentMessages(t, body)
			lastRole := messages[len(messages)-1]["role"]
			wantLast := "user"
			if f.name == config.LLMProviderOpenAI {
				wantLast = "tool"
			}
			if lastRole != wantLast || bytes.Contains(body, []byte("ALLDONE")) || !bytes.Contains(body, []byte("SHORTENED")) {
				t.Fatalf("last role = %v, want %s, with the plain answer dropped:\n%s", lastRole, wantLast, body)
			}
			if f.name == config.LLMProviderAnthropic && bytes.Count(body, []byte("sig-abc==")) != 1 {
				t.Fatalf("the last assistant turn must keep its thinking although a user turn follows it:\n%s", body)
			}
		})
	}
}

func sentResults(t *testing.T, f flavor, body []byte) []string {
	t.Helper()
	var out []string
	for _, m := range sentMessages(t, body) {
		if f.name == config.LLMProviderOpenAI {
			if m["role"] == "tool" {
				out = append(out, m["content"].(string))
			}
			continue
		}
		for _, block := range m["content"].([]any) {
			b := block.(map[string]any)
			if b["type"] == "tool_result" {
				out = append(out, b["content"].([]any)[0].(map[string]any)["text"].(string))
			}
		}
	}
	return out
}

func pruningRun(t *testing.T, f flavor, pruning llm.Pruning, turns int, prunable func(turn int) bool) *upstream {
	t.Helper()
	var bodies []string
	for i := 1; i <= turns; i++ {
		bodies = append(bodies, f.toolCalls(fakeCall{fmt.Sprintf("call_%d", i), "query", fmt.Sprintf(`{"limit":%d}`, i)}))
	}
	bodies = append(bodies, f.endTurn("done"))
	up := newUpstream(t, replies(bodies...))
	p, _ := newProvider(t, f, up, options{})
	req := request(queryTool())
	req.Pruning = pruning
	session, err := p.Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= turns+1; i++ {
		var results []llm.ToolResult
		if i > 1 {
			results = []llm.ToolResult{{
				CallID:   fmt.Sprintf("call_%d", i-1),
				Content:  fmt.Sprintf("result-%d-%s", i-1, strings.Repeat("p", 400)),
				Prunable: prunable(i - 1),
			}}
		}
		if _, err := session.Next(context.Background(), results); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	return up
}

func stubbed(results []string) []int {
	var out []int
	for i, r := range results {
		if r == llm.PrunedResult {
			out = append(out, i+1)
		}
	}
	return out
}

func TestPruningWaitsForAStepThenReplacesOldReadResults(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			odd := func(turn int) bool { return turn%2 == 1 }
			up := pruningRun(t, f, llm.Pruning{KeepTurns: 2, StepBytes: 1000}, 8, odd)
			reqs := up.requests()

			for i := range 7 {
				if got := stubbed(sentResults(t, f, reqs[i].Body)); len(got) != 0 {
					t.Fatalf("request %d pruned %v before the step was reached", i+1, got)
				}
			}
			eighth := sentResults(t, f, reqs[7].Body)
			if got := stubbed(eighth); fmt.Sprint(got) != "[1 3 5]" {
				t.Fatalf("request 8 pruned results %v, want the old read results 1, 3 and 5", got)
			}
			for _, kept := range []int{2, 4, 6, 7} {
				if !strings.HasPrefix(eighth[kept-1], fmt.Sprintf("result-%d-", kept)) {
					t.Fatalf("result %d was changed: %.40q", kept, eighth[kept-1])
				}
			}

			ninth := sentResults(t, f, reqs[8].Body)
			if got := stubbed(ninth); fmt.Sprint(got) != "[1 3 5]" {
				t.Fatalf("request 9 pruned results %v, want the same stubs", got)
			}
			before, after := sentMessages(t, reqs[7].Body), sentMessages(t, reqs[8].Body)
			for i := range 11 {
				if mustMarshal(before[i]) != mustMarshal(after[i]) {
					t.Fatalf("message %d of the stable prefix changed between requests 8 and 9", i)
				}
			}
			if f.name == config.LLMProviderAnthropic && bytes.Count(reqs[8].Body, []byte("sig-abc==")) != 8 {
				t.Fatalf("thinking blocks changed: %d signatures", bytes.Count(reqs[8].Body, []byte("sig-abc==")))
			}
		})
	}
}

func TestPruningNeverTouchesTheLastTurns(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			all := func(int) bool { return true }
			up := pruningRun(t, f, llm.Pruning{KeepTurns: 3, StepBytes: 100}, 6, all)
			last := sentResults(t, f, up.requests()[6].Body)
			if got := stubbed(last); fmt.Sprint(got) != "[1 2 3]" {
				t.Fatalf("pruned results %v, want only those before the last three turns", got)
			}

			short := pruningRun(t, f, llm.Pruning{KeepTurns: 3, StepBytes: 100}, 2, all)
			if got := stubbed(sentResults(t, f, short.requests()[2].Body)); len(got) != 0 {
				t.Fatalf("a session shorter than the kept turns pruned %v", got)
			}

			off := pruningRun(t, f, llm.Pruning{}, 6, all)
			if got := stubbed(sentResults(t, f, off.requests()[6].Body)); len(got) != 0 {
				t.Fatalf("zero pruning pruned %v", got)
			}
		})
	}
}

func TestAPrunedErrorResultStaysAnError(t *testing.T) {
	for _, f := range flavors {
		t.Run(f.name, func(t *testing.T) {
			var bodies []string
			for i := 1; i <= 3; i++ {
				bodies = append(bodies, f.toolCalls(fakeCall{fmt.Sprintf("call_%d", i), "query", `{"limit":1}`}))
			}
			bodies = append(bodies, f.endTurn("done"))
			up := newUpstream(t, replies(bodies...))
			p, _ := newProvider(t, f, up, options{})
			req := request(queryTool())
			req.Pruning = llm.Pruning{KeepTurns: 1, StepBytes: 100}
			session, err := p.Start(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			big := strings.Repeat("p", 400)
			steps := [][]llm.ToolResult{
				nil,
				{{CallID: "call_1", Content: big, IsError: true, Prunable: true}},
				{{CallID: "call_2", Content: big, Prunable: true}},
				{{CallID: "call_3", Content: big, Prunable: true}},
			}
			for _, results := range steps {
				if _, err := session.Next(context.Background(), results); err != nil {
					t.Fatal(err)
				}
			}

			messages := sentMessages(t, up.requests()[3].Body)
			if f.name == config.LLMProviderOpenAI {
				if got := messages[3]["content"]; got != "ERROR: "+llm.PrunedResult {
					t.Fatalf("stub = %v", got)
				}
				return
			}
			block := messages[2]["content"].([]any)[0].(map[string]any)
			if block["is_error"] != true || block["content"].([]any)[0].(map[string]any)["text"] != llm.PrunedResult {
				t.Fatalf("stub = %v", block)
			}
		})
	}
}
