package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

const stubPanel = "<!doctype html><title>stub panel</title>"

var appsCapabilities = map[string]any{
	"extensions": map[string]any{appsExtension: map[string]any{"mimeTypes": []string{panelMIME}}},
}

func initializeAs(t *testing.T, f *fixture, authorization string, capabilities map[string]any) string {
	t.Helper()
	rec := f.post(rpc(1, "initialize", map[string]any{
		"protocolVersion": "2025-11-25",
		"capabilities":    capabilities,
		"clientInfo":      map[string]any{"name": "host", "version": "0"},
	}), map[string]string{"Authorization": authorization})
	if rec.Code != http.StatusOK {
		t.Fatalf("initialize status %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Header().Get("Mcp-Session-Id")
}

type uiListedTool struct {
	Name string         `json:"name"`
	Meta map[string]any `json:"_meta"`
}

func showPanelListed(t *testing.T, f *fixture, headers map[string]string, params map[string]any) uiListedTool {
	t.Helper()
	rec := f.post(rpc(2, "tools/list", params), headers)
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/list status %d: %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Tools []uiListedTool `json:"tools"`
	}
	if err := json.Unmarshal(decodeRPC(t, rec).Result, &result); err != nil {
		t.Fatal(err)
	}
	for _, tool := range result.Tools {
		if tool.Name == tools.NameShowPanel {
			return tool
		}
	}
	t.Fatalf("show_panel is not listed")
	return uiListedTool{}
}

func resourceURI(tool uiListedTool) string {
	ui, _ := tool.Meta["ui"].(map[string]any)
	uri, _ := ui["resourceUri"].(string)
	return uri
}

func TestShowPanelPointsAtTheChartOnlyForAClientThatAnnouncedApps(t *testing.T) {
	f := newFixtureWithPanel(t, []byte(stubPanel))
	announced := initializeAs(t, f, goodKey, appsCapabilities)
	plain := initializeAs(t, f, goodKey, map[string]any{})
	wrongType := initializeAs(t, f, goodKey, map[string]any{
		"extensions": map[string]any{appsExtension: map[string]any{"mimeTypes": []string{"text/html"}}},
	})
	oauth := initializeAs(t, f, namedOAuth, appsCapabilities)
	tests := []struct {
		name    string
		headers map[string]string
		params  map[string]any
		want    string
	}{
		{"announced on initialize", map[string]string{"Mcp-Session-Id": announced}, nil, panelURI},
		{"announced on initialize by an OAuth client", map[string]string{"Authorization": namedOAuth, "Mcp-Session-Id": oauth}, nil, panelURI},
		{"announced with every request", nil, map[string]any{"_meta": map[string]any{"io.modelcontextprotocol/clientCapabilities": appsCapabilities}}, panelURI},
		{"announced nothing", map[string]string{"Mcp-Session-Id": plain}, nil, ""},
		{"announced another mime type", map[string]string{"Mcp-Session-Id": wrongType}, nil, ""},
		{"no session", nil, nil, ""},
		{"forged label flag", map[string]string{"Mcp-Session-Id": "so1.zz.ui"}, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := showPanelListed(t, f, tt.headers, tt.params)
			if got := resourceURI(tool); got != tt.want {
				t.Errorf("resourceUri = %q, want %q (meta %v)", got, tt.want, tool.Meta)
			}
			if tt.want == "" && tool.Meta != nil {
				t.Errorf("a client with no apps sees _meta %v", tool.Meta)
			}
		})
	}
}

