package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/clientgone"
	"github.com/slashobserve/slashobserve/internal/grantmode"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/runsession"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	serverName       = "slashobserve"
	maxBodyBytes     = 8 << 20
	maxInFlight      = 8
	maxTotalInFlight = 32
	internalErrorMsg = "internal error"
)

type Credential struct {
	Principal auth.Principal
	OAuth     bool
	Client    string
	Claimer   *claims.Claimer
	Run       *revisions.Request
	Loopback  bool
	ClaimOnly bool
	Mode      grantmode.Mode
}

type Authenticator interface {
	Authenticate(ctx context.Context, header string) (Credential, bool, error)
}

type RunSessions interface {
	Scope(ctx context.Context, req revisions.Request) (tools.Run, error)
	Call(ctx context.Context, req revisions.Request, call runsession.Call, dispatch func(context.Context) (tools.Result, error)) (runsession.Answer, error)
}

type Options struct {
	PublicURL           *url.URL
	ResourceMetadataURL string
	Version             string
	Logger              *slog.Logger
	Runs                RunSessions
}

type registeredClientKey struct{}

func withRegisteredClient(ctx context.Context) context.Context {
	return context.WithValue(ctx, registeredClientKey{}, true)
}

func hasRegisteredClient(ctx context.Context) bool {
	registered, _ := ctx.Value(registeredClientKey{}).(bool)
	return registered
}

type handler struct {
	call        func(ctx context.Context, name string, input json.RawMessage) (tools.Result, error)
	callClaim   func(ctx context.Context, name string, input json.RawMessage) (tools.Result, error)
	authn       Authenticator
	logger      *slog.Logger
	origin      *url.URL
	metadataURL string
	transport   http.Handler
	slots       slots
	types       func() []spec.DatasourceType

	runs           RunSessions
	runTransports  map[tools.Mode]http.Handler
	claimTransport http.Handler
}

func NewHandler(reg *tools.Registry, authn Authenticator, opts Options) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	h := &handler{call: reg.Call, callClaim: reg.CallClaim, authn: authn, logger: logger, origin: opts.PublicURL, metadataURL: opts.ResourceMetadataURL, slots: slots{held: map[string]int{}}, types: reg.DatasourceTypes, runs: opts.Runs}
	srv := mcpserver.NewMCPServer(serverName, opts.Version,
		mcpserver.WithToolCapabilities(false),
		mcpserver.WithPromptCapabilities(false),
		mcpserver.WithRecovery(),
		mcpserver.WithInstructions(tools.ServerInstructions))
	for _, t := range reg.List() {
		if t.AgentOnly {
			continue
		}
		srv.AddTool(toolFor(t), h.callTool(t.Name))
	}
	srv.AddPrompt(observePrompt(), h.observePromptHandler())
	h.transport = h.newTransport(srv)
	claim := mcpserver.NewMCPServer(serverName, opts.Version,
		mcpserver.WithToolCapabilities(false),
		mcpserver.WithRecovery())
	for _, t := range reg.List() {
		if t.Claim {
			claim.AddTool(toolFor(t), h.callClaimTool(t.Name))
		}
	}
	h.claimTransport = h.newTransport(claim)
	h.runTransports = map[tools.Mode]http.Handler{}
	for _, mode := range []tools.Mode{tools.ModeCreate, tools.ModeRevise, tools.ModeInvestigate, tools.ModeSuggest} {
		run := mcpserver.NewMCPServer(serverName, opts.Version,
			mcpserver.WithToolCapabilities(false),
			mcpserver.WithRecovery())
		for _, t := range reg.List() {
			if t.OfferedIn(mode) {
				run.AddTool(toolFor(t.ForAgent()), h.callRunTool(t.Name))
			}
		}
		h.runTransports[mode] = h.newTransport(run)
	}
	return h
}

