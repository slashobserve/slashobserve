package observeagent

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
)

type record struct {
	level slog.Level
	msg   string
	attrs map[string]any
}

type recorder struct {
	mu      sync.Mutex
	records []record
	live    chan record
	preset  []slog.Attr
	shared  *recorder
}

func newRecorder() *recorder {
	r := &recorder{live: make(chan record, 256)}
	r.shared = r
	return r
}

func (r *recorder) logger() *slog.Logger { return slog.New(r) }

func (r *recorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *recorder) Handle(_ context.Context, rec slog.Record) error {
	entry := record{level: rec.Level, msg: rec.Message, attrs: map[string]any{}}
	for _, a := range r.preset {
		entry.attrs[a.Key] = a.Value.Any()
	}
	rec.Attrs(func(a slog.Attr) bool {
		entry.attrs[a.Key] = a.Value.Any()
		return true
	})
	root := r.shared
	root.mu.Lock()
	root.records = append(root.records, entry)
	root.mu.Unlock()
	root.live <- entry
	return nil
}

func (r *recorder) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &recorder{shared: r.shared, preset: append(append([]slog.Attr{}, r.preset...), attrs...)}
}

func (r *recorder) WithGroup(string) slog.Handler { return r }

func (r *recorder) all() []record {
	r.shared.mu.Lock()
	defer r.shared.mu.Unlock()
	return append([]record{}, r.shared.records...)
}

func TestRelay(t *testing.T) {
	longLine := strings.Repeat("x", 10*1024)
	tests := []struct {
		name  string
		input string
		want  []record
	}{
		{
			name:  "a json line is logged at its level with its fields",
			input: `{"level":"warn","msg":"claim failed","attempt":3,"ratio":0.5,"retry":true,"run":"r-1","nested":{"a":1},"list":[1]}` + "\n",
			want: []record{{level: slog.LevelWarn, msg: "claim failed", attrs: map[string]any{
				"attempt": int64(3), "ratio": 0.5, "retry": true, "run": "r-1", "component": "observe-agent",
			}}},
		},
		{
			name:  "every known level is mapped and an unknown one is info",
			input: `{"level":"debug","msg":"a"}` + "\n" + `{"level":"error","msg":"b"}` + "\n" + `{"level":"info","msg":"c"}` + "\n" + `{"level":"trace","msg":"d"}` + "\n",
			want: []record{
				{level: slog.LevelDebug, msg: "a", attrs: map[string]any{"component": "observe-agent"}},
				{level: slog.LevelError, msg: "b", attrs: map[string]any{"component": "observe-agent"}},
				{level: slog.LevelInfo, msg: "c", attrs: map[string]any{"component": "observe-agent"}},
				{level: slog.LevelInfo, msg: "d", attrs: map[string]any{"component": "observe-agent"}},
			},
		},
		{
			name:  "a plain line is logged as output",
			input: "listening on :9\n",
			want: []record{{level: slog.LevelInfo, msg: "observe-agent output", attrs: map[string]any{
				"line": "listening on :9", "component": "observe-agent",
			}}},
		},
		{
			name:  "a last line without a newline is still logged",
			input: "bye",
			want: []record{{level: slog.LevelInfo, msg: "observe-agent output", attrs: map[string]any{
				"line": "bye", "component": "observe-agent",
			}}},
		},
		{
			name:  "invalid json is a plain line",
			input: `{"level":"warn","msg":` + "\n",
			want: []record{{level: slog.LevelInfo, msg: "observe-agent output", attrs: map[string]any{
				"line": `{"level":"warn","msg":`, "component": "observe-agent",
			}}},
		},
		{
			name:  "json without a string level or msg is a plain line",
			input: `{"level":3,"msg":"x"}` + "\n" + `{"msg":"x"}` + "\n",
			want: []record{
				{level: slog.LevelInfo, msg: "observe-agent output", attrs: map[string]any{"line": `{"level":3,"msg":"x"}`, "component": "observe-agent"}},
				{level: slog.LevelInfo, msg: "observe-agent output", attrs: map[string]any{"line": `{"msg":"x"}`, "component": "observe-agent"}},
			},
		},
		{
			name:  "a child field named component cannot replace the real one",
			input: `{"level":"info","msg":"m","component":"spoof","truncated":true}` + "\n",
			want:  []record{{level: slog.LevelInfo, msg: "m", attrs: map[string]any{"component": "observe-agent"}}},
		},
		{
			name:  "windows line endings and blank lines",
			input: "one\r\n\r\ntwo\r\n",
			want: []record{
				{level: slog.LevelInfo, msg: "observe-agent output", attrs: map[string]any{"line": "one", "component": "observe-agent"}},
				{level: slog.LevelInfo, msg: "observe-agent output", attrs: map[string]any{"line": "two", "component": "observe-agent"}},
			},
		},
		{
			name:  "a line over 4 KiB is cut and marked, and the next line is intact",
			input: longLine + "\nnext\n",
			want: []record{
				{level: slog.LevelInfo, msg: "observe-agent output", attrs: map[string]any{"line": longLine[:4096], "component": "observe-agent", "truncated": true}},
				{level: slog.LevelInfo, msg: "observe-agent output", attrs: map[string]any{"line": "next", "component": "observe-agent"}},
			},
		},
		{
			name:  "a line of exactly 4 KiB is not cut",
			input: longLine[:4096] + "\n",
			want: []record{
				{level: slog.LevelInfo, msg: "observe-agent output", attrs: map[string]any{"line": longLine[:4096], "component": "observe-agent"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := newRecorder()
			relay(strings.NewReader(tt.input), rec.logger())
			got := rec.all()
			if len(got) != len(tt.want) {
				t.Fatalf("got %d records, want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				assertRecord(t, got[i], tt.want[i])
			}
		})
	}
}

func assertRecord(t *testing.T, got, want record) {
	t.Helper()
	if got.level != want.level || got.msg != want.msg {
		t.Fatalf("record = %v %q, want %v %q", got.level, got.msg, want.level, want.msg)
	}
	if len(got.attrs) != len(want.attrs) {
		t.Fatalf("attrs = %v, want %v", got.attrs, want.attrs)
	}
	for k, v := range want.attrs {
		if got.attrs[k] != v {
			t.Fatalf("attr %q = %v, want %v", k, got.attrs[k], v)
		}
	}
}

func unsetForTest(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
}
