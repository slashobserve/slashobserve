package contextsources

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/egress"
)

const (
	discoverTimeout = 30 * time.Second
	maxMessageBytes = 4 << 20

	FailureNotAllowed   = "outbound_domain_not_allowed"
	FailureUnauthorized = "unauthorized"
	FailureTimeout      = "timeout"
	FailureNotMCP       = "not_an_mcp_server"
	FailureUnreachable  = "unreachable"
	FailureTooManyTools = "too_many_tools"
	FailureNoSecretKey  = "secret_key_missing"
	FailureUnreadable   = "secret_unreadable"
)

type Target struct {
	URL    string
	Header string
	Secret datasource.Secret
}

type Connector interface {
	Tools(ctx context.Context, target Target) ([]Found, error)
}

type Failure struct {
	Code string
}

func (f *Failure) Error() string { return "context source: " + f.Code }

func failureCode(err error) string {
	var failure *Failure
	if errors.As(err, &failure) {
		return failure.Code
	}
	return FailureUnreachable
}

type MCPConnector struct {
	Trip    http.RoundTripper
	Version string
}

func (c MCPConnector) Tools(ctx context.Context, target Target) ([]Found, error) {
	ctx, cancel := context.WithTimeout(ctx, discoverTimeout)
	defer cancel()
	mc, err := c.connect(ctx, target)
	if err != nil {
		return nil, classify(ctx, err)
	}
	defer func() { _ = mc.Close() }()
	if _, err := mc.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{
		ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		ClientInfo:      mcp.Implementation{Name: "slashobserve", Version: c.Version},
	}}); err != nil {
		return nil, classify(ctx, err)
	}
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

func (c MCPConnector) connect(ctx context.Context, target Target) (*client.Client, error) {
	headers := map[string]string{}
	if target.Header != "" {
		headers[target.Header] = target.Secret.Reveal()
	}
	httpClient := &http.Client{
		Transport:     c.Trip,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	mc, err := client.NewStreamableHttpClient(target.URL,
		transport.WithHTTPBasicClient(httpClient),
		transport.WithHTTPHeaders(headers),
		transport.WithHTTPLogger(slog.New(slog.DiscardHandler)),
		transport.WithHTTPMaxMessageBytes(maxMessageBytes),
	)
	if err != nil {
		return nil, err
	}
	if err := mc.Start(ctx); err != nil {
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