func (h *handler) newTransport(srv *mcpserver.MCPServer) http.Handler {
	// The library rejects loopback connections whose Host is not loopback, which breaks a same-host reverse proxy;
	// the origin check and bearer authentication cover DNS rebinding here.
	return mcpserver.NewStreamableHTTPServer(srv,
		mcpserver.WithStateLess(true),
		mcpserver.WithDisableStreaming(true),
		mcpserver.WithDisableLocalhostProtection(true),
		mcpserver.WithStreamableHTTPLogger(h.logger),
		mcpserver.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			ctx = origin.With(ctx, origin.From(r.Context()))
			ctx = grantmode.With(ctx, grantmode.From(r.Context()))
			if hasRegisteredClient(r.Context()) {
				ctx = withRegisteredClient(ctx)
			}
			if c, ok := claims.ClaimerFrom(r.Context()); ok {
				ctx = claims.WithClaimer(ctx, c)
			}
			if run, ok := tools.RunFrom(r.Context()); ok {
				ctx = tools.WithRun(ctx, run)
			}
			if req, ok := runRequestFrom(r.Context()); ok {
				ctx = withRunRequest(ctx, req)
			}
			if p, ok := auth.FromContext(r.Context()); ok {
				return auth.WithPrincipal(ctx, p)
			}
			return ctx
		}))
}

func toolFor(t tools.Tool) mcplib.Tool {
	tool := mcplib.NewToolWithRawSchema(t.Name, t.Description, t.InputSchema)
	tool.Annotations.ReadOnlyHint = mcplib.ToBoolPtr(t.ReadOnly)
	if !t.ReadOnly {
		tool.Annotations.DestructiveHint = mcplib.ToBoolPtr(false)
	}
	return tool
}

type callKind int

const (
	callOutsideRun callKind = iota
	callInsideRun
	callClaimOnly
)

func (h *handler) callTool(name string) mcpserver.ToolHandlerFunc {
	return h.invoke(name, callOutsideRun)
}

func (h *handler) callClaimTool(name string) mcpserver.ToolHandlerFunc {
	return h.invoke(name, callClaimOnly)
}

func (h *handler) invoke(name string, kind callKind) mcpserver.ToolHandlerFunc {
	inRun := kind == callInsideRun
	return func(ctx context.Context, req mcplib.CallToolRequest) (result *mcplib.CallToolResult, _ error) {
		defer func() {
			if p := recover(); p != nil {
				h.logger.ErrorContext(ctx, "mcp tool call panicked",
					slog.String("tool", name),
					slog.String("panic_type", fmt.Sprintf("%T", p)))
				result = mcplib.NewToolResultError(internalErrorMsg)
			}
		}()
		start := time.Now()
		principal, ok := auth.FromContext(ctx)
		if !ok && kind == callClaimOnly {
			principal, ok = claimerAsCaller(ctx)
		}
		if !ok {
			h.logger.ErrorContext(ctx, "mcp tool call without principal", slog.String("tool", name))
			return mcplib.NewToolResultError(internalErrorMsg), nil
		}
		if _, scoped := tools.RunFrom(ctx); inRun && !scoped {
			h.logger.ErrorContext(ctx, "mcp run tool call without run scope", slog.String("tool", name))
			return mcplib.NewToolResultError(internalErrorMsg), nil
		}
		input, err := rawInput(req.GetRawArguments())
		if err != nil {
			h.logger.ErrorContext(ctx, "mcp tool arguments not encodable", h.failureAttrs(ctx, name, principal, "invalid_arguments", err)...)
			return mcplib.NewToolResultError(internalErrorMsg), nil
		}
		ctx = origin.With(ctx, clientOrigin(ctx, req))
		answer, err := h.answerFor(ctx, name, input, req, kind)
		if err != nil {
			if ctx.Err() != nil {
				h.logger.InfoContext(ctx, "mcp tool call canceled by client", h.failureAttrs(ctx, name, principal, "canceled", err)...)
				return mcplib.NewToolResultError(internalErrorMsg), nil
			}
			if errors.Is(err, errResultNotEncodable) {
				h.logger.ErrorContext(ctx, "mcp tool result not encodable", h.failureAttrs(ctx, name, principal, "result_not_encodable", err)...)
				return mcplib.NewToolResultError(internalErrorMsg), nil
			}
			h.logger.ErrorContext(ctx, "mcp tool call failed", h.failureAttrs(ctx, name, principal, tools.FailureCode(err), err)...)
			return mcplib.NewToolResultError(internalErrorMsg), nil
		}
		h.logger.InfoContext(ctx, "mcp tool call", append([]any{
			slog.String("tool", name),
			slog.String("principal_kind", string(principal.Kind)),
			slog.String("principal_id", principal.ID),
			slog.Bool("is_error", answer.IsError),
			slog.Duration("duration", time.Since(start))}, runAttrs(ctx)...)...)
		out := mcplib.NewToolResultStructured(json.RawMessage(answer.Content), answer.Text())
		out.IsError = answer.IsError
		if inRun {
			out.Meta = mcplib.NewMetaFromMap(map[string]any{runMetaKey: answer.Meta})
		}
		return out, nil
	}
}

