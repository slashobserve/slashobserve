package main

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func connect(t *testing.T) *client.Client {
	t.Helper()
	code, _ := fixture(t)
	ts := httptest.NewServer(NewHandler(code, "test"))
	t.Cleanup(ts.Close)
	c, err := client.NewStreamableHttpClient(ts.URL + endpointPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	init := mcp.InitializeRequest{}
	init.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcp.Implementation{Name: "test", Version: "0"}
	if _, err := c.Initialize(ctx, init); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEveryToolIsMarkedReadOnlyAndNotDestructive(t *testing.T) {
	c := connect(t)
	list, err := c.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, tool := range list.Tools {
		got[tool.Name] = true
		a := tool.Annotations
		if a.ReadOnlyHint == nil || !*a.ReadOnlyHint || a.DestructiveHint == nil || *a.DestructiveHint {
			t.Errorf("%s is not marked read-only and not destructive: %+v", tool.Name, a)
		}
	}
	if len(got) != 3 || !got["list_dir"] || !got["read_file"] || !got["grep"] {
		t.Fatalf("tools: %v", got)
	}
}

func call3(t *testing.T, c *client.Client, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := c.CallTool(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestToolsOverHTTP(t *testing.T) {
	c := connect(t)
	res := call3(t, c, "grep", map[string]any{"pattern": "paymentFailure", "glob": "*.go"})
	if res.IsError || !strings.Contains(text(res), "pkg/a.go:3:") {
		t.Fatalf("grep: %+v", res)
	}
	res = call3(t, c, "read_file", map[string]any{"path": "main.go", "offset": 3, "limit": 1})
	if res.IsError || !strings.Contains(text(res), "func flagValue") {
		t.Fatalf("read_file: %+v", res)
	}
	res = call3(t, c, "list_dir", map[string]any{"path": "."})
	if res.IsError || !strings.Contains(text(res), "pkg/") {
		t.Fatalf("list_dir: %+v", res)
	}
}

func TestEscapesAreToolErrorsThatNameNoServerPath(t *testing.T) {
	c := connect(t)
	for _, args := range []map[string]any{{"path": "../secret.txt"}, {"path": "/etc/passwd"}, {"path": "link-file"}} {
		res := call3(t, c, "read_file", args)
		if !res.IsError {
			t.Fatalf("%v was served: %s", args, text(res))
		}
		if strings.Contains(text(res), "/") && !strings.Contains(text(res), "relative to the root") {
			t.Fatalf("error leaks a path: %s", text(res))
		}
	}
	res := call3(t, c, "read_file", map[string]any{})
	if !res.IsError || text(res) != "path is required" {
		t.Fatalf("missing path: %+v", res)
	}
}

func TestCallerMistakesKeepTheirOwnMessage(t *testing.T) {
	c := connect(t)
	cases := map[string]string{
		"main.go/x":               "not a directory",
		strings.Repeat("a", 5000): "path is not valid",
		"pkg":                     "is a directory; use list_dir",
	}
	for path, want := range cases {
		res := call3(t, c, "read_file", map[string]any{"path": path})
		if !res.IsError || text(res) != want {
			t.Errorf("%.20s: got %q, want %q", path, text(res), want)
		}
	}
}
