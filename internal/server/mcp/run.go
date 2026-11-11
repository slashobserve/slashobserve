package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/grantmode"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const claimRunTool = "claim_run"

var errNoRunTransport = errors.New("no run tools for this run mode")

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
	return h.invoke(name, true)
}

func (h *handler) serveRun(w http.ResponseWriter, r *http.Request, cred Credential) {
	req := *cred.Run
	h.serve(w, r, serving{
		key: "run:" + req.ID,
		prepare: func(ctx context.Context) (context.Context, http.Handler, error) {
			ctx = auth.WithPrincipal(ctx, cred.Principal)
			ctx = withRegisteredClient(origin.With(ctx, origin.Origin{Kind: req.Origin.Kind, Client: req.Origin.Client}))
			if cred.Mode != "" {
				ctx = grantmode.With(ctx, cred.Mode)
			}
			scope, err := h.scope(ctx, req)
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

func (h *handler) recordStep(ctx context.Context, name string, start time.Time, res tools.Result, callErr error) {
	req, ok := runRequestFrom(ctx)
	if h.steps == nil || !ok {
		return
	}
	step := revisions.Step{At: start, Phase: tools.Phase(name), Tool: name, Status: "ok", Message: res.Step, DurationMS: time.Since(start).Milliseconds()}
	if callErr != nil {
		step.Status, step.Message = "error", internalErrorMsg
	} else if res.IsError {
		step.Status = "error"
	}
	h.steps.RecordStep(context.WithoutCancel(ctx), req, step)
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
