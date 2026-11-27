package inboundmcp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/egress"
)

const (
	discoverTimeout = 30 * time.Second
	maxMessageBytes = 4 << 20
	leftOutPart     = "[a part of the result that is not text was left out]"

	FailureNotAllowed   = "outbound_domain_not_allowed"
	FailureUnauthorized = "unauthorized"
	FailureTimeout      = "timeout"
	FailureNotMCP       = "not_an_mcp_server"
	FailureUnreachable  = "unreachable"
	FailureTooManyTools = "too_many_tools"
	FailureNoSecretKey  = "secret_key_missing"
	FailureUnreadable   = "secret_unreadable"
)

var errNoTripper = errors.New("no round tripper for this source")

type Target struct {
	URL     string
	Header  string
	Secret  datasource.Secret
	Private bool
}

func connectTarget(row dbsqlc.McpSource, secret datasource.Secret) Target {
	return Target{URL: row.Url, Header: row.SecretHeader.String, Secret: secret, Private: row.AllowPrivate}
}

type Called struct {
	Text    string
	IsError bool
}

type Connector interface {
	Tools(ctx context.Context, target Target) ([]Found, error)
	Call(ctx context.Context, target Target, name string, arguments map[string]any) (Called, error)
}

type Failure struct {
	Code string
}

func (f *Failure) Error() string { return "MCP source: " + f.Code }

func failureCode(err error) string {
	var failure *Failure
	if errors.As(err, &failure) {
		return failure.Code
	}
	return FailureUnreachable
}

type MCPConnector struct {
	Trip        http.RoundTripper
	PrivateTrip http.RoundTripper
	Version     string
}

func (c MCPConnector) Tools(ctx context.Context, target Target) ([]Found, error) {
	ctx, cancel := context.WithTimeout(ctx, discoverTimeout)
	defer cancel()
	mc, err := c.open(ctx, target)
	if err != nil {
		return nil, classify(ctx, err)
	}
	defer func() { _ = mc.Close() }()
	listed, err := mc.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, classify(ctx, err)
	}
	if len(listed.Tools) > maxTools {
		return nil, &Failure{Code: FailureTooManyTools}
	}
	found := make([]Found, len(listed.Tools))
	for i, tool := range listed.Tools {
		schema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			schema = nil
		}
		found[i] = Found{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: schema,
			ReadOnly:    tool.Annotations.ReadOnlyHint,
			Destructive: tool.Annotations.DestructiveHint,
		}
	}
	return found, nil
}

func (c MCPConnector) Call(ctx context.Context, target Target, name string, arguments map[string]any) (Called, error) {
	mc, err := c.open(ctx, target)
	if err != nil {
		return Called{}, classify(ctx, err)
	}
	defer func() { _ = mc.Close() }()
	result, err := mc.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: name, Arguments: arguments}})
	if err != nil {
		return Called{}, classify(ctx, err)
	}
	return Called{Text: textOf(result), IsError: result.IsError}, nil
}

func textOf(result *mcp.CallToolResult) string {
	var parts []string
	for _, content := range result.Content {
		if text, ok := mcp.AsTextContent(content); ok {
			parts = append(parts, text.Text)
			continue
		}
		parts = append(parts, leftOutPart)
	}
	if len(parts) == 0 && result.StructuredContent != nil {
		if raw, err := json.Marshal(result.StructuredContent); err == nil {
			parts = append(parts, string(raw))
		}
	}
	return strings.Join(parts, "\n")
}

func (c MCPConnector) open(ctx context.Context, target Target) (*client.Client, error) {
	mc, err := c.session(ctx, target, streamable)
	if !errors.Is(err, transport.ErrLegacySSEServer) {
		return mc, err
	}
	legacy, sseErr := c.session(ctx, target, legacySSE)
	switch {
	case sseErr == nil:
		return legacy, nil
	case classify(ctx, sseErr).Code != FailureUnreachable:
		return nil, sseErr
	}
	return nil, err
}

type dialer func(address string, httpClient *http.Client, headers map[string]string) (*client.Client, error)

func streamable(address string, httpClient *http.Client, headers map[string]string) (*client.Client, error) {
	return client.NewStreamableHttpClient(address,
		transport.WithHTTPBasicClient(httpClient),
		transport.WithHTTPHeaders(headers),
		transport.WithHTTPLogger(slog.New(slog.DiscardHandler)),
		transport.WithHTTPMaxMessageBytes(maxMessageBytes),
	)
}

func legacySSE(address string, httpClient *http.Client, headers map[string]string) (*client.Client, error) {
	return client.NewSSEMCPClient(address,
		transport.WithHTTPClient(httpClient),
		transport.WithHeaders(headers),
		transport.WithSSELogger(slog.New(slog.DiscardHandler)),
		transport.WithMaxMessageBytes(maxMessageBytes),
	)
}

func (c MCPConnector) session(ctx context.Context, target Target, dial dialer) (*client.Client, error) {
	headers := map[string]string{}
	if target.Header != "" {
		headers[target.Header] = target.Secret.Reveal()
	}
	trip := c.Trip
	if target.Private {
		trip = c.PrivateTrip
	}
	if trip == nil {
		return nil, errNoTripper
	}
	mc, err := dial(target.URL, &http.Client{
		Transport:     trip,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, headers)
	if err != nil {
		return nil, err
	}
	if err := mc.Start(ctx); err != nil {
		_ = mc.Close()
		return nil, err
	}
	if _, err := mc.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{
		ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		ClientInfo:      mcp.Implementation{Name: "slashobserve", Version: c.Version},
	}}); err != nil {
		_ = mc.Close()
		return nil, err
	}
	return mc, nil
}

func classify(ctx context.Context, err error) *Failure {
	var needsAuth *transport.AuthorizationRequiredError
	switch {
	case errors.Is(err, egress.ErrNotAllowed):
		return &Failure{Code: FailureNotAllowed}
	case errors.As(err, &needsAuth), errors.Is(err, transport.ErrUnauthorized):
		return &Failure{Code: FailureUnauthorized}
	case errors.Is(ctx.Err(), context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		return &Failure{Code: FailureTimeout}
	case errors.Is(err, transport.ErrLegacySSEServer), isProtocolError(err):
		return &Failure{Code: FailureNotMCP}
	}
	return &Failure{Code: FailureUnreachable}
}

func isProtocolError(err error) bool {
	var syntax *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &syntax) || errors.As(err, &typeErr)
}
