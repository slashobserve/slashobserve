package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/claims"
	"github.com/slashobserve/slashobserve/internal/grantmode"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/revisions"
	"github.com/slashobserve/slashobserve/internal/runsession"
)

const (
	claimRunTool = "claim_run"
	turnMetaKey  = "slashobserve.com/turn"
	runMetaKey   = "slashobserve.com/run"
)

var (
	errNoRunTransport     = errors.New("no run tools for this run mode")
	errNoRunSessions      = errors.New("run sessions are not configured")
	errResultNotEncodable = errors.New("tool result not encodable")
)

type runRequestKey struct{}

func withRunRequest(ctx context.Context, req revisions.Request) context.Context {
	return context.WithValue(ctx, runRequestKey{}, req)
}

func runRequestFrom(ctx context.Context) (revisions.Request, bool) {
	req, ok := ctx.Value(runRequestKey{}).(revisions.Request)
	return req, ok
}

func runAttrs(ctx context.Context) []any {
	req, ok := runRequestFrom(ctx)
	if !ok {
		return nil
	}
	return []any{slog.String("agent_request_id", req.ID)}
}

func (h *handler) callRunTool(name string) mcpserver.ToolHandlerFunc {
	return h.invoke(name, callInsideRun)
}

func (h *handler) serveRun(w http.ResponseWriter, r *http.Request, cred Credential) {
	req := *cred.Run
	h.serve(w, r, serving{
		key: "run:" + req.ID,
		prepare: func(ctx context.Context) (context.Context, http.Handler, error) {
			ctx = auth.WithPrincipal(ctx, cred.Principal)
			ctx = origin.With(ctx, req.WriteOrigin(origin.Origin{Kind: origin.MCP, Client: req.Origin.Client}))
			if cred.Mode != "" {
				ctx = grantmode.With(ctx, cred.Mode)
			}
			if h.runs == nil {
				h.logger.ErrorContext(ctx, "open run session", logattrs.Failure("run_scope_failed", errNoRunSessions, slog.String("agent_request_id", req.ID))...)
				return nil, nil, errNoRunSessions
			}
			scope, err := h.runs.Scope(ctx, req)
			if err != nil {
				h.logger.ErrorContext(ctx, "open run session", logattrs.Failure("run_scope_failed", err, slog.String("agent_request_id", req.ID))...)
				return nil, nil, err
			}
			transport, ok := h.runTransports[scope.Mode]
			if !ok {
				h.logger.ErrorContext(ctx, "open run session", logattrs.Failure("run_scope_failed", errNoRunTransport, slog.String("agent_request_id", req.ID))...)
				return nil, nil, errNoRunTransport
			}
			return withRunRequest(tools.WithRun(ctx, scope), req), transport, nil
		},
	})
}

func isLoopbackPeer(remoteAddr string) bool {
	addr, err := netip.ParseAddrPort(remoteAddr)
	return err == nil && addr.Addr().Unmap().IsLoopback()
}

func (h *handler) serveClaimOnly(w http.ResponseWriter, r *http.Request, claimer claims.Claimer) {
	h.serve(w, r, serving{
		key: "claim:" + string(claimer.Kind),
		prepare: func(ctx context.Context) (context.Context, http.Handler, error) {
			return claims.WithClaimer(origin.With(ctx, origin.Origin{Kind: origin.MCP}), claimer), h.claimTransport, nil
		},
	})
}

func (h *handler) callInRun(ctx context.Context, name string, input json.RawMessage, req mcplib.CallToolRequest, dispatch func(context.Context) (tools.Result, error)) (runsession.Answer, error) {
	run, ok := runRequestFrom(ctx)
	if !ok || h.runs == nil {
		return runsession.Answer{}, errNoRunSessions
	}
	return h.runs.Call(ctx, run, runsession.Call{Name: name, Input: input, Turn: turnOf(req)}, dispatch)
}

func turnOf(req mcplib.CallToolRequest) *runsession.Turn {
	if req.Params.Meta == nil {
		return nil
	}
	raw, ok := req.Params.Meta.AdditionalFields[turnMetaKey]
	if !ok {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var turn runsession.Turn
	if err := json.Unmarshal(encoded, &turn); err != nil {
		return nil
	}
	return &turn
}

func isClaimPoll(body []byte) bool {
	if !bytes.Contains(body, []byte(claimRunTool)) {
		return false
	}
	var msg struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	return json.Unmarshal(body, &msg) == nil && msg.Method == "tools/call" && msg.Params.Name == claimRunTool
}