func TestInitializeIssuesALabelOnlyForAppsOrNamedKeys(t *testing.T) {
	f := newFixtureWithPanel(t, []byte(stubPanel))
	tests := []struct {
		name          string
		authorization string
		capabilities  map[string]any
		wantSuffix    string
		wantLabel     bool
	}{
		{"key with apps", goodKey, appsCapabilities, ".ui", true},
		{"key without apps", goodKey, map[string]any{}, ".aG9zdA", true},
		{"OAuth with apps", namedOAuth, appsCapabilities, ".ui", true},
		{"OAuth without apps", namedOAuth, map[string]any{}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label := initializeAs(t, f, tt.authorization, tt.capabilities)
			if (label != "") != tt.wantLabel || !strings.HasSuffix(label, tt.wantSuffix) {
				t.Errorf("label %q, want label %v ending %q", label, tt.wantLabel, tt.wantSuffix)
			}
		})
	}
}

func TestTheChartResourceIsServedWithTheAppsType(t *testing.T) {
	f := newFixtureWithPanel(t, []byte(stubPanel))
	list := decodeRPC(t, f.post(rpc(1, "resources/list", map[string]any{}), nil))
	var listed struct {
		Resources []struct {
			URI      string `json:"uri"`
			MIMEType string `json:"mimeType"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(list.Result, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Resources) != 1 || listed.Resources[0].URI != panelURI || listed.Resources[0].MIMEType != "text/html;profile=mcp-app" {
		t.Fatalf("resources = %+v", listed.Resources)
	}
	read := decodeRPC(t, f.post(rpc(2, "resources/read", map[string]any{"uri": panelURI}), nil))
	var contents struct {
		Contents []struct {
			URI      string         `json:"uri"`
			MIMEType string         `json:"mimeType"`
			Text     string         `json:"text"`
			Meta     map[string]any `json:"_meta"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(read.Result, &contents); err != nil {
		t.Fatal(err)
	}
	if len(contents.Contents) != 1 {
		t.Fatalf("contents = %+v", contents)
	}
	got := contents.Contents[0]
	if got.URI != panelURI || got.MIMEType != "text/html;profile=mcp-app" || got.Text != stubPanel {
		t.Errorf("contents = %+v", got)
	}
	ui, _ := got.Meta["ui"].(map[string]any)
	if _, declared := ui["csp"]; declared {
		t.Errorf("the chart declares outside domains: %v", ui)
	}
}

func TestWithoutABuiltChartThereIsNoResourceAndNoUIMetadata(t *testing.T) {
	f := newFixture(t)
	label := initializeAs(t, f, goodKey, appsCapabilities)
	tool := showPanelListed(t, f, map[string]string{"Mcp-Session-Id": label}, nil)
	if tool.Meta != nil {
		t.Errorf("_meta = %v with no chart to point at", tool.Meta)
	}
	rec := f.post(rpc(2, "resources/list", map[string]any{}), nil)
	if strings.Contains(rec.Body.String(), panelURI) {
		t.Errorf("resources/list = %s", rec.Body.String())
	}
}

func TestShowPanelGivesTheChartDataOnlyToAnAppsHost(t *testing.T) {
	f := newFixtureWithPanel(t, []byte(stubPanel))
	f.handler.(*handler).call = func(_ context.Context, _ string, _ json.RawMessage) (tools.Result, error) {
		return tools.Result{Content: map[string]string{"summary": "text"}, App: json.RawMessage(`{"title":"chart"}`)}, nil
	}
	label := initializeAs(t, f, goodKey, appsCapabilities)
	call := func(headers map[string]string) toolResult {
		rec := f.post(rpc(3, "tools/call", map[string]any{"name": tools.NameShowPanel, "arguments": map[string]any{}}), headers)
		var out toolResult
		if err := json.Unmarshal(decodeRPC(t, rec).Result, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	app := call(map[string]string{"Mcp-Session-Id": label})
	if app.StructuredContent["title"] != "chart" || len(app.Content) != 1 || !strings.Contains(app.Content[0].Text, "text") {
		t.Errorf("apps host result = %+v", app)
	}
	plain := call(nil)
	if plain.StructuredContent["summary"] != "text" || plain.StructuredContent["title"] != nil {
		t.Errorf("other client result = %+v", plain)
	}
}
