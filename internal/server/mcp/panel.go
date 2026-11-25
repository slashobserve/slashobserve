package mcp

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"slices"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
)

const (
	panelURI       = "ui://slashobserve/panel"
	panelMIME      = "text/html;profile=mcp-app"
	appsExtension  = "io.modelcontextprotocol/ui"
	panelResource  = "SlashObserve panel"
	panelResourceD = "Draws the chart of show_panel with the panel renderer of SlashObserve. It makes no network request."
)

//go:embed all:panelapp
var panelFiles embed.FS

func BuiltPanel() []byte {
	html, _ := panelFiles.ReadFile("panelapp/panel.html")
	return html
}

type appsClientKey struct{}

func withAppsClient(ctx context.Context) context.Context {
	return context.WithValue(ctx, appsClientKey{}, true)
}

func hasAppsClient(ctx context.Context) bool {
	announced, _ := ctx.Value(appsClientKey{}).(bool)
	return announced
}

type announcement struct {
	appsOnInitialize bool
	appsPerRequest   bool
}

type capabilities struct {
	Extensions map[string]struct {
		MIMETypes []string `json:"mimeTypes"`
	} `json:"extensions"`
}

func (c capabilities) draws() bool {
	return slices.Contains(c.Extensions[appsExtension].MIMETypes, panelMIME)
}

func announcedApps(body []byte) announcement {
	if !bytes.Contains(body, []byte(appsExtension)) {
		return announcement{}
	}
	var msg struct {
		Method string `json:"method"`
		Params struct {
			Capabilities capabilities               `json:"capabilities"`
			Meta         map[string]json.RawMessage `json:"_meta"`
		} `json:"params"`
	}
	if json.Unmarshal(body, &msg) != nil {
		return announcement{}
	}
	var out announcement
	if msg.Method == "initialize" {
		out.appsOnInitialize = msg.Params.Capabilities.draws()
	}
	var perRequest capabilities
	if raw, ok := msg.Params.Meta[mcplib.MetaKeyClientCapabilities]; ok && json.Unmarshal(raw, &perRequest) == nil {
		out.appsPerRequest = perRequest.draws()
	}
	return out
}

func panelResourceFor(html []byte) (mcplib.Resource, func(context.Context, mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error)) {
	resource := mcplib.NewResource(panelURI, panelResource, mcplib.WithResourceDescription(panelResourceD), mcplib.WithMIMEType(panelMIME))
	read := func(context.Context, mcplib.ReadResourceRequest) ([]mcplib.ResourceContents, error) {
		return []mcplib.ResourceContents{mcplib.TextResourceContents{
			URI:      panelURI,
			MIMEType: panelMIME,
			Text:     string(html),
			Meta:     map[string]any{"ui": map[string]any{"prefersBorder": false}},
		}}, nil
	}
	return resource, read
}

func withPanelUI(html []byte) func(context.Context, []mcplib.Tool) []mcplib.Tool {
	return func(ctx context.Context, listed []mcplib.Tool) []mcplib.Tool {
		if len(html) == 0 || !hasAppsClient(ctx) {
			return listed
		}
		index := slices.IndexFunc(listed, func(t mcplib.Tool) bool { return t.Name == tools.NameShowPanel })
		if index < 0 {
			return listed
		}
		out := slices.Clone(listed)
		out[index].Meta = &mcplib.Meta{AdditionalFields: map[string]any{"ui": map[string]any{"resourceUri": panelURI}}}
		return out
	}
}