func clientOrigin(ctx context.Context, req mcplib.CallToolRequest) origin.Origin {
	o := origin.Origin{Kind: origin.MCP, Client: origin.From(ctx).Client}
	if info := req.Params.Meta.ClientInfo(); info != nil && !hasRegisteredClient(ctx) {
		if name := origin.CleanClient(info.Name); name != "" {
			o.Client = name
		}
	}
	return o
}

func (h *handler) answerFor(ctx context.Context, name string, input json.RawMessage, req mcplib.CallToolRequest, kind callKind) (runsession.Answer, error) {
	call := h.call
	if kind == callClaimOnly {
		call = h.callClaim
	}
	dispatch := func(ctx context.Context) (tools.Result, error) { return call(ctx, name, input) }
	if kind == callInsideRun {
		return h.callInRun(ctx, name, input, req, dispatch)
	}
	return h.callOutsideRun(ctx, dispatch)
}

func claimerAsCaller(ctx context.Context) (auth.Principal, bool) {
	claimer, ok := claims.ClaimerFrom(ctx)
	if !ok {
		return auth.Principal{}, false
	}
	return auth.Principal{Kind: auth.PrincipalKind(claimer.Kind), ID: claimer.ID}, true
}

func (h *handler) callOutsideRun(ctx context.Context, dispatch func(context.Context) (tools.Result, error)) (runsession.Answer, error) {
	res, err := dispatch(ctx)
	if err != nil {
		return runsession.Answer{}, err
	}
	body, err := json.Marshal(res.Content)
	if err != nil {
		return runsession.Answer{}, fmt.Errorf("%w: %w", errResultNotEncodable, err)
	}
	return runsession.Answer{Content: body, IsError: res.IsError}, nil
}

// Error text can echo tool input, query text or credentials, so a failure is logged by its code and type only.
func (h *handler) failureAttrs(ctx context.Context, tool string, principal auth.Principal, code string, err error) []any {
	attrs := append([]any{
		slog.String("tool", tool),
		slog.String("principal_kind", string(principal.Kind)),
		slog.String("principal_id", principal.ID),
		slog.String("error_code", code),
	}, runAttrs(ctx)...)
	return append(attrs, logattrs.Safe(err)...)
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
		h.unauthenticated(w, "authentication required", false)
		return
	}
	cred, ok, err := h.authn.Authenticate(r.Context(), header)
	if err != nil {
		if clientgone.Is(r.Context(), err) {
			clientgone.WriteHeader(w, r, h.logger, "resolve bearer key")
			return
		}
		h.logger.ErrorContext(r.Context(), "resolve bearer key", logattrs.Failure("key_lookup_failed", err)...)
		writeError(w, http.StatusInternalServerError, "internal", internalErrorMsg)
		return
	}
	if !ok {
		h.unauthenticated(w, "invalid credentials", true)
		return
	}
	if cred.Loopback && !isLoopbackPeer(r.RemoteAddr) {
		h.unauthenticated(w, "invalid credentials", true)
		return
	}
	if cred.Run != nil {
		h.serveRun(w, r, cred)
		return
	}
	if cred.ClaimOnly && cred.Claimer != nil {
		h.serveClaimOnly(w, r, *cred.Claimer)
		return
	}
	h.servePrincipal(w, r, cred)
}

func (h *handler) servePrincipal(w http.ResponseWriter, r *http.Request, cred Credential) {
	principal := cred.Principal
	client := cred.Client
	if !cred.OAuth {
		client = labelClient(r.Header.Get(mcpserver.HeaderKeySessionID))
	}
	h.serve(w, r, serving{
		key: principalKey(principal),
		prepare: func(ctx context.Context) (context.Context, http.Handler, error) {
			ctx = auth.WithPrincipal(ctx, principal)
			ctx = origin.With(ctx, origin.Origin{Kind: origin.MCP, Client: client})
			if cred.OAuth {
				ctx = withRegisteredClient(ctx)
			}
			if cred.Mode != "" {
				ctx = grantmode.With(ctx, cred.Mode)
			}
			if cred.Claimer != nil {
				claimer := *cred.Claimer
				if !cred.OAuth {
					claimer.Client = client
				}
				ctx = claims.WithClaimer(ctx, claimer)
			}
			return ctx, h.transport, nil
		},
		labelsClient: !cred.OAuth,
	})
}

