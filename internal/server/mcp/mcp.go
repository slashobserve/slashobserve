package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
)

const (
	serverName       = "slashobserve"
	maxBodyBytes     = 8 << 20
	maxInFlight      = 8
	internalErrorMsg = "internal error"

	instructions = "SlashObserve builds Grafana-style dashboards as ObserveSpec JSON. " +
		"Call get_spec_guide before your first create_dashboard. " +
		"Discover metric, label, table and column names with the list_* and get_schema tools, " +
		"check queries with lint_query and query_range, then save with create_dashboard or update_dashboard; " +
		"a save runs every panel query and rejects panels that fail or return no data."
)

type Authenticator interface {
	BearerPrincipal(ctx context.Context, header string) (auth.Principal, bool, error)
}

type Options struct {
	PublicURL *url.URL
	Version   string
	Logger    *slog.Logger
}

type handler struct {
	registry  *tools.Registry
	authn     Authenticator
	logger    *slog.Logger
	origin    *url.URL
	transport http.Handler
	slots     slots
}

func NewHandler(reg *tools.Registry, authn Authenticator, opts Options) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	h := &handler{registry: reg, authn: authn, logger: logger, origin: opts.PublicURL, slots: slots{held: map[string]int{}}}
	srv := mcpserver.NewMCPServer(serverName, opts.Version,
		mcpserver.WithToolCapabilities(false),
		mcpserver.WithRecovery(),
		mcpserver.WithInstructions(instructions))
	for _, t := range reg.List() {
		srv.AddTool(toolFor(t), h.callTool(t.Name))
	}
	// The library rejects loopback connections whose Host is not loopback, which breaks a same-host reverse proxy;
	// the origin check and bearer authentication cover DNS rebinding here.
	h.transport = mcpserver.NewStreamableHTTPServer(srv,
		mcpserver.WithStateLess(true),
		mcpserver.WithDisableStreaming(true),
		mcpserver.WithDisableLocalhostProtection(true),
		mcpserver.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			if p, ok := auth.FromContext(r.Context()); ok {
				return auth.WithPrincipal(ctx, p)
			}
			return ctx
		}))
	return h
}

func toolFor(t tools.Tool) mcplib.Tool {
	tool := mcplib.NewToolWithRawSchema(t.Name, t.Description, t.InputSchema)
	tool.Annotations.ReadOnlyHint = mcplib.ToBoolPtr(t.ReadOnly)
	return tool
}

func (h *handler) callTool(name string) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		start := time.Now()
		principal, ok := auth.FromContext(ctx)
		if !ok {
			h.logger.ErrorContext(ctx, "mcp tool call without principal", slog.String("tool", name))
			return mcplib.NewToolResultError(internalErrorMsg), nil
		}
		input, err := rawInput(req.GetRawArguments())
		if err != nil {
			h.logger.ErrorContext(ctx, "mcp tool arguments not encodable", slog.String("tool", name), slog.Any("error", err))
			return mcplib.NewToolResultError(internalErrorMsg), nil
		}
		res, err := h.registry.Call(ctx, name, input)
		if err != nil {
			h.logger.ErrorContext(ctx, "mcp tool call failed", slog.String("tool", name), slog.Any("error", err))
			return mcplib.NewToolResultError(internalErrorMsg), nil
		}
		body, err := json.Marshal(res.Content)
		if err != nil {
			h.logger.ErrorContext(ctx, "mcp tool result not encodable", slog.String("tool", name), slog.Any("error", err))
			return mcplib.NewToolResultError(internalErrorMsg), nil
		}
		h.logger.InfoContext(ctx, "mcp tool call",
			slog.String("tool", name),
			slog.String("principal_kind", string(principal.Kind)),
			slog.String("principal_id", principal.ID),
			slog.Bool("is_error", res.IsError),
			slog.Duration("duration", time.Since(start)))
		out := mcplib.NewToolResultStructured(json.RawMessage(body), string(body))
		out.IsError = res.IsError
		return out, nil
	}
}

func rawInput(args any) (json.RawMessage, error) {
	switch v := args.(type) {
	case nil:
		return json.RawMessage(`{}`), nil
	case json.RawMessage:
		return v, nil
	default:
		return json.Marshal(v)
	}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !h.originAllowed(r.Header.Get("Origin")) {
		writeError(w, http.StatusForbidden, "forbidden_origin", "origin not allowed")
		return
	}
	header := r.Header.Get("Authorization")
	if header == "" {
		unauthenticated(w, "authentication required")
		return
	}
	principal, ok, err := h.authn.BearerPrincipal(r.Context(), header)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "resolve bearer key", slog.Any("error", err))
		writeError(w, http.StatusInternalServerError, "internal", internalErrorMsg)
		return
	}
	if !ok {
		unauthenticated(w, "invalid credentials")
		return
	}
	key := string(principal.Kind) + ":" + principal.ID
	if !h.slots.acquire(key) {
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusTooManyRequests, "too_many_requests", "too many concurrent requests")
		return
	}
	defer h.slots.release(key)
	if r.Body != nil {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", "request body is too large")
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "could not read request body")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	h.transport.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
}

func (h *handler) originAllowed(origin string) bool {
	if origin == "" {
		return true
	}
	if h.origin == nil {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Path != "" || u.RawQuery != "" || u.User != nil {
		return false
	}
	return strings.EqualFold(u.Scheme, h.origin.Scheme) && strings.EqualFold(u.Host, h.origin.Host)
}

func unauthenticated(w http.ResponseWriter, message string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="slashobserve"`)
	writeError(w, http.StatusUnauthorized, "unauthenticated", message)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	body, _ := json.Marshal(map[string]string{"error": message, "code": code})
	_, _ = w.Write(body)
}

type slots struct {
	mu   sync.Mutex
	held map[string]int
}

func (s *slots) acquire(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held[key] >= maxInFlight {
		return false
	}
	s.held[key]++
	return true
}

func (s *slots) release(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held[key] <= 1 {
		delete(s.held, key)
		return
	}
	s.held[key]--
}