type serving struct {
	key          string
	prepare      func(ctx context.Context) (context.Context, http.Handler, error)
	labelsClient bool
}

func (h *handler) serve(w http.ResponseWriter, r *http.Request, s serving) {
	if !h.slots.acquire(s.key) {
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusTooManyRequests, "too_many_requests", "too many concurrent requests")
		return
	}
	defer h.slots.release(s.key)
	ctx, transport, err := s.prepare(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", internalErrorMsg)
		return
	}
	r = r.Clone(ctx)
	r.Header.Del(mcpserver.HeaderKeySessionID)
	var body []byte
	if r.Body != nil {
		var readErr error
		body, readErr = io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
		var tooLarge *http.MaxBytesError
		if errors.As(readErr, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", "request body is too large")
			return
		}
		if readErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "could not read request body")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	if !isClaimPoll(body) {
		if !h.slots.acquireGlobal() {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusServiceUnavailable, "busy", "server is busy")
			return
		}
		defer h.slots.releaseGlobal()
	}
	if client := initializeClient(body); client != "" && s.labelsClient {
		w = &labelWriter{ResponseWriter: w, label: clientLabel(client)}
	}
	transport.ServeHTTP(w, r)
}

type labelWriter struct {
	http.ResponseWriter
	label       string
	wroteHeader bool
}

func (l *labelWriter) WriteHeader(status int) {
	if !l.wroteHeader {
		l.wroteHeader = true
		if status == http.StatusOK {
			l.Header().Set(mcpserver.HeaderKeySessionID, l.label)
		}
	}
	l.ResponseWriter.WriteHeader(status)
}

func (l *labelWriter) Write(p []byte) (int, error) {
	if !l.wroteHeader {
		l.WriteHeader(http.StatusOK)
	}
	return l.ResponseWriter.Write(p)
}

func (l *labelWriter) Flush() {
	if !l.wroteHeader {
		l.WriteHeader(http.StatusOK)
	}
	_ = http.NewResponseController(l.ResponseWriter).Flush()
}

func (l *labelWriter) Unwrap() http.ResponseWriter { return l.ResponseWriter }

func initializeClient(body []byte) string {
	if !bytes.Contains(body, []byte("initialize")) {
		return ""
	}
	var msg struct {
		Method string `json:"method"`
		Params struct {
			ClientInfo struct {
				Name string `json:"name"`
			} `json:"clientInfo"`
		} `json:"params"`
	}
	if json.Unmarshal(body, &msg) != nil || msg.Method != "initialize" {
		return ""
	}
	return origin.CleanClient(msg.Params.ClientInfo.Name)
}

func (h *handler) originAllowed(header string) bool {
	if header == "" {
		return true
	}
	if h.origin == nil {
		return false
	}
	u, err := url.Parse(header)
	if err != nil || u.Path != "" || u.RawQuery != "" || u.User != nil {
		return false
	}
	return strings.EqualFold(u.Scheme, h.origin.Scheme) && canonicalHost(u) == canonicalHost(h.origin)
}

func canonicalHost(u *url.URL) string {
	port := u.Port()
	if port == "" {
		switch strings.ToLower(u.Scheme) {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	return net.JoinHostPort(strings.ToLower(u.Hostname()), port)
}

func (h *handler) unauthenticated(w http.ResponseWriter, message string, tokenRefused bool) {
	challenge := `Bearer realm="slashobserve"`
	if h.metadataURL != "" {
		challenge += `, resource_metadata="` + h.metadataURL + `"`
		if tokenRefused {
			challenge += `, error="invalid_token"`
		}
	}
	w.Header().Set("WWW-Authenticate", challenge)
	writeError(w, http.StatusUnauthorized, "unauthenticated", message)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	body, _ := json.Marshal(map[string]string{"error": message, "code": code})
	_, _ = w.Write(body)
}

func principalKey(p auth.Principal) string {
	return string(p.Kind) + ":" + p.ID
}

type slots struct {
	mu     sync.Mutex
	held   map[string]int
	global int
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

func (s *slots) acquireGlobal() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.global >= maxTotalInFlight {
		return false
	}
	s.global++
	return true
}

func (s *slots) releaseGlobal() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.global--
}
